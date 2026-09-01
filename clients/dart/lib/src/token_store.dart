import 'dart:async';

/// Where the access token lives.
///
/// Deliberately an interface. A token is a bearer credential, so on device it
/// must go to the iOS Keychain or Android Keystore — never SharedPreferences,
/// which is world-readable on a rooted phone and survives in plaintext
/// backups. Keeping storage behind this contract means the transport layer
/// cannot accidentally be wired to an insecure implementation, and tests can
/// use an in-memory one without pulling in platform channels.
abstract interface class TokenStore {
  /// Returns the current token, or null when signed out.
  Future<String?> read();

  /// Persists a token.
  Future<void> save(String token);

  /// Removes the token. Called on sign-out and on unrecoverable auth failure.
  Future<void> clear();

  /// Whether a session exists. Separate from [read] so the transport can
  /// decide whether a 401 is worth refreshing without loading the secret.
  Future<bool> hasSession();
}

/// In-memory store for tests and for previewing without a device keystore.
///
/// Never use in a shipped app: the token is lost on restart, and more
/// importantly it sets the expectation that plain storage is acceptable.
final class InMemoryTokenStore implements TokenStore {
  String? _token;

  @override
  Future<String?> read() async => _token;

  @override
  Future<void> save(String token) async => _token = token;

  @override
  Future<void> clear() async => _token = null;

  @override
  Future<bool> hasSession() async => _token != null && _token!.isNotEmpty;
}
