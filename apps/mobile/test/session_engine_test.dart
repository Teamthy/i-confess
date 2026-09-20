import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:iconfess/src/core/di/providers.dart';
import 'package:iconfess/src/core/persistence/persistence.dart';
import 'package:iconfess/src/features/player/audio_playback_service.dart';
import 'package:iconfess/src/features/player/player_providers.dart';
import 'package:iconfess_api/iconfess_api.dart';
import 'package:shared_preferences/shared_preferences.dart';

import 'support/fake_api_client.dart';

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  late FakeApiClient api;
  late InMemoryTokenStore tokens;
  late TestAudioPlaybackService audio;
  late ProviderContainer container;

  setUp(() async {
    SharedPreferences.setMockInitialValues({});
    tokens = InMemoryTokenStore();
    api = FakeApiClient(tokens: tokens);
    audio = TestAudioPlaybackService();

    // Default scripted endpoints
    api.respond('/sessions/s-1/queue', {
      'session_id': 's-1',
      'status': 'READY',
      'items': [
        {
          'id': 'qi-1',
          'confession_id': 'c-1',
          'title': 'I am healed',
          'text': 'By His stripes I was healed.',
          'audio_url': 'https://cdn.iconfess.app/audio/1.mp3?sig=test1',
          'duration_seconds': 60,
          'status': 'QUEUED',
          'position': 0,
        },
        {
          'id': 'qi-2',
          'confession_id': 'c-2',
          'title': 'Peace in my heart',
          'text': 'The peace of God surpasses understanding.',
          'audio_url': 'https://cdn.iconfess.app/audio/2.mp3?sig=test2',
          'duration_seconds': 90,
          'status': 'QUEUED',
          'position': 1,
        },
      ],
      'counts': {'QUEUED': 2},
      'items_total': 2,
      'items_completed': 0,
    });

    api.respond('/sessions/s-1/start', {'id': 's-1', 'status': 'ACTIVE'});
    api.respond('/sessions/s-1/pause', {'id': 's-1', 'status': 'PAUSED'});
    api.respond('/sessions/s-1/resume', {'id': 's-1', 'status': 'ACTIVE'});
    api.respond('/sessions/s-1/interrupt', {'id': 's-1', 'status': 'INTERRUPTED'});
    api.respond('/sessions/s-1/skip', {'skipped': 'qi-1', 'advanced': true});
    api.respond('/sessions/s-1/complete', {'status': 'COMPLETED'});
    api.respond('/sessions/s-1/progress', {
      'applied': true,
      'progress': {'session_id': 's-1', 'position_ms': 5000},
    });
    api.respond('/me/history', {'recorded': true});

    final prefs = await SharedPreferences.getInstance();
    container = ProviderContainer(
      overrides: [
        sharedPreferencesProvider.overrideWithValue(prefs),
        tokenStoreProvider.overrideWithValue(tokens),
        apiClientProvider.overrideWithValue(api),
        audioPlaybackServiceProvider.overrideWithValue(audio),
      ],
    );
  });

  tearDown(() {
    container.dispose();
  });

  test('loads session queue and prepares first playable item with signed URL', () async {
    final engine = container.read(sessionEngineProvider('s-1').notifier);
    await engine.load();

    final state = container.read(sessionEngineProvider('s-1'));
    expect(state.status, PlayerLifecycleStatus.ready);
    expect(state.items, hasLength(2));
    expect(state.currentItem?.id, 'qi-1');
    expect(audio.loadedUrl, 'https://cdn.iconfess.app/audio/1.mp3?sig=test1');
  });

  test('play, pause, seek, and resume lifecycle', () async {
    final engine = container.read(sessionEngineProvider('s-1').notifier);
    await engine.load();

    // 1. Play
    await engine.play();
    expect(container.read(sessionEngineProvider('s-1')).status, PlayerLifecycleStatus.playing);
    expect(audio.playCount, 1);
    expect(api.callCount('/sessions/s-1/start'), 1);

    // 2. Pause
    await engine.pause();
    expect(container.read(sessionEngineProvider('s-1')).status, PlayerLifecycleStatus.paused);
    expect(audio.pauseCount, 1);
    expect(api.callCount('/sessions/s-1/pause'), 1);

    // 3. Seek
    await engine.seek(15000);
    expect(container.read(sessionEngineProvider('s-1')).positionMs, 15000);
    expect(audio.lastSeek, const Duration(milliseconds: 15000));

    // 4. Resume
    await engine.resume();
    expect(container.read(sessionEngineProvider('s-1')).status, PlayerLifecycleStatus.playing);
    expect(api.callCount('/sessions/s-1/resume'), 1);
  });

  test('skip advances queue to next item and loads next signed URL', () async {
    final engine = container.read(sessionEngineProvider('s-1').notifier);
    await engine.load();

    await engine.skip();

    final state = container.read(sessionEngineProvider('s-1'));
    expect(state.currentIndex, 1);
    expect(state.currentItem?.id, 'qi-2');
    expect(state.skippedItemIds, contains('qi-1'));
    expect(audio.loadedUrl, 'https://cdn.iconfess.app/audio/2.mp3?sig=test2');
    expect(api.callCount('/sessions/s-1/skip'), 1);
  });

  test('previous seeks to 0 if > 3s, else navigates to previous item', () async {
    final engine = container.read(sessionEngineProvider('s-1').notifier);
    await engine.load();
    await engine.skip(); // Move to index 1

    // Case 1: position > 3000ms -> seeks to 0 on same item
    await engine.seek(5000);
    await engine.previous();
    expect(container.read(sessionEngineProvider('s-1')).currentIndex, 1);
    expect(container.read(sessionEngineProvider('s-1')).positionMs, 0);

    // Case 2: position <= 3000ms -> moves to index 0
    await engine.previous();
    expect(container.read(sessionEngineProvider('s-1')).currentIndex, 0);
    expect(container.read(sessionEngineProvider('s-1')).currentItem?.id, 'qi-1');
  });

  test('completes session and records history', () async {
    final engine = container.read(sessionEngineProvider('s-1').notifier);
    await engine.load();

    await engine.complete();

    final state = container.read(sessionEngineProvider('s-1'));
    expect(state.status, PlayerLifecycleStatus.completed);
    expect(api.callCount('/sessions/s-1/complete'), 1);
    expect(api.callCount('/me/history'), 1);
  });

  test('refreshes expired signed URLs without losing playback position (Criterion 6)', () async {
    final engine = container.read(sessionEngineProvider('s-1').notifier);
    await engine.load();
    await engine.seek(22000);

    // Mock refreshed queue with new signed URL
    api.respond('/sessions/s-1/queue', {
      'session_id': 's-1',
      'status': 'ACTIVE',
      'items': [
        {
          'id': 'qi-1',
          'confession_id': 'c-1',
          'title': 'I am healed',
          'text': 'By His stripes I was healed.',
          'audio_url': 'https://cdn.iconfess.app/audio/1.mp3?sig=fresh_signature_999',
          'duration_seconds': 60,
          'status': 'QUEUED',
          'position': 0,
        },
      ],
      'counts': {'QUEUED': 1},
      'items_total': 1,
      'items_completed': 0,
    });

    await engine.refreshSignedUrls();

    final state = container.read(sessionEngineProvider('s-1'));
    expect(state.positionMs, 22000, reason: 'position must be preserved across URL refresh');
    expect(audio.loadedUrl, 'https://cdn.iconfess.app/audio/1.mp3?sig=fresh_signature_999');
    expect(audio.loadedInitialPosition, const Duration(milliseconds: 22000));
  });

  test('handles audio interruptions by pausing and setting status to interrupted', () async {
    final engine = container.read(sessionEngineProvider('s-1').notifier);
    await engine.load();
    await engine.play();

    audio.emitInterruption();

    final state = container.read(sessionEngineProvider('s-1'));
    expect(state.status, PlayerLifecycleStatus.interrupted);
    expect(api.callCount('/sessions/s-1/interrupt'), 1);
  });

  test('handles route changes (becoming noisy) by pausing playback', () async {
    final engine = container.read(sessionEngineProvider('s-1').notifier);
    await engine.load();
    await engine.play();

    audio.emitBecomingNoisy();

    final state = container.read(sessionEngineProvider('s-1'));
    expect(state.status, PlayerLifecycleStatus.paused);
  });

  test('deterministic conflict resolution applies server if server timestamp is newer', () async {
    final storage = container.read(keyValueStoreProvider);
    // Write older local state (10:00:00Z)
    await storage.writeJson(
      StoreKeys.sessionPlayback('s-1'),
      const PersistedPlaybackState(
        sessionId: 's-1',
        currentIndex: 0,
        currentItemId: 'qi-1',
        positionMs: 3000,
        completedItemIds: {},
        skippedItemIds: {},
        lastPlaybackState: 'PAUSED',
        lastUpdatedAt: '2026-09-20T10:00:00Z',
      ).toJson(),
    );

    // Server queue returns progress with newer timestamp (10:15:00Z)
    api.respond('/sessions/s-1/queue', {
      'session_id': 's-1',
      'status': 'ACTIVE',
      'items': [
        {'id': 'qi-1', 'confession_id': 'c-1', 'title': 'T1', 'duration_seconds': 60, 'audio_url': 'url1'},
        {'id': 'qi-2', 'confession_id': 'c-2', 'title': 'T2', 'duration_seconds': 90, 'audio_url': 'url2'},
      ],
      'counts': {'QUEUED': 2},
      'items_total': 2,
      'items_completed': 0,
      'progress': {
        'session_id': 's-1',
        'queue_item_id': 'qi-2',
        'position_ms': 18000,
        'completed_items': 1,
        'last_updated_at': '2026-09-20T10:15:00Z',
      },
    });

    final engine = container.read(sessionEngineProvider('s-1').notifier);
    await engine.load();

    final state = container.read(sessionEngineProvider('s-1'));
    expect(state.currentIndex, 1, reason: 'server queue_item_id qi-2 should win');
    expect(state.positionMs, 18000, reason: 'server position 18000ms should win');
  });
}
