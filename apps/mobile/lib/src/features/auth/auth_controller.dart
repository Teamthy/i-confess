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

  /// [userId] is filled in when the response that created the session carried
  /// it. Sign-in and registration both return the user object; a session
  /// restored from the keystore does not, so this is null on a cold start until
  /// the profile is fetched.
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
///
/// The token itself lives in the API client's transport ([ApiClient.attachSession]),
/// which is the layer that attaches it to requests and rotates it. This notifier
/// tracks the *state*, not the credential.
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
  /// A successful sign-in persists the token inside the repository call, before
  /// this returns. Order matters: a screen that navigates on the outcome must
  /// not be able to reach an authenticated request before the credential is
  /// stored.
  Future<SignInOutcome> signIn({
    required String email,
    required String password,
    String? code,
  }) async {
    final outcome = await ref
        .read(authRepositoryProvider)
        .signIn(email: email, password: password, code: code);
    if (outcome is SignedIn) {
      state = AuthState.signedIn(userId: outcome.userId);
      if (outcome.userId case final id?) attachUser(id);
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

  /// Creates an account.
  ///
  /// Three outcomes and only one of them is a failure. A duplicate address comes
  /// back as [CheckYourEmail] — the server answers it exactly as it answers a
  /// fresh signup, minus the session, so that registration cannot be used to
  /// find out who has an account (S65). Both non-failure paths end on the
  /// verification screen; only one of them is signed in, which is why the
  /// screen asks the router rather than assuming.
  Future<RegisterOutcome> register({
    required String email,
    required String password,
    String? displayName,
    String? timezone,
  }) async {
    final outcome = await ref.read(authRepositoryProvider).register(
          email: email,
          password: password,
          displayName: displayName,
          timezone: timezone,
        );
    if (outcome is AccountCreated) {
      state = AuthState.signedIn(userId: outcome.userId);
      if (outcome.userId case final id?) attachUser(id);
      ref.read(analyticsProvider).track(AnalyticsEvents.signUpSucceeded);
    }
    return outcome;
  }

  /// Asks the server to email a reset link.
  ///
  /// The result is returned rather than swallowed so the screen can show an
  /// offline failure, but the *content* of a success must stay neutral: the
  /// server answers identically for known and unknown addresses, and a client
  /// that worded the two differently would undo that (S33).
  Future<WriteResult<void>> requestPasswordReset(String email) =>
      ref.read(authRepositoryProvider).requestPasswordReset(email);

  /// Sets a new password with a token from the email link.
  ///
  /// The server ends every session for the account when this succeeds, because
  /// whoever held the old password might have been an attacker (S34). The local
  /// session is cleared here for the same reason — leaving a token on a device
  /// whose password was just reset would defeat the point.
  Future<WriteResult<void>> resetPassword({
    required String token,
    required String password,
  }) async {
    final result =
        await ref.read(authRepositoryProvider).resetPassword(token: token, password: password);
    if (result.succeeded) {
      await ref.read(tokenStoreProvider).clear();
      ref.read(analyticsProvider).reset();
      state = const AuthState.signedOut();
    }
    return result;
  }

  /// Confirms an email address with a one-time token.
  Future<WriteResult<void>> verifyEmail(String token) =>
      ref.read(authRepositoryProvider).verifyEmail(token);

  /// Asks for the confirmation email to be sent again.
  Future<WriteResult<void>> resendVerification(String email) =>
      ref.read(authRepositoryProvider).resendVerification(email);

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
