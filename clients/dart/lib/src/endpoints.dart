import 'api_client.dart';

/// Typed endpoint methods, generated from contracts/openapi.json.
///
/// Method names come from the spec's operationIds, so a rename on the server
/// surfaces here as a compile error rather than a 404 at runtime.
///
/// Returns are deliberately `Map<String, dynamic>`: modelling every response
/// as a class would double this file and force a client update whenever the
/// server adds a field. Feature code maps these into its own domain types,
/// which is where the shape actually matters.
extension IConfessEndpoints on ApiClient {
  // ---- account ----
  /// Cancel a scheduled deletion
  Future<Map<String, dynamic>> deleteMeDeletion() => delete('/me/deletion');

  /// Preview what deletion removes and retains
  Future<Map<String, dynamic>> getMeDeletion() => get('/me/deletion');

  /// Schedule account deletion
  Future<Map<String, dynamic>> postMeDeletion([Map<String, dynamic>? body]) =>
      post('/me/deletion', body);

  /// Download all personal data
  Future<Map<String, dynamic>> getMeExport() => get('/me/export');

  // ---- auth ----
  /// Change password and revoke other sessions
  Future<Map<String, dynamic>> postAuthChangePassword(
          [Map<String, dynamic>? body]) =>
      post('/auth/change-password', body);

  /// Record a consent decision
  Future<Map<String, dynamic>> postAuthConsent([Map<String, dynamic>? body]) =>
      post('/auth/consent', body);

  /// List linked sign-in providers
  Future<Map<String, dynamic>> getAuthIdentities() => get('/auth/identities');

  /// Unlink a provider
  Future<Map<String, dynamic>> deleteAuthIdentitiesByProvider(
          String provider) =>
      delete('/auth/identities/$provider');

  /// Link a provider to this account
  Future<Map<String, dynamic>> postAuthIdentitiesByProvider(String provider,
          [Map<String, dynamic>? body]) =>
      post('/auth/identities/$provider', body);

  /// Sign in; returns mfa_required when a second factor is enrolled
  Future<Map<String, dynamic>> postAuthLogin([Map<String, dynamic>? body]) =>
      post('/auth/login', body);

  /// End this session
  Future<Map<String, dynamic>> postAuthLogout([Map<String, dynamic>? body]) =>
      post('/auth/logout', body);

  /// End every session for this account
  Future<Map<String, dynamic>> postAuthLogoutAll(
          [Map<String, dynamic>? body]) =>
      post('/auth/logout-all', body);

  /// Rotate the session and issue a new access token
  Future<Map<String, dynamic>> postAuthRefresh([Map<String, dynamic>? body]) =>
      post('/auth/refresh', body);

  /// Create an account and send a verification email
  Future<Map<String, dynamic>> postAuthRegister([Map<String, dynamic>? body]) =>
      post('/auth/register', body);

  /// Request a password reset link
  Future<Map<String, dynamic>> postAuthRequestPasswordReset(
          [Map<String, dynamic>? body]) =>
      post('/auth/request-password-reset', body);

  /// Resend the verification email
  Future<Map<String, dynamic>> postAuthResendVerification(
          [Map<String, dynamic>? body]) =>
      post('/auth/resend-verification', body);

  /// Set a new password using a reset token
  Future<Map<String, dynamic>> postAuthResetPassword(
          [Map<String, dynamic>? body]) =>
      post('/auth/reset-password', body);

  /// Record a client-side security event
  Future<Map<String, dynamic>> postAuthSecurityEvents(
          [Map<String, dynamic>? body]) =>
      post('/auth/security-events', body);

  /// List live sign-ins
  Future<Map<String, dynamic>> getAuthSessions() => get('/auth/sessions');

  /// Sign out one device
  Future<Map<String, dynamic>> deleteAuthSessionsById(String id) =>
      delete('/auth/sessions/$id');

  /// Exchange a provider identity token for a session
  Future<Map<String, dynamic>> postAuthSocialByProvider(String provider,
          [Map<String, dynamic>? body]) =>
      post('/auth/social/$provider', body);

  /// Verify an email address with a one-time token
  Future<Map<String, dynamic>> postAuthVerifyEmail(
          [Map<String, dynamic>? body]) =>
      post('/auth/verify-email', body);

  // ---- content ----
  /// Published categories
  Future<Map<String, dynamic>> getCategories() => get('/categories');

  /// Confessions in a category
  Future<Map<String, dynamic>> getCategoriesByIdConfessions(String id) =>
      get('/categories/$id/confessions');

  /// Published collections
  Future<Map<String, dynamic>> getCollections() => get('/collections');

  /// One confession
  Future<Map<String, dynamic>> getConfessionsById(String id) =>
      get('/confessions/$id');

  /// Available voices
  Future<Map<String, dynamic>> getVoices() => get('/voices');

  // ---- devices ----
  /// List devices
  Future<Map<String, dynamic>> getMeDevices() => get('/me/devices');

  /// Register a device and its push token
  Future<Map<String, dynamic>> postMeDevices([Map<String, dynamic>? body]) =>
      post('/me/devices', body);

  /// Remove a device
  Future<Map<String, dynamic>> deleteMeDevicesById(String id) =>
      delete('/me/devices/$id');

  /// Notification preferences
  Future<Map<String, dynamic>> getMeNotifications() => get('/me/notifications');

  /// Update notification preferences
  Future<Map<String, dynamic>> patchMeNotifications(
          [Map<String, dynamic>? body]) =>
      patch('/me/notifications', body);

  // ---- downloads ----
  /// List offline licences
  Future<Map<String, dynamic>> getMeDownloads() => get('/me/downloads');

  /// Take a confession offline (premium)
  Future<Map<String, dynamic>> postMeDownloads([Map<String, dynamic>? body]) =>
      post('/me/downloads', body);

  /// Release an offline licence
  Future<Map<String, dynamic>> deleteMeDownloadsById(String id) =>
      delete('/me/downloads/$id');

  /// Renew an offline licence
  Future<Map<String, dynamic>> postMeDownloadsByIdRefresh(String id,
          [Map<String, dynamic>? body]) =>
      post('/me/downloads/$id/refresh', body);

  // ---- library ----
  /// List collections
  Future<Map<String, dynamic>> getMeCollections() => get('/me/collections');

  /// Create a collection (private by default)
  Future<Map<String, dynamic>> postMeCollections(
          [Map<String, dynamic>? body]) =>
      post('/me/collections', body);

  /// Delete a collection
  Future<Map<String, dynamic>> deleteMeCollectionsById(String id) =>
      delete('/me/collections/$id');

  /// Read a collection
  Future<Map<String, dynamic>> getMeCollectionsById(String id) =>
      get('/me/collections/$id');

  /// Update a collection
  Future<Map<String, dynamic>> patchMeCollectionsById(String id,
          [Map<String, dynamic>? body]) =>
      patch('/me/collections/$id', body);

  /// Add a confession to a collection
  Future<Map<String, dynamic>> postMeCollectionsByIdItems(String id,
          [Map<String, dynamic>? body]) =>
      post('/me/collections/$id/items', body);

  /// Remove an item
  Future<Map<String, dynamic>> deleteMeCollectionsByIdItemsByConfessionId(
          String id, String confessionId) =>
      delete('/me/collections/$id/items/$confessionId');

  /// Reorder items
  Future<Map<String, dynamic>> patchMeCollectionsByIdReorder(String id,
          [Map<String, dynamic>? body]) =>
      patch('/me/collections/$id/reorder', body);

  /// List personal confessions
  Future<Map<String, dynamic>> getMeConfessions() => get('/me/confessions');

  /// Create a personal confession
  Future<Map<String, dynamic>> postMeConfessions(
          [Map<String, dynamic>? body]) =>
      post('/me/confessions', body);

  /// Remove a favourite
  Future<Map<String, dynamic>> deleteMeFavorites() => delete('/me/favorites');

  /// List favourites
  Future<Map<String, dynamic>> getMeFavorites() => get('/me/favorites');

  /// Add a favourite
  Future<Map<String, dynamic>> postMeFavorites([Map<String, dynamic>? body]) =>
      post('/me/favorites', body);

  /// Listening history
  Future<Map<String, dynamic>> getMeHistory() => get('/me/history');

  /// Record playback
  Future<Map<String, dynamic>> postMeHistory([Map<String, dynamic>? body]) =>
      post('/me/history', body);

  // ---- mfa ----
  /// Two-factor status
  Future<Map<String, dynamic>> getAuthMfa() => get('/auth/mfa');

  /// Start enrolment; returns a server-generated secret
  Future<Map<String, dynamic>> postAuthMfaBegin([Map<String, dynamic>? body]) =>
      post('/auth/mfa/begin', body);

  /// Confirm enrolment and receive recovery codes
  Future<Map<String, dynamic>> postAuthMfaConfirm(
          [Map<String, dynamic>? body]) =>
      post('/auth/mfa/confirm', body);

  /// Disable two-factor authentication
  Future<Map<String, dynamic>> postAuthMfaDisable(
          [Map<String, dynamic>? body]) =>
      post('/auth/mfa/disable', body);

  // ---- profile ----
  /// Account summary
  Future<Map<String, dynamic>> getMe() => get('/me');

  /// Remove the avatar
  Future<Map<String, dynamic>> deleteMeAvatar() => delete('/me/avatar');

  /// Upload an avatar; EXIF is stripped
  Future<Map<String, dynamic>> postMeAvatar([Map<String, dynamic>? body]) =>
      post('/me/avatar', body);

  /// Everything needed to start the app
  Future<Map<String, dynamic>> getMeBootstrap() => get('/me/bootstrap');

  /// Interests, split into explicit and inferred
  Future<Map<String, dynamic>> getMeInterests() => get('/me/interests');

  /// Replace explicitly chosen interests
  Future<Map<String, dynamic>> putMeInterests([Map<String, dynamic>? body]) =>
      put('/me/interests', body);

  /// Read preferences
  Future<Map<String, dynamic>> getMePreferences() => get('/me/preferences');

  /// Update preferences
  Future<Map<String, dynamic>> patchMePreferences(
          [Map<String, dynamic>? body]) =>
      patch('/me/preferences', body);

  /// Read profile
  Future<Map<String, dynamic>> getMeProfile() => get('/me/profile');

  /// Update profile
  Future<Map<String, dynamic>> patchMeProfile([Map<String, dynamic>? body]) =>
      patch('/me/profile', body);

  // ---- schedules ----
  /// List schedules
  Future<Map<String, dynamic>> getSchedules() => get('/schedules');

  /// Create a schedule
  Future<Map<String, dynamic>> postSchedules([Map<String, dynamic>? body]) =>
      post('/schedules', body);

  /// Delete a schedule
  Future<Map<String, dynamic>> deleteSchedulesById(String id) =>
      delete('/schedules/$id');

  /// Update a schedule
  Future<Map<String, dynamic>> patchSchedulesById(String id,
          [Map<String, dynamic>? body]) =>
      patch('/schedules/$id', body);

  // ---- sessions ----
  /// List sessions
  Future<Map<String, dynamic>> getSessions() => get('/sessions');

  /// Compose a session; audio URLs are signed
  Future<Map<String, dynamic>> postSessions([Map<String, dynamic>? body]) =>
      post('/sessions', body);

  /// Read a session with freshly signed audio
  Future<Map<String, dynamic>> getSessionsById(String id) =>
      get('/sessions/$id');

  /// Update session status
  Future<Map<String, dynamic>> patchSessionsById(String id,
          [Map<String, dynamic>? body]) =>
      patch('/sessions/$id', body);
}
