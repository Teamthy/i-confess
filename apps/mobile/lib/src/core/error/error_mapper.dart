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

  static UserFacingError _describeApi(int status, String code, Duration? retryAfter) {
    // Auth failures are matched on the server's stable codes rather than the
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
