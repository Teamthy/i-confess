/// Audio generation job model for tracking async audio generation.
///
/// This model represents a job in the audio generation pipeline.
import 'package:freezed_annotation/freezed_annotation.dart';

part 'audio_generation_job.freezed.dart';
part 'audio_generation_job.g.dart';

/// Status of an audio generation job.
enum AudioJobStatus {
  queued,
  processing,
  succeeded,
  failed,
  cancelled,
}

/// Audio generation job model.
@freezed
class AudioGenerationJob with _$AudioGenerationJob {
  const factory AudioGenerationJob({
    required String id,
    required String confessionId,
    String? contentVersionId,
    String? variantId,
    required String voiceId,
    required String provider,
    String? qualityTier,
    String? format,
    @Default(AudioJobStatus.queued) AudioJobStatus status,
    @Default(0) int attemptCount,
    @Default(3) int maxAttempts,
    String? requestedBy,
    String? startedAt,
    String? completedAt,
    String? errorCode,
    String? errorMessage,
    String? audioAssetId,
    String? idempotencyKey,
    required String createdAt,
    required String updatedAt,
  }) = _AudioGenerationJob;

  factory AudioGenerationJob.fromJson(Map<String, dynamic> json) =>
      _$AudioGenerationJobFromJson(json);
}

/// Extension methods for AudioGenerationJob.
extension AudioGenerationJobExtensions on AudioGenerationJob {
  /// Check if the job is in a terminal state.
  bool get isTerminal => 
      status == AudioJobStatus.succeeded ||
      status == AudioJobStatus.failed ||
      status == AudioJobStatus.cancelled;

  /// Check if the job can be retried.
  bool get canRetry => 
      !isTerminal && attemptCount < maxAttempts;

  /// Check if the job is currently being processed.
  bool get isProcessing => status == AudioJobStatus.processing;

  /// Check if the job is waiting in the queue.
  bool get isQueued => status == AudioJobStatus.queued;
}
