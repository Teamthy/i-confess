/// Audio generation request model.
///
/// Used to request audio generation for a confession.
import 'package:freezed_annotation/freezed_annotation.dart';

part 'audio_generation_request.freezed.dart';
part 'audio_generation_request.g.dart';

/// Audio generation request.
@freezed
class AudioGenerationRequest with _$AudioGenerationRequest {
  const factory AudioGenerationRequest({
    required String confessionId,
    String? contentVersionId,
    String? variantId,
    required String voiceId,
    @Default('elevenlabs') String provider,
    @Default('standard') String qualityTier,
    String? text,
  }) = _AudioGenerationRequest;

  factory AudioGenerationRequest.fromJson(Map<String, dynamic> json) =>
      _$AudioGenerationRequestFromJson(json);

  /// Convert to JSON for API requests.
  Map<String, dynamic> toApiJson() => {
    'confession_id': confessionId,
    if (contentVersionId != null) 'content_version_id': contentVersionId,
    if (variantId != null) 'variant_id': variantId,
    'voice_id': voiceId,
    'provider': provider,
    'quality_tier': qualityTier,
    if (text != null) 'text': text,
  };
}

/// Batch audio generation request.
@freezed
class BatchAudioGenerationRequest with _$BatchAudioGenerationRequest {
  const factory BatchAudioGenerationRequest({
    required List<String> confessionIds,
    required String voiceId,
    @Default('elevenlabs') String provider,
    @Default('standard') String qualityTier,
  }) = _BatchAudioGenerationRequest;

  factory BatchAudioGenerationRequest.fromJson(Map<String, dynamic> json) =>
      _$BatchAudioGenerationRequestFromJson(json);

  /// Convert to JSON for API requests.
  Map<String, dynamic> toApiJson() => {
    'confession_ids': confessionIds,
    'voice_id': voiceId,
    'provider': provider,
    'quality_tier': qualityTier,
  };
}

/// Response for creating an audio generation job.
@freezed
class AudioGenerationJobResponse with _$AudioGenerationJobResponse {
  const factory AudioGenerationJobResponse({
    required String jobId,
    required String status,
    required String message,
  }) = _AudioGenerationJobResponse;

  factory AudioGenerationJobResponse.fromJson(Map<String, dynamic> json) =>
      _$AudioGenerationJobResponseFromJson(json);
}

/// Response for batch audio generation.
@freezed
class BatchAudioGenerationResponse with _$BatchAudioGenerationResponse {
  const factory BatchAudioGenerationResponse({
    required String message,
    required List<String> jobIds,
    required int totalRequested,
    required int successCount,
    required int failCount,
  }) = _BatchAudioGenerationResponse;

  factory BatchAudioGenerationResponse.fromJson(Map<String, dynamic> json) =>
      _$BatchAudioGenerationResponseFromJson(json);
}

/// Audio generation statistics.
@freezed
class AudioGenerationStats with _$AudioGenerationStats {
  const factory AudioGenerationStats({
    @Default(0) int total,
    @Default({}) Map<String, int> byStatus,
    @Default({}) Map<String, int> byProvider,
    @Default({}) Map<String, int> byVoice,
    @Default([]) List<RecentFailure> recentFailures,
  }) = _AudioGenerationStats;

  factory AudioGenerationStats.fromJson(Map<String, dynamic> json) =>
      _$AudioGenerationStatsFromJson(json);
}

/// Recent failure information.
@freezed
class RecentFailure with _$RecentFailure {
  const factory RecentFailure({
    required String jobId,
    required String confession,
    required String voice,
    String? errorCode,
    String? error,
    required int attempts,
  }) = _RecentFailure;

  factory RecentFailure.fromJson(Map<String, dynamic> json) =>
      _$RecentFailureFromJson(json);
}
