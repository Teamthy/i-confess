/// Audio generation job model for tracking asynchronous renders.
import 'package:flutter/foundation.dart';

/// Lifecycle states used by the audio-generation API.
enum AudioJobStatus { queued, processing, succeeded, failed, cancelled }

@immutable
class AudioGenerationJob {
  const AudioGenerationJob({
    required this.id,
    this.confessionId = '',
    this.contentVersionId,
    this.variantId,
    this.voiceId = '',
    this.provider = 'default',
    this.qualityTier,
    this.format,
    this.status = AudioJobStatus.queued,
    this.attemptCount = 0,
    this.maxAttempts = 3,
    this.requestedBy,
    this.startedAt,
    this.completedAt,
    this.errorCode,
    this.errorMessage,
    this.audioAssetId,
    this.idempotencyKey,
    this.progress,
    this.createdAt = '',
    this.updatedAt = '',
  });

  final String id;
  final String confessionId;
  final String? contentVersionId;
  final String? variantId;
  final String voiceId;
  final String provider;
  final String? qualityTier;
  final String? format;
  final AudioJobStatus status;
  final int attemptCount;
  final int maxAttempts;
  final String? requestedBy;
  final String? startedAt;
  final String? completedAt;
  final String? errorCode;
  final String? errorMessage;
  final String? audioAssetId;
  final String? idempotencyKey;
  final double? progress;
  final String createdAt;
  final String updatedAt;

  factory AudioGenerationJob.fromJson(Map<String, dynamic> json) {
    final rawStatus = _string(json, 'status', 'queued').toLowerCase();
    final status = AudioJobStatus.values.firstWhere(
      (value) => value.name == rawStatus,
      orElse: () => AudioJobStatus.queued,
    );
    final rawProgress = json['progress'];

    return AudioGenerationJob(
      id: _string(json, 'id', _string(json, 'job_id', _string(json, 'jobId'))),
      confessionId: _string(json, 'confession_id', _string(json, 'confessionId')),
      contentVersionId:
          _optionalString(json, 'content_version_id', 'contentVersionId'),
      variantId: _optionalString(json, 'variant_id', 'variantId'),
      voiceId: _string(json, 'voice_id', _string(json, 'voiceId')),
      provider: _string(json, 'provider', 'default'),
      qualityTier: _optionalString(json, 'quality_tier', 'qualityTier'),
      format: _optionalString(json, 'format'),
      status: status,
      attemptCount: _integer(
        json['attempt_count'] ?? json['attemptCount'] ?? json['attempt'],
      ),
      maxAttempts: _integer(json['max_attempts'] ?? json['maxAttempts'], 3),
      requestedBy: _optionalString(json, 'requested_by', 'requestedBy'),
      startedAt: _optionalString(json, 'started_at', 'startedAt'),
      completedAt: _optionalString(json, 'completed_at', 'completedAt'),
      errorCode: _optionalString(json, 'error_code', 'errorCode'),
      errorMessage: _optionalString(json, 'error_message', 'errorMessage'),
      audioAssetId: _optionalString(json, 'audio_asset_id', 'audioAssetId'),
      idempotencyKey:
          _optionalString(json, 'idempotency_key', 'idempotencyKey'),
      progress: rawProgress is num ? rawProgress.toDouble() : null,
      createdAt: _string(json, 'created_at', _string(json, 'createdAt')),
      updatedAt: _string(json, 'updated_at', _string(json, 'updatedAt')),
    );
  }

  Map<String, dynamic> toJson() => {
        'id': id,
        'confession_id': confessionId,
        if (contentVersionId != null) 'content_version_id': contentVersionId,
        if (variantId != null) 'variant_id': variantId,
        'voice_id': voiceId,
        'provider': provider,
        if (qualityTier != null) 'quality_tier': qualityTier,
        if (format != null) 'format': format,
        'status': status.name,
        'attempt_count': attemptCount,
        'max_attempts': maxAttempts,
        if (requestedBy != null) 'requested_by': requestedBy,
        if (startedAt != null) 'started_at': startedAt,
        if (completedAt != null) 'completed_at': completedAt,
        if (errorCode != null) 'error_code': errorCode,
        if (errorMessage != null) 'error_message': errorMessage,
        if (audioAssetId != null) 'audio_asset_id': audioAssetId,
        if (idempotencyKey != null) 'idempotency_key': idempotencyKey,
        if (progress != null) 'progress': progress,
        'created_at': createdAt,
        'updated_at': updatedAt,
      };

  AudioGenerationJob copyWith({
    String? id,
    String? confessionId,
    String? contentVersionId,
    String? variantId,
    String? voiceId,
    String? provider,
    String? qualityTier,
    String? format,
    AudioJobStatus? status,
    int? attemptCount,
    int? maxAttempts,
    String? requestedBy,
    String? startedAt,
    String? completedAt,
    String? errorCode,
    String? errorMessage,
    String? audioAssetId,
    String? idempotencyKey,
    double? progress,
    String? createdAt,
    String? updatedAt,
  }) =>
      AudioGenerationJob(
        id: id ?? this.id,
        confessionId: confessionId ?? this.confessionId,
        contentVersionId: contentVersionId ?? this.contentVersionId,
        variantId: variantId ?? this.variantId,
        voiceId: voiceId ?? this.voiceId,
        provider: provider ?? this.provider,
        qualityTier: qualityTier ?? this.qualityTier,
        format: format ?? this.format,
        status: status ?? this.status,
        attemptCount: attemptCount ?? this.attemptCount,
        maxAttempts: maxAttempts ?? this.maxAttempts,
        requestedBy: requestedBy ?? this.requestedBy,
        startedAt: startedAt ?? this.startedAt,
        completedAt: completedAt ?? this.completedAt,
        errorCode: errorCode ?? this.errorCode,
        errorMessage: errorMessage ?? this.errorMessage,
        audioAssetId: audioAssetId ?? this.audioAssetId,
        idempotencyKey: idempotencyKey ?? this.idempotencyKey,
        progress: progress ?? this.progress,
        createdAt: createdAt ?? this.createdAt,
        updatedAt: updatedAt ?? this.updatedAt,
      );
}

extension AudioGenerationJobExtensions on AudioGenerationJob {
  bool get isTerminal =>
      status == AudioJobStatus.succeeded ||
      status == AudioJobStatus.failed ||
      status == AudioJobStatus.cancelled;

  bool get canRetry =>
      (status == AudioJobStatus.failed || status == AudioJobStatus.cancelled) &&
      attemptCount < maxAttempts;

  bool get isProcessing => status == AudioJobStatus.processing;

  bool get isQueued => status == AudioJobStatus.queued;
}

String _string(Map<String, dynamic> json, String key, [String fallback = '']) {
  final value = json[key];
  return value is String ? value : fallback;
}

String? _optionalString(Map<String, dynamic> json, String key, [String? alias]) {
  final value = json[key] ?? (alias == null ? null : json[alias]);
  return value is String ? value : null;
}

int _integer(Object? value, [int fallback = 0]) {
  if (value is int) return value;
  if (value is num) return value.toInt();
  if (value is String) return int.tryParse(value) ?? fallback;
  return fallback;
}
