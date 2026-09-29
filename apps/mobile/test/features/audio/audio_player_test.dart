import 'package:flutter_test/flutter_test.dart';
import 'package:iconfess_api/iconfess_api.dart';
import 'package:iconfess/src/features/audio/controllers/audio_player_controller.dart';
import 'package:iconfess/src/features/audio/services/audio_generation_service.dart';
import 'package:iconfess/src/features/audio/services/audio_url_service.dart';
import 'package:iconfess/src/features/player/audio_playback_service.dart';

class PlaybackFakeApiClient extends ApiClient {
  PlaybackFakeApiClient()
      : super(
          baseUrl: 'https://api.example.test',
          tokens: InMemoryTokenStore(),
        );

  final Map<String, Map<String, dynamic>> gets = {};
  final Map<String, Map<String, dynamic>> posts = {};

  @override
  Future<Map<String, dynamic>> get(
    String path, {
    Map<String, String>? query,
  }) async {
    final response = gets[path];
    if (response == null) throw StateError('No fake GET response for $path');
    return response;
  }

  @override
  Future<Map<String, dynamic>> post(String path, [Object? body]) async {
    final response = posts[path];
    if (response == null) throw StateError('No fake POST response for $path');
    return response;
  }
}

void main() {
  late PlaybackFakeApiClient client;
  late TestAudioPlaybackService playback;
  late AudioPlayerNotifier controller;

  setUp(() {
    client = PlaybackFakeApiClient();
    playback = TestAudioPlaybackService();
    controller = AudioPlayerNotifier(
      playbackService: playback,
      urlService: AudioUrlService(client: client),
      generationService: AudioGenerationService(client: client),
    )..init();
  });

  tearDown(() async {
    controller.dispose();
    await playback.dispose();
  });

  test('starts an asset using a URL minted in its server session', () async {
    client.gets['/sessions/session-1/queue'] = {
      'items': [
        {
          'id': 'item-1',
          'audio_asset_id': 'asset-1',
          'audio_url': 'https://cdn.example.test/a.m4a?expires=4102444800&sig=ok',
          'locked': false,
        },
      ],
    };

    await controller.playAsset(
      assetId: 'asset-1',
      sessionId: 'session-1',
      confessionId: 'confession-1',
      voiceId: 'voice-1',
    );

    expect(controller.state.currentAssetId, 'asset-1');
    expect(controller.state.currentConfessionId, 'confession-1');
    expect(controller.state.currentVoiceId, 'voice-1');
    expect(playback.loadedUrl, contains('sig=ok'));
    expect(playback.loadedMetadata?.id, 'asset-1');
    expect(playback.playCount, 1);
  });

  test('refuses to play an unscoped asset URL', () async {
    await expectLater(
      controller.playAsset(assetId: 'asset-1'),
      throwsA(isA<StateError>()),
    );
    expect(controller.state.error, contains('server-authorized session'));
  });

  test('mute remembers and restores the last audible volume', () async {
    await controller.setVolume(0.6);
    await controller.toggleMute();
    expect(controller.state.isMuted, isTrue);
    expect(playback.volume, 0);

    await controller.toggleMute();
    expect(controller.state.isMuted, isFalse);
    expect(controller.state.volume, 0.6);
    expect(playback.volume, 0.6);
  });

  test('generation polls the API and plays only through a session item', () async {
    client.posts['/admin/audio/generate/job'] = {
      'job_id': 'job-1',
      'status': 'queued',
    };
    client.gets['/admin/audio/generate/job/job-1'] = {
      'id': 'job-1',
      'status': 'succeeded',
      'audio_asset_id': 'asset-1',
    };
    client.gets['/sessions/session-1/queue'] = {
      'items': [
        {
          'id': 'item-1',
          'audio_asset_id': 'asset-1',
          'audio_url': 'https://cdn.example.test/a.m4a?expires=4102444800&sig=ok',
          'locked': false,
        },
      ],
    };

    await controller.generateAndPlay(
      confessionId: 'confession-1',
      contentVersionId: 'version-1',
      voiceId: 'voice-1',
      sessionId: 'session-1',
      pollInterval: 0,
    );

    expect(controller.state.currentAssetId, 'asset-1');
    expect(playback.playCount, 1);
  });
}
