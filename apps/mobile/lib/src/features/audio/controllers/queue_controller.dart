/// Queue controller for managing the audio playback queue.
///
/// This controller provides full queue management functionality including
/// add, remove, reorder, shuffle, repeat, and navigation operations.
import 'dart:async';
import 'package:flutter/foundation.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_riverpod/legacy.dart';
import '../models/audio_queue.dart';
import '../models/audio_asset.dart';
import 'audio_player_controller.dart';

/// State for the audio queue.
class QueueState {
  final AudioQueue queue;
  final bool isLoading;
  final String? error;

  const QueueState({
    this.queue = const AudioQueue(),
    this.isLoading = false,
    this.error,
  });

  QueueState copyWith({
    AudioQueue? queue,
    bool? isLoading,
    String? error,
  }) {
    return QueueState(
      queue: queue ?? this.queue,
      isLoading: isLoading ?? this.isLoading,
      error: error,
    );
  }
}

/// Notifier for the audio queue.
class QueueNotifier extends StateNotifier<QueueState> {
  final AudioPlayerNotifier _audioController;
  
  QueueNotifier(this._audioController) : super(const QueueState());

  @override
  void dispose() {
    super.dispose();
  }

  /// Gets the current queue.
  AudioQueue get currentQueue => state.queue;

  /// Gets the current queue item.
  AudioQueueItem? get currentItem => state.queue.currentItem;

  /// Gets the next queue item.
  AudioQueueItem? get nextItem => state.queue.nextItem;

  /// Gets the previous queue item.
  AudioQueueItem? get previousItem => state.queue.previousItem;

  /// Whether the queue is empty.
  bool get isEmpty => state.queue.isEmpty;

  /// Whether the queue is not empty.
  bool get isNotEmpty => state.queue.isNotEmpty;

  /// Whether shuffle is enabled.
  bool get isShuffled => state.queue.isShuffled;

  /// Gets the current repeat mode.
  RepeatMode get repeatMode => state.queue.repeatMode;

  /// Adds an item to the queue.
  ///
  /// If [playNext] is true, the item is added after the current item.
  /// Otherwise, it's added to the end of the queue.
  void addItem(AudioQueueItem item, {bool playNext = false}) {
    final currentQueue = state.queue;
    final newItems = List<AudioQueueItem>.from(currentQueue.items);
    
    if (playNext && currentQueue.hasCurrentItem) {
      // Insert after current item
      final insertIndex = currentQueue.currentIndex + 1;
      newItems.insert(insertIndex.clamp(0, newItems.length), item);
    } else {
      // Add to end
      newItems.add(item);
    }
    
    // If queue was empty, start playing this item
    if (currentQueue.isEmpty) {
      _playItemAtIndex(0, newItems);
    }
    
    state = state.copyWith(
      queue: currentQueue.copyWith(
        items: newItems,
        currentIndex: currentQueue.isEmpty ? 0 : currentQueue.currentIndex,
      ),
      isLoading: false,
    );
  }

  /// Adds multiple items to the queue.
  void addItems(List<AudioQueueItem> items, {bool playNext = false}) {
    for (final item in items) {
      addItem(item, playNext: playNext);
    }
  }

  /// Adds an asset to the queue.
  void addAsset(AudioAsset asset, {bool playNext = false, String? title}) {
    final item = AudioQueueItem.fromAsset(
      asset: asset,
      title: title,
      orderIndex: state.queue.length,
    );
    addItem(item, playNext: playNext);
  }

  /// Adds a confession to the queue by ID.
  ///
  /// This will create a placeholder item and generate the audio if needed.
  Future<void> addConfession({
    required String confessionId,
    String? voiceId,
    bool playNext = false,
  }) async {
    // Create a placeholder item
    final item = AudioQueueItem.minimal(
      assetId: 'placeholder_$confessionId',
      confessionId: confessionId,
      voiceId: voiceId ?? 'default',
      title: 'Confession $confessionId',
      duration: Duration(minutes: 2), // Placeholder duration
      orderIndex: state.queue.length,
    );
    
    addItem(item, playNext: playNext);
    
    // In a real implementation, we would:
    // 1. Check if audio already exists for this confession/voice
    // 2. If not, queue a generation job
    // 3. Update the item when generation completes
  }

  /// Removes an item from the queue by index.
  void removeItemAt(int index) {
    if (index < 0 || index >= state.queue.items.length) return;
    
    final currentQueue = state.queue;
    final newItems = List<AudioQueueItem>.from(currentQueue.items);
    
    // Adjust current index if needed
    final currentIndex = currentQueue.currentIndex;
    final newCurrentIndex = currentIndex > index 
        ? currentIndex - 1 
        : currentIndex;
    
    newItems.removeAt(index);
    
    // If we removed the current item, move to next
    if (currentIndex == index) {
      if (newItems.isNotEmpty) {
        // Play the item that's now at this index
        _playItemAtIndex(newCurrentIndex.clamp(0, newItems.length - 1), newItems);
      } else {
        // Queue is empty, stop playback
        _audioController.stop();
      }
    }
    
    state = state.copyWith(
      queue: currentQueue.copyWith(
        items: newItems,
        currentIndex: newCurrentIndex.clamp(-1, newItems.length - 1),
      ),
    );
  }

  /// Removes an item from the queue by ID.
  void removeItemById(String itemId) {
    final index = state.queue.items.indexWhere((item) => item.id == itemId);
    if (index >= 0) {
      removeItemAt(index);
    }
  }

  /// Clears all items from the queue.
  void clear() {
    _audioController.stop();
    state = state.copyWith(
      queue: const AudioQueue(),
    );
  }

  /// Moves an item from one index to another.
  void moveItem(int fromIndex, int toIndex) {
    if (fromIndex < 0 || fromIndex >= state.queue.items.length) return;
    if (toIndex < 0 || toIndex >= state.queue.items.length) return;
    
    final currentQueue = state.queue;
    final newItems = List<AudioQueueItem>.from(currentQueue.items);
    
    final item = newItems.removeAt(fromIndex);
    newItems.insert(toIndex, item);
    
    // Update order indices
    for (var i = 0; i < newItems.length; i++) {
      newItems[i] = newItems[i].copyWith(orderIndex: i);
    }
    
    // Adjust current index if needed
    final currentIndex = currentQueue.currentIndex;
    if (currentIndex == fromIndex) {
      // Current item was moved
      state = state.copyWith(
        queue: currentQueue.copyWith(
          items: newItems,
          currentIndex: toIndex,
        ),
      );
    } else if (currentIndex > fromIndex && currentIndex <= toIndex) {
      // Current index shifted left
      state = state.copyWith(
        queue: currentQueue.copyWith(
          items: newItems,
          currentIndex: currentIndex - 1,
        ),
      );
    } else if (currentIndex >= toIndex && currentIndex < fromIndex) {
      // Current index shifted right
      state = state.copyWith(
        queue: currentQueue.copyWith(
          items: newItems,
          currentIndex: currentIndex + 1,
        ),
      );
    } else {
      state = state.copyWith(
        queue: currentQueue.copyWith(
          items: newItems,
        ),
      );
    }
  }

  /// Plays the item at the specified index.
  void playItemAt(int index) {
    if (index < 0 || index >= state.queue.items.length) return;
    
    final newItems = List<AudioQueueItem>.from(state.queue.items);
    _playItemAtIndex(index, newItems);
  }

  /// Plays the next item in the queue.
  Future<void> playNext() async {
    final nextItem = state.queue.nextItem;
    if (nextItem == null) {
      // No next item
      if (state.queue.repeatMode == RepeatMode.all && state.queue.isNotEmpty) {
        // Repeat all: go to first item
        playItemAt(0);
      } else {
        // End of queue
        await _audioController.stop();
      }
      return;
    }
    
    final nextIndex = state.queue.items.indexOf(nextItem);
    if (nextIndex >= 0) {
      playItemAt(nextIndex);
    }
  }

  /// Plays the previous item in the queue.
  void playPrevious() {
    final previousItem = state.queue.previousItem;
    if (previousItem == null) {
      // No previous item
      if (state.queue.repeatMode == RepeatMode.all && state.queue.isNotEmpty) {
        // Repeat all: go to last item
        playItemAt(state.queue.items.length - 1);
      }
      return;
    }
    
    final previousIndex = state.queue.items.indexOf(previousItem);
    if (previousIndex >= 0) {
      playItemAt(previousIndex);
    }
  }

  /// Toggles shuffle mode.
  void toggleShuffle() {
    state = state.copyWith(
      queue: state.queue.copyWith(
        isShuffled: !state.queue.isShuffled,
      ),
    );
  }

  /// Cycles to the next repeat mode.
  void cycleRepeatMode() {
    state = state.copyWith(
      queue: state.queue.copyWith(
        repeatMode: state.queue.repeatMode.next,
      ),
    );
  }

  /// Sets the repeat mode.
  void setRepeatMode(RepeatMode mode) {
    state = state.copyWith(
      queue: state.queue.copyWith(
        repeatMode: mode,
      ),
    );
  }

  /// Marks the current item as played.
  void markCurrentAsPlayed() {
    if (!state.queue.hasCurrentItem) return;
    
    final currentIndex = state.queue.currentIndex;
    final currentItem = state.queue.currentItem!;
    final newItems = List<AudioQueueItem>.from(state.queue.items);
    
    newItems[currentIndex] = currentItem.copyWith(played: true);
    
    state = state.copyWith(
      queue: state.queue.copyWith(items: newItems),
    );
  }

  /// Updates the last position of the current item.
  void updateCurrentPosition(Duration position) {
    if (!state.queue.hasCurrentItem) return;
    
    final currentIndex = state.queue.currentIndex;
    final currentItem = state.queue.currentItem!;
    final newItems = List<AudioQueueItem>.from(state.queue.items);
    
    newItems[currentIndex] = currentItem.copyWith(lastPosition: position);
    
    state = state.copyWith(
      queue: state.queue.copyWith(items: newItems),
    );
  }

  /// Gets the index of an item by ID.
  int? getIndexById(String itemId) {
    return state.queue.items.indexWhere((item) => item.id == itemId);
  }

  /// Gets an item by ID.
  AudioQueueItem? getItemById(String itemId) {
    return state.queue.items.firstWhere(
      (item) => item.id == itemId,
      orElse: () => null,
    );
  }

  /// Gets items by confession ID.
  List<AudioQueueItem> getItemsByConfession(String confessionId) {
    return state.queue.items
        .where((item) => item.confessionId == confessionId)
        .toList();
  }

  /// Whether a confession is already in the queue.
  bool containsConfession(String confessionId) {
    return state.queue.items
        .any((item) => item.confessionId == confessionId);
  }

  /// Replaces all items in the queue.
  void replaceAll(List<AudioQueueItem> items, {int startIndex = 0}) {
    if (items.isEmpty) {
      clear();
      return;
    }
    
    state = state.copyWith(
      queue: state.queue.copyWith(
        items: items,
        currentIndex: startIndex.clamp(0, items.length - 1),
      ),
    );
    
    // Play the first item
    if (startIndex >= 0 && startIndex < items.length) {
      _playItemAtIndex(startIndex, items);
    }
  }

  /// Plays the item at the specified index with the given items list.
  void _playItemAtIndex(int index, List<AudioQueueItem> items) {
    final item = items[index];
    
    // Update queue state
    state = state.copyWith(
      queue: state.queue.copyWith(
        items: items,
        currentIndex: index,
      ),
    );
    
    // Play the item
    unawaited(
      _audioController.playAsset(
        assetId: item.asset.id,
        confessionId: item.confessionId,
        voiceId: item.voiceId,
        initialPosition: item.lastPosition,
      ).catchError((Object error) {
        state = state.copyWith(error: error.toString());
      }),
    );
    
    // Mark as played if it was previously played
    if (item.played) {
      // Resume from last position
      if (item.lastPosition != null) {
        _audioController.seek(item.lastPosition!);
      }
    }
  }

  /// Gets the queue as a JSON-serializable map.
  Map<String, dynamic> toJson() {
    return state.queue.toJson();
  }

  /// Restores the queue from a JSON map.
  void restoreFromJson(Map<String, dynamic> json) {
    state = state.copyWith(
      queue: AudioQueue.fromJson(json),
    );
  }
}

/// Provider for the queue controller.
final queueControllerProvider = StateNotifierProvider<QueueNotifier, QueueState>((ref) {
  final audioController = ref.watch(audioPlayerControllerProvider.notifier);
  return QueueNotifier(audioController);
});

/// Provider for the current queue.
final currentQueueProvider = Provider<AudioQueue>((ref) {
  return ref.watch(queueControllerProvider).queue;
});

/// Provider for whether the queue is empty.
final isQueueEmptyProvider = Provider<bool>((ref) {
  return ref.watch(queueControllerProvider).queue.isEmpty;
});

/// Provider for whether the queue is not empty.
final isQueueNotEmptyProvider = Provider<bool>((ref) {
  return ref.watch(queueControllerProvider).queue.isNotEmpty;
});

/// Provider for the current queue item.
final currentQueueItemProvider = Provider<AudioQueueItem?>((ref) {
  return ref.watch(queueControllerProvider).queue.currentItem;
});

/// Provider for whether shuffle is enabled.
final isShuffledProvider = Provider<bool>((ref) {
  return ref.watch(queueControllerProvider).queue.isShuffled;
});

/// Provider for the current repeat mode.
final repeatModeProvider = Provider<RepeatMode>((ref) {
  return ref.watch(queueControllerProvider).queue.repeatMode;
});

/// Provider for the queue length.
final queueLengthProvider = Provider<int>((ref) {
  return ref.watch(queueControllerProvider).queue.length;
});
