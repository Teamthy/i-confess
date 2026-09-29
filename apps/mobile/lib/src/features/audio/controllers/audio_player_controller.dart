/// Audio player controller for managing audio playback.
///
/// This controller integrates the audio playback service with the audio generation
/// service to provide a complete audio experience.
import 'dart:async';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:just_audio/just_audio.dart';
import '../models/audio_asset.dart';
import '../models/audio_generation_job.dart';
import '../services/audio_generation_service.dart';
import '../services/audio_url_service.dart';
import '../../player/audio_playback_service.dart';

/// State for the audio player.
class AudioPlayerState {
  final String? currentAssetId;
  final String? currentConfessionId;
  final String? currentVoiceId;
  final PlayerState playerState;
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
    this.playerState = PlayerState.idle,
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
    PlayerState? playerState,
    Duration? position,
    Duration? duration,
    bool? isBuffering,
    String? error,
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
      error: error,
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
  StreamSubscription<PlayerState>? _playerStateSub;
  StreamSubscription<String>? _errorSub;

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
    _positionSub?.cancel();
    _durationSub?.cancel();
    _playerStateSub?.cancel();
    _errorSub?.cancel();
    _playbackService.dispose();
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
      // Map AudioPlaybackStatus to PlayerState
      final newState = _mapPlayerState(playerState);
      state = state.copyWith(
        playerState: newState,
        isBuffering: playerState == AudioPlaybackStatus.loading ||
                    playerState == AudioPlaybackStatus.buffering,
      );
    });

    _errorSub = _playbackService.errorStream.listen((error) {
      state = state.copyWith(error: error);
    });
  }

  /// Maps AudioPlaybackStatus to PlayerState.
  PlayerState _mapPlayerState(AudioPlaybackStatus status) {
    switch (status) {
      case AudioPlaybackStatus.idle:
        return PlayerState.idle;
      case AudioPlaybackStatus.loading:
      case AudioPlaybackStatus.buffering:
        return PlayerState.loading;
      case AudioPlaybackStatus.ready:
        return PlayerState.ready;
      case AudioPlaybackStatus.playing:
        return PlayerState.playing;
      case AudioPlaybackStatus.paused:
        return PlayerState.paused;
      case AudioPlaybackStatus.completed:
        return PlayerState.completed;
      case AudioPlaybackStatus.error:
        return PlayerState.error;
    }
  }

  /// Plays an audio asset.
  ///
  /// If the asset doesn't have a ready audio file, it will generate one first.
  Future<void> playAsset({
    required String assetId,
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
      // Get the signed URL
      final url = await _urlService.getStreamUrl(assetId);
      
      // Load and play
      await _playbackService.load(url, initialPosition: initialPosition);
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
    String? voiceId,
    Duration? initialPosition,
  }) async {
    try {
      // For now, we'll assume there's a ready audio asset
      // In a real implementation, we would:
      // 1. Check if there's a ready audio asset for this confession/voice
      // 2. If not, queue a generation job
      // 3. Wait for the job to complete
      // 4. Then play the generated asset
      
      // For Phase 2, we'll use a placeholder asset ID
      // In production, this would come from the backend
      final assetId = 'asset_$confessionId_${voiceId ?? 'default'}';
      
      await playAsset(
        assetId: assetId,
        confessionId: confessionId,
        voiceId: voiceId,
        initialPosition: initialPosition,
      );
    } catch (e) {
      state = state.copyWith(error: 'Failed to play confession: $e');
      rethrow;
    }
  }

  /// Generates audio for a confession and plays it.
  ///
  /// This combines generation and playback into one operation.
  Future<void> generateAndPlay({
    required String confessionId,
    String? contentVersionId,
    String? variantId,
    required String voiceId,
    String provider = 'elevenlabs',
    String qualityTier = 'standard',
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
        playerState: PlayerState.loading,
      );

      // Wait for the job to complete
      final completedJob = await _generationService.pollJobUntilComplete(
        jobId: job.id,
        interval: 2,
        timeout: 120,
      );

      if (completedJob.status == AudioJobStatus.succeeded) {
        // Play the generated asset
        await playAsset(
          assetId: completedJob.audioAssetId ?? '',
          confessionId: confessionId,
          voiceId: voiceId,
        );
      } else {
        throw Exception('Audio generation failed: ${completedJob.errorMessage}');
      }
    } catch (e) {
      state = state.copyWith(
        error: 'Failed to generate and play: $e',
        playerState: PlayerState.error,
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
        playerState: PlayerState.idle,
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
    try {
      await _playbackService.setVolume(volume);
      state = state.copyWith(volume: volume);
    } catch (e) {
      state = state.copyWith(error: 'Failed to set volume: $e');
      rethrow;
    }
  }

  /// Sets the playback speed.
  Future<void> setPlaybackSpeed(double speed) async {
    try {
      await _playbackService.setSpeed(speed);
      state = state.copyWith(playbackSpeed: speed);
    } catch (e) {
      state = state.copyWith(error: 'Failed to set playback speed: $e');
      rethrow;
    }
  }

  /// Toggles mute.
  Future<void> toggleMute() async {
    final newMuted = !state.isMuted;
    await setVolume(newMuted ? 0.0 : state.volume);
    state = state.copyWith(isMuted: newMuted);
  }

  /// Gets the current playback state.
  AudioPlayerState get currentState => state;

  /// Gets the current position as a percentage of duration.
  double get positionPercentage {
    if (state.duration.inSeconds == 0) return 0.0;
    return state.position.inMilliseconds / state.duration.inMilliseconds;
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
  final urlService = AudioUrlService();
  final generationService = AudioGenerationService();
  
  final controller = AudioPlayerNotifier(
    playbackService: playbackService,
    urlService: urlService,
    generationService: generationService,
  );
  
  controller.init();
  ref.onDispose(() => controller.dispose());
  
  return controller;
});
