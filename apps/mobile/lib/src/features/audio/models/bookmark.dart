/// Bookmark model for I-Confess.
///
/// This model allows users to save specific positions in audio for later
/// quick access, similar to bookmarks in a book.
import 'package:flutter/foundation.dart';

/// Represents a bookmark in an audio asset.
///
/// A bookmark saves a specific position in an audio file so the user
/// can quickly return to that position later.
@immutable
class AudioBookmark {
  /// Unique identifier for this bookmark.
  final String id;
  
  /// The ID of the audio asset this bookmark belongs to.
  final String assetId;
  
  /// The ID of the confession this bookmark belongs to.
  final String confessionId;
  
  /// The position in the audio where the bookmark is set.
  final Duration position;
  
  /// The title/name of this bookmark.
  final String title;
  
  /// Optional description/notes for this bookmark.
  final String? description;
  
  /// When this bookmark was created.
  final DateTime createdAt;
  
  /// When this bookmark was last accessed.
  final DateTime lastAccessedAt;
  
  /// The color associated with this bookmark (for visual distinction).
  final int color;
  
  /// The icon associated with this bookmark.
  final String icon;

  const AudioBookmark({
    required this.id,
    required this.assetId,
    required this.confessionId,
    required this.position,
    required this.title,
    this.description,
    required this.createdAt,
    required this.lastAccessedAt,
    this.color = 0xFFFFFFFF,
    this.icon = 'bookmark',
  });

  /// Creates a new bookmark with auto-generated ID and timestamps.
  factory AudioBookmark.create({
    required String assetId,
    required String confessionId,
    required Duration position,
    required String title,
    String? description,
    int color = 0xFFFFFFFF,
    String icon = 'bookmark',
  }) {
    final now = DateTime.now();
    return AudioBookmark(
      id: '${assetId}_${now.millisecondsSinceEpoch}',
      assetId: assetId,
      confessionId: confessionId,
      position: position,
      title: title,
      description: description,
      createdAt: now,
      lastAccessedAt: now,
      color: color,
      icon: icon,
    );
  }

  /// Gets the position as a formatted string.
  String get positionString {
    final minutes = position.inMinutes;
    final seconds = position.inSeconds.remainder(60);
    return '${minutes.toString().padLeft(2, '0')}:${seconds.toString().padLeft(2, '0')}';
  }

  /// Copy with new values.
  AudioBookmark copyWith({
    String? id,
    String? assetId,
    String? confessionId,
    Duration? position,
    String? title,
    String? description,
    DateTime? createdAt,
    DateTime? lastAccessedAt,
    int? color,
    String? icon,
  }) {
    return AudioBookmark(
      id: id ?? this.id,
      assetId: assetId ?? this.assetId,
      confessionId: confessionId ?? this.confessionId,
      position: position ?? this.position,
      title: title ?? this.title,
      description: description ?? this.description,
      createdAt: createdAt ?? this.createdAt,
      lastAccessedAt: lastAccessedAt ?? this.lastAccessedAt,
      color: color ?? this.color,
      icon: icon ?? this.icon,
    );
  }

  /// Marks this bookmark as accessed (updates lastAccessedAt).
  AudioBookmark markAsAccessed() {
    return copyWith(lastAccessedAt: DateTime.now());
  }

  /// Convert to JSON.
  Map<String, dynamic> toJson() {
    return {
      'id': id,
      'assetId': assetId,
      'confessionId': confessionId,
      'positionMs': position.inMilliseconds,
      'title': title,
      'description': description,
      'createdAt': createdAt.toIso8601String(),
      'lastAccessedAt': lastAccessedAt.toIso8601String(),
      'color': color,
      'icon': icon,
    };
  }

  /// Create from JSON.
  factory AudioBookmark.fromJson(Map<String, dynamic> json) {
    return AudioBookmark(
      id: json['id'] as String,
      assetId: json['assetId'] as String,
      confessionId: json['confessionId'] as String,
      position: Duration(milliseconds: json['positionMs'] as int),
      title: json['title'] as String,
      description: json['description'] as String?,
      createdAt: DateTime.parse(json['createdAt'] as String),
      lastAccessedAt: DateTime.parse(json['lastAccessedAt'] as String),
      color: json['color'] as int? ?? 0xFFFFFFFF,
      icon: json['icon'] as String? ?? 'bookmark',
    );
  }

  @override
  String toString() {
    return 'AudioBookmark(id: $id, assetId: $assetId, title: $title, position: $positionString)';
  }

  @override
  bool operator ==(Object other) {
    if (identical(this, other)) return true;
    return other is AudioBookmark && other.id == id;
  }

  @override
  int get hashCode => id.hashCode;
}

/// Represents a collection of bookmarks.
///
/// Manages multiple bookmarks for different audio assets.
@immutable
class BookmarkCollection {
  /// List of all bookmarks.
  final List<AudioBookmark> bookmarks;
  
  /// Maximum number of bookmarks to keep.
  static const int maxBookmarks = 500;

  const BookmarkCollection({this.bookmarks = const []});

  /// Whether the collection is empty.
  bool get isEmpty => bookmarks.isEmpty;
  
  /// Whether the collection is not empty.
  bool get isNotEmpty => bookmarks.isNotEmpty;
  
  /// Total number of bookmarks.
  int get length => bookmarks.length;

  /// Get bookmarks for a specific asset.
  List<AudioBookmark> forAsset(String assetId) {
    return bookmarks
        .where((bookmark) => bookmark.assetId == assetId)
        .toList();
  }

  /// Get bookmarks for a specific confession.
  List<AudioBookmark> forConfession(String confessionId) {
    return bookmarks
        .where((bookmark) => bookmark.confessionId == confessionId)
        .toList();
  }

  /// Get a specific bookmark by ID.
  AudioBookmark? getBookmark(String id) {
    return bookmarks.firstWhere(
      (bookmark) => bookmark.id == id,
      orElse: () => null,
    );
  }

  /// Check if a bookmark exists for an asset at a position.
  bool hasBookmark(String assetId, Duration position, {Duration tolerance = const Duration(seconds: 5)}) {
    return bookmarks.any((bookmark) => 
        bookmark.assetId == assetId &&
        (bookmark.position - position).abs() <= tolerance
    );
  }

  /// Add a new bookmark.
  BookmarkCollection add(AudioBookmark bookmark) {
    final newBookmarks = [...bookmarks, bookmark];
    
    // Limit to maxBookmarks
    if (newBookmarks.length > maxBookmarks) {
      newBookmarks.removeAt(0); // Remove oldest
    }
    
    return BookmarkCollection(bookmarks: newBookmarks);
  }

  /// Remove a bookmark by ID.
  BookmarkCollection remove(String id) {
    return BookmarkCollection(
      bookmarks: bookmarks.where((bookmark) => bookmark.id != id).toList(),
    );
  }

  /// Remove all bookmarks for a specific asset.
  BookmarkCollection removeForAsset(String assetId) {
    return BookmarkCollection(
      bookmarks: bookmarks.where((bookmark) => bookmark.assetId != assetId).toList(),
    );
  }

  /// Remove all bookmarks for a specific confession.
  BookmarkCollection removeForConfession(String confessionId) {
    return BookmarkCollection(
      bookmarks: bookmarks.where((bookmark) => bookmark.confessionId != confessionId).toList(),
    );
  }

  /// Remove all bookmarks.
  BookmarkCollection clear() {
    return const BookmarkCollection(bookmarks: []);
  }

  /// Update a bookmark.
  BookmarkCollection update(AudioBookmark bookmark) {
    final index = bookmarks.indexWhere((b) => b.id == bookmark.id);
    if (index >= 0) {
      final newBookmarks = List<AudioBookmark>.from(bookmarks);
      newBookmarks[index] = bookmark;
      return BookmarkCollection(bookmarks: newBookmarks);
    }
    return this;
  }

  /// Mark a bookmark as accessed.
  BookmarkCollection markAsAccessed(String id) {
    final bookmark = getBookmark(id);
    if (bookmark != null) {
      return update(bookmark.markAsAccessed());
    }
    return this;
  }

  /// Sort bookmarks by creation date (newest first).
  BookmarkCollection sortByDate() {
    return BookmarkCollection(
      bookmarks: [...bookmarks]..sort((a, b) => b.createdAt.compareTo(a.createdAt)),
    );
  }

  /// Sort bookmarks by last accessed (most recent first).
  BookmarkCollection sortByAccessed() {
    return BookmarkCollection(
      bookmarks: [...bookmarks]..sort((a, b) => b.lastAccessedAt.compareTo(a.lastAccessedAt)),
    );
  }

  /// Sort bookmarks by position.
  BookmarkCollection sortByPosition() {
    return BookmarkCollection(
      bookmarks: [...bookmarks]..sort((a, b) => a.position.compareTo(b.position)),
    );
  }

  /// Copy with new bookmarks.
  BookmarkCollection copyWith({List<AudioBookmark>? bookmarks}) {
    return BookmarkCollection(bookmarks: bookmarks ?? this.bookmarks);
  }

  /// Convert to JSON.
  Map<String, dynamic> toJson() {
    return {
      'bookmarks': bookmarks.map((bookmark) => bookmark.toJson()).toList(),
    };
  }

  /// Create from JSON.
  factory BookmarkCollection.fromJson(Map<String, dynamic> json) {
    return BookmarkCollection(
      bookmarks: (json['bookmarks'] as List<dynamic>? ?? [])
          .map((bookmark) => AudioBookmark.fromJson(bookmark as Map<String, dynamic>))
          .toList(),
    );
  }

  @override
  String toString() {
    return 'BookmarkCollection(bookmarks: $length)';
  }
}

/// Predefined bookmark colors.
class BookmarkColors {
  static const int red = 0xFFFF0000;
  static const int orange = 0xFFFF8C00;
  static const int yellow = 0xFFFFFF00;
  static const int green = 0xFF00FF00;
  static const int blue = 0xFF0000FF;
  static const int purple = 0xFF800080;
  static const int pink = 0xFFFF00FF;
  static const int white = 0xFFFFFFFF;
  static const int black = 0xFF000000;
  
  static const List<int> all = [
    red,
    orange,
    yellow,
    green,
    blue,
    purple,
    pink,
    white,
  ];
  
  static int getColor(int index) {
    return all[index % all.length];
  }
}

/// Predefined bookmark icons.
class BookmarkIcons {
  static const String bookmark = 'bookmark';
  static const String star = 'star';
  static const String heart = 'heart';
  static const String flag = 'flag';
  static const String note = 'note';
  static const String pin = 'pin';
  static const String tag = 'tag';
  static const String ribbon = 'ribbon';
  
  static const List<String> all = [
    bookmark,
    star,
    heart,
    flag,
    note,
    pin,
    tag,
    ribbon,
  ];
  
  static String getIcon(int index) {
    return all[index % all.length];
  }
}
