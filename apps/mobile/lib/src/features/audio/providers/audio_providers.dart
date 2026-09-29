/// Riverpod providers for the audio generation and playback features.
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_riverpod/legacy.dart';

import '../../../core/di/providers.dart';
import '../controllers/audio_player_controller.dart';
import '../models/audio_asset.dart';
import '../models/audio_generation_job.dart';
import '../models/audio_generation_request.dart';
import '../models/tts_provider.dart';
import '../services/audio_generation_service.dart';
import '../services/audio_url_service.dart';

final audioGenerationServiceProvider = Provider<AudioGenerationService>((ref) {
  return AudioGenerationService(client: ref.watch(apiClientProvider));
});

final audioUrlServiceProvider = Provider<AudioUrlService>((ref) {
  return AudioUrlService(client: ref.watch(apiClientProvider));
});

final currentAudioJobProvider = StateProvider<AudioGenerationJob?>((ref) => null);
final currentAudioAssetProvider = StateProvider<AudioAsset?>((ref) => null);

final ttsProvidersProvider = FutureProvider<List<TtsProvider>>((ref) async {
  final response = await ref.watch(audioGenerationServiceProvider).getProviders();
  return response.providers;
});

final audioJobsProvider = FutureProvider<List<AudioGenerationJob>>((ref) {
  return ref.watch(audioGenerationServiceProvider).listJobs();
});

final audioGenerationStatsProvider = FutureProvider<AudioGenerationStats>((ref) {
  return ref.watch(audioGenerationServiceProvider).getStats();
});

/// Main state provider for the feature-level player controls.
final audioPlayerProvider = audioPlayerControllerProvider;

final isPlayingProvider = Provider<bool>((ref) {
  return ref.watch(audioPlayerProvider).playerState == AudioPlayerPhase.playing;
});

final isLoadingProvider = Provider<bool>((ref) {
  final state = ref.watch(audioPlayerProvider);
  return state.playerState == AudioPlayerPhase.loading || state.isBuffering;
});

final isPausedProvider = Provider<bool>((ref) {
  return ref.watch(audioPlayerProvider).playerState == AudioPlayerPhase.paused;
});

final positionPercentageProvider = Provider<double>((ref) {
  return ref.watch(audioPlayerProvider.notifier).positionPercentage;
});

final displayPositionProvider = Provider<String>((ref) {
  return ref.watch(audioPlayerProvider.notifier).displayPosition;
});

final displayDurationProvider = Provider<String>((ref) {
  return ref.watch(audioPlayerProvider.notifier).displayDuration;
});

final audioErrorProvider = Provider<String?>((ref) {
  return ref.watch(audioPlayerProvider).error;
});

final createAudioJobProvider =
    FutureProvider.family<AudioGenerationJob, AudioGenerationRequest>(
  (ref, request) =>
      ref.watch(audioGenerationServiceProvider).createJob(request),
);

final getAudioJobProvider = FutureProvider.family<AudioGenerationJob, String>(
  (ref, jobId) => ref.watch(audioGenerationServiceProvider).getJob(jobId),
);

final retryAudioJobProvider = FutureProvider.family<AudioGenerationJob, String>(
  (ref, jobId) => ref.watch(audioGenerationServiceProvider).retryJob(jobId),
);

final cancelAudioJobProvider = FutureProvider.family<AudioGenerationJob, String>(
  (ref, jobId) => ref.watch(audioGenerationServiceProvider).cancelJob(jobId),
);

final createBatchAudioJobsProvider =
    FutureProvider.family<BatchAudioGenerationResponse, BatchAudioGenerationRequest>(
  (ref, request) =>
      ref.watch(audioGenerationServiceProvider).batchGenerate(request),
);
