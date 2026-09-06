import 'package:flutter/foundation.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:iconfess_api/iconfess_api.dart';

import '../../core/analytics/analytics.dart';
import '../../core/di/providers.dart';

/// Where the listener is, as far as access is concerned.
enum AuthStatus {
  /// Not yet known. The token is being read from the keystore. The router must
  /// not decide anything during this state, or a signed-in user sees the sign-in
  /// screen for a frame on every launch.
  unknown,
  signedOut,
  signedIn,
}

@immutable
class AuthState {
  const AuthState(this.status, {this.userId});

  const AuthState.unknown() : this(AuthStatus.unknown);
  const AuthState.signedOut() : this(AuthStatus.signedOut);

  /// [userId] is filled in once the profile has been fetched; sign-in itself
  /// returns only a token.
  const AuthState.signedIn({this.userId}) : status = AuthStatus.signedIn;

  final AuthStatus status;
  final String? userId;

  bool get isResolved => status != AuthStatus.unknown;
  bool get isSignedIn => status == AuthStatus.signedIn;

  @override
  bool operator ==(Object other) =>
      other is AuthState && other.status == status && other.userId == userId;

  @override
  int get hashCode => Object.hash(status, userId);

  @override
  String toString() => 'AuthState($status${userId == null ? '' : ', $userId'})';
}

/// Owns the session.
///
/// The router reads this to decide where to send the listener; screens read it to
/// decide what to show. Keeping it in one notifier is what stops the two from
/// disagreeing — a router that checks the token directly and a screen that checks
/// a flag will eventually route somewhere the screen cannot render.
class AuthController extends Notifier<AuthState> {
  @override
  AuthState build() => const AuthState.unknown();

  /// Reads the keystore to find out whether a session exists.
  ///
  /// Called once at startup, before the first frame is routed. It only checks
  /// for the presence of a token: it does not validate it, because a network
  /// round trip on the launch path would put a spinner in front of every cold
  /// start, and an invalid token is discovered by the first real request.
  Future<void> restore() async {
    if (state.isResolved) return;
    final hasSession = await ref.read(tokenStoreProvider).hasSession();
    state = hasSession ? const AuthState.signedIn() : const AuthState.signedOut();
  }

  /// Signs in and records the outcome.
  ///
  /// Returns the repository's sealed outcome rather than collapsing it to a bool:
  /// the screen has to tell bad credentials apart from being offline apart from
  /// an MFA challenge, and those are three different screens.
  ///
  /// The token itself is persisted by the client's transport, not here — keeping
  /// that in one place is what stops a second sign-in path from forgetting it.
  Future<SignInOutcome> signIn({
    required String email,
    required String password,
    String? code,
  }) async {
    final outcome = await ref
        .read(authRepositoryProvider)
        .signIn(email: email, password: password, code: code);
    if (outcome is SignedIn) {
      state = const AuthState.signedIn();
      ref.read(analyticsProvider).track(AnalyticsEvents.signInSucceeded);
    }
    return outcome;
  }

  /// Records the listener once the profile is known, so events carry attribution.
  void attachUser(String userId) {
    if (!state.isSignedIn) return;
    state = AuthState.signedIn(userId: userId);
    ref.read(analyticsProvider).identify(userId);
  }

  Future<WriteResult<String>> register({
    required String email,
    required String password,
    String? displayName,
  }) =>
      ref.read(authRepositoryProvider).register(
            email: email,
            password: password,
            displayName: displayName,
          );

  /// Signs out locally and best-effort remotely.
  ///
  /// The local clear is not conditional on the server call succeeding. A listener
  /// who taps sign out on a plane must still be signed out when they land;
  /// leaving them signed in because a revoke request failed is the worse failure.
  Future<void> signOut() async {
    final repo = ref.read(authRepositoryProvider);
    try {
      await repo.signOut();
    } on Object catch (_) {
      // Deliberate: see above.
    }
    await ref.read(tokenStoreProvider).clear();
    ref.read(analyticsProvider).reset();
    state = const AuthState.signedOut();
  }

  /// Called when the server reports the session was lost mid-use.
  void onAuthenticationLost(AuthLossReason reason) {
    state = const AuthState.signedOut();
    ref.read(analyticsProvider).track(
      AnalyticsEvents.signOut,
      properties: {'reason': reason.name},
    );
  }
}

final authControllerProvider =
    NotifierProvider<AuthController, AuthState>(AuthController.new);

/// Convenience for widgets that only need the boolean.
final isSignedInProvider = Provider<bool>((ref) => ref.watch(authControllerProvider).isSignedIn);
