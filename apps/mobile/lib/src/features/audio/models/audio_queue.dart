/// Audio queue model for I-Confess.
///
/// This model represents a queue of audio items to be played sequentially.
import 'package:flutter/material.dart';
import 'audio_asset.dart';

/// Represents an item in the audio queue.
@immutable
class AudioQueueItem {
  /// Unique identifier for this queue item.
  final String id;
  
  /// The audio asset to play.
  final AudioAsset asset;
  
  /// The confession ID this audio belongs to.
  final String confessionId;
  
  /// The voice ID used for this audio.
  final String voiceId;
  
  /// Display title for this item.
  final String title;
  
  /// Display subtitle (e.g., voice name, date).
  final String? subtitle;
  
  /// Duration of the audio.
  final Duration duration;
  
  /// Custom order index (for manual reordering).
  final int orderIndex;
  
  /// Whether this item has been played.
  final bool played;
  
  /// Position where playback was last paused (if any).
  final Duration? lastPosition;

  const AudioQueueItem({
    required this.id,
    required this.asset,
    required this.confessionId,
    required this.voiceId,
    required this.title,
    this.subtitle,
    required this.duration,
    this.orderIndex = 0,
    this.played = false,
    this.lastPosition,
  });

  /// Creates a queue item from an audio asset.
  factory AudioQueueItem.fromAsset({
    required AudioAsset asset,
    String? title,
    String? subtitle,
    int orderIndex = 0,
  }) {
    return AudioQueueItem(
      id: '${asset.id}_${DateTime.now().millisecondsSinceEpoch}',
      asset: asset,
      confessionId: asset.confessionId,
      voiceId: asset.voiceId,
      title: title ?? 'Confession ${asset.confessionId}',
      subtitle: subtitle,
      duration: asset.duration,
      orderIndex: orderIndex,
    );
  }

  /// Creates a queue item from minimal information.
  factory AudioQueueItem.minimal({
    required String assetId,
    required String confessionId,
    required String voiceId,
    required String title,
    String? subtitle,
    required Duration duration,
    int orderIndex = 0,
  }) {
    return AudioQueueItem(
      id: '${assetId}_${DateTime.now().millisecondsSinceEpoch}',
      asset: AudioAsset(
        id: assetId,
        confessionId: confessionId,
        voiceId: voiceId,
        status: AudioAssetStatus.ready,
        durationSeconds: duration.inSeconds,
        sizeBytes: 0,
        createdAt: DateTime.now().toIso8601String(),
        updatedAt: DateTime.now().toIso8601String(),
      ),
      confessionId: confessionId,
      voiceId: voiceId,
      title: title,
      subtitle: subtitle,
      duration: duration,
      orderIndex: orderIndex,
    );
  }

  /// Copy with new values.
  AudioQueueItem copyWith({
    String? id,
    AudioAsset? asset,
    String? confessionId,
    String? voiceId,
    String? title,
    String? subtitle,
    Duration? duration,
    int? orderIndex,
    bool? played,
    Duration? lastPosition,
  }) {
    return AudioQueueItem(
      id: id ?? this.id,
      asset: asset ?? this.asset,
      confessionId: confessionId ?? this.confessionId,
      voiceId: voiceId ?? this.voiceId,
      title: title ?? this.title,
      subtitle: subtitle ?? this.subtitle,
      duration: duration ?? this.duration,
      orderIndex: orderIndex ?? this.orderIndex,
      played: played ?? this.played,
      lastPosition: lastPosition ?? this.lastPosition,
    );
  }

  /// Convert to JSON.
  Map<String, dynamic> toJson() {
    return {
      'id': id,
      'assetId': asset.id,
      'confessionId': confessionId,
      'voiceId': voiceId,
      'title': title,
      'subtitle': subtitle,
      'durationMs': duration.inMilliseconds,
      'orderIndex': orderIndex,
      'played': played,
      'lastPositionMs': lastPosition?.inMilliseconds,
    };
  }

  /// Create from JSON.
  factory AudioQueueItem.fromJson(Map<String, dynamic> json) {
    return AudioQueueItem(
      id: json['id'] as String,
      asset: AudioAsset(
        id: json['assetId'] as String,
        confessionId: json['confessionId'] as String,
        voiceId: json['voiceId'] as String,
        status: AudioAssetStatus.ready,
        durationSeconds: (json['durationMs'] as int) ~/ 1000,
        sizeBytes: 0,
        createdAt: DateTime.now().toIso8601String(),
        updatedAt: DateTime.now().toIso8601String(),
      ),
      confessionId: json['confessionId'] as String,
      voiceId: json['voiceId'] as String,
      title: json['title'] as String,
      subtitle: json['subtitle'] as String?,
      duration: Duration(milliseconds: json['durationMs'] as int),
      orderIndex: json['orderIndex'] as int? ?? 0,
      played: json['played'] as bool? ?? false,
      lastPosition: json['lastPositionMs'] != null 
          ? Duration(milliseconds: json['lastPositionMs'] as int) 
          : null,
    );
  }

  @override
  String toString() {
    return 'AudioQueueItem(id: $id, title: $title, duration: $duration)';
  }

  @override
  bool operator ==(Object other) {
    if (identical(this, other)) return true;
    return other is AudioQueueItem && other.id == id;
  }

  @override
  int get hashCode => id.hashCode;
}

/// Represents the audio queue.
@immutable
class AudioQueue {
  /// List of items in the queue.
  final List<AudioQueueItem> items;
  
  /// Index of the currently playing item.
  final int currentIndex;
  
  /// Whether the queue is in shuffle mode.
  final bool isShuffled;
  
  /// Whether the queue is in repeat mode.
  final RepeatMode repeatMode;
  
  /// Whether the queue is currently being modified.
  final bool isUpdating;

  const AudioQueue({
    this.items = const [],
    this.currentIndex = -1,
    this.isShuffled = false,
    this.repeatMode = RepeatMode.none,
    this.isUpdating = false,
  });

  /// Whether the queue is empty.
  bool get isEmpty => items.isEmpty;
  
  /// Whether the queue has items.
  bool get isNotEmpty => items.isNotEmpty;
  
  /// Total number of items in the queue.
  int get length => items.length;
  
  /// Total duration of all items in the queue.
  Duration get totalDuration {
    return items.fold(
      Duration.zero,
      (sum, item) => sum + item.duration,
    );
  }
  
  /// Number of items played.
  int get playedCount => items.where((item) => item.played).length;
  
  /// Number of items remaining.
  int get remainingCount => items.where((item) => !item.played).length;
  
  /// Whether there is a current item.
  bool get hasCurrentItem => currentIndex >= 0 && currentIndex < items.length;
  
  /// Gets the current item, if any.
  AudioQueueItem? get currentItem {
    if (!hasCurrentItem) return null;
    return items[currentIndex];
  }
  
  /// Gets the next item, if any.
  AudioQueueItem? get nextItem {
    if (isEmpty) return null;
    
    if (isShuffled) {
      // In shuffle mode, pick a random unplayed item
      final unplayed = items.where((item) => !item.played).toList();
      if (unplayed.isEmpty) {
        // If all played, start from beginning
        return items.first;
      }
      return unplayed[DateTime.now().millisecondsSinceEpoch % unplayed.length];
    }
    
    // Normal mode
    if (currentIndex < items.length - 1) {
      return items[currentIndex + 1];
    }
    
    // At end of queue
    if (repeatMode == RepeatMode.all) {
      return items.first;
    }
    
    return null;
  }
  
  /// Gets the previous item, if any.
  AudioQueueItem? get previousItem {
    if (isEmpty) return null;
    
    if (isShuffled) {
      // In shuffle mode, pick a random played item
      final played = items.where((item) => item.played).toList();
      if (played.isEmpty) {
        return null;
      }
      return played[DateTime.now().millisecondsSinceEpoch % played.length];
    }
    
    // Normal mode
    if (currentIndex > 0) {
      return items[currentIndex - 1];
    }
    
    // At beginning of queue
    if (repeatMode == RepeatMode.all) {
      return items.last;
    }
    
    return null;
  }

  /// Copy with new values.
  AudioQueue copyWith({
    List<AudioQueueItem>? items,
    int? currentIndex,
    bool? isShuffled,
    RepeatMode? repeatMode,
    bool? isUpdating,
  }) {
    return AudioQueue(
      items: items ?? this.items,
      currentIndex: currentIndex ?? this.currentIndex,
      isShuffled: isShuffled ?? this.isShuffled,
      repeatMode: repeatMode ?? this.repeatMode,
      isUpdating: isUpdating ?? this.isUpdating,
    );
  }

  /// Convert to JSON.
  Map<String, dynamic> toJson() {
    return {
      'items': items.map((item) => item.toJson()).toList(),
      'currentIndex': currentIndex,
      'isShuffled': isShuffled,
      'repeatMode': repeatMode.name,
    };
  }

  /// Create from JSON.
  factory AudioQueue.fromJson(Map<String, dynamic> json) {
    return AudioQueue(
      items: (json['items'] as List<dynamic>? ?? [])
          .map((item) => AudioQueueItem.fromJson(item as Map<String, dynamic>))
          .toList(),
      currentIndex: json['currentIndex'] as int? ?? -1,
      isShuffled: json['isShuffled'] as bool? ?? false,
      repeatMode: RepeatMode.values.firstWhere(
        (mode) => mode.name == json['repeatMode'],
        orElse: () => RepeatMode.none,
      ),
    );
  }

  @override
  String toString() {
    return 'AudioQueue(items: $length, currentIndex: $currentIndex, isShuffled: $isShuffled, repeatMode: $repeatMode)';
  }
}

/// Repeat modes for the audio queue.
enum RepeatMode {
  /// No repeat - play queue once and stop.
  none,
  
  /// Repeat all - play queue repeatedly.
  all,
  
  /// Repeat one - repeat the current item.
  one,
}

/// Extension methods for RepeatMode.
extension RepeatModeExtension on RepeatMode {
  /// Gets the next mode in the cycle.
  RepeatMode get next {
    switch (this) {
      case RepeatMode.none:
        return RepeatMode.all;
      case RepeatMode.all:
        return RepeatMode.one;
      case RepeatMode.one:
        return RepeatMode.none;
    }
  }

  /// Gets the display name.
  String get displayName {
    switch (this) {
      case RepeatMode.none:
        return 'No Repeat';
      case RepeatMode.all:
        return 'Repeat All';
      case RepeatMode.one:
        return 'Repeat One';
    }
  }

  /// Gets the icon for this mode.
  IconData get icon {
    switch (this) {
      case RepeatMode.none:
        return Icons.repeat;
      case RepeatMode.all:
        return Icons.repeat;
      case RepeatMode.one:
        return Icons.repeat_one;
    }
  }
}
