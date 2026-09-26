import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:iconfess/src/core/di/providers.dart';
import 'package:iconfess/src/core/theme/theme.dart';
import 'package:iconfess/src/features/player/audio_playback_service.dart';
import 'package:iconfess/src/features/player/player_providers.dart';
import 'package:iconfess/src/features/player/player_screen.dart';
import 'package:iconfess_api/iconfess_api.dart';
import 'package:shared_preferences/shared_preferences.dart';

import 'support/fake_api_client.dart';

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  late FakeApiClient api;
  late InMemoryTokenStore tokens;
  late TestAudioPlaybackService audio;

  setUp(() {
    SharedPreferences.setMockInitialValues({});
    tokens = InMemoryTokenStore();
    api = FakeApiClient(tokens: tokens);
    audio = TestAudioPlaybackService();

    api.respond('/sessions/s-1/queue', {
      'session_id': 's-1',
      'status': 'READY',
      'items': [
        {
          'id': 'qi-1',
          'confession_id': 'c-1',
          'title': 'Divine Healing',
          'text': 'By His stripes I was healed.',
          'audio_url': 'https://cdn.iconfess.app/audio/1.mp3?sig=test1',
          'duration_seconds': 60,
          'status': 'QUEUED',
          'position': 0,
        },
        {
          'id': 'qi-2',
          'confession_id': 'c-2',
          'title': 'Endless Peace',
          'text': 'Peace that surpasses all understanding.',
          'audio_url': 'https://cdn.iconfess.app/audio/2.mp3?sig=test2',
          'duration_seconds': 120,
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
    api.respond('/sessions/s-1/skip', {'skipped': 'qi-1', 'advanced': true});
    api.respond('/sessions/s-1/complete', {'status': 'COMPLETED'});
    api.respond('/sessions/s-1/progress', {
      'applied': true,
      'progress': {'session_id': 's-1', 'position_ms': 0},
    });
    api.respond('/me/history', {'recorded': true});
  });

  Future<void> pumpPlayer(WidgetTester tester, {String sessionId = 's-1'}) async {
    final prefs = await SharedPreferences.getInstance();
    await tester.pumpWidget(
      ProviderScope(
        overrides: [
          sharedPreferencesProvider.overrideWithValue(prefs),
          tokenStoreProvider.overrideWithValue(tokens),
          apiClientProvider.overrideWithValue(api),
          audioPlaybackServiceProvider.overrideWithValue(audio),
        ],
        child: MaterialApp(
          theme: AppTheme.immersive(),
          home: PlayerScreen(sessionId: sessionId),
        ),
      ),
    );
    await tester.pumpAndSettle();
  }

  testWidgets('renders player with queue count, confession title and text', (tester) async {
    await pumpPlayer(tester);

    expect(find.text('1 / 2'), findsOneWidget);
    // Title appears in both the now-playing card and the queue rail, so scope
    // the finder to the card that contains the full confession text.
    final textFinder = find.text('By His stripes I was healed.');
    final cardFinder = find.ancestor(of: textFinder, matching: find.byType(Container));
    expect(find.descendant(of: cardFinder, matching: find.text('Divine Healing')), findsOneWidget);
    expect(textFinder, findsOneWidget);
    expect(find.byType(Slider), findsOneWidget);
    expect(find.byIcon(Icons.play_arrow_rounded), findsOneWidget);
  });

  testWidgets('play and pause toggle audio controls', (tester) async {
    await pumpPlayer(tester);

    // Tap play
    await tester.tap(find.byIcon(Icons.play_arrow_rounded));
    await tester.pumpAndSettle();

    expect(find.byIcon(Icons.pause_rounded), findsOneWidget);
    expect(audio.playCount, 1);

    // Tap pause
    await tester.tap(find.byIcon(Icons.pause_rounded));
    await tester.pumpAndSettle();

    expect(find.byIcon(Icons.play_arrow_rounded), findsOneWidget);
    expect(audio.pauseCount, 1);
  });

  testWidgets('skip button advances to next confession', (tester) async {
    await pumpPlayer(tester);

    await tester.tap(find.byIcon(Icons.skip_next_rounded));
    await tester.pumpAndSettle();

    expect(find.text('2 / 2'), findsOneWidget);
    // After skip, the new title is in both the now-playing card and the queue.
    final nextTextFinder = find.text('Peace that surpasses all understanding.');
    final nextCardFinder = find.ancestor(of: nextTextFinder, matching: find.byType(Container));
    expect(find.descendant(of: nextCardFinder, matching: find.text('Endless Peace')), findsOneWidget);
    expect(nextTextFinder, findsOneWidget);
  });

  testWidgets('shows locked banner and upgrade button when item is locked', (tester) async {
    api.respond('/sessions/s-locked/queue', {
      'session_id': 's-locked',
      'status': 'READY',
      'items': [
        {
          'id': 'qi-locked',
          'confession_id': 'c-1',
          'title': 'Premium Anointing',
          'text': 'A locked confession text.',
          'audio_url': '',
          'duration_seconds': 60,
          'locked': true,
          'lock_reason': 'Premium voice requires subscription',
          'status': 'QUEUED',
          'position': 0,
        },
      ],
      'counts': {'QUEUED': 1},
      'items_total': 1,
      'items_completed': 0,
    });

    await pumpPlayer(tester, sessionId: 's-locked');

    expect(find.text('Premium voice requires subscription'), findsOneWidget);
    expect(find.text('Upgrade to Premium'), findsOneWidget);
  });

  testWidgets('shows completed state when session is finished', (tester) async {
    api.respond('/sessions/s-done/queue', {
      'session_id': 's-done',
      'status': 'COMPLETED',
      'items': [
        {
          'id': 'qi-done',
          'confession_id': 'c-1',
          'title': 'Finished track',
          'text': 'Done.',
          'audio_url': 'url',
          'duration_seconds': 60,
          'status': 'COMPLETED',
          'position': 0,
        },
      ],
      'counts': {'COMPLETED': 1},
      'items_total': 1,
      'items_completed': 1,
    });

    await pumpPlayer(tester, sessionId: 's-done');

    expect(find.text('Session Complete'), findsOneWidget);
    expect(find.text('Done'), findsOneWidget);
  });

  testWidgets('shows actionable error state with retry on failure', (tester) async {
    api.respondWith('/sessions/s-err/queue', const ApiError(status: 500, code: 'SERVER_ERROR', message: 'Failed to load'));

    await pumpPlayer(tester, sessionId: 's-err');

    expect(find.text('Could not load session'), findsOneWidget);
    expect(find.text('Retry'), findsOneWidget);
  });
}
