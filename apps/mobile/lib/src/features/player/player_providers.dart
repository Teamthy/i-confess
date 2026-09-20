import 'dart:async';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_riverpod/legacy.dart';
import 'package:iconfess_api/iconfess_api.dart';

import '../../core/di/providers.dart';
import '../../core/persistence/persistence.dart';
import 'audio_playback_service.dart';

/// Provider for audio playback service. Can be overridden in widget/unit tests.
final audioPlaybackServiceProvider = Provider<AudioPlaybackService>((ref) {
  final service = JustAudioPlaybackService();
  ref.onDispose(service.dispose);
  return service;
});

/// High-level session lifecycle statuses.
enum PlayerLifecycleStatus {
  idle,
  loading,
  ready,
  playing,
  paused,
  interrupted,
  completed,
  error,
}

/// Persisted state for a session's playback progress.
class PersistedPlaybackState {
  const PersistedPlaybackState({
    required this.sessionId,
    required this.currentIndex,
    required this.currentItemId,
    required this.positionMs,
    required this.completedItemIds,
    required this.skippedItemIds,
    required this.lastPlaybackState,
    required this.lastUpdatedAt,
  });

  final String sessionId;
  final int currentIndex;
  final String currentItemId;
  final int positionMs;
  final Set<String> completedItemIds;
  final Set<String> skippedItemIds;
  final String lastPlaybackState;
  final String lastUpdatedAt;

  Map<String, dynamic> toJson() => {
        'session_id': sessionId,
        'current_index': currentIndex,
        'current_item_id': currentItemId,
        'position_ms': positionMs,
        'completed_item_ids': completedItemIds.toList(),
        'skipped_item_ids': skippedItemIds.toList(),
        'last_playback_state': lastPlaybackState,
        'last_updated_at': lastUpdatedAt,
      };

  factory PersistedPlaybackState.fromJson(Map<String, dynamic> json) =>
      PersistedPlaybackState(
        sessionId: json['session_id'] as String? ?? '',
        currentIndex: json['current_index'] as int? ?? 0,
        currentItemId: json['current_item_id'] as String? ?? '',
        positionMs: json['position_ms'] as int? ?? 0,
        completedItemIds: (json['completed_item_ids'] as List<dynamic>?)
                ?.map((e) => e.toString())
                .toSet() ??
            {},
        skippedItemIds: (json['skipped_item_ids'] as List<dynamic>?)
                ?.map((e) => e.toString())
                .toSet() ??
            {},
        lastPlaybackState: json['last_playback_state'] as String? ?? 'READY',
        lastUpdatedAt: json['last_updated_at'] as String? ?? '',
      );
}

/// Immutable business state for session playback.
class SessionPlaybackState {
  const SessionPlaybackState({
    this.sessionId = '',
    this.status = PlayerLifecycleStatus.idle,
    this.items = const [],
    this.currentIndex = 0,
    this.positionMs = 0,
    this.totalDurationSeconds = 0,
    this.completedItemIds = const {},
    this.skippedItemIds = const {},
    this.lastUpdatedAt = '',
    this.errorMessage = '',
    this.isRefreshingUrl = false,
  });

  final String sessionId;
  final PlayerLifecycleStatus status;
  final List<SessionItem> items;
  final int currentIndex;
  final int positionMs;
  final int totalDurationSeconds;
  final Set<String> completedItemIds;
  final Set<String> skippedItemIds;
  final String lastUpdatedAt;
  final String errorMessage;
  final bool isRefreshingUrl;

  SessionItem? get currentItem {
    if (items.isEmpty || currentIndex < 0 || currentIndex >= items.length) {
      return null;
    }
    return items[currentIndex];
  }

  bool get hasLockedItems => items.any((i) => i.locked);
  bool get isCompleted => status == PlayerLifecycleStatus.completed;
  bool get isPlaying => status == PlayerLifecycleStatus.playing;
  bool get isPaused => status == PlayerLifecycleStatus.paused;
  bool get isInterrupted => status == PlayerLifecycleStatus.interrupted;
  bool get isLoading => status == PlayerLifecycleStatus.loading;
  bool get isError => status == PlayerLifecycleStatus.error;

  SessionPlaybackState copyWith({
    String? sessionId,
    PlayerLifecycleStatus? status,
    List<SessionItem>? items,
    int? currentIndex,
    int? positionMs,
    int? totalDurationSeconds,
    Set<String>? completedItemIds,
    Set<String>? skippedItemIds,
    String? lastUpdatedAt,
    String? errorMessage,
    bool? isRefreshingUrl,
  }) =>
      SessionPlaybackState(
        sessionId: sessionId ?? this.sessionId,
        status: status ?? this.status,
        items: items ?? this.items,
        currentIndex: currentIndex ?? this.currentIndex,
        positionMs: positionMs ?? this.positionMs,
        totalDurationSeconds: totalDurationSeconds ?? this.totalDurationSeconds,
        completedItemIds: completedItemIds ?? this.completedItemIds,
        skippedItemIds: skippedItemIds ?? this.skippedItemIds,
        lastUpdatedAt: lastUpdatedAt ?? this.lastUpdatedAt,
        errorMessage: errorMessage ?? this.errorMessage,
        isRefreshingUrl: isRefreshingUrl ?? this.isRefreshingUrl,
      );
}

/// The Session Engine owns business lifecycle state (Criterion 2).
/// It coordinates audio playback, signed URL refreshes, conflict resolution,
/// interruptions, and server progress reporting.
class SessionEngine extends StateNotifier<SessionPlaybackState> {
  SessionEngine(this.ref, this.sessionId) : super(SessionPlaybackState(sessionId: sessionId)) {
    _init();
  }

  final Ref ref;
  final String sessionId;

  StreamSubscription<Duration>? _posSub;
  StreamSubscription<AudioPlaybackStatus>? _statusSub;
  StreamSubscription<String>? _errorSub;
  StreamSubscription<void>? _interruptionSub;
  StreamSubscription<void>? _noisySub;

  AudioPlaybackService get _audio => ref.read(audioPlaybackServiceProvider);
  KeyValueStore get _storage => ref.read(keyValueStoreProvider);
  ApiClient get _api => ref.read(apiClientProvider);
  ContentRepository get _content => ref.read(contentRepositoryProvider);

  bool _disposed = false;
  Timer? _progressDebounce;
  int _lastSyncedPosition = 0;

  void _init() {
    _posSub = _audio.positionStream.listen((d) {
      if (_disposed) return;
      final ms = d.inMilliseconds;
      state = state.copyWith(positionMs: ms);
      _scheduleProgressSync();
    });

    _statusSub = _audio.statusStream.listen((status) {
      if (_disposed) return;
      if (status == AudioPlaybackStatus.completed) {
        _onCurrentTrackCompleted();
      }
    });

    _errorSub = _audio.errorStream.listen((err) {
      if (_disposed) return;
      _onAudioError(err);
    });

    _interruptionSub = _audio.interruptionStream.listen((_) {
      if (_disposed) return;
      onInterruption();
    });

    _noisySub = _audio.becomingNoisyStream.listen((_) {
      if (_disposed) return;
      onBecomingNoisy();
    });

    load();
  }

  /// Loads session queue snapshot, restores persisted state, and applies deterministic
  /// conflict resolution (Criterion 4 & 5).
  Future<void> load() async {
    state = state.copyWith(status: PlayerLifecycleStatus.loading, errorMessage: '');

    // 1. Restore local persisted state if available
    PersistedPlaybackState? localState;
    try {
      final json = await _storage.readJson(StoreKeys.sessionPlayback(sessionId));
      if (json != null) {
        localState = PersistedPlaybackState.fromJson(json);
      }
    } catch (_) {}

    // 2. Fetch queue snapshot from server (freshly signed URLs)
    final queueLoadable = await _content.sessionQueue(sessionId);
    if (queueLoadable is LoadFailed<SessionQueueResponse>) {
      if (localState != null) {
        // Fallback to local state if offline
        state = state.copyWith(
          status: PlayerLifecycleStatus.paused,
          currentIndex: localState.currentIndex,
          positionMs: localState.positionMs,
          completedItemIds: localState.completedItemIds,
          skippedItemIds: localState.skippedItemIds,
          lastUpdatedAt: localState.lastUpdatedAt,
          errorMessage: 'Offline: loaded saved progress',
        );
        return;
      }
      state = state.copyWith(
        status: PlayerLifecycleStatus.error,
        errorMessage: queueLoadable.error.message,
      );
      return;
    }

    final queue = (queueLoadable as LoadLoaded<SessionQueueResponse>).value;
    final items = queue.items;

    if (items.isEmpty) {
      state = state.copyWith(
        status: PlayerLifecycleStatus.ready,
        items: items,
      );
      return;
    }

    // 3. Deterministic conflict resolution (Criterion 5)
    int resolvedIndex = 0;
    int resolvedPosition = 0;
    Set<String> completedIds = {};
    Set<String> skippedIds = {};
    String resolvedTimestamp = DateTime.now().toUtc().toIso8601String();

    // Collect statuses from server queue snapshot
    for (final it in items) {
      if (it.status.toUpperCase() == 'COMPLETED') {
        completedIds.add(it.id);
      } else if (it.status.toUpperCase() == 'SKIPPED') {
        skippedIds.add(it.id);
      }
    }

    final serverProg = queue.progress;
    if (serverProg != null && localState != null) {
      final sTime = serverProg.lastUpdatedAt;
      final lTime = localState.lastUpdatedAt;
      final sWins = sTime.isNotEmpty && (lTime.isEmpty || sTime.compareTo(lTime) > 0);

      if (sWins) {
        // Server wins
        resolvedPosition = serverProg.positionMs;
        resolvedTimestamp = sTime;
        if (serverProg.queueItemId.isNotEmpty) {
          final idx = items.indexWhere((it) => it.id == serverProg.queueItemId);
          if (idx >= 0) resolvedIndex = idx;
        }
      } else {
        // Local wins
        resolvedIndex = localState.currentIndex.clamp(0, items.length - 1);
        resolvedPosition = localState.positionMs;
        resolvedTimestamp = lTime;
        completedIds.addAll(localState.completedItemIds);
        skippedIds.addAll(localState.skippedItemIds);
      }
    } else if (serverProg != null) {
      resolvedPosition = serverProg.positionMs;
      resolvedTimestamp = serverProg.lastUpdatedAt;
      if (serverProg.queueItemId.isNotEmpty) {
        final idx = items.indexWhere((it) => it.id == serverProg.queueItemId);
        if (idx >= 0) resolvedIndex = idx;
      }
    } else if (localState != null) {
      resolvedIndex = localState.currentIndex.clamp(0, items.length - 1);
      resolvedPosition = localState.positionMs;
      resolvedTimestamp = localState.lastUpdatedAt;
      completedIds.addAll(localState.completedItemIds);
      skippedIds.addAll(localState.skippedItemIds);
    } else {
      // Find first queued item
      for (int i = 0; i < items.length; i++) {
        if (!completedIds.contains(items[i].id) && !skippedIds.contains(items[i].id)) {
          resolvedIndex = i;
          break;
        }
      }
    }

    // Check if session was already completed
    final allDone = items.every((i) => completedIds.contains(i.id) || skippedIds.contains(i.id));
    final initialStatus = allDone || queue.status == 'COMPLETED'
        ? PlayerLifecycleStatus.completed
        : PlayerLifecycleStatus.ready;

    state = state.copyWith(
      status: initialStatus,
      items: items,
      currentIndex: resolvedIndex,
      positionMs: resolvedPosition,
      completedItemIds: completedIds,
      skippedItemIds: skippedIds,
      lastUpdatedAt: resolvedTimestamp,
      errorMessage: '',
    );

    // 4. Load audio track at resolved position if playable
    if (initialStatus != PlayerLifecycleStatus.completed) {
      final current = state.currentItem;
      if (current != null && current.isPlayable) {
        try {
          await _audio.load(
            current.audioUrl,
            initialPosition: Duration(milliseconds: resolvedPosition),
          );
        } catch (_) {
          // Audio load error handled via error stream / refresh
        }
      }
    }

    _persist();
  }

  /// Begins or resumes audio playback.
  Future<void> play() async {
    final current = state.currentItem;
    if (current == null) return;

    if (current.locked) {
      state = state.copyWith(
        status: PlayerLifecycleStatus.paused,
        errorMessage: current.lockReason.isNotEmpty
            ? current.lockReason
            : 'Premium subscription required to play this confession',
      );
      return;
    }

    final prevStatus = state.status;
    state = state.copyWith(status: PlayerLifecycleStatus.playing, errorMessage: '');

    try {
      // Start or resume session on backend
      if (prevStatus == PlayerLifecycleStatus.ready) {
        await _api.postSessionsByIdStart(sessionId);
      } else {
        await _api.postSessionsByIdResume(sessionId);
      }

      await _audio.play();
      _persist();
      _syncProgressNow();
    } catch (e) {
      state = state.copyWith(status: PlayerLifecycleStatus.error, errorMessage: e.toString());
    }
  }

  /// Pauses playback.
  Future<void> pause() async {
    state = state.copyWith(status: PlayerLifecycleStatus.paused);
    try {
      await _audio.pause();
      await _api.postSessionsByIdPause(sessionId);
    } catch (_) {}
    _persist();
    _syncProgressNow();
  }

  /// Resumes playback.
  Future<void> resume() => play();

  /// Seeks to a position in milliseconds.
  Future<void> seek(int positionMs) async {
    final current = state.currentItem;
    final maxMs = (current?.durationSeconds ?? 0) * 1000;
    final clamped = positionMs.clamp(0, maxMs > 0 ? maxMs : positionMs);
    state = state.copyWith(positionMs: clamped);
    await _audio.seek(Duration(milliseconds: clamped));
    _persist();
    _scheduleProgressSync();
  }

  /// Skips the current confession and advances the queue.
  Future<void> skip() async {
    final current = state.currentItem;
    if (current == null) return;

    await _audio.stop();

    final updatedSkipped = Set<String>.from(state.skippedItemIds)..add(current.id);
    state = state.copyWith(skippedItemIds: updatedSkipped);

    try {
      await _api.postSessionsByIdSkip(sessionId, {'item_id': current.id});
    } catch (_) {}

    _persist();

    if (state.currentIndex < state.items.length - 1) {
      await _transitionToItem(state.currentIndex + 1);
    } else {
      await complete();
    }
  }

  /// Navigates to previous confession or restarts current if > 3s.
  Future<void> previous() async {
    if (state.positionMs > 3000) {
      await seek(0);
      return;
    }
    if (state.currentIndex > 0) {
      await _transitionToItem(state.currentIndex - 1);
    }
  }

  /// Selects a specific item from the queue snapshot.
  Future<void> selectQueueItem(int index) async {
    if (index < 0 || index >= state.items.length || index == state.currentIndex) return;
    await _transitionToItem(index);
  }

  /// Completes the session.
  Future<void> complete() async {
    await _audio.stop();
    state = state.copyWith(status: PlayerLifecycleStatus.completed);

    try {
      await _api.postSessionsByIdComplete(sessionId);
      await _api.postMeHistory({
        'session_id': sessionId,
        'completed': true,
      });
    } catch (_) {}

    _persist();
  }

  /// Refreshes expired signed URLs from server without losing position (Criterion 6).
  Future<void> refreshSignedUrls() async {
    if (state.isRefreshingUrl) return;
    state = state.copyWith(isRefreshingUrl: true);

    final savedPos = state.positionMs;

    final queueLoadable = await _content.sessionQueue(sessionId);
    if (queueLoadable is LoadLoaded<SessionQueueResponse>) {
      final queue = queueLoadable.value;
      final items = queue.items;

      state = state.copyWith(
        items: items,
        isRefreshingUrl: false,
        errorMessage: '',
      );

      final current = state.currentItem;
      if (current != null && current.isPlayable) {
        try {
          await _audio.load(
            current.audioUrl,
            initialPosition: Duration(milliseconds: savedPos),
          );
          if (state.isPlaying) {
            await _audio.play();
          }
        } catch (e) {
          state = state.copyWith(status: PlayerLifecycleStatus.error, errorMessage: e.toString());
        }
      } else if (current != null && current.locked) {
        state = state.copyWith(
          status: PlayerLifecycleStatus.paused,
          errorMessage: current.lockReason.isNotEmpty
              ? current.lockReason
              : 'Premium subscription required to play this confession',
        );
      }
    } else {
      state = state.copyWith(
        isRefreshingUrl: false,
        status: PlayerLifecycleStatus.error,
        errorMessage: 'Failed to refresh signed audio URLs',
      );
    }
  }

  /// Handles incoming audio interruptions (e.g. phone call, Siri).
  void onInterruption() {
    _audio.pause();
    state = state.copyWith(status: PlayerLifecycleStatus.interrupted);
    _persist();
    try {
      _api.postSessionsByIdInterrupt(sessionId);
    } catch (_) {}
  }

  /// Handles audio becoming noisy (e.g. headphones unplugged).
  void onBecomingNoisy() {
    pause();
  }

  /// Retry action for actionable error states.
  Future<void> retry() async {
    state = state.copyWith(errorMessage: '');
    await refreshSignedUrls();
  }

  void _onCurrentTrackCompleted() async {
    final current = state.currentItem;
    if (current == null) return;

    final updatedCompleted = Set<String>.from(state.completedItemIds)..add(current.id);
    state = state.copyWith(completedItemIds: updatedCompleted);

    // Sync progress indicating track completed
    try {
      await _api.postSessionsByIdProgress(sessionId, {
        'queue_item_id': current.id,
        'item_status': 'completed',
        'position_ms': current.durationSeconds * 1000,
      });
    } catch (_) {}

    _persist();

    if (state.currentIndex < state.items.length - 1) {
      await _transitionToItem(state.currentIndex + 1);
    } else {
      await complete();
    }
  }

  Future<void> _transitionToItem(int newIndex) async {
    final wasPlaying = state.isPlaying;
    await _audio.stop();

    state = state.copyWith(
      currentIndex: newIndex,
      positionMs: 0,
      status: wasPlaying ? PlayerLifecycleStatus.playing : PlayerLifecycleStatus.ready,
      errorMessage: '',
    );

    final item = state.currentItem;
    if (item != null && item.isPlayable) {
      try {
        await _audio.load(item.audioUrl);
        if (wasPlaying) {
          await _audio.play();
        }
      } catch (e) {
        _onAudioError(e.toString());
      }
    } else if (item != null && item.locked) {
      state = state.copyWith(
        status: PlayerLifecycleStatus.paused,
        errorMessage: item.lockReason.isNotEmpty
            ? item.lockReason
            : 'Premium subscription required to play this confession',
      );
    }

    _persist();
    _syncProgressNow();
  }

  void _onAudioError(String error) {
    if (error.contains('403') || error.contains('expired') || error.contains('unauthorized')) {
      refreshSignedUrls();
    } else {
      state = state.copyWith(
        status: PlayerLifecycleStatus.error,
        errorMessage: error,
      );
    }
  }

  void _scheduleProgressSync() {
    // Only schedule if position changed by at least 3 seconds
    if ((state.positionMs - _lastSyncedPosition).abs() < 3000) return;
    _progressDebounce?.cancel();
    _progressDebounce = Timer(const Duration(seconds: 3), _syncProgressNow);
  }

  void _syncProgressNow() {
    _lastSyncedPosition = state.positionMs;
    final current = state.currentItem;
    final now = DateTime.now().toUtc().toIso8601String();
    state = state.copyWith(lastUpdatedAt: now);

    _content.syncProgress(
      sessionId,
      positionMs: state.positionMs,
      queueItemId: current?.id,
      itemStatus: state.isPlaying ? 'playing' : 'queued',
      lastUpdatedAt: now,
    );
  }

  void _persist() {
    final current = state.currentItem;
    final now = DateTime.now().toUtc().toIso8601String();
    state = state.copyWith(lastUpdatedAt: now);

    final persisted = PersistedPlaybackState(
      sessionId: sessionId,
      currentIndex: state.currentIndex,
      currentItemId: current?.id ?? '',
      positionMs: state.positionMs,
      completedItemIds: state.completedItemIds,
      skippedItemIds: state.skippedItemIds,
      lastPlaybackState: state.status.name,
      lastUpdatedAt: now,
    );

    _storage.writeJson(StoreKeys.sessionPlayback(sessionId), persisted.toJson());
  }

  @override
  void dispose() {
    _disposed = true;
    _progressDebounce?.cancel();
    _posSub?.cancel();
    _statusSub?.cancel();
    _errorSub?.cancel();
    _interruptionSub?.cancel();
    _noisySub?.cancel();
    super.dispose();
  }
}

/// Session Engine Provider: manages the playback lifecycle for a session.
final sessionEngineProvider =
    StateNotifierProvider.family<SessionEngine, SessionPlaybackState, String>((ref, id) {
  return SessionEngine(ref, id);
});

/// Legacy compatibility providers for screens watching player data:
final playerSessionProvider =
    FutureProvider.family<Loadable<ListeningSession>, String>((ref, id) async {
  return ref.watch(contentRepositoryProvider).session(id);
});

enum PlayerStatus { idle, playing, paused, completed, error }

final playerStatusProvider = StateProvider<PlayerStatus>((ref) => PlayerStatus.idle);
final playerPositionProvider = StateProvider<int>((ref) => 0);
final playerCurrentIndexProvider = StateProvider<int>((ref) => 0);
