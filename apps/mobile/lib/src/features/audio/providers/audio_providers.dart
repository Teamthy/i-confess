/// Riverpod providers for audio features.
///
/// This file contains all the providers needed for audio functionality,
/// including services, controllers, and state management.
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:http/http.dart';

import '../../../core/services/api_service.dart';
import '../models/audio_asset.dart';
import '../models/audio_generation_job.dart';
import '../models/audio_generation_request.dart';
import '../models/tts_provider.dart';
import '../services/audio_generation_service.dart';
import '../services/audio_url_service.dart';
import '../controllers/audio_player_controller.dart';

/// Provider for the HTTP client.
final httpClientProvider = Provider<Client>((ref) {
  return Client();
});

/// Provider for the API service.
final apiServiceProvider = Provider<ApiService>((ref) {
  return ApiService(baseUrl: const String.fromEnvironment('API_BASE_URL'));
});

/// Provider for the audio generation service.
final audioGenerationServiceProvider = Provider<AudioGenerationService>((ref) {
  final apiService = ref.watch(apiServiceProvider);
  return AudioGenerationService(apiService: apiService);
});

/// Provider for the audio URL service.
final audioUrlServiceProvider = Provider<AudioUrlService>((ref) {
  return AudioUrlService();
});

/// Provider for the current audio generation job.
final currentAudioJobProvider = StateProvider<AudioGenerationJob?>((ref) => null);

/// Provider for the current audio asset.
final currentAudioAssetProvider = StateProvider<AudioAsset?>((ref) => null);

/// Provider for the list of TTS providers.
final ttsProvidersProvider = FutureProvider<List<TtsProvider>>((ref) async {
  final service = ref.watch(audioGenerationServiceProvider);
  return service.getTtsProviders();
});

/// Provider for the list of audio generation jobs.
final audioJobsProvider = FutureProvider<List<AudioGenerationJob>>((ref) async {
  final service = ref.watch(audioGenerationServiceProvider);
  return service.getJobs();
});

/// Provider for the audio generation job statistics.
final audioGenerationStatsProvider = FutureProvider<Map<String, dynamic>>((ref) async {
  final service = ref.watch(audioGenerationServiceProvider);
  return service.getStats();
});

/// Provider for the audio player controller.
///
/// This is the main provider for controlling audio playback.
/// Use this to play, pause, stop, seek, and control audio.
final audioPlayerProvider = audioPlayerControllerProvider;

/// Provider for whether audio is currently playing.
final isPlayingProvider = Provider<bool>((ref) {
  final state = ref.watch(audioPlayerProvider);
  return state.playerState == PlayerState.playing;
});

/// Provider for whether audio is loading.
final isLoadingProvider = Provider<bool>((ref) {
  final state = ref.watch(audioPlayerProvider);
  return state.playerState == PlayerState.loading || state.isBuffering;
});

/// Provider for whether audio is paused.
final isPausedProvider = Provider<bool>((ref) {
  final state = ref.watch(audioPlayerProvider);
  return state.playerState == PlayerState.paused;
});

/// Provider for the current position as a percentage.
final positionPercentageProvider = Provider<double>((ref) {
  final controller = ref.watch(audioPlayerProvider.notifier);
  return controller.positionPercentage;
});

/// Provider for the display position string.
final displayPositionProvider = Provider<String>((ref) {
  final controller = ref.watch(audioPlayerProvider.notifier);
  return controller.displayPosition;
});

/// Provider for the display duration string.
final displayDurationProvider = Provider<String>((ref) {
  final controller = ref.watch(audioPlayerProvider.notifier);
  return controller.displayDuration;
});

/// Provider for the current error message.
final audioErrorProvider = Provider<String?>((ref) {
  final state = ref.watch(audioPlayerProvider);
  return state.error;
});

/// Provider for creating a new audio generation job.
final createAudioJobProvider = FutureProvider.family<AudioGenerationJob, AudioGenerationRequest>(
  (ref, request) async {
    final service = ref.watch(audioGenerationServiceProvider);
    return service.createJob(request);
  },
);

/// Provider for getting a specific audio generation job.
final getAudioJobProvider = FutureProvider.family<AudioGenerationJob, String>(
  (ref, jobId) async {
    final service = ref.watch(audioGenerationServiceProvider);
    return service.getJob(jobId);
  },
);

/// Provider for retrying an audio generation job.
final retryAudioJobProvider = FutureProvider.family<AudioGenerationJob, String>(
  (ref, jobId) async {
    final service = ref.watch(audioGenerationServiceProvider);
    return service.retryJob(jobId);
  },
);

/// Provider for canceling an audio generation job.
final cancelAudioJobProvider = FutureProvider.family<AudioGenerationJob, String>(
  (ref, jobId) async {
    final service = ref.watch(audioGenerationServiceProvider);
    return service.cancelJob(jobId);
  },
);

/// Provider for creating a batch of audio generation jobs.
final createBatchAudioJobsProvider = FutureProvider.family<List<AudioGenerationJob>, BatchAudioGenerationRequest>(
  (ref, request) async {
    final service = ref.watch(audioGenerationServiceProvider);
    return service.createBatchJobs(request);
  },
);
