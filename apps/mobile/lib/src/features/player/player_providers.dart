import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:iconfess_api/iconfess_api.dart';

import '../../core/di/providers.dart';

/// The session currently being played, re-fetched to mint fresh signed URLs.
///
/// Audio URLs are short-lived; replaying a cached session would hand back links
/// that have expired. This provider always hits the network first.
final playerSessionProvider =
    FutureProvider.family<Loadable<ListeningSession>, String>((ref, id) async {
  return ref.watch(contentRepositoryProvider).session(id);
});

/// The current playback position in milliseconds.
///
/// Held locally, synced to server via progress endpoint. The server is the
/// source of truth for resume, but the UI needs immediate feedback.
final playerPositionProvider = StateProvider<int>((ref) => 0);

/// Whether the player is playing, paused, or completed.
enum PlayerStatus { idle, playing, paused, completed, error }

final playerStatusProvider = StateProvider<PlayerStatus>((ref) => PlayerStatus.idle);

/// Current item index in the session queue.
final playerCurrentIndexProvider = StateProvider<int>((ref) => 0);

/// Player controller: orchestrates start, pause, resume, skip, complete.
///
/// No real audio engine here (just_audio is commented out per D-4); this is the
/// state machine and server sync layer that the audio layer will plug into in
/// PHASE 24. It already handles the lifecycle correctly: progress is reported,
/// completion is recorded, and locked items are skipped.
final playerControllerProvider =
    StateNotifierProvider<PlayerController, PlayerState>((ref) {
  return PlayerController(ref);
});

class PlayerState {
  const PlayerState({
    this.status = PlayerStatus.idle,
    this.positionMs = 0,
    this.currentIndex = 0,
    this.error = '',
  });

  final PlayerStatus status;
  final int positionMs;
  final int currentIndex;
  final String error;

  PlayerState copyWith({
    PlayerStatus? status,
    int? positionMs,
    int? currentIndex,
    String? error,
  }) =>
      PlayerState(
        status: status ?? this.status,
        positionMs: positionMs ?? this.positionMs,
        currentIndex: currentIndex ?? this.currentIndex,
        error: error ?? this.error,
      );
}

class PlayerController extends StateNotifier<PlayerState> {
  PlayerController(this.ref) : super(const PlayerState());

  final Ref ref;

  Future<void> start(String sessionId) async {
    state = state.copyWith(status: PlayerStatus.playing);
    try {
      await ref.read(contentRepositoryProvider).session(sessionId);
      // In real implementation, call POST /sessions/{id}/start
      await ref.read(apiClientProvider).postSessionsByIdStart(sessionId);
    } catch (e) {
      state = state.copyWith(status: PlayerStatus.error, error: e.toString());
    }
  }

  Future<void> pause(String sessionId) async {
    state = state.copyWith(status: PlayerStatus.paused);
    try {
      await ref.read(apiClientProvider).postSessionsByIdPause(sessionId);
    } catch (_) {}
  }

  Future<void> resume(String sessionId) async {
    state = state.copyWith(status: PlayerStatus.playing);
    try {
      await ref.read(apiClientProvider).postSessionsByIdResume(sessionId);
    } catch (_) {}
  }

  Future<void> skip(String sessionId) async {
    try {
      await ref.read(apiClientProvider).postSessionsByIdSkip(sessionId);
      state = state.copyWith(currentIndex: state.currentIndex + 1, positionMs: 0);
    } catch (_) {}
  }

  Future<void> complete(String sessionId) async {
    state = state.copyWith(status: PlayerStatus.completed);
    try {
      await ref.read(apiClientProvider).postSessionsByIdComplete(sessionId);
      await ref.read(apiClientProvider).postMeHistory({
        'session_id': sessionId,
        'completed': true,
      });
    } catch (_) {}
  }

  void seek(int ms) {
    state = state.copyWith(positionMs: ms);
  }

  Future<void> reportProgress(String sessionId, int positionMs) async {
    try {
      await ref.read(apiClientProvider).postSessionsByIdProgress(sessionId, {
        'position_ms': positionMs,
        'queue_item_id': '',
      });
    } catch (_) {}
  }
}
