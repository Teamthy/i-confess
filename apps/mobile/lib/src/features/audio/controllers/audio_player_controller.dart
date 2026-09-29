/// Audio player controller for managing audio playback.
///
/// This controller integrates the audio playback service with the audio generation
/// service to provide a complete audio experience.
import 'dart:async';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_riverpod/legacy.dart';
import '../../../core/di/providers.dart';
import '../models/audio_generation_job.dart';
import '../services/audio_generation_service.dart';
import '../services/audio_url_service.dart';
import '../../player/audio_playback_service.dart';
import '../../player/player_providers.dart' show audioPlaybackServiceProvider;

/// UI-facing playback phase, distinct from just_audio's PlayerState object.
enum AudioPlayerPhase { idle, loading, ready, playing, paused, completed, error }

const _unchanged = Object();

/// State for the audio player.
class AudioPlayerState {
  final String? currentAssetId;
  final String? currentConfessionId;
  final String? currentVoiceId;
  final AudioPlayerPhase playerState;
  final Duration position;
  final Duration duration;
  final bool isBuffering;
  final String? error;
  final double volume;
  final bool isMuted;
  final double playbackSpeed;

  const AudioPlayerState({
    this.currentAssetId,
    this.currentConfessionId,
    this.currentVoiceId,
    this.playerState = AudioPlayerPhase.idle,
    this.position = Duration.zero,
    this.duration = Duration.zero,
    this.isBuffering = false,
    this.error,
    this.volume = 1.0,
    this.isMuted = false,
    this.playbackSpeed = 1.0,
  });

  AudioPlayerState copyWith({
    String? currentAssetId,
    String? currentConfessionId,
    String? currentVoiceId,
    AudioPlayerPhase? playerState,
    Duration? position,
    Duration? duration,
    bool? isBuffering,
    Object? error = _unchanged,
    double? volume,
    bool? isMuted,
    double? playbackSpeed,
  }) {
    return AudioPlayerState(
      currentAssetId: currentAssetId ?? this.currentAssetId,
      currentConfessionId: currentConfessionId ?? this.currentConfessionId,
      currentVoiceId: currentVoiceId ?? this.currentVoiceId,
      playerState: playerState ?? this.playerState,
      position: position ?? this.position,
      duration: duration ?? this.duration,
      isBuffering: isBuffering ?? this.isBuffering,
      error: identical(error, _unchanged) ? this.error : error as String?,
      volume: volume ?? this.volume,
      isMuted: isMuted ?? this.isMuted,
      playbackSpeed: playbackSpeed ?? this.playbackSpeed,
    );
  }
}

/// Notifier for the audio player state.
class AudioPlayerNotifier extends StateNotifier<AudioPlayerState> {
  final AudioPlaybackService _playbackService;
  final AudioUrlService _urlService;
  final AudioGenerationService _generationService;
  
  StreamSubscription<Duration>? _positionSub;
  StreamSubscription<Duration?>? _durationSub;
  StreamSubscription<AudioPlaybackStatus>? _playerStateSub;
  StreamSubscription<String>? _errorSub;
  double _lastAudibleVolume = 1.0;

  AudioPlayerNotifier({
    required AudioPlaybackService playbackService,
    required AudioUrlService urlService,
    required AudioGenerationService generationService,
  })  : _playbackService = playbackService,
        _urlService = urlService,
        _generationService = generationService,
        super(const AudioPlayerState());

  @override
  void dispose() {
    if (_positionSub != null) unawaited(_positionSub!.cancel());
    if (_durationSub != null) unawaited(_durationSub!.cancel());
    if (_playerStateSub != null) unawaited(_playerStateSub!.cancel());
    if (_errorSub != null) unawaited(_errorSub!.cancel());
    super.dispose();
  }

  /// Initializes the audio player controller.
  void init() {
    _positionSub = _playbackService.positionStream.listen((position) {
      state = state.copyWith(position: position);
    });

    _durationSub = _playbackService.durationStream.listen((duration) {
      state = state.copyWith(duration: duration ?? Duration.zero);
    });

    _playerStateSub = _playbackService.statusStream.listen((playerState) {
      // Map AudioPlaybackStatus to AudioPlayerPhase
      final newState = _mapAudioPlayerPhase(playerState);
      state = state.copyWith(
        playerState: newState,
        isBuffering: playerState == AudioPlaybackStatus.loading,
      );
    });

    _errorSub = _playbackService.errorStream.listen((error) {
      state = state.copyWith(error: error);
    });
  }

  /// Maps AudioPlaybackStatus to AudioPlayerPhase.
  AudioPlayerPhase _mapAudioPlayerPhase(AudioPlaybackStatus status) {
    switch (status) {
      case AudioPlaybackStatus.idle:
        return AudioPlayerPhase.idle;
      case AudioPlaybackStatus.loading:
        return AudioPlayerPhase.loading;
      case AudioPlaybackStatus.ready:
        return AudioPlayerPhase.ready;
      case AudioPlaybackStatus.playing:
        return AudioPlayerPhase.playing;
      case AudioPlaybackStatus.paused:
        return AudioPlayerPhase.paused;
      case AudioPlaybackStatus.completed:
        return AudioPlayerPhase.completed;
      case AudioPlaybackStatus.error:
        return AudioPlayerPhase.error;
    }
  }

  /// Plays an audio asset.
  ///
  /// If the asset doesn't have a ready audio file, it will generate one first.
  Future<void> playAsset({
    required String assetId,
    String? sessionId,
    String? sessionItemId,
    String? confessionId,
    String? voiceId,
    Duration? initialPosition,
  }) async {
    // Store the current asset info
    state = state.copyWith(
      currentAssetId: assetId,
      currentConfessionId: confessionId,
      currentVoiceId: voiceId,
      error: null,
    );

    try {
      // The API only mints audio URLs as part of an entitlement-checked session.
      if (sessionId == null || sessionId.isEmpty) {
        throw StateError('Playback requires a server-authorized session.');
      }
      final url = await _urlService.getStreamUrl(
        sessionId,
        itemId: sessionItemId ?? assetId,
      );
      
      // Load and play
      await _playbackService.load(
        url,
        initialPosition: initialPosition,
        metadata: PlaybackMediaMetadata(
          id: sessionItemId ?? assetId,
          title: confessionId ?? 'I-Confess audio',
          artist: voiceId ?? 'I-Confess',
        ),
      );
      await _playbackService.play();
    } catch (e) {
      state = state.copyWith(error: 'Failed to play: $e');
      rethrow;
    }
  }

  /// Plays a confession by ID.
  ///
  /// This will first check if there's a ready audio asset, and if not,
  /// it will generate one.
  Future<void> playConfession({
    required String confessionId,
    String? sessionId,
    String? sessionItemId,
    String? voiceId,
    Duration? initialPosition,
  }) async {
    await playAsset(
      assetId: sessionItemId ?? confessionId,
      sessionId: sessionId,
      sessionItemId: sessionItemId ?? confessionId,
      confessionId: confessionId,
      voiceId: voiceId,
      initialPosition: initialPosition,
    );
  }

  /// Generates audio for a confession and plays it.
  ///
  /// This combines generation and playback into one operation.
  Future<void> generateAndPlay({
    required String confessionId,
    String? contentVersionId,
    String? variantId,
    required String voiceId,
    String? sessionId,
    String? sessionItemId,
    String provider = 'elevenlabs',
    String qualityTier = 'standard',
    int pollInterval = 2,
    int pollTimeout = 120,
  }) async {
    try {
      // Create the generation request
      final request = AudioGenerationRequest(
        confessionId: confessionId,
        contentVersionId: contentVersionId,
        variantId: variantId,
        voiceId: voiceId,
        provider: provider,
        qualityTier: qualityTier,
      );

      // Queue the generation job
      final job = await _generationService.createJob(request);
      
      // Show loading state
      state = state.copyWith(
        currentConfessionId: confessionId,
        currentVoiceId: voiceId,
        playerState: AudioPlayerPhase.loading,
      );

      // Wait for the job to complete
      final completedJob = await _generationService.pollJobUntilComplete(
        jobId: job.id,
        interval: pollInterval,
        timeout: pollTimeout,
      );

      if (completedJob.status == AudioJobStatus.succeeded) {
        // Play the generated asset
        await playAsset(
          assetId: completedJob.audioAssetId ?? '',
          sessionId: sessionId,
          sessionItemId: sessionItemId,
          confessionId: confessionId,
          voiceId: voiceId,
        );
      } else {
        throw Exception('Audio generation failed: ${completedJob.errorMessage}');
      }
    } catch (e) {
      state = state.copyWith(
        error: 'Failed to generate and play: $e',
        playerState: AudioPlayerPhase.error,
      );
      rethrow;
    }
  }

  /// Pauses playback.
  Future<void> pause() async {
    try {
      await _playbackService.pause();
    } catch (e) {
      state = state.copyWith(error: 'Failed to pause: $e');
      rethrow;
    }
  }

  /// Resumes playback.
  Future<void> resume() async {
    try {
      await _playbackService.play();
    } catch (e) {
      state = state.copyWith(error: 'Failed to resume: $e');
      rethrow;
    }
  }

  /// Stops playback.
  Future<void> stop() async {
    try {
      await _playbackService.stop();
      state = state.copyWith(
        playerState: AudioPlayerPhase.idle,
        position: Duration.zero,
      );
    } catch (e) {
      state = state.copyWith(error: 'Failed to stop: $e');
      rethrow;
    }
  }

  /// Seeks to a specific position.
  Future<void> seek(Duration position) async {
    try {
      await _playbackService.seek(position);
    } catch (e) {
      state = state.copyWith(error: 'Failed to seek: $e');
      rethrow;
    }
  }

  /// Sets the volume.
  Future<void> setVolume(double volume) async {
    final safeVolume = volume.clamp(0.0, 1.0).toDouble();
    try {
      await _playbackService.setVolume(safeVolume);
      if (safeVolume > 0) _lastAudibleVolume = safeVolume;
      state = state.copyWith(volume: safeVolume, isMuted: safeVolume == 0);
    } catch (e) {
      state = state.copyWith(error: 'Failed to set volume: $e');
      rethrow;
    }
  }

  /// Sets the playback speed.
  Future<void> setPlaybackSpeed(double speed) async {
    final safeSpeed = speed.clamp(0.5, 2.0).toDouble();
    try {
      await _playbackService.setSpeed(safeSpeed);
      state = state.copyWith(playbackSpeed: safeSpeed);
    } catch (e) {
      state = state.copyWith(error: 'Failed to set playback speed: $e');
      rethrow;
    }
  }

  /// Toggles mute.
  Future<void> toggleMute() async {
    if (state.isMuted) {
      await setVolume(_lastAudibleVolume > 0 ? _lastAudibleVolume : 1.0);
    } else {
      if (state.volume > 0) _lastAudibleVolume = state.volume;
      await setVolume(0.0);
    }
  }

  /// Gets the current playback state.
  AudioPlayerState get currentState => state;

  /// Gets the current position as a percentage of duration.
  double get positionPercentage {
    final durationMs = state.duration.inMilliseconds;
    if (durationMs <= 0) return 0.0;
    return (state.position.inMilliseconds / durationMs).clamp(0.0, 1.0).toDouble();
  }

  /// Gets the current position as a display string.
  String get displayPosition {
    final minutes = state.position.inMinutes;
    final seconds = state.position.inSeconds.remainder(60);
    return '${minutes.toString().padLeft(2, '0')}:${seconds.toString().padLeft(2, '0')}';
  }

  /// Gets the duration as a display string.
  String get displayDuration {
    final minutes = state.duration.inMinutes;
    final seconds = state.duration.inSeconds.remainder(60);
    return '${minutes.toString().padLeft(2, '0')}:${seconds.toString().padLeft(2, '0')}';
  }
}

/// Provider for the audio player controller.
final audioPlayerControllerProvider = StateNotifierProvider<AudioPlayerNotifier, AudioPlayerState>((ref) {
  final playbackService = ref.watch(audioPlaybackServiceProvider);
  final client = ref.watch(apiClientProvider);
  final urlService = AudioUrlService(client: client);
  final generationService = AudioGenerationService(client: client);
  
  final controller = AudioPlayerNotifier(
    playbackService: playbackService,
    urlService: urlService,
    generationService: generationService,
  );
  
  controller.init();
  return controller;
});
