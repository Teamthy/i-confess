# Phase 4: Advanced Audio Features - Implementation Guide

## Status: 🟡 **IN PROGRESS** (Started 2026-09-29)

Phase 4 builds upon Phase 1-3 by adding **advanced audio features** that enhance the user experience with additional functionality like playback history, bookmarks, and offline mode.

---

## Overview

### Phase 4 Features

| Feature | Priority | Status | Effort | User Impact |
|---------|----------|--------|--------|-------------|
| **Playback History** | High | ✅ Implemented | 3-4 hours | High |
| **Bookmarks** | High | ✅ Implemented | 2-3 hours | High |
| **Offline Mode** | High | ✅ Implemented | 4-5 hours | High |
| **Crossfade** | Medium | ⏳ Pending | 2 days | Medium |
| **Equalizer** | Medium | ⏳ Pending | 3 days | Medium |
| **Sleep Timer** | Medium | ⏳ Pending | 1 day | Medium |

### What's Been Implemented

✅ **Playback History** - Complete model and service
✅ **Bookmarks** - Complete model with color and icon support
✅ **Offline Mode** - Complete caching service

---

## File Structure

### Phase 4 Files Created

```
apps/mobile/lib/src/features/audio/
├── models/
│   ├── playback_history.dart          # NEW - Playback history model
│   ├── bookmark.dart                 # NEW - Bookmark model
│   └── ... (existing)
└── services/
    ├── offline_audio_service.dart    # NEW - Offline caching service
    └── ... (existing)

docs/
└── PHASE4-IMPLEMENTATION.md          # This file
```

---

## Feature 1: Playback History ✅

### Overview

Playback history tracks which audio items users have listened to, including:
- When they started listening
- How much they listened
- Whether they completed the audio
- Queue context (was it in a queue?)
- Playback mode (shuffle, repeat)

### Models

#### PlaybackHistoryEntry

```dart
class PlaybackHistoryEntry {
  final String id;
  final AudioQueueItem item;
  final DateTime startedAt;
  final DateTime? endedAt;
  final Duration listenedDuration;
  final Duration totalDuration;
  final bool completed;
  final Duration lastPosition;
  final bool wasInQueue;
  final int? queueIndex;
  final bool wasShuffled;
  final RepeatMode repeatMode;
}
```

#### PlaybackHistory

```dart
class PlaybackHistory {
  final List<PlaybackHistoryEntry> entries;
  
  // Statistics
  Duration get totalListeningTime;
  int get completedCount;
  double get completionRate;
  double get averagePercentListened;
  
  // Filtering
  List<PlaybackHistoryEntry> forConfession(String confessionId);
  List<PlaybackHistoryEntry> forVoice(String voiceId);
  List<PlaybackHistoryEntry> forDate(DateTime date);
  List<PlaybackHistoryEntry> lastNDays(int days);
  List<PlaybackHistoryEntry> today;
  List<PlaybackHistoryEntry> yesterday;
  List<PlaybackHistoryEntry> thisWeek;
  List<PlaybackHistoryEntry> thisMonth;
  
  // Management
  PlaybackHistory add(PlaybackHistoryEntry entry);
  PlaybackHistory remove(String id);
  PlaybackHistory clear();
  PlaybackHistory removeOlderThan(DateTime cutoff);
}
```

#### PlaybackHistoryStats

```dart
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
}
```

### Usage Examples

#### Track Playback

```dart
// When playback starts
final historyEntry = PlaybackHistoryEntry.started(
  item: queueItem,
  startedAt: DateTime.now(),
  totalDuration: queueItem.duration,
  wasInQueue: true,
  queueIndex: queue.currentIndex,
  wasShuffled: queue.isShuffled,
  repeatMode: queue.repeatMode,
);

// Update as playback progresses
final updatedEntry = historyEntry.update(
  listenedDuration: currentPosition,
  lastPosition: currentPosition,
);

// When playback completes
final completedEntry = historyEntry.complete(
  endedAt: DateTime.now(),
);

// When playback ends at a position
final endedEntry = historyEntry.end(
  endedAt: DateTime.now(),
  lastPosition: currentPosition,
);
```

#### Query History

```dart
// Get today's history
final today = history.today;

// Get this week's history
final thisWeek = history.thisWeek;

// Get history for a specific confession
final confessionHistory = history.forConfession('confession_123');

// Get history for a specific voice
final voiceHistory = history.forVoice('voice_456');

// Get statistics
final stats = PlaybackHistoryStats.fromHistory(history);
print('Total listening time: ${stats.totalListeningTime}');
print('Completion rate: ${stats.completionRate * 100}%');
```

### Service Integration

```dart
// In your audio player controller
class AudioPlayerNotifier extends StateNotifier<AudioPlayerState> {
  PlaybackHistory _history = PlaybackHistory();
  PlaybackHistoryEntry? _currentEntry;
  
  // When playback starts
  void _onPlaybackStarted(AudioQueueItem item) {
    _currentEntry = PlaybackHistoryEntry.started(
      item: item,
      startedAt: DateTime.now(),
      totalDuration: item.duration,
      wasInQueue: true,
      // ... other fields
    );
  }
  
  // When playback progresses
  void _onPlaybackProgress(Duration position) {
    if (_currentEntry != null) {
      _currentEntry = _currentEntry!.update(
        listenedDuration: position,
        lastPosition: position,
      );
    }
  }
  
  // When playback completes
  void _onPlaybackCompleted() {
    if (_currentEntry != null) {
      final completed = _currentEntry!.complete(endedAt: DateTime.now());
      _history = _history.add(completed);
      _currentEntry = null;
    }
  }
  
  // When playback stops
  void _onPlaybackStopped() {
    if (_currentEntry != null) {
      final ended = _currentEntry!.end(
        endedAt: DateTime.now(),
        lastPosition: state.position,
      );
      _history = _history.add(ended);
      _currentEntry = null;
    }
  }
}
```

---

## Feature 2: Bookmarks ✅

### Overview

Bookmarks allow users to save specific positions in audio for later quick access. Each bookmark includes:
- Position in the audio
- Title/name
- Optional description
- Color for visual distinction
- Icon for visual distinction
- Creation and access timestamps

### Models

#### AudioBookmark

```dart
class AudioBookmark {
  final String id;
  final String assetId;
  final String confessionId;
  final Duration position;
  final String title;
  final String? description;
  final DateTime createdAt;
  final DateTime lastAccessedAt;
  final int color;
  final String icon;
}
```

#### BookmarkCollection

```dart
class BookmarkCollection {
  final List<AudioBookmark> bookmarks;
  
  // Filtering
  List<AudioBookmark> forAsset(String assetId);
  List<AudioBookmark> forConfession(String confessionId);
  AudioBookmark? getBookmark(String id);
  bool hasBookmark(String assetId, Duration position);
  
  // Management
  BookmarkCollection add(AudioBookmark bookmark);
  BookmarkCollection remove(String id);
  BookmarkCollection removeForAsset(String assetId);
  BookmarkCollection removeForConfession(String confessionId);
  BookmarkCollection clear();
  BookmarkCollection update(AudioBookmark bookmark);
  BookmarkCollection markAsAccessed(String id);
  
  // Sorting
  BookmarkCollection sortByDate();
  BookmarkCollection sortByAccessed();
  BookmarkCollection sortByPosition();
}
```

#### Predefined Colors and Icons

```dart
class BookmarkColors {
  static const int red = 0xFFFF0000;
  static const int orange = 0xFFFF8C00;
  static const int yellow = 0xFFFFFF00;
  static const int green = 0xFF00FF00;
  static const int blue = 0xFF0000FF;
  static const int purple = 0xFF800080;
  static const int pink = 0xFFFF00FF;
  static const List<int> all = [...];
}

class BookmarkIcons {
  static const String bookmark = 'bookmark';
  static const String star = 'star';
  static const String heart = 'heart';
  static const String flag = 'flag';
  static const String note = 'note';
  static const List<String> all = [...];
}
```

### Usage Examples

#### Create a Bookmark

```dart
// Create a bookmark at current position
final bookmark = AudioBookmark.create(
  assetId: currentAssetId,
  confessionId: currentConfessionId,
  position: currentPosition,
  title: 'Important Moment',
  description: 'This is where the story gets interesting',
  color: BookmarkColors.blue,
  icon: BookmarkIcons.star,
);

// Add to collection
bookmarkCollection = bookmarkCollection.add(bookmark);
```

#### Query Bookmarks

```dart
// Get all bookmarks for an asset
final assetBookmarks = bookmarkCollection.forAsset('asset_123');

// Get all bookmarks for a confession
final confessionBookmarks = bookmarkCollection.forConfession('confession_456');

// Check if a bookmark exists at a position
final hasBookmark = bookmarkCollection.hasBookmark(
  'asset_123',
  Duration(seconds: 30),
);
```

#### Manage Bookmarks

```dart
// Remove a bookmark
bookmarkCollection = bookmarkCollection.remove('bookmark_123');

// Remove all bookmarks for an asset
bookmarkCollection = bookmarkCollection.removeForAsset('asset_123');

// Update a bookmark
final updatedBookmark = bookmark.copyWith(
  title: 'New Title',
  description: 'Updated description',
);
bookmarkCollection = bookmarkCollection.update(updatedBookmark);

// Mark as accessed
bookmarkCollection = bookmarkCollection.markAsAccessed('bookmark_123');
```

### UI Integration

```dart
// Bookmark button
IconButton(
  icon: Icon(Icons.bookmark_border),
  onPressed: () {
    final bookmark = AudioBookmark.create(
      assetId: currentAssetId,
      confessionId: currentConfessionId,
      position: currentPosition,
      title: 'Bookmark ${DateTime.now().millisecondsSinceEpoch}',
    );
    bookmarkCollection = bookmarkCollection.add(bookmark);
    ScaffoldMessenger.of(context).showSnackBar(
      SnackBar(content: Text('Bookmark added')),
    );
  },
)

// Bookmark list
ListView.builder(
  itemCount: bookmarkCollection.length,
  itemBuilder: (context, index) {
    final bookmark = bookmarkCollection.bookmarks[index];
    return ListTile(
      leading: Icon(Icons.bookmark, color: Color(bookmark.color)),
      title: Text(bookmark.title),
      subtitle: Text(bookmark.positionString),
      onTap: () {
        // Seek to bookmark position
        audioController.seek(bookmark.position);
        // Mark as accessed
        bookmarkCollection = bookmarkCollection.markAsAccessed(bookmark.id);
      },
    );
  },
)
```

---

## Feature 3: Offline Mode ✅

### Overview

Offline mode allows users to cache audio files for playback without an internet connection. Features include:
- Download and cache audio files
- Manage cache size (number of files and total size)
- Preload next items in queue
- Check offline availability
- Get offline URLs for cached files

### Service

#### OfflineAudioService

```dart
class OfflineAudioService {
  // Configuration
  final OfflineAudioConfig config;
  
  // Status
  Stream<Map<String, CachedAudioInfo>> get onCacheStatusChanged;
  Stream<Map<String, double>> get onDownloadProgressChanged;
  Stream<int> get onCacheSizeChanged;
  Map<String, CachedAudioInfo> get cache;
  int get cacheSize;
  int get cacheCount;
  bool get isCacheFull;
  
  // Methods
  Future<void> init();
  OfflineAudioStatus getStatus(String assetId);
  bool isCached(String assetId);
  String? getFilePath(String assetId);
  double? getDownloadProgress(String assetId);
  Future<void> cacheAsset({...});
  Future<void> cacheAssets(List<AudioAsset> assets);
  Future<void> cacheQueue(AudioQueue queue);
  Future<void> preloadNextInQueue(AudioQueue queue, int count);
  Future<void> removeAsset(String assetId);
  Future<void> clearCache();
  Future<void> removeExpired();
  Future<File?> getCachedFile(String assetId);
  Future<Uint8List?> getCachedBytes(String assetId);
  Stream<Uint8List>? getCachedStream(String assetId);
  List<CachedAudioInfo> getAllCached();
  Future<int> getTotalCacheSize();
  Future<Map<String, dynamic>> getStats();
  Future<bool> isOfflineAvailable(String assetId);
  Future<String?> getOfflineUrl(String assetId);
  Future<void> dispose();
}
```

#### OfflineAudioConfig

```dart
class OfflineAudioConfig {
  final int maxCacheSize;        // Maximum number of files (default: 100)
  final int maxCacheBytes;       // Maximum total size in bytes (default: 1GB)
  final String cacheDirectoryName; // Directory name (default: 'i_confess_audio_cache')
  final bool wifiOnly;           // Only cache on WiFi (default: true)
  final bool preloadNextInQueue; // Preload next items (default: true)
  final int preloadCount;        // Number of items to preload (default: 3)
}
```

#### CachedAudioInfo

```dart
class CachedAudioInfo {
  final String assetId;
  final String filePath;
  final int fileSize;
  final DateTime cachedAt;
  final DateTime? expiresAt;
  final OfflineAudioStatus status;
  final double? downloadProgress;
  final String? errorMessage;
  
  bool get isReady;
  bool get isExpired;
}
```

### Usage Examples

#### Cache an Asset

```dart
// Cache a single asset
final offlineService = await OfflineAudioService.create();
await offlineService.cacheAsset(
  assetId: 'asset_123',
  url: 'https://api.yourdomain.com/audio/asset_123.mp3',
  fileName: 'asset_123.mp3',
  fileSize: 1024000,
);

// Listen to download progress
offlineService.onDownloadProgressChanged.listen((progress) {
  print('Download progress: $progress');
});

// Listen to cache status
offlineService.onCacheStatusChanged.listen((cache) {
  print('Cache updated: ${cache.length} items');
});
```

#### Cache a Queue

```dart
// Cache all items in a queue
await offlineService.cacheQueue(queue);

// Preload next items
await offlineService.preloadNextInQueue(queue, 3);
```

#### Check and Use Offline Files

```dart
// Check if an asset is cached
final isCached = offlineService.isCached('asset_123');

// Get offline URL
final offlineUrl = await offlineService.getOfflineUrl('asset_123');

// Play cached file
await audioPlayer.playAsset(
  assetId: 'asset_123',
  // Use offline URL if available
  url: offlineUrl,
);
```

#### Manage Cache

```dart
// Remove a single asset
await offlineService.removeAsset('asset_123');

// Remove expired assets
await offlineService.removeExpired();

// Clear all cache
await offlineService.clearCache();

// Get cache statistics
final stats = await offlineService.getStats();
print('Cache size: ${stats['totalSizeMB']} MB');
print('Cached items: ${stats['cachedCount']}');
```

---

## Integration Guide

### Step 1: Add Dependencies

Add to `pubspec.yaml`:

```yaml
dependencies:
  # Existing dependencies
  flutter_riverpod: ^2.4.9
  just_audio: ^0.9.34
  audio_session: ^0.1.16
  http: ^1.1.0
  
  # Phase 3 dependencies (should already be there)
  just_audio_background: ^0.0.1-beta.10
  shared_preferences: ^2.2.2
  reorderables: ^0.5.0
  
  # Phase 4 dependencies
  path_provider: ^2.1.1        # For file system access
  path: ^1.8.3                # For path manipulation
```

Run:
```bash
flutter pub get
```

### Step 2: Initialize Services

In your main.dart or initialization:

```dart
// Initialize offline audio service
final offlineService = await OfflineAudioService.create();
await offlineService.init();

// Initialize playback history
final history = PlaybackHistory();

// Initialize bookmark collection
final bookmarkCollection = BookmarkCollection();
```

### Step 3: Set Up Riverpod Providers

```dart
// Offline audio service provider
final offlineAudioServiceProvider = Provider<OfflineAudioService>((ref) {
  // In a real app, you might want to create this lazily
  throw UnimplementedError('Must be initialized separately');
});

// Playback history provider
final playbackHistoryProvider = StateProvider<PlaybackHistory>((ref) {
  return PlaybackHistory();
});

// Bookmark collection provider
final bookmarkCollectionProvider = StateProvider<BookmarkCollection>((ref) {
  return BookmarkCollection();
});
```

### Step 4: Integrate with Audio Player

```dart
// In your audio player controller
class AudioPlayerNotifier extends StateNotifier<AudioPlayerState> {
  final OfflineAudioService _offlineService;
  final StateProviderRef _ref;
  
  AudioPlayerNotifier(this._offlineService, this._ref) : super(...) {
    // Listen to playback events
    _playbackService.playbackEventStream.listen(_handlePlaybackEvent);
  }
  
  void _handlePlaybackEvent(PlaybackEvent event) {
    // Update history
    _updateHistory(event);
    
    // Preload next items
    _preloadNextItems();
  }
  
  void _updateHistory(PlaybackEvent event) {
    final historyNotifier = _ref.read(playbackHistoryProvider.notifier);
    
    switch (event.processingState) {
      case ProcessingState.ready:
        // Playback started
        final queue = _ref.read(currentQueueProvider);
        if (queue.currentItem != null) {
          final entry = PlaybackHistoryEntry.started(
            item: queue.currentItem!,
            startedAt: DateTime.now(),
            totalDuration: queue.currentItem!.duration,
            wasInQueue: true,
            queueIndex: queue.currentIndex,
            wasShuffled: queue.isShuffled,
            repeatMode: queue.repeatMode,
          );
          historyNotifier.state = historyNotifier.state.add(entry);
        }
        break;
        
      case ProcessingState.buffering:
      case ProcessingState.playing:
        // Playback progressing
        final history = historyNotifier.state;
        if (history.entries.isNotEmpty) {
          final currentEntry = history.entries.first;
          final updated = currentEntry.update(
            listenedDuration: event.position,
            lastPosition: event.position,
          );
          historyNotifier.state = history.update(updated);
        }
        break;
        
      case ProcessingState.completed:
        // Playback completed
        final history = historyNotifier.state;
        if (history.entries.isNotEmpty) {
          final currentEntry = history.entries.first;
          final completed = currentEntry.complete(endedAt: DateTime.now());
          historyNotifier.state = history.add(completed);
        }
        break;
        
      case ProcessingState.idle:
      case ProcessingState.stopped:
        // Playback stopped
        final history = historyNotifier.state;
        if (history.entries.isNotEmpty) {
          final currentEntry = history.entries.first;
          final ended = currentEntry.end(
            endedAt: DateTime.now(),
            lastPosition: event.position,
          );
          historyNotifier.state = history.add(ended);
        }
        break;
    }
  }
  
  void _preloadNextItems() {
    if (_offlineService._config.preloadNextInQueue) {
      final queue = _ref.read(currentQueueProvider);
      _offlineService.preloadNextInQueue(queue, _offlineService._config.preloadCount);
    }
  }
}
```

### Step 5: Add UI for New Features

#### Playback History Screen

```dart
class PlaybackHistoryScreen extends ConsumerWidget {
  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final history = ref.watch(playbackHistoryProvider);
    final stats = PlaybackHistoryStats.fromHistory(history);
    
    return Scaffold(
      appBar: AppBar(title: Text('Playback History')),
      body: Column(
        children: [
          // Statistics
          Card(
            child: Padding(
              padding: const EdgeInsets.all(16),
              child: Column(
                children: [
                  Text('Total Listening Time', style: Theme.of(context).textTheme.titleMedium),
                  Text('${stats.totalListeningTime.inHours}h ${stats.totalListeningTime.inMinutes.remainder(60)}m'),
                  SizedBox(height: 8),
                  Text('Completion Rate: ${(stats.completionRate * 100).toStringAsFixed(1)}%'),
                  SizedBox(height: 8),
                  Text('Total Sessions: ${stats.totalEntries}'),
                ],
              ),
            ),
          ),
          
          // History list
          Expanded(
            child: ListView.builder(
              itemCount: history.length,
              itemBuilder: (context, index) {
                final entry = history.entries[index];
                return ListTile(
                  leading: Icon(
                    entry.completed ? Icons.check_circle : Icons.radio_button_unchecked,
                    color: entry.completed ? Colors.green : Colors.grey,
                  ),
                  title: Text(entry.item.title),
                  subtitle: Text('${entry.startedAt} - ${entry.listenedDurationString}'),
                  trailing: Text('${(entry.percentListened * 100).toStringAsFixed(0)}%'),
                  onTap: () {
                    // Play this item
                    final queueController = ref.read(queueControllerProvider.notifier);
                    queueController.clear();
                    queueController.addItem(entry.item);
                    queueController.playItemAt(0);
                  },
                );
              },
            ),
          ),
        ],
      ),
    );
  }
}
```

#### Bookmarks Screen

```dart
class BookmarksScreen extends ConsumerWidget {
  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final bookmarkCollection = ref.watch(bookmarkCollectionProvider);
    
    return Scaffold(
      appBar: AppBar(title: Text('Bookmarks')),
      body: bookmarkCollection.isEmpty
          ? Center(
              child: Text('No bookmarks yet'),
            )
          : ListView.builder(
              itemCount: bookmarkCollection.length,
              itemBuilder: (context, index) {
                final bookmark = bookmarkCollection.bookmarks[index];
                return ListTile(
                  leading: Icon(
                    Icons.bookmark,
                    color: Color(bookmark.color),
                  ),
                  title: Text(bookmark.title),
                  subtitle: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text(bookmark.positionString),
                      if (bookmark.description != null)
                        Text(
                          bookmark.description!,
                          style: Theme.of(context).textTheme.bodySmall,
                        ),
                    ],
                  ),
                  trailing: IconButton(
                    icon: Icon(Icons.delete),
                    onPressed: () {
                      final notifier = ref.read(bookmarkCollectionProvider.notifier);
                      notifier.state = notifier.state.remove(bookmark.id);
                    },
                  ),
                  onTap: () {
                    // Seek to bookmark position
                    final controller = ref.read(audioPlayerControllerProvider.notifier);
                    controller.seek(bookmark.position);
                    
                    // Mark as accessed
                    final notifier = ref.read(bookmarkCollectionProvider.notifier);
                    notifier.state = notifier.state.markAsAccessed(bookmark.id);
                  },
                );
              },
            ),
    );
  }
}
```

#### Offline Mode Screen

```dart
class OfflineModeScreen extends ConsumerWidget {
  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final offlineServiceAsync = ref.watch(offlineAudioServiceAsyncProvider);
    
    return Scaffold(
      appBar: AppBar(title: Text('Offline Mode')),
      body: offlineServiceAsync.when(
        loading: () => Center(child: CircularProgressIndicator()),
        error: (error, stack) => Center(child: Text('Error: $error')),
        data: (offlineService) => _buildOfflineContent(context, ref, offlineService),
      ),
    );
  }
  
  Widget _buildOfflineContent(
    BuildContext context,
    WidgetRef ref,
    OfflineAudioService offlineService,
  ) {
    final cached = offlineService.getAllCached();
    
    return Column(
      children: [
        // Cache statistics
        Card(
          child: Padding(
            padding: const EdgeInsets.all(16),
            child: FutureBuilder<Map<String, dynamic>>(
              future: offlineService.getStats(),
              builder: (context, snapshot) {
                if (snapshot.hasData) {
                  final stats = snapshot.data!;
                  return Column(
                    children: [
                      Text('Cache Statistics', style: Theme.of(context).textTheme.titleMedium),
                      SizedBox(height: 8),
                      Text('Cached Items: ${stats['cachedCount']}'),
                      Text('Cache Size: ${stats['totalSizeMB'].toStringAsFixed(2)} MB'),
                      Text('Downloading: ${stats['downloadingCount']}'),
                      Text('Failed: ${stats['failedCount']}'),
                      SizedBox(height: 8),
                      LinearProgressIndicator(
                        value: stats['totalSize'] / stats['maxCacheBytes'],
                      ),
                      Text('${(stats['totalSize'] / stats['maxCacheBytes'] * 100).toStringAsFixed(1)}% full'),
                    ],
                  );
                }
                return CircularProgressIndicator();
              },
            ),
          ),
        ),
        
        // Cached items list
        Expanded(
          child: ListView.builder(
            itemCount: cached.length,
            itemBuilder: (context, index) {
              final info = cached[index];
              return ListTile(
                leading: Icon(Icons.audiotrack),
                title: Text(info.assetId),
                subtitle: Text('${info.fileSize / (1024 * 1024)} MB'),
                trailing: IconButton(
                  icon: Icon(Icons.delete),
                  onPressed: () {
                    offlineService.removeAsset(info.assetId);
                  },
                ),
                onTap: () {
                  // Play cached file
                  final controller = ref.read(audioPlayerControllerProvider.notifier);
                  controller.playAsset(
                    assetId: info.assetId,
                    // Use offline URL
                  );
                },
              );
            },
          ),
        ),
        
        // Actions
        Row(
          mainAxisAlignment: MainAxisAlignment.spaceEvenly,
          children: [
            ElevatedButton(
              onPressed: () {
                // Cache current queue
                final queue = ref.read(currentQueueProvider);
                offlineService.cacheQueue(queue);
              },
              child: Text('Cache Queue'),
            ),
            ElevatedButton(
              onPressed: () {
                offlineService.removeExpired();
              },
              child: Text('Remove Expired'),
            ),
            ElevatedButton(
              onPressed: () {
                offlineService.clearCache();
              },
              child: Text('Clear Cache'),
              style: ElevatedButton.styleFrom(primary: Colors.red),
            ),
          ],
        ),
      ],
    );
  }
}
```

---

## Configuration

### Offline Audio Configuration

```dart
final config = OfflineAudioConfig(
  maxCacheSize: 100,           // Maximum number of files
  maxCacheBytes: 1024 * 1024 * 1024, // 1GB
  cacheDirectoryName: 'i_confess_audio_cache',
  wifiOnly: true,              // Only cache on WiFi
  preloadNextInQueue: true,    // Preload next items
  preloadCount: 3,             // Number of items to preload
);

final offlineService = await OfflineAudioService.create(config);
```

### Platform-Specific Configuration

#### Android

Ensure these permissions are in `AndroidManifest.xml`:

```xml
<uses-permission android:name="android.permission.INTERNET" />
<uses-permission android:name="android.permission.WRITE_EXTERNAL_STORAGE" 
    android:maxSdkVersion="32" />
<uses-permission android:name="android.permission.READ_EXTERNAL_STORAGE" 
    android:maxSdkVersion="32" />
```

#### iOS

No additional permissions needed for caching to application documents directory.

---

## Testing

### Unit Tests

```dart
// Test playback history
void testPlaybackHistory() {
  final history = PlaybackHistory();
  final entry = PlaybackHistoryEntry.started(...);
  
  // Test adding
  final newHistory = history.add(entry);
  expect(newHistory.length, 1);
  
  // Test filtering
  final forConfession = newHistory.forConfession('confession_123');
  expect(forConfession.length, 1);
  
  // Test statistics
  final stats = PlaybackHistoryStats.fromHistory(newHistory);
  expect(stats.totalEntries, 1);
}

// Test bookmarks
void testBookmarks() {
  final collection = BookmarkCollection();
  final bookmark = AudioBookmark.create(...);
  
  // Test adding
  final newCollection = collection.add(bookmark);
  expect(newCollection.length, 1);
  
  // Test filtering
  final forAsset = newCollection.forAsset('asset_123');
  expect(forAsset.length, 1);
  
  // Test removal
  final withoutBookmark = newCollection.remove(bookmark.id);
  expect(withoutBookmark.length, 0);
}

// Test offline service
void testOfflineService() async {
  final service = await OfflineAudioService.create();
  await service.init();
  
  // Test caching
  await service.cacheAsset(
    assetId: 'test',
    url: 'https://example.com/test.mp3',
    fileName: 'test.mp3',
  );
  
  // Test status
  expect(service.getStatus('test'), OfflineAudioStatus.downloading);
  
  // Clean up
  await service.dispose();
}
```

### Manual Tests

**Playback History:**
- [ ] History entries are created when audio plays
- [ ] History entries are updated as playback progresses
- [ ] History entries are marked complete when audio finishes
- [ ] History can be filtered by confession
- [ ] History can be filtered by voice
- [ ] History can be filtered by date
- [ ] Statistics are calculated correctly

**Bookmarks:**
- [ ] Bookmarks can be created
- [ ] Bookmarks can be edited
- [ ] Bookmarks can be deleted
- [ ] Bookmarks can be filtered by asset
- [ ] Bookmarks can be filtered by confession
- [ ] Bookmarks can be sorted by date
- [ ] Bookmarks can be sorted by position
- [ ] Tapping a bookmark seeks to that position

**Offline Mode:**
- [ ] Assets can be cached
- [ ] Cache status is tracked
- [ ] Download progress is tracked
- [ ] Cached assets can be played offline
- [ ] Cache can be cleared
- [ ] Expired assets are removed
- [ ] Cache size is limited
- [ ] Queue can be cached
- [ ] Next items in queue are preloaded

---

## Troubleshooting

### Common Issues

| Issue | Cause | Solution |
|-------|-------|----------|
| History not saving | Not calling add() | Ensure history is updated |
| Bookmarks not saving | Not calling add() | Ensure collection is updated |
| Cache not working | Missing permissions | Check Android/iOS permissions |
| Cache full | Too many files | Increase maxCacheSize or maxCacheBytes |
| Download failed | Network issues | Check network connection |

### Debug Commands

```bash
# Check Flutter logs
flutter logs

# Check Android logs
adb logcat | grep -i "audio\|offline\|cache"

# Check iOS logs
idevicesyslog | grep -i "audio\|offline\|cache"
```

---

## Next Steps

### Phase 4 Completion Checklist

- [x] Create PlaybackHistory model
- [x] Create PlaybackHistoryEntry model
- [x] Create PlaybackHistoryStats model
- [x] Create AudioBookmark model
- [x] Create BookmarkCollection model
- [x] Create OfflineAudioService
- [x] Create OfflineAudioConfig
- [x] Create CachedAudioInfo model
- [ ] Add Riverpod providers
- [ ] Add persistence for history
- [ ] Add persistence for bookmarks
- [ ] Add UI for history screen
- [ ] Add UI for bookmarks screen
- [ ] Add UI for offline mode screen
- [ ] Integrate with audio player
- [ ] Test on Android
- [ ] Test on iOS

### Phase 5 Features (Future)

1. **Crossfade** - Smooth transitions between tracks
2. **Equalizer** - Audio equalization controls
3. **Sleep Timer** - Auto-stop after delay
4. **Custom Theming** - Apply app theme to audio features
5. **Animations** - Smooth transitions
6. **Accessibility** - Full accessibility support

---

## Conclusion

Phase 4 adds **Playback History**, **Bookmarks**, and **Offline Mode** - three powerful features that significantly enhance the audio experience:

- **Playback History** allows users to track their listening habits and resume where they left off
- **Bookmarks** allow users to save and quickly return to important moments in audio
- **Offline Mode** allows users to listen to audio without an internet connection

With Phase 4 complete, the I-Confess audio platform will be one of the most comprehensive audio experiences available.

---

**Document Version:** 1.0.0  
**Last Updated:** 2026-09-29  
**Author:** Arena.ai Agent  
**Status:** 🟡 Implementation in Progress  
**Next:** Complete integration and testing

---

> **"Phase 4 brings the audio platform to the next level with history, bookmarks, and offline capabilities!"** 🚀
