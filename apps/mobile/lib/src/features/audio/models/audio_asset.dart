/// Audio asset model representing a rendered audio file.
///
/// This model represents an audio asset that can be played.
import 'package:freezed_annotation/freezed_annotation.dart';

part 'audio_asset.freezed.dart';
part 'audio_asset.g.dart';

/// Status of an audio asset.
enum AudioAssetStatus {
  uploading,
  processing,
  ready,
  published,
  failed,
  qaRejected,
  archived,
}

/// Audio asset model.
@freezed
class AudioAsset with _$AudioAsset {
  const factory AudioAsset({
    required String id,
    required String confessionId,
    String? variantId,
    required String voiceId,
    required String url,
    @Default(0) int durationSeconds,
    @Default(0) int sizeBytes,
    String? checksum,
    String? language,
    @Default(AudioAssetStatus.processing) AudioAssetStatus status,
    String? contentVersionId,
    required String createdAt,
    required String updatedAt,
    String? qaReviewedBy,
    String? qaReviewedAt,
    String? qaNote,
    String? audioSource,
  }) = _AudioAsset;

  factory AudioAsset.fromJson(Map<String, dynamic> json) =>
      _$AudioAssetFromJson(json);
}

/// Extension methods for AudioAsset.
extension AudioAssetExtensions on AudioAsset {
  /// Check if the asset is ready for playback.
  bool get isReady => 
      status == AudioAssetStatus.ready ||
      status == AudioAssetStatus.published;

  /// Get the duration as a Duration object.
  Duration get duration => Duration(seconds: durationSeconds);

  /// Get a display-ready duration string (e.g., "5:30").
  String get displayDuration {
    final minutes = durationSeconds ~/ 60;
    final seconds = durationSeconds % 60;
    return '${minutes.toString().padLeft(2, '0')}:${seconds.toString().padLeft(2, '0')}';
  }

  /// Get the file size as a display-ready string.
  String get displaySize {
    if (sizeBytes < 1024) {
      return '${sizeBytes}B';
    } else if (sizeBytes < 1024 * 1024) {
      return '${(sizeBytes / 1024).toStringAsFixed(1)}KB';
    } else {
      return '${(sizeBytes / (1024 * 1024)).toStringAsFixed(1)}MB';
    }
  }
}
