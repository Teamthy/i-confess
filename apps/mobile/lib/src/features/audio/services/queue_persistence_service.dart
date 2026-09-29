/// Queue persistence service for saving and restoring the audio queue.
///
/// This service provides functionality to persist the queue state across app
/// restarts, allowing users to resume their listening session.
import 'dart:async';
import 'dart:convert';
import 'package:flutter/foundation.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:shared_preferences/shared_preferences.dart';
import '../controllers/queue_controller.dart';
import '../models/audio_queue.dart';

/// Key for storing queue in shared preferences.
const String _queueKey = 'audio_queue_v1';

/// Key for storing last played position.
const String _lastPositionKey = 'audio_last_position';

/// Key for storing playback history.
const String _historyKey = 'audio_history_v1';

/// Service for persisting queue state.
///
/// This service saves the queue to SharedPreferences and can restore it
/// when the app is restarted.
class QueuePersistenceService {
  final SharedPreferences _prefs;
  
  // Stream controller for persistence events
  final _onSaveController = StreamController<void>.broadcast();
  final _onRestoreController = StreamController<AudioQueue>.broadcast();
  
  /// Whether persistence is enabled.
  bool get isEnabled => true;
  
  /// Stream that fires when the queue is saved.
  Stream<void> get onSave => _onSaveController.stream;
  
  /// Stream that fires when the queue is restored.
  Stream<AudioQueue> get onRestore => _onRestoreController.stream;

  /// Creates a queue persistence service.
  QueuePersistenceService(this._prefs);

  /// Creates a queue persistence service with default SharedPreferences.
  static Future<QueuePersistenceService> create() async {
    final prefs = await SharedPreferences.getInstance();
    return QueuePersistenceService(prefs);
  }

  /// Saves the queue to persistent storage.
  ///
  /// Returns true if the save was successful.
  Future<bool> saveQueue(AudioQueue queue) async {
    try {
      final json = queue.toJson();
      final jsonString = jsonEncode(json);
      
      await _prefs.setString(_queueKey, jsonString);
      
      // Also save timestamp
      await _prefs.setInt('${_queueKey}_timestamp', DateTime.now().millisecondsSinceEpoch);
      
      _onSaveController.add(null);
      debugPrint('[QueuePersistence] Queue saved: ${queue.length} items');
      return true;
    } catch (e) {
      debugPrint('[QueuePersistence] Error saving queue: $e');
      return false;
    }
  }

  /// Restores the queue from persistent storage.
  ///
  /// Returns the restored queue, or an empty queue if no saved queue exists.
  Future<AudioQueue> restoreQueue() async {
    try {
      final jsonString = _prefs.getString(_queueKey);
      
      if (jsonString == null || jsonString.isEmpty) {
        debugPrint('[QueuePersistence] No saved queue found');
        return const AudioQueue();
      }
      
      final json = jsonDecode(jsonString) as Map<String, dynamic>;
      final queue = AudioQueue.fromJson(json);
      
      _onRestoreController.add(queue);
      debugPrint('[QueuePersistence] Queue restored: ${queue.length} items');
      return queue;
    } catch (e) {
      debugPrint('[QueuePersistence] Error restoring queue: $e');
      return const AudioQueue();
    }
  }

  /// Clears the saved queue.
  Future<bool> clearQueue() async {
    try {
      await _prefs.remove(_queueKey);
      await _prefs.remove('${_queueKey}_timestamp');
      
      debugPrint('[QueuePersistence] Queue cleared');
      return true;
    } catch (e) {
      debugPrint('[QueuePersistence] Error clearing queue: $e');
      return false;
    }
  }

  /// Checks if a saved queue exists.
  Future<bool> hasSavedQueue() async {
    return _prefs.containsKey(_queueKey);
  }

  /// Gets the timestamp when the queue was last saved.
  Future<DateTime?> getLastSavedTimestamp() async {
    final timestamp = _prefs.getInt('${_queueKey}_timestamp');
    if (timestamp == null) return null;
    return DateTime.fromMillisecondsSinceEpoch(timestamp);
  }

  /// Saves the last playback position for a specific asset.
  Future<bool> saveLastPosition(String assetId, Duration position) async {
    try {
      await _prefs.setInt('${_lastPositionKey}_$assetId', position.inMilliseconds);
      debugPrint('[QueuePersistence] Last position saved for $assetId: $position');
      return true;
    } catch (e) {
      debugPrint('[QueuePersistence] Error saving last position: $e');
      return false;
    }
  }

  /// Gets the last playback position for a specific asset.
  Future<Duration?> getLastPosition(String assetId) async {
    try {
      final milliseconds = _prefs.getInt('${_lastPositionKey}_$assetId');
      if (milliseconds == null) return null;
      return Duration(milliseconds: milliseconds);
    } catch (e) {
      debugPrint('[QueuePersistence] Error getting last position: $e');
      return null;
    }
  }

  /// Clears the last position for a specific asset.
  Future<bool> clearLastPosition(String assetId) async {
    try {
      await _prefs.remove('${_lastPositionKey}_$assetId');
      return true;
    } catch (e) {
      debugPrint('[QueuePersistence] Error clearing last position: $e');
      return false;
    }
  }

  /// Saves an item to playback history.
  Future<bool> saveToHistory(AudioQueueItem item) async {
    try {
      final historyJson = _prefs.getString(_historyKey) ?? '[]';
      final historyList = jsonDecode(historyJson) as List<dynamic>;
      
      // Check if this item is already in history
      final exists = historyList.any((e) => e['id'] == item.id);
      
      if (!exists) {
        // Add to beginning of history
        historyList.insert(0, item.toJson());
        
        // Limit history to 50 items
        if (historyList.length > 50) {
          historyList.removeLast();
        }
        
        await _prefs.setString(_historyKey, jsonEncode(historyList));
        debugPrint('[QueuePersistence] Item added to history: ${item.title}');
      }
      
      return true;
    } catch (e) {
      debugPrint('[QueuePersistence] Error saving to history: $e');
      return false;
    }
  }

  /// Gets the playback history.
  Future<List<AudioQueueItem>> getHistory() async {
    try {
      final historyJson = _prefs.getString(_historyKey) ?? '[]';
      final historyList = jsonDecode(historyJson) as List<dynamic>;
      
      return historyList
          .map((json) => AudioQueueItem.fromJson(json as Map<String, dynamic>))
          .toList();
    } catch (e) {
      debugPrint('[QueuePersistence] Error getting history: $e');
      return [];
    }
  }

  /// Clears the playback history.
  Future<bool> clearHistory() async {
    try {
      await _prefs.remove(_historyKey);
      debugPrint('[QueuePersistence] History cleared');
      return true;
    } catch (e) {
      debugPrint('[QueuePersistence] Error clearing history: $e');
      return false;
    }
  }

  /// Saves the current queue state for auto-resume.
  ///
  /// This saves both the queue and the current playback position.
  Future<bool> saveCurrentState({
    required AudioQueue queue,
    Duration? currentPosition,
  }) async {
    try {
      // Save queue
      await saveQueue(queue);
      
      // Save current position if available
      if (currentPosition != null && queue.currentItem != null) {
        await saveLastPosition(
          queue.currentItem!.asset.id,
          currentPosition,
        );
      }
      
      debugPrint('[QueuePersistence] Current state saved');
      return true;
    } catch (e) {
      debugPrint('[QueuePersistence] Error saving current state: $e');
      return false;
    }
  }

  /// Restores the queue state for auto-resume.
  ///
  /// Returns a tuple of (queue, lastPosition).
  Future<(AudioQueue, Duration?)> restoreCurrentState() async {
    try {
      final queue = await restoreQueue();
      
      if (queue.isNotEmpty && queue.currentItem != null) {
        final lastPosition = await getLastPosition(queue.currentItem!.asset.id);
        return (queue, lastPosition);
      }
      
      return (queue, null);
    } catch (e) {
      debugPrint('[QueuePersistence] Error restoring current state: $e');
      return (const AudioQueue(), null);
    }
  }

  /// Disposes the service and cleans up resources.
  void dispose() {
    _onSaveController.close();
    _onRestoreController.close();
  }
}

/// Provider for the queue persistence service.
///
/// Usage:
/// ```dart
/// final persistence = ref.watch(queuePersistenceServiceProvider);
/// await persistence.saveQueue(queue);
/// final restoredQueue = await persistence.restoreQueue();
/// ```
final queuePersistenceServiceProvider = Provider<QueuePersistenceService>((ref) {
  // In a real app, you might want to create this lazily
  // For now, we'll create it immediately
  throw UnimplementedError('QueuePersistenceService must be initialized separately');
});

/// Async provider for the queue persistence service.
final queuePersistenceServiceAsyncProvider = FutureProvider<QueuePersistenceService>((ref) async {
  return QueuePersistenceService.create();
});

/// Extension methods for easy access to persistence.
extension QueuePersistenceExtension on QueueNotifier {
  /// Saves the current queue state.
  Future<bool> saveQueueState() async {
    final service = await QueuePersistenceService.create();
    return service.saveQueue(currentQueue);
  }

  /// Restores the queue state.
  Future<AudioQueue> restoreQueueState() async {
    final service = await QueuePersistenceService.create();
    return service.restoreQueue();
  }

  /// Saves the current state (queue + position).
  Future<bool> saveCurrentState() async {
    final service = await QueuePersistenceService.create();
    return service.saveCurrentState(
      queue: currentQueue,
      currentPosition: currentPosition,
    );
  }

  /// Restores the current state (queue + position).
  Future<(AudioQueue, Duration?)> restoreCurrentState() async {
    final service = await QueuePersistenceService.create();
    return service.restoreCurrentState();
  }
}

/// Mixin for adding persistence to queue controllers.
mixin QueuePersistenceMixin {
  QueuePersistenceService? _persistenceService;

  /// Initializes persistence.
  Future<void> initPersistence() async {
    _persistenceService = await QueuePersistenceService.create();
  }

  /// Saves the queue.
  Future<bool> saveQueue(AudioQueue queue) async {
    if (_persistenceService == null) {
      await initPersistence();
    }
    return _persistenceService!.saveQueue(queue);
  }

  /// Restores the queue.
  Future<AudioQueue> restoreQueue() async {
    if (_persistenceService == null) {
      await initPersistence();
    }
    return _persistenceService!.restoreQueue();
  }

  /// Disposes persistence.
  void disposePersistence() {
    _persistenceService?.dispose();
    _persistenceService = null;
  }
}
