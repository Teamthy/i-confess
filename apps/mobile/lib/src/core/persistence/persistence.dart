import 'dart:convert';

import 'package:flutter_secure_storage/flutter_secure_storage.dart';
import 'package:iconfess_api/iconfess_api.dart';
import 'package:shared_preferences/shared_preferences.dart';

/// Non-secret persistence.
///
/// An interface for the same reason the API client's [TokenStore] is one: a
/// widget or a notifier should not know it is talking to SharedPreferences, and
/// a test should not need a platform channel.
abstract interface class KeyValueStore {
  Future<String?> readString(String key);
  Future<void> writeString(String key, String value);
  Future<void> remove(String key);

}

/// JSON helpers as an extension rather than interface members.
///
/// An `interface` class does not pass implementations to its implementers, so
/// declaring these with bodies would have forced every implementation to
/// reimplement them. As an extension they are available on any [KeyValueStore],
/// including a test fake that only implements the three primitives.
extension KeyValueStoreJson on KeyValueStore {
  /// Reads and decodes JSON, returning null when absent or unreadable.
  ///
  /// Unreadable is folded into absent rather than thrown: a corrupt cache entry
  /// should cost the listener a refetch, not a crash on launch.
  Future<Map<String, Object?>?> readJson(String key) async {
    final raw = await readString(key);
    if (raw == null || raw.isEmpty) return null;
    try {
      final decoded = jsonDecode(raw);
      return decoded is Map<String, Object?> ? decoded : null;
    } on FormatException {
      await remove(key);
      return null;
    }
  }

  Future<void> writeJson(String key, Map<String, Object?> value) =>
      writeString(key, jsonEncode(value));
}

/// SharedPreferences-backed store.
final class PreferencesKeyValueStore implements KeyValueStore {
  PreferencesKeyValueStore(this._prefs);

  final SharedPreferences _prefs;

  /// Everything this store writes is namespaced, so a key cannot collide with
  /// one written by a plugin.
  static String _k(String key) => 'iconfess.$key';

  @override
  Future<String?> readString(String key) async => _prefs.getString(_k(key));

  @override
  Future<void> writeString(String key, String value) => _prefs.setString(_k(key), value);

  @override
  Future<void> remove(String key) => _prefs.remove(_k(key));
}

/// In-memory store for tests.
final class InMemoryKeyValueStore implements KeyValueStore {
  final Map<String, String> _data = {};

  /// Set to make the next operation fail, so error paths can be exercised.
  bool failNext = false;

  @override
  Future<String?> readString(String key) async {
    if (failNext) throw StateError('store unavailable');
    return _data[key];
  }

  @override
  Future<void> writeString(String key, String value) async {
    if (failNext) throw StateError('store unavailable');
    _data[key] = value;
  }

  @override
  Future<void> remove(String key) async => _data.remove(key);

  Map<String, String> get snapshot => Map.unmodifiable(_data);
}

/// Keys the app owns.
///
/// Centralised because a key typed twice in two files is a bug that reads as a
/// cache miss, which is the hardest kind to notice.
abstract final class StoreKeys {
  static const onboardingSeen = 'onboarding.seen';
  static const appMode = 'appearance.mode';
  static const lastCategoryIds = 'builder.categories';
  static const lastDuration = 'builder.duration';
  static const lastVoiceId = 'builder.voice';
  static const analyticsConsent = 'analytics.consent';
}

/// Keystore adapter for [SecureStorage], which the API client requires.
///
/// The token is a bearer credential. On device it goes to the iOS Keychain or
/// Android EncryptedSharedPreferences — never SharedPreferences, which is
/// world-readable on a rooted phone and survives in plaintext backups.
final class KeystoreSecureStorage implements SecureStorage {
  KeystoreSecureStorage({FlutterSecureStorage? storage})
      : _storage = storage ??
            const FlutterSecureStorage(
              // v11 defaults to AES-GCM storage with an RSA-OAEP wrapped key,
              // which is the hardware-backed configuration we want; there is no
              // weaker mode to opt out of.
              aOptions: AndroidOptions(),
              iOptions: IOSOptions(
                // Available after the first unlock of this boot, and never
                // migrated to a new device in a backup. A token that outlives the
                // device it was issued to is a token that can be replayed.
                accessibility: KeychainAccessibility.first_unlock_this_device,
              ),
            );

  final FlutterSecureStorage _storage;

  @override
  Future<String?> read(String key) => _storage.read(key: key);

  @override
  Future<void> write(String key, String value) => _storage.write(key: key, value: value);

  @override
  Future<void> delete(String key) => _storage.delete(key: key);
}
