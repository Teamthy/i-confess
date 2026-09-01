import 'dart:async';

import 'token_store.dart';

/// Platform keystore access, kept behind an interface (§24).
///
/// The Flutter app supplies an implementation backed by `flutter_secure_storage`
/// (iOS Keychain / Android EncryptedSharedPreferences). Keeping it abstract
/// means this package has no Flutter dependency and stays testable, and it
/// makes the security requirement explicit rather than incidental: whatever is
/// plugged in here must be hardware-backed storage, not a plain file.
abstract interface class SecureStorage {
  Future<String?> read(String key);
  Future<void> write(String key, String value);
  Future<void> delete(String key);
}

/// Token storage backed by the platform keystore.
///
/// Wraps [SecureStorage] with two things the raw keystore does not give you:
///
///  1. An in-memory cache. Keychain reads cost several milliseconds and the
///     token is needed on every request; without caching, a screen issuing a
///     handful of parallel calls pays that repeatedly for no benefit.
///  2. Fail-closed behaviour. If the keystore is unavailable — which happens on
///     Android before the device is unlocked after a reboot — the user is
///     treated as signed out rather than the app crashing on launch.
final class SecureTokenStore implements TokenStore {
  SecureTokenStore(this._storage, {this.onStorageFailure});

  final SecureStorage _storage;

  /// Reports a keystore failure so the app can tell the user something useful
  /// instead of silently behaving as if they never signed in.
  final void Function(Object error)? onStorageFailure;

  static const _tokenKey = 'iconfess.access_token';

  /// Cached value. `_loaded` distinguishes "not read yet" from "read, and
  /// there is no token" — without it a signed-out user would hit the keystore
  /// on every single request.
  String? _cached;
  bool _loaded = false;

  @override
  Future<String?> read() async {
    if (_loaded) return _cached;

    try {
      _cached = await _storage.read(_tokenKey);
    } on Object catch (e) {
      // A locked or corrupt keystore must not crash the app on launch.
      // Reporting signed-out is recoverable; a crash is not.
      onStorageFailure?.call(e);
      _cached = null;
    }
    _loaded = true;
    return _cached;
  }

  @override
  Future<void> save(String token) async {
    // Cache first so an in-flight request sees the new token even if the
    // keystore write is slow. A failed write costs the user a re-login on next
    // launch; a stale cache would break the session they are using right now.
    _cached = token;
    _loaded = true;
    try {
      await _storage.write(_tokenKey, token);
    } on Object catch (e) {
      onStorageFailure?.call(e);
    }
  }

  @override
  Future<void> clear() async {
    _cached = null;
    _loaded = true;
    try {
      await _storage.delete(_tokenKey);
    } on Object catch (e) {
      // Sign-out must always succeed locally. If the keystore delete fails the
      // token is still gone from memory, and the server session was revoked
      // separately — leaving the user "signed in" would be worse.
      onStorageFailure?.call(e);
    }
  }

  @override
  Future<bool> hasSession() async {
    final token = await read();
    return token != null && token.isNotEmpty;
  }
}
