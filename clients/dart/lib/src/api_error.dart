/// Errors the API can return, as a closed set the UI can switch on.
///
/// The backend sends a stable `code` alongside a human message. Clients must
/// branch on the code, never the message: messages are localised and reworded,
/// and string-matching them is how "this is a Premium feature" degrades into
/// "something went wrong".
library;

/// A failure from the API or the transport beneath it.
sealed class ApiException implements Exception {
  const ApiException(this.message);

  /// Human-readable text. Safe to show, but prefer mapping [ApiError] to your
  /// own copy so it can be localised.
  final String message;

  @override
  String toString() => '$runtimeType: $message';
}

/// The request never reached the server, or the reply was unintelligible.
///
/// Distinct from an auth failure on purpose: showing "invalid password" when
/// the user is simply offline is one of the most common client bugs, and it
/// makes people change a password that was never wrong.
final class NetworkException extends ApiException {
  const NetworkException(super.message, {this.cause});

  final Object? cause;
}

/// The server answered with an error status.
final class ApiError extends ApiException {
  const ApiError({
    required this.status,
    required this.code,
    required String message,
    this.retryAfter,
  }) : super(message);

  /// HTTP status code.
  final int status;

  /// Stable machine-readable code. Empty when the server sent none.
  final String code;

  /// How long to wait before retrying, present on 429.
  final Duration? retryAfter;

  /// The caller must sign in again. Covers an expired session, a revoked one,
  /// and a detected token reuse.
  bool get requiresReauth => status == 401;

  /// The session was ended because a token was replayed, which usually means
  /// it was copied. Worth telling the user explicitly rather than showing a
  /// generic "signed out": it is a security event they should know about.
  bool get wasTokenReused => code == ErrorCodes.tokenReused;

  /// The action needs a subscription the account does not have.
  bool get requiresSubscription =>
      status == 402 || code == ErrorCodes.entitlementRequired;

  /// The caller is authenticated but not permitted.
  bool get isForbidden => status == 403;

  /// Too many attempts. Honour [retryAfter] rather than retrying immediately.
  bool get isRateLimited => status == 429;

  /// A second factor is required to complete sign-in.
  bool get needsSecondFactor => code == ErrorCodes.mfaCodeInvalid;

  /// Server-side fault. Worth retrying; a 4xx generally is not.
  bool get isServerFault => status >= 500;

  @override
  String toString() => 'ApiError($status, $code): $message';
}

/// Stable error codes emitted by the backend.
///
/// Kept as constants rather than an enum so an unrecognised code from a newer
/// server degrades to "unknown" instead of throwing during deserialisation —
/// an old client must not crash because the API added a case.
abstract final class ErrorCodes {
  // Authentication
  static const invalidCredentials = 'AUTH_INVALID_CREDENTIALS';

  /// An account that cannot sign in, for a reason the API will not state.
  ///
  /// Deliberately distinct from [accountSuspended]. The login handler refuses a
  /// restricted account with "this account is not available" because moderation
  /// state is not the caller's business (S80); a code that named the reason
  /// would disclose exactly what the message withholds.
  static const accountUnavailable = 'AUTH_ACCOUNT_UNAVAILABLE';

  /// The auth service itself is unavailable — for example, sign-in cannot read
  /// the second-factor enrolment and refuses rather than skipping it.
  static const authUnavailable = 'AUTH_UNAVAILABLE';

  /// The request body or a field in it was rejected. The human message carries
  /// the specific rule, which is the actionable part.
  static const validationFailed = 'AUTH_VALIDATION_FAILED';
  static const sessionExpired = 'AUTH_SESSION_EXPIRED';
  static const sessionRevoked = 'AUTH_SESSION_REVOKED';
  static const tokenInvalid = 'AUTH_TOKEN_INVALID';
  static const tokenReused = 'AUTH_TOKEN_REUSED';
  static const rateLimited = 'AUTH_RATE_LIMITED';
  static const accountSuspended = 'AUTH_ACCOUNT_SUSPENDED';
  static const insufficientPermission = 'AUTH_INSUFFICIENT_PERMISSION';
  static const providerDisabled = 'AUTH_PROVIDER_DISABLED';
  static const providerUnavailable = 'AUTH_PROVIDER_UNAVAILABLE';
  static const linkRequiresSignIn = 'AUTH_LINK_REQUIRES_SIGN_IN';
  static const identityInUse = 'AUTH_IDENTITY_IN_USE';
  static const lastCredential = 'AUTH_LAST_CREDENTIAL';

  // Two-factor
  static const mfaCodeRequired = 'MFA_CODE_REQUIRED';
  static const mfaCodeInvalid = 'MFA_CODE_INVALID';
  static const mfaAlreadyEnabled = 'MFA_ALREADY_ENABLED';
  static const mfaNotStarted = 'MFA_NOT_STARTED';
  static const mfaNotEnabled = 'MFA_NOT_ENABLED';

  // Profile
  static const profileInvalid = 'PROFILE_INVALID';
  static const profileUpdateFailed = 'PROFILE_UPDATE_FAILED';
  static const usernameTaken = 'USERNAME_TAKEN';
  static const usernameInvalid = 'USERNAME_INVALID';
  static const invalidTimezone = 'INVALID_TIMEZONE';
  static const invalidLocale = 'INVALID_LOCALE';

  // Content and entitlement
  static const voiceUnavailable = 'VOICE_UNAVAILABLE';
  static const categoryNotFound = 'CATEGORY_NOT_FOUND';
  static const resourceNotFound = 'RESOURCE_NOT_FOUND';
  static const entitlementRequired = 'ENTITLEMENT_REQUIRED';
  static const downloadLimitReached = 'DOWNLOAD_LIMIT_REACHED';
  static const audioNotPublished = 'AUDIO_NOT_PUBLISHED';

  // Account lifecycle
  static const deletionNotConfirmed = 'DELETION_NOT_CONFIRMED';
  static const deletionAlreadyScheduled = 'DELETION_ALREADY_SCHEDULED';
  static const deletionAlreadyDone = 'DELETION_ALREADY_DONE';
  static const deletionNotScheduled = 'DELETION_NOT_SCHEDULED';
}
