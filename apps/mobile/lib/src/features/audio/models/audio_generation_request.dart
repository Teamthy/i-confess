/// Models for audio-generation requests and response payloads.
import 'package:flutter/foundation.dart';

@immutable
class AudioGenerationRequest {
  const AudioGenerationRequest({
    required this.confessionId,
    this.contentVersionId,
    this.variantId,
    required this.voiceId,
    this.provider = 'elevenlabs',
    this.qualityTier = 'standard',
    this.text,
  });

  final String confessionId;
  final String? contentVersionId;
  final String? variantId;
  final String voiceId;
  final String provider;
  final String qualityTier;
  final String? text;

  Map<String, dynamic> toApiJson() => {
        'confession_id': confessionId,
        if (contentVersionId != null) 'content_version_id': contentVersionId,
        if (variantId != null) 'variant_id': variantId,
        'voice_id': voiceId,
        'provider': provider,
        'quality_tier': qualityTier,
        if (text != null) 'text': text,
      };

  factory AudioGenerationRequest.fromJson(Map<String, dynamic> json) =>
      AudioGenerationRequest(
        confessionId: _string(json, 'confession_id', alias: 'confessionId'),
        contentVersionId:
            _optionalString(json, 'content_version_id', 'contentVersionId'),
        variantId: _optionalString(json, 'variant_id', 'variantId'),
        voiceId: _string(json, 'voice_id', alias: 'voiceId'),
        provider: _string(json, 'provider', fallback: 'elevenlabs'),
        qualityTier: _string(
          json,
          'quality_tier',
          alias: 'qualityTier',
          fallback: 'standard',
        ),
        text: _optionalString(json, 'text'),
      );
}

@immutable
class BatchAudioGenerationRequest {
  const BatchAudioGenerationRequest({
    required this.confessionIds,
    required this.voiceId,
    this.provider = 'elevenlabs',
    this.qualityTier = 'standard',
  });

  final List<String> confessionIds;
  final String voiceId;
  final String provider;
  final String qualityTier;

  Map<String, dynamic> toApiJson() => {
        'confession_ids': confessionIds,
        'voice_id': voiceId,
        'provider': provider,
        'quality_tier': qualityTier,
      };

  factory BatchAudioGenerationRequest.fromJson(Map<String, dynamic> json) =>
      BatchAudioGenerationRequest(
        confessionIds: (json['confession_ids'] ?? json['confessionIds']) is List
            ? ((json['confession_ids'] ?? json['confessionIds']) as List)
                .whereType<String>()
                .toList(growable: false)
            : const [],
        voiceId: _string(json, 'voice_id', alias: 'voiceId'),
        provider: _string(json, 'provider', fallback: 'elevenlabs'),
        qualityTier: _string(
          json,
          'quality_tier',
          alias: 'qualityTier',
          fallback: 'standard',
        ),
      );
}

@immutable
class AudioGenerationJobResponse {
  const AudioGenerationJobResponse({
    required this.jobId,
    required this.status,
    required this.message,
  });

  final String jobId;
  final String status;
  final String message;

  factory AudioGenerationJobResponse.fromJson(Map<String, dynamic> json) =>
      AudioGenerationJobResponse(
        jobId: _string(json, 'job_id', alias: 'jobId'),
        status: _string(json, 'status'),
        message: _string(json, 'message'),
      );
}

@immutable
class BatchAudioGenerationResponse {
  const BatchAudioGenerationResponse({
    required this.message,
    required this.jobIds,
    required this.totalRequested,
    required this.successCount,
    required this.failCount,
  });

  final String message;
  final List<String> jobIds;
  final int totalRequested;
  final int successCount;
  final int failCount;

  factory BatchAudioGenerationResponse.fromJson(Map<String, dynamic> json) =>
      BatchAudioGenerationResponse(
        message: _string(json, 'message'),
        jobIds: (json['job_ids'] ?? json['jobIds']) is List
            ? ((json['job_ids'] ?? json['jobIds']) as List)
                .whereType<String>()
                .toList(growable: false)
            : const [],
        totalRequested: _integer(json['total_requested'] ?? json['totalRequested']),
        successCount: _integer(json['success_count'] ?? json['successCount']),
        failCount: _integer(json['fail_count'] ?? json['failCount']),
      );
}

@immutable
class RecentFailure {
  const RecentFailure({
    required this.jobId,
    required this.confession,
    required this.voice,
    this.errorCode,
    this.error,
    required this.attempts,
  });

  final String jobId;
  final String confession;
  final String voice;
  final String? errorCode;
  final String? error;
  final int attempts;

  factory RecentFailure.fromJson(Map<String, dynamic> json) => RecentFailure(
        jobId: _string(json, 'job_id', alias: 'jobId'),
        confession: _string(json, 'confession'),
        voice: _string(json, 'voice'),
        errorCode: _optionalString(json, 'error_code', 'errorCode'),
        error: _optionalString(json, 'error'),
        attempts: _integer(json['attempts']),
      );
}

@immutable
class AudioGenerationStats {
  const AudioGenerationStats({
    this.total = 0,
    this.byStatus = const {},
    this.byProvider = const {},
    this.byVoice = const {},
    this.recentFailures = const [],
  });

  final int total;
  final Map<String, int> byStatus;
  final Map<String, int> byProvider;
  final Map<String, int> byVoice;
  final List<RecentFailure> recentFailures;

  factory AudioGenerationStats.fromJson(Map<String, dynamic> json) =>
      AudioGenerationStats(
        total: _integer(json['total']),
        byStatus: _intMap(json['by_status'] ?? json['byStatus']),
        byProvider: _intMap(json['by_provider'] ?? json['byProvider']),
        byVoice: _intMap(json['by_voice'] ?? json['byVoice']),
        recentFailures: (json['recent_failures'] ?? json['recentFailures']) is List
            ? ((json['recent_failures'] ?? json['recentFailures']) as List)
                .whereType<Map>()
                .map((item) => RecentFailure.fromJson(
                      Map<String, dynamic>.from(item),
                    ))
                .toList(growable: false)
            : const [],
      );
}

String _string(
  Map<String, dynamic> json,
  String key, {
  String? alias,
  String fallback = '',
}) {
  final value = json[key] ?? (alias == null ? null : json[alias]);
  return value is String ? value : fallback;
}

String? _optionalString(Map<String, dynamic> json, String key, [String? alias]) {
  final value = json[key] ?? (alias == null ? null : json[alias]);
  return value is String ? value : null;
}

int _integer(Object? value) {
  if (value is int) return value;
  if (value is num) return value.toInt();
  if (value is String) return int.tryParse(value) ?? 0;
  return 0;
}

Map<String, int> _intMap(Object? value) {
  if (value is! Map) return const {};
  return value.map((key, entry) => MapEntry(key.toString(), _integer(entry)));
}
