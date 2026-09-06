import 'package:iconfess_api/iconfess_api.dart';

/// What the app shows when something goes wrong.
///
/// Section 38 of the directive is explicit that "Something went wrong." is not
/// an acceptable answer. A message has to say what happened, what the listener
/// can do, and — where it matters — that their work was not lost. Three
/// sentences is the budget; a paragraph on a phone screen is not an explanation.
///
/// This is a pure mapping with no Flutter dependency, so every branch is
/// testable without a widget tree.
class UserFacingError {
  const UserFacingError({
    required this.title,
    required this.message,
    this.primaryAction,
    this.secondaryAction,
    this.retryable = false,
  });

  /// Short label, shown as the heading of an error state.
  final String title;

  /// One or two sentences: what happened and what to do about it.
  final String message;

  /// The recovery the app can perform, if any. Null means there is nothing to
  /// offer but acknowledgement, and the UI must not invent a button.
  final ErrorAction? primaryAction;
  final ErrorAction? secondaryAction;

  /// Whether trying again is likely to help. Drives whether "Try again" appears
  /// at all: offering a retry for a 400 is offering a button that fails.
  final bool retryable;
}

/// Which auth endpoint a failure came from, so a status can be read correctly.
enum AuthSurface {
  /// `/auth/login`, `/auth/register`, `/auth/request-password-reset`.
  credentials,

  /// `/auth/verify-email`, `/auth/reset-password`: endpoints that consume a
  /// one-time token rather than a password.
  token,
}

/// A recovery the error state can offer.
enum ErrorAction {
  retry,
  signIn,
  viewDownloads,
  refreshSession,
  upgrade,
  contactSupport,
  goBack,
}

/// Maps a transport or API failure onto copy a listener can act on.
abstract final class ErrorMapper {
  static UserFacingError describe(ApiException error) => switch (error) {
        NetworkException() => const UserFacingError(
            title: 'You’re offline',
            message: 'Downloaded sessions are still available. '
                'We’ll pick up where you left off when you’re back.',
            primaryAction: ErrorAction.viewDownloads,
            secondaryAction: ErrorAction.retry,
            retryable: true,
          ),
        ApiError(:final status, :final code, :final retryAfter) => _describeApi(status, code, retryAfter),
      };

  /// Errors on the unauthenticated auth surface: sign-in, registration,
  /// verification and password reset.
  ///
  /// A separate mapping rather than a branch inside [describe], because the same
  /// status means different things on the two sides of the session boundary. A
  /// 401 from an authenticated call means "your session ended, sign in again";
  /// a 401 from `/auth/login` means the password was wrong. Routing the second
  /// through the first tells someone who mistyped that their session has ended —
  /// which is not merely unhelpful, it is a claim about their account that the
  /// server never made.
  ///
  /// It matches on the server's stable code first and falls back to status.
  ///
  /// The fallback is not decoration. `contracts/openapi.json` has always
  /// described the envelope as `{error, code}` with "Do not parse; use code", but
  /// the six public auth handlers answered with `httpx.WriteError`, which writes
  /// `{error}` and nothing else — so PHASE 19 shipped status-only matching and
  /// recorded it as condition C-2. The handlers were then fixed to emit codes
  /// (`AUTH_INVALID_CREDENTIALS`, `AUTH_TOKEN_INVALID`,
  /// `AUTH_ACCOUNT_UNAVAILABLE`, `AUTH_UNAVAILABLE`, `AUTH_VALIDATION_FAILED`),
  /// and the status branch stays as the behaviour against a server that has not
  /// been upgraded yet. An old client must not break against a new server, and a
  /// new client must not break against an old one.
  /// Which side of the session boundary a failure came from.
  ///
  /// The auth endpoints answer 401 for two entirely different things: bad
  /// credentials on `/auth/login`, and an invalid or expired one-time token on
  /// `/auth/verify-email` and `/auth/reset-password`. Neither carries a `code`,
  /// so the caller has to say which endpoint it was talking to. Getting this
  /// wrong tells someone who pasted a stale link that their password was wrong.
  static UserFacingError describeAuth(
    ApiException error, {
    AuthSurface surface = AuthSurface.credentials,
  }) =>
      switch (error) {
        NetworkException() => const UserFacingError(
            title: 'You’re offline',
            message: 'Nothing was sent. Check your connection and try again — '
                'what you typed is still here.',
            primaryAction: ErrorAction.retry,
            retryable: true,
          ),
        ApiError(:final status, :final code, :final retryAfter, :final message) =>
          _describeAuthApi(status, code, retryAfter, message, surface),
      };

  static UserFacingError _describeAuthApi(
    int status,
    String code,
    Duration? retryAfter,
    String message,
    AuthSurface surface,
  ) {
    // Codes first: they are what the contract promises, and what survives a
    // rewording of the human message.
    switch (code) {
      case ErrorCodes.invalidCredentials:
        return const UserFacingError(
          title: 'That didn’t match',
          message: 'Check the email and password and try again. '
              'If you’ve forgotten the password, we can send a reset link.',
          primaryAction: ErrorAction.retry,
        );
      case ErrorCodes.accountUnavailable:
        return const UserFacingError(
          title: 'This account can’t sign in',
          message: 'If you think this is a mistake, contact support and we’ll take a look.',
          primaryAction: ErrorAction.contactSupport,
        );
      case ErrorCodes.authUnavailable:
        return const UserFacingError(
          title: 'Sign-in is unavailable right now',
          message: 'This is on our side. Try again in a moment — nothing was lost.',
          primaryAction: ErrorAction.retry,
          retryable: true,
        );
      case ErrorCodes.tokenInvalid:
        // The same code means different things on the two surfaces, which is
        // exactly why [AuthSurface] exists.
        if (surface == AuthSurface.token) {
          return const UserFacingError(
            title: 'That code has expired',
            message: 'These codes work once and then expire. Request a new one '
                'and use the newest email you were sent.',
          );
        }
      case ErrorCodes.validationFailed:
        return UserFacingError(
          title: 'That didn’t work',
          message: message.isEmpty ? 'Check the form and try again.' : _sentence(message),
        );
    }

    if (code == ErrorCodes.rateLimited || status == 429) {
      final wait = retryAfter == null ? 'a moment' : _humanise(retryAfter);
      return UserFacingError(
        title: 'Too many attempts',
        message: 'Wait $wait and try again. Your account is not locked.',
        primaryAction: ErrorAction.retry,
        retryable: true,
      );
    }

    return switch (status) {
      401 => surface == AuthSurface.token
          // An expired or already-used one-time token. The recovery is a new
          // one, and the screens that accept a token both offer a resend.
          ? const UserFacingError(
              title: 'That code has expired',
              message: 'These codes work once and then expire. Request a new one '
                  'and use the newest email you were sent.',
            )
          // "Invalid credentials". Deliberately not distinguished from any other
          // 401 on this surface, because the server does not distinguish it
          // either: one message for a wrong password and for an unknown address
          // is what stops this endpoint being used to find out who has an
          // account (S19).
          : const UserFacingError(
              title: 'That didn’t match',
              message: 'Check the email and password and try again. '
                  'If you’ve forgotten the password, we can send a reset link.',
              primaryAction: ErrorAction.retry,
            ),
      // The server's own words are "this account is not available", and it keeps
      // them vague on purpose: moderation state is not the caller's business
      // (S80). Support is the only useful recovery.
      403 => const UserFacingError(
          title: 'This account can’t sign in',
          message: 'If you think this is a mistake, contact support and we’ll take a look.',
          primaryAction: ErrorAction.contactSupport,
        ),
      // A rejection of the input: password policy, a malformed or expired
      // token. The server's message is the useful part — "password must be at
      // least 8 characters" is actionable where "that request was incomplete"
      // is not — so it is passed through rather than replaced.
      400 => UserFacingError(
          title: 'That didn’t work',
          message: message.isEmpty
              ? 'Check the form and try again.'
              : _sentence(message),
        ),
      404 => const UserFacingError(
          title: 'We couldn’t find that',
          message: 'The link may be incomplete. Open it again from the email.',
          primaryAction: ErrorAction.goBack,
        ),
      503 => const UserFacingError(
          title: 'Sign-in is unavailable right now',
          message: 'This is on our side. Try again in a moment — nothing was lost.',
          primaryAction: ErrorAction.retry,
          retryable: true,
        ),
      >= 500 => const UserFacingError(
          title: 'We couldn’t reach the server',
          message: 'This is on our side, not yours. Try again in a moment.',
          primaryAction: ErrorAction.retry,
          retryable: true,
        ),
      _ => UserFacingError(
          title: 'Something interrupted that',
          message: message.isEmpty
              ? 'Try again. If it keeps happening, contact support.'
              : _sentence(message),
          primaryAction: ErrorAction.retry,
          secondaryAction: ErrorAction.contactSupport,
          retryable: true,
        ),
    };
  }

  /// Capitalises a server message for use as a sentence.
  ///
  /// The server's validation strings are lowercase by convention ("password must
  /// be at least 8 characters"). Shown verbatim at the start of a line they read
  /// as a fragment rather than an explanation.
  static String _sentence(String message) {
    final trimmed = message.trim();
    if (trimmed.isEmpty) return trimmed;
    return trimmed[0].toUpperCase() + trimmed.substring(1);
  }

  static UserFacingError _describeApi(int status, String code, Duration? retryAfter) {    // Auth failures are matched on the server's stable codes rather than the
    // status alone: a 401 can mean an expired token, a revoked session or a
    // reused refresh token, and those need different recovery.
    switch (code) {
      case ErrorCodes.sessionExpired:
      case ErrorCodes.tokenInvalid:
      case ErrorCodes.sessionRevoked:
        return const UserFacingError(
          title: 'Your session has ended',
          message: 'Sign in again to continue. Your listening history is saved.',
          primaryAction: ErrorAction.signIn,
        );
      case ErrorCodes.rateLimited:
        final wait = retryAfter == null ? 'a moment' : _humanise(retryAfter);
        return UserFacingError(
          title: 'Too many attempts',
          message: 'Wait $wait and try again. Nothing was lost.',
          primaryAction: ErrorAction.retry,
          retryable: true,
        );
      case ErrorCodes.accountSuspended:
        return const UserFacingError(
          title: 'This account is suspended',
          message: 'If you think this is a mistake, contact support and we’ll take a look.',
          primaryAction: ErrorAction.contactSupport,
        );
      case ErrorCodes.entitlementRequired:
      case ErrorCodes.downloadLimitReached:
        return const UserFacingError(
          title: 'That’s part of Premium',
          message: 'Your membership includes this. You can keep listening to everything free.',
          primaryAction: ErrorAction.upgrade,
          secondaryAction: ErrorAction.goBack,
        );
      case ErrorCodes.audioNotPublished:
      case ErrorCodes.voiceUnavailable:
        return const UserFacingError(
          title: 'Audio isn’t available right now',
          message: 'This recording is still being prepared. Try another confession, '
              'or come back shortly.',
          primaryAction: ErrorAction.goBack,
          secondaryAction: ErrorAction.retry,
          retryable: true,
        );
      case ErrorCodes.resourceNotFound:
      case ErrorCodes.categoryNotFound:
        return const UserFacingError(
          title: 'This is no longer available',
          message: 'It may have been removed or renamed. Your session has been refreshed '
              'with the latest content.',
          primaryAction: ErrorAction.refreshSession,
          secondaryAction: ErrorAction.goBack,
        );
      case ErrorCodes.invalidCredentials:
        return const UserFacingError(
          title: 'That didn’t match',
          message: 'Check the email and password and try again.',
          primaryAction: ErrorAction.retry,
          retryable: true,
        );
    }

    return switch (status) {
      >= 500 => const UserFacingError(
          title: 'We couldn’t reach the server',
          message: 'This is on our side, not yours. Try again in a moment.',
          primaryAction: ErrorAction.retry,
          retryable: true,
        ),
      429 => const UserFacingError(
          title: 'Slow down for a moment',
          message: 'You’ve made a lot of requests. Give it a few seconds.',
          primaryAction: ErrorAction.retry,
          retryable: true,
        ),
      400 => const UserFacingError(
          title: 'That request was incomplete',
          message: 'Check the form and try again. If it keeps happening, contact support.',
          primaryAction: ErrorAction.contactSupport,
        ),
      401 || 403 => const UserFacingError(
          title: 'You need to sign in',
          message: 'Your session has ended. Sign in again to continue.',
          primaryAction: ErrorAction.signIn,
        ),
      404 => const UserFacingError(
          title: 'Not found',
          message: 'We couldn’t find what you were looking for.',
          primaryAction: ErrorAction.goBack,
        ),
      _ => const UserFacingError(
          title: 'Something interrupted that',
          message: 'Try again. If it keeps happening, contact support and we’ll look into it.',
          primaryAction: ErrorAction.retry,
          secondaryAction: ErrorAction.contactSupport,
          retryable: true,
        ),
    };
  }

  static String _humanise(Duration d) {
    if (d.inSeconds < 60) return '${d.inSeconds}s';
    if (d.inMinutes < 60) return '${d.inMinutes} min';
    return '${d.inHours} h';
  }
}
