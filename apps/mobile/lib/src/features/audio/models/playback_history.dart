/// Playback history model for I-Confess.
///
/// This model tracks which audio items users have listened to,
/// including when they listened, how much they listened, and other metadata.
import 'package:flutter/foundation.dart';
import 'audio_queue.dart';

/// Represents a playback history entry.
///
/// Tracks when a user listened to an audio item, how much they listened,
/// and whether they completed it.
@immutable
class PlaybackHistoryEntry {
  /// Unique identifier for this history entry.
  final String id;
  
  /// The queue item that was played.
  final AudioQueueItem item;
  
  /// Timestamp when playback started.
  final DateTime startedAt;
  
  /// Timestamp when playback ended (null if still playing).
  final DateTime? endedAt;
  
  /// Total duration listened.
  final Duration listenedDuration;
  
  /// Total duration of the audio.
  final Duration totalDuration;
  
  /// Whether the audio was completed (listened to end).
  final bool completed;
  
  /// Last position where playback stopped.
  final Duration lastPosition;
  
  /// Whether this was part of a queue or standalone.
  final bool wasInQueue;
  
  /// Index in the queue when played (if applicable).
  final int? queueIndex;
  
  /// Whether this was played in shuffle mode.
  final bool wasShuffled;
  
  /// Repeat mode when played.
  final RepeatMode repeatMode;

  const PlaybackHistoryEntry({
    required this.id,
    required this.item,
    required this.startedAt,
    this.endedAt,
    required this.listenedDuration,
    required this.totalDuration,
    this.completed = false,
    required this.lastPosition,
    this.wasInQueue = false,
    this.queueIndex,
    this.wasShuffled = false,
    this.repeatMode = RepeatMode.none,
  });

  /// Creates a new history entry when playback starts.
  factory PlaybackHistoryEntry.started({
    required AudioQueueItem item,
    required DateTime startedAt,
    required Duration totalDuration,
    bool wasInQueue = false,
    int? queueIndex,
    bool wasShuffled = false,
    RepeatMode repeatMode = RepeatMode.none,
  }) {
    return PlaybackHistoryEntry(
      id: '${item.id}_${startedAt.millisecondsSinceEpoch}',
      item: item,
      startedAt: startedAt,
      endedAt: null,
      listenedDuration: Duration.zero,
      totalDuration: totalDuration,
      completed: false,
      lastPosition: Duration.zero,
      wasInQueue: wasInQueue,
      queueIndex: queueIndex,
      wasShuffled: wasShuffled,
      repeatMode: repeatMode,
    );
  }

  /// Updates the entry when playback progresses.
  PlaybackHistoryEntry update({
    Duration? listenedDuration,
    Duration? lastPosition,
  }) {
    return PlaybackHistoryEntry(
      id: id,
      item: item,
      startedAt: startedAt,
      endedAt: endedAt,
      listenedDuration: listenedDuration ?? this.listenedDuration,
      totalDuration: totalDuration,
      completed: completed,
      lastPosition: lastPosition ?? this.lastPosition,
      wasInQueue: wasInQueue,
      queueIndex: queueIndex,
      wasShuffled: wasShuffled,
      repeatMode: repeatMode,
    );
  }

  /// Marks the entry as completed.
  PlaybackHistoryEntry complete({required DateTime endedAt}) {
    return PlaybackHistoryEntry(
      id: id,
      item: item,
      startedAt: startedAt,
      endedAt: endedAt,
      listenedDuration: totalDuration, // Listened to all
      totalDuration: totalDuration,
      completed: true,
      lastPosition: totalDuration, // At the end
      wasInQueue: wasInQueue,
      queueIndex: queueIndex,
      wasShuffled: wasShuffled,
      repeatMode: repeatMode,
    );
  }

  /// Marks the entry as ended at a specific position.
  PlaybackHistoryEntry end({
    required DateTime endedAt,
    required Duration lastPosition,
  }) {
    final listenedDuration = lastPosition;
    final completed = lastPosition >= totalDuration;
    
    return PlaybackHistoryEntry(
      id: id,
      item: item,
      startedAt: startedAt,
      endedAt: endedAt,
      listenedDuration: listenedDuration,
      totalDuration: totalDuration,
      completed: completed,
      lastPosition: lastPosition,
      wasInQueue: wasInQueue,
      queueIndex: queueIndex,
      wasShuffled: wasShuffled,
      repeatMode: repeatMode,
    );
  }

  /// Gets the percentage of audio listened to.
  double get percentListened {
    if (totalDuration.inMilliseconds == 0) return 0.0;
    return (listenedDuration.inMilliseconds / totalDuration.inMilliseconds).clamp(0.0, 1.0);
  }

  /// Gets the duration listened as a formatted string.
  String get listenedDurationString {
    final minutes = listenedDuration.inMinutes;
    final seconds = listenedDuration.inSeconds.remainder(60);
    return '${minutes.toString().padLeft(2, '0')}:${seconds.toString().padLeft(2, '0')}';
  }

  /// Gets the total duration as a formatted string.
  String get totalDurationString {
    final minutes = totalDuration.inMinutes;
    final seconds = totalDuration.inSeconds.remainder(60);
    return '${minutes.toString().padLeft(2, '0')}:${seconds.toString().padLeft(2, '0')}';
  }

  /// Gets the time listened as a formatted string.
  String get timeListenedString {
    final duration = endedAt?.difference(startedAt) ?? Duration.zero;
    final minutes = duration.inMinutes;
    final seconds = duration.inSeconds.remainder(60);
    return '${minutes.toString().padLeft(2, '0')}:${seconds.toString().padLeft(2, '0')}';
  }

  /// Copy with new values.
  PlaybackHistoryEntry copyWith({
    String? id,
    AudioQueueItem? item,
    DateTime? startedAt,
    DateTime? endedAt,
    Duration? listenedDuration,
    Duration? totalDuration,
    bool? completed,
    Duration? lastPosition,
    bool? wasInQueue,
    int? queueIndex,
    bool? wasShuffled,
    RepeatMode? repeatMode,
  }) {
    return PlaybackHistoryEntry(
      id: id ?? this.id,
      item: item ?? this.item,
      startedAt: startedAt ?? this.startedAt,
      endedAt: endedAt ?? this.endedAt,
      listenedDuration: listenedDuration ?? this.listenedDuration,
      totalDuration: totalDuration ?? this.totalDuration,
      completed: completed ?? this.completed,
      lastPosition: lastPosition ?? this.lastPosition,
      wasInQueue: wasInQueue ?? this.wasInQueue,
      queueIndex: queueIndex ?? this.queueIndex,
      wasShuffled: wasShuffled ?? this.wasShuffled,
      repeatMode: repeatMode ?? this.repeatMode,
    );
  }

  /// Convert to JSON.
  Map<String, dynamic> toJson() {
    return {
      'id': id,
      'item': item.toJson(),
      'startedAt': startedAt.toIso8601String(),
      'endedAt': endedAt?.toIso8601String(),
      'listenedDurationMs': listenedDuration.inMilliseconds,
      'totalDurationMs': totalDuration.inMilliseconds,
      'completed': completed,
      'lastPositionMs': lastPosition.inMilliseconds,
      'wasInQueue': wasInQueue,
      'queueIndex': queueIndex,
      'wasShuffled': wasShuffled,
      'repeatMode': repeatMode.name,
    };
  }

  /// Create from JSON.
  factory PlaybackHistoryEntry.fromJson(Map<String, dynamic> json) {
    return PlaybackHistoryEntry(
      id: json['id'] as String,
      item: AudioQueueItem.fromJson(json['item'] as Map<String, dynamic>),
      startedAt: DateTime.parse(json['startedAt'] as String),
      endedAt: json['endedAt'] != null 
          ? DateTime.parse(json['endedAt'] as String) 
          : null,
      listenedDuration: Duration(
        milliseconds: json['listenedDurationMs'] as int,
      ),
      totalDuration: Duration(
        milliseconds: json['totalDurationMs'] as int,
      ),
      completed: json['completed'] as bool? ?? false,
      lastPosition: Duration(
        milliseconds: json['lastPositionMs'] as int,
      ),
      wasInQueue: json['wasInQueue'] as bool? ?? false,
      queueIndex: json['queueIndex'] as int?,
      wasShuffled: json['wasShuffled'] as bool? ?? false,
      repeatMode: RepeatMode.values.firstWhere(
        (mode) => mode.name == json['repeatMode'],
        orElse: () => RepeatMode.none,
      ),
    );
  }

  @override
  String toString() {
    return 'PlaybackHistoryEntry(id: $id, item: ${item.title}, '\
           'started: $startedAt, ended: $endedAt, '\
           'listened: $listenedDurationString/$totalDurationString, '\
           'completed: $completed)';
  }

  @override
  bool operator ==(Object other) {
    if (identical(this, other)) return true;
    return other is PlaybackHistoryEntry && other.id == id;
  }

  @override
  int get hashCode => id.hashCode;
}

/// Represents the playback history.
///
/// Contains all history entries with methods for filtering and analysis.
@immutable
class PlaybackHistory {
  /// List of history entries, sorted by date (newest first).
  final List<PlaybackHistoryEntry> entries;
  
  /// Maximum number of entries to keep.
  static const int maxEntries = 1000;

  const PlaybackHistory({this.entries = const []});

  /// Whether the history is empty.
  bool get isEmpty => entries.isEmpty;
  
  /// Whether the history is not empty.
  bool get isNotEmpty => entries.isNotEmpty;
  
  /// Total number of entries.
  int get length => entries.length;
  
  /// Total listening time across all entries.
  Duration get totalListeningTime {
    return entries.fold(
      Duration.zero,
      (sum, entry) => sum + entry.listenedDuration,
    );
  }
  
  /// Total number of completed entries.
  int get completedCount => entries.where((e) => e.completed).length;
  
  /// Completion rate (0.0 to 1.0).
  double get completionRate {
    if (entries.isEmpty) return 0.0;
    return completedCount / entries.length;
  }
  
  /// Average listening percentage.
  double get averagePercentListened {
    if (entries.isEmpty) return 0.0;
    final total = entries.fold<double>(
      0.0,
      (sum, entry) => sum + entry.percentListened,
    );
    return total / entries.length;
  }
  
  /// Most listened confession.
  PlaybackHistoryEntry? get mostListened {
    if (entries.isEmpty) return null;
    return entries.reduce((a, b) => a.listenedDuration > b.listenedDuration ? a : b);
  }
  
  /// Most recently played.
  PlaybackHistoryEntry? get mostRecent {
    if (entries.isEmpty) return null;
    return entries.first;
  }
  
  /// Get entries for a specific confession.
  List<PlaybackHistoryEntry> forConfession(String confessionId) {
    return entries
        .where((entry) => entry.item.confessionId == confessionId)
        .toList();
  }
  
  /// Get entries for a specific voice.
  List<PlaybackHistoryEntry> forVoice(String voiceId) {
    return entries
        .where((entry) => entry.item.voiceId == voiceId)
        .toList();
  }
  
  /// Get entries from a specific date.
  List<PlaybackHistoryEntry> forDate(DateTime date) {
    final startOfDay = DateTime(date.year, date.month, date.day);
    final endOfDay = startOfDay.add(const Duration(days: 1));
    
    return entries
        .where((entry) => 
            entry.startedAt.isAfter(startOfDay) &&
            entry.startedAt.isBefore(endOfDay)
        )
        .toList();
  }
  
  /// Get entries from the last N days.
  List<PlaybackHistoryEntry> lastNDays(int days) {
    final cutoff = DateTime.now().subtract(Duration(days: days));
    return entries
        .where((entry) => entry.startedAt.isAfter(cutoff))
        .toList();
  }
  
  /// Get today's entries.
  List<PlaybackHistoryEntry> get today => forDate(DateTime.now());
  
  /// Get yesterday's entries.
  List<PlaybackHistoryEntry> get yesterday => forDate(
    DateTime.now().subtract(const Duration(days: 1)),
  );
  
  /// Get this week's entries.
  List<PlaybackHistoryEntry> get thisWeek => lastNDays(7);
  
  /// Get this month's entries.
  List<PlaybackHistoryEntry> get thisMonth => lastNDays(30);
  
  /// Get entries by completion status.
  List<PlaybackHistoryEntry> byCompletion(bool completed) {
    return entries.where((entry) => entry.completed == completed).toList();
  }
  
  /// Get entries by queue status.
  List<PlaybackHistoryEntry> byQueueStatus(bool wasInQueue) {
    return entries.where((entry) => entry.wasInQueue == wasInQueue).toList();
  }
  
  /// Add a new entry.
  PlaybackHistory add(PlaybackHistoryEntry entry) {
    final newEntries = [entry, ...entries];
    
    // Limit to maxEntries
    if (newEntries.length > maxEntries) {
      newEntries.removeLast();
    }
    
    return PlaybackHistory(entries: newEntries);
  }
  
  /// Remove an entry by ID.
  PlaybackHistory remove(String id) {
    return PlaybackHistory(
      entries: entries.where((entry) => entry.id != id).toList(),
    );
  }
  
  /// Remove all entries.
  PlaybackHistory clear() {
    return const PlaybackHistory(entries: []);
  }
  
  /// Remove entries older than a certain date.
  PlaybackHistory removeOlderThan(DateTime cutoff) {
    return PlaybackHistory(
      entries: entries.where((entry) => entry.startedAt.isAfter(cutoff)).toList(),
    );
  }
  
  /// Copy with new entries.
  PlaybackHistory copyWith({List<PlaybackHistoryEntry>? entries}) {
    return PlaybackHistory(entries: entries ?? this.entries);
  }
  
  /// Convert to JSON.
  Map<String, dynamic> toJson() {
    return {
      'entries': entries.map((entry) => entry.toJson()).toList(),
    };
  }
  
  /// Create from JSON.
  factory PlaybackHistory.fromJson(Map<String, dynamic> json) {
    return PlaybackHistory(
      entries: (json['entries'] as List<dynamic>? ?? [])
          .map((entry) => PlaybackHistoryEntry.fromJson(entry as Map<String, dynamic>))
          .toList(),
    );
  }
  
  @override
  String toString() {
    return 'PlaybackHistory(entries: $length, totalTime: $totalListeningTime)';
  }
}

/// Statistics for playback history.
class PlaybackHistoryStats {
  final int totalEntries;
  final int completedEntries;
  final int incompleteEntries;
  final Duration totalListeningTime;
  final double completionRate;
  final double averagePercentListened;
  final Map<String, int> byConfession;
  final Map<String, int> byVoice;
  final Map<String, int> byDate;

  const PlaybackHistoryStats({
    required this.totalEntries,
    required this.completedEntries,
    required this.incompleteEntries,
    required this.totalListeningTime,
    required this.completionRate,
    required this.averagePercentListened,
    required this.byConfession,
    required this.byVoice,
    required this.byDate,
  });

  /// Create stats from a playback history.
  factory PlaybackHistoryStats.fromHistory(PlaybackHistory history) {
    final byConfession = <String, int>{};
    final byVoice = <String, int>{};
    final byDate = <String, int>{};
    
    for (final entry in history.entries) {
      // Count by confession
      byConfession[entry.item.confessionId] = 
          (byConfession[entry.item.confessionId] ?? 0) + 1;
      
      // Count by voice
      byVoice[entry.item.voiceId] = 
          (byVoice[entry.item.voiceId] ?? 0) + 1;
      
      // Count by date
      final dateKey = '${entry.startedAt.year}-${entry.startedAt.month.toString().padLeft(2, '0')}-${entry.startedAt.day.toString().padLeft(2, '0')}';
      byDate[dateKey] = (byDate[dateKey] ?? 0) + 1;
    }
    
    return PlaybackHistoryStats(
      totalEntries: history.length,
      completedEntries: history.completedCount,
      incompleteEntries: history.length - history.completedCount,
      totalListeningTime: history.totalListeningTime,
      completionRate: history.completionRate,
      averagePercentListened: history.averagePercentListened,
      byConfession: byConfession,
      byVoice: byVoice,
      byDate: byDate,
    );
  }
}
