import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:iconfess_api/iconfess_api.dart';

import '../../core/di/providers.dart';

/// The session currently being played, re-fetched to mint fresh signed URLs.
final playerSessionProvider =
    FutureProvider.family<Loadable<ListeningSession>, String>((ref, id) async {
  return ref.watch(contentRepositoryProvider).session(id);
});

/// The current playback position in milliseconds.
final playerPositionProvider = StateProvider<int>((ref) => 0);

/// Whether the player is playing, paused, or completed.
enum PlayerStatus { idle, playing, paused, completed, error }

final playerStatusProvider = StateProvider<PlayerStatus>((ref) => PlayerStatus.idle);

/// Current item index in the session queue.
final playerCurrentIndexProvider = StateProvider<int>((ref) => 0);

/// Player controller: orchestrates start, pause, resume, skip, complete with rollback and error surfacing (IC-002, IC-021).
final playerControllerProvider =
    StateNotifierProvider<PlayerController, PlayerState>((ref) {
  return PlayerController(ref);
});

class PlayerState {
  const PlayerState({
    this.status = PlayerStatus.idle,
    this.positionMs = 0,
    this.currentIndex = 0,
    this.currentQueueItemId = '',
    this.error = '',
  });

  final PlayerStatus status;
  final int positionMs;
  final int currentIndex;
  final String currentQueueItemId;
  final String error;

  PlayerState copyWith({
    PlayerStatus? status,
    int? positionMs,
    int? currentIndex,
    String? currentQueueItemId,
    String? error,
  }) =>
      PlayerState(
        status: status ?? this.status,
        positionMs: positionMs ?? this.positionMs,
        currentIndex: currentIndex ?? this.currentIndex,
        currentQueueItemId: currentQueueItemId ?? this.currentQueueItemId,
        error: error ?? this.error,
      );
}

class PlayerController extends StateNotifier<PlayerState> {
  PlayerController(this.ref) : super(const PlayerState());

  final Ref ref;

  Future<void> start(String sessionId) async {
    final prev = state;
    state = state.copyWith(status: PlayerStatus.playing, error: '');
    try {
      final sessLoadable = await ref.read(contentRepositoryProvider).session(sessionId);
      final session = sessLoadable.valueOrNull;
      var queueItemId = '';
      if (session != null && session.items.isNotEmpty) {
        queueItemId = session.items[0].id;
      }
      await ref.read(apiClientProvider).postSessionsByIdStart(sessionId);
      state = state.copyWith(currentQueueItemId: queueItemId);
    } catch (e) {
      state = prev.copyWith(status: PlayerStatus.error, error: e.toString());
    }
  }

  Future<void> pause(String sessionId) async {
    final prev = state;
    state = state.copyWith(status: PlayerStatus.paused);
    try {
      await ref.read(apiClientProvider).postSessionsByIdPause(sessionId);
    } catch (e) {
      state = prev.copyWith(error: e.toString());
    }
  }

  Future<void> resume(String sessionId) async {
    final prev = state;
    state = state.copyWith(status: PlayerStatus.playing);
    try {
      await ref.read(apiClientProvider).postSessionsByIdResume(sessionId);
    } catch (e) {
      state = prev.copyWith(error: e.toString());
    }
  }

  Future<void> skip(String sessionId, [String? queueItemId]) async {
    final prev = state;
    try {
      await ref.read(apiClientProvider).postSessionsByIdSkip(sessionId);
      state = state.copyWith(currentIndex: state.currentIndex + 1, positionMs: 0, error: '');
    } catch (e) {
      state = prev.copyWith(error: e.toString());
    }
  }

  Future<void> complete(String sessionId) async {
    final prev = state;
    state = state.copyWith(status: PlayerStatus.completed);
    try {
      await ref.read(apiClientProvider).postSessionsByIdComplete(sessionId);
      await ref.read(apiClientProvider).postMeHistory({
        'session_id': sessionId,
        'completed': true,
      });
    } catch (e) {
      state = prev.copyWith(status: PlayerStatus.error, error: e.toString());
    }
  }

  void seek(int ms) {
    state = state.copyWith(positionMs: ms);
  }

  Future<void> reportProgress(String sessionId, int positionMs, [String? queueItemId]) async {
    final qId = queueItemId ?? state.currentQueueItemId;
    try {
      await ref.read(apiClientProvider).postSessionsByIdProgress(sessionId, {
        'position_ms': positionMs,
        'queue_item_id': qId,
      });
    } catch (e) {
      state = state.copyWith(error: e.toString());
    }
  }
}
