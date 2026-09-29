/// Audio asset model representing a rendered audio file.
///
/// The API deliberately omits storage keys and playable URLs from this model.
/// Signed URLs are returned only on entitlement-checked session queue items.
import 'package:flutter/foundation.dart';

/// Status of a rendered audio asset.
enum AudioAssetStatus {
  uploading,
  processing,
  ready,
  published,
  failed,
  qaRejected,
  archived,
}

@immutable
class AudioAsset {
  const AudioAsset({
    required this.id,
    required this.confessionId,
    this.variantId,
    required this.voiceId,
    this.durationSeconds = 0,
    this.sizeBytes = 0,
    this.checksum,
    this.language,
    this.status = AudioAssetStatus.processing,
    this.contentVersionId,
    this.createdAt = '',
    this.updatedAt = '',
    this.qaReviewedBy,
    this.qaReviewedAt,
    this.qaNote,
    this.audioSource,
  });

  final String id;
  final String confessionId;
  final String? variantId;
  final String voiceId;
  final int durationSeconds;
  final int sizeBytes;
  final String? checksum;
  final String? language;
  final AudioAssetStatus status;
  final String? contentVersionId;
  final String createdAt;
  final String updatedAt;
  final String? qaReviewedBy;
  final String? qaReviewedAt;
  final String? qaNote;
  final String? audioSource;

  /// Duration of the asset. The backend stores whole seconds.
  Duration get duration => Duration(seconds: durationSeconds);

  /// Whether this asset is eligible for playback.
  bool get isReady =>
      status == AudioAssetStatus.ready || status == AudioAssetStatus.published;

  /// Display-ready duration, for example `05:30`.
  String get displayDuration {
    final minutes = durationSeconds ~/ 60;
    final seconds = durationSeconds % 60;
    return '${minutes.toString().padLeft(2, '0')}:${seconds.toString().padLeft(2, '0')}';
  }

  /// Display-ready file size.
  String get displaySize {
    if (sizeBytes < 1024) return '${sizeBytes}B';
    if (sizeBytes < 1024 * 1024) {
      return '${(sizeBytes / 1024).toStringAsFixed(1)}KB';
    }
    return '${(sizeBytes / (1024 * 1024)).toStringAsFixed(1)}MB';
  }

  AudioAsset copyWith({
    String? id,
    String? confessionId,
    String? variantId,
    String? voiceId,
    int? durationSeconds,
    int? sizeBytes,
    String? checksum,
    String? language,
    AudioAssetStatus? status,
    String? contentVersionId,
    String? createdAt,
    String? updatedAt,
    String? qaReviewedBy,
    String? qaReviewedAt,
    String? qaNote,
    String? audioSource,
  }) =>
      AudioAsset(
        id: id ?? this.id,
        confessionId: confessionId ?? this.confessionId,
        variantId: variantId ?? this.variantId,
        voiceId: voiceId ?? this.voiceId,
        durationSeconds: durationSeconds ?? this.durationSeconds,
        sizeBytes: sizeBytes ?? this.sizeBytes,
        checksum: checksum ?? this.checksum,
        language: language ?? this.language,
        status: status ?? this.status,
        contentVersionId: contentVersionId ?? this.contentVersionId,
        createdAt: createdAt ?? this.createdAt,
        updatedAt: updatedAt ?? this.updatedAt,
        qaReviewedBy: qaReviewedBy ?? this.qaReviewedBy,
        qaReviewedAt: qaReviewedAt ?? this.qaReviewedAt,
        qaNote: qaNote ?? this.qaNote,
        audioSource: audioSource ?? this.audioSource,
      );

  Map<String, dynamic> toJson() => {
        'id': id,
        'confession_id': confessionId,
        if (variantId != null) 'variant_id': variantId,
        'voice_id': voiceId,
        'duration_seconds': durationSeconds,
        'size_bytes': sizeBytes,
        if (checksum != null) 'checksum': checksum,
        if (language != null) 'language': language,
        'status': _statusJson(status),
        if (contentVersionId != null) 'content_version_id': contentVersionId,
        'created_at': createdAt,
        'updated_at': updatedAt,
        if (qaReviewedBy != null) 'qa_reviewed_by': qaReviewedBy,
        if (qaReviewedAt != null) 'qa_reviewed_at': qaReviewedAt,
        if (qaNote != null) 'qa_note': qaNote,
        if (audioSource != null) 'audio_source': audioSource,
      };

  factory AudioAsset.fromJson(Map<String, dynamic> json) => AudioAsset(
        id: _string(json, 'id'),
        confessionId: _string(json, 'confession_id', _string(json, 'confessionId')),
        variantId: _optionalString(json, 'variant_id', 'variantId'),
        voiceId: _string(json, 'voice_id', _string(json, 'voiceId')),
        durationSeconds: _integer(
          json['duration_seconds'] ?? json['durationSeconds'],
        ),
        sizeBytes: _integer(json['size_bytes'] ?? json['sizeBytes']),
        checksum: _optionalString(json, 'checksum'),
        language: _optionalString(json, 'language'),
        status: _parseStatus(_string(json, 'status', 'processing')),
        contentVersionId:
            _optionalString(json, 'content_version_id', 'contentVersionId'),
        createdAt: _string(json, 'created_at', _string(json, 'createdAt')),
        updatedAt: _string(json, 'updated_at', _string(json, 'updatedAt')),
        qaReviewedBy: _optionalString(json, 'qa_reviewed_by', 'qaReviewedBy'),
        qaReviewedAt: _optionalString(json, 'qa_reviewed_at', 'qaReviewedAt'),
        qaNote: _optionalString(json, 'qa_note', 'qaNote'),
        audioSource: _optionalString(json, 'audio_source', 'audioSource'),
      );
}

AudioAssetStatus _parseStatus(String value) {
  final normalized = value.replaceAll('_', '').toLowerCase();
  return AudioAssetStatus.values.firstWhere(
    (status) => status.name.toLowerCase() == normalized,
    orElse: () => AudioAssetStatus.processing,
  );
}

String _statusJson(AudioAssetStatus status) => switch (status) {
      AudioAssetStatus.qaRejected => 'qa_rejected',
      _ => status.name,
    };

String _string(Map<String, dynamic> json, String key, [String fallback = '']) {
  final value = json[key];
  return value is String ? value : fallback;
}

String? _optionalString(Map<String, dynamic> json, String key, [String? alias]) {
  final value = json[key] ?? (alias == null ? null : json[alias]);
  return value is String ? value : null;
}

int _integer(Object? value) {
  if (value is int) return value;
  if (value is num) return value.round();
  if (value is String) return int.tryParse(value) ?? 0;
  return 0;
}
