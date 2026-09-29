/// Integration tests for the audio player feature.
///
/// These tests verify that the audio player correctly integrates with
/// the audio generation service and playback service.
import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/mockito.dart';
import 'package:i_confess/src/features/audio/controllers/audio_player_controller.dart';
import 'package:i_confess/src/features/audio/models/audio_generation_job.dart';
import 'package:i_confess/src/features/audio/models/audio_generation_request.dart';
import 'package:i_confess/src/features/audio/services/audio_generation_service.dart';
import 'package:i_confess/src/features/audio/services/audio_url_service.dart';
import 'package:i_confess/src/features/player/audio_playback_service.dart';
import 'package:just_audio/just_audio.dart';

// Mock classes
class MockAudioPlaybackService extends Mock implements AudioPlaybackService {
  @override
  Stream<Duration> get positionStream => Stream.value(Duration.zero);
  
  @override
  Stream<Duration?> get durationStream => Stream.value(Duration.zero);
  
  @override
  Stream<AudioPlaybackStatus> get statusStream => Stream.value(AudioPlaybackStatus.idle);
  
  @override
  Stream<String> get errorStream => Stream.value('');
}

class MockAudioUrlService extends Mock implements AudioUrlService {}

class MockAudioGenerationService extends Mock implements AudioGenerationService {}

void main() {
  group('AudioPlayerNotifier', () {
    late AudioPlayerNotifier controller;
    late MockAudioPlaybackService mockPlaybackService;
    late MockAudioUrlService mockUrlService;
    late MockAudioGenerationService mockGenerationService;

    setUp(() {
      mockPlaybackService = MockAudioPlaybackService();
      mockUrlService = MockAudioUrlService();
      mockGenerationService = MockAudioGenerationService();
      
      controller = AudioPlayerNotifier(
        playbackService: mockPlaybackService,
        urlService: mockUrlService,
        generationService: mockGenerationService,
      );
    });

    tearDown(() {
      controller.dispose();
    });

    test('initial state is idle', () {
      expect(controller.state.playerState, PlayerState.idle);
      expect(controller.state.currentAssetId, isNull);
      expect(controller.state.currentConfessionId, isNull);
      expect(controller.state.position, Duration.zero);
      expect(controller.state.duration, Duration.zero);
    });

    test('playAsset updates state with asset info', () async {
      const assetId = 'test_asset';
      const confessionId = 'test_confession';
      const voiceId = 'test_voice';
      const url = 'https://example.com/audio.mp3';

      when(mockUrlService.getStreamUrl(assetId)).thenAnswer((_) async => url);
      when(mockPlaybackService.load(url, initialPosition: null))
          .thenAnswer((_) async {});
      when(mockPlaybackService.play()).thenAnswer((_) async {});

      await controller.playAsset(
        assetId: assetId,
        confessionId: confessionId,
        voiceId: voiceId,
      );

      expect(controller.state.currentAssetId, assetId);
      expect(controller.state.currentConfessionId, confessionId);
      expect(controller.state.currentVoiceId, voiceId);
    });

    test('playConfession calls playAsset with generated asset ID', () async {
      const confessionId = 'test_confession';
      const voiceId = 'test_voice';
      const url = 'https://example.com/audio.mp3';

      when(mockUrlService.getStreamUrl(any)).thenAnswer((_) async => url);
      when(mockPlaybackService.load(any, initialPosition: anyNamed('initialPosition')))
          .thenAnswer((_) async {});
      when(mockPlaybackService.play()).thenAnswer((_) async {});

      await controller.playConfession(
        confessionId: confessionId,
        voiceId: voiceId,
      );

      expect(controller.state.currentConfessionId, confessionId);
      expect(controller.state.currentVoiceId, voiceId);
    });

    test('generateAndPlay creates job and plays on success', () async {
      const confessionId = 'test_confession';
      const voiceId = 'test_voice';
      const jobId = 'test_job';
      const assetId = 'test_asset';
      const url = 'https://example.com/audio.mp3';

      final job = AudioGenerationJob(
        id: jobId,
        confessionId: confessionId,
        voiceId: voiceId,
        status: AudioJobStatus.succeeded,
        audioAssetId: assetId,
        progress: 1.0,
      );

      when(mockGenerationService.createJob(any))
          .thenAnswer((_) async => job);
      when(mockGenerationService.pollJobUntilComplete(
        jobId: jobId,
        interval: anyNamed('interval'),
        timeout: anyNamed('timeout'),
      )).thenAnswer((_) async => job);
      when(mockUrlService.getStreamUrl(assetId)).thenAnswer((_) async => url);
      when(mockPlaybackService.load(url, initialPosition: null))
          .thenAnswer((_) async {});
      when(mockPlaybackService.play()).thenAnswer((_) async {});

      await controller.generateAndPlay(
        confessionId: confessionId,
        voiceId: voiceId,
      );

      expect(controller.state.currentConfessionId, confessionId);
      expect(controller.state.currentVoiceId, voiceId);
      expect(controller.state.currentAssetId, assetId);
    });

    test('pause calls playbackService.pause', () async {
      when(mockPlaybackService.pause()).thenAnswer((_) async {});

      await controller.pause();

      verify(mockPlaybackService.pause()).called(1);
    });

    test('resume calls playbackService.play', () async {
      when(mockPlaybackService.play()).thenAnswer((_) async {});

      await controller.resume();

      verify(mockPlaybackService.play()).called(1);
    });

    test('stop calls playbackService.stop and resets state', () async {
      when(mockPlaybackService.stop()).thenAnswer((_) async {});

      await controller.stop();

      verify(mockPlaybackService.stop()).called(1);
      expect(controller.state.playerState, PlayerState.idle);
      expect(controller.state.position, Duration.zero);
    });

    test('seek calls playbackService.seek', () async {
      const position = Duration(seconds: 30);
      when(mockPlaybackService.seek(position)).thenAnswer((_) async {});

      await controller.seek(position);

      verify(mockPlaybackService.seek(position)).called(1);
    });

    test('setVolume updates state and calls playbackService', () async {
      const volume = 0.5;
      when(mockPlaybackService.setVolume(volume)).thenAnswer((_) async {});

      await controller.setVolume(volume);

      verify(mockPlaybackService.setVolume(volume)).called(1);
      expect(controller.state.volume, volume);
    });

    test('setPlaybackSpeed updates state and calls playbackService', () async {
      const speed = 1.5;
      when(mockPlaybackService.setSpeed(speed)).thenAnswer((_) async {});

      await controller.setPlaybackSpeed(speed);

      verify(mockPlaybackService.setSpeed(speed)).called(1);
      expect(controller.state.playbackSpeed, speed);
    });

    test('toggleMute toggles mute state', () async {
      const volume = 0.5;
      when(mockPlaybackService.setVolume(any)).thenAnswer((_) async {});

      await controller.setVolume(volume);
      expect(controller.state.isMuted, false);

      await controller.toggleMute();
      verify(mockPlaybackService.setVolume(0.0)).called(1);
      expect(controller.state.isMuted, true);

      await controller.toggleMute();
      verify(mockPlaybackService.setVolume(volume)).called(1);
      expect(controller.state.isMuted, false);
    });

    test('positionPercentage returns correct percentage', () {
      controller.state = controller.state.copyWith(
        position: Duration(seconds: 30),
        duration: Duration(minutes: 1),
      );

      expect(controller.positionPercentage, 0.5);
    });

    test('displayPosition formats position correctly', () {
      controller.state = controller.state.copyWith(
        position: Duration(minutes: 2, seconds: 30),
      );

      expect(controller.displayPosition, '02:30');
    });

    test('displayDuration formats duration correctly', () {
      controller.state = controller.state.copyWith(
        duration: Duration(minutes: 5, seconds: 15),
      );

      expect(controller.displayDuration, '05:15');
    });
  });

  group('AudioPlayerState', () {
    test('copyWith creates new state with updated values', () {
      const initialState = AudioPlayerState(
        currentAssetId: 'asset1',
        playerState: PlayerState.idle,
      );

      final newState = initialState.copyWith(
        currentAssetId: 'asset2',
        playerState: PlayerState.playing,
      );

      expect(newState.currentAssetId, 'asset2');
      expect(newState.playerState, PlayerState.playing);
    });

    test('copyWith preserves unchanged values', () {
      const initialState = AudioPlayerState(
        currentAssetId: 'asset1',
        currentConfessionId: 'confession1',
        volume: 0.5,
      );

      final newState = initialState.copyWith(
        currentAssetId: 'asset2',
      );

      expect(newState.currentAssetId, 'asset2');
      expect(newState.currentConfessionId, 'confession1');
      expect(newState.volume, 0.5);
    });
  });
}
