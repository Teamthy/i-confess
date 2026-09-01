import 'dart:async';
import 'dart:convert';

/// Local persistence for offline reads (§102).
///
/// Deliberately an interface with no shipped default. The Flutter app supplies
/// a file- or database-backed implementation; keeping it abstract means this
/// package stays free of platform dependencies and testable.
///
/// Nothing sensitive belongs in here. This cache exists so a user on the
/// underground can see their library, not to duplicate the keystore — tokens
/// go to [SecureTokenStore], and cached payloads should be treated as readable
/// by anyone with the device.
abstract interface class LocalCache {
  Future<String?> read(String key);
  Future<void> write(String key, String value);
  Future<void> delete(String key);
  Future<void> clear();
}

/// In-memory cache for tests and for a first run before storage is wired.
final class InMemoryCache implements LocalCache {
  final Map<String, String> _entries = {};

  @override
  Future<String?> read(String key) async => _entries[key];

  @override
  Future<void> write(String key, String value) async => _entries[key] = value;

  @override
  Future<void> delete(String key) async => _entries.remove(key);

  @override
  Future<void> clear() async => _entries.clear();
}

/// A cached value with the time it was stored.
final class CachedEntry<T> {
  const CachedEntry({required this.value, required this.storedAt});

  final T value;
  final DateTime storedAt;

  Duration ageAt(DateTime now) => now.difference(storedAt);
}

/// Reads and writes typed JSON through a [LocalCache].
///
/// The age is stored alongside the payload rather than inferred from file
/// mtime: mtime is unreliable across backup and restore, and a value restored
/// from a months-old backup must not appear fresh.
final class JsonCache {
  const JsonCache(this._cache, {this.now = DateTime.now});

  final LocalCache _cache;
  final DateTime Function() now;

  Future<CachedEntry<T>?> read<T>(
    String key,
    T Function(Map<String, dynamic>) decode,
  ) async {
    final raw = await _cache.read(key);
    if (raw == null || raw.isEmpty) return null;

    try {
      final decoded = jsonDecode(raw);
      if (decoded is! Map<String, dynamic>) return null;

      final storedAt = DateTime.tryParse(decoded['_cached_at'] as String? ?? '');
      final payload = decoded['payload'];
      if (storedAt == null || payload is! Map<String, dynamic>) return null;

      return CachedEntry(value: decode(payload), storedAt: storedAt);
    } on Object {
      // A cache that cannot be parsed is a cache miss, never a crash. Corrupt
      // entries happen — interrupted writes, schema changes between app
      // versions — and none of them should stop the app starting.
      await _cache.delete(key);
      return null;
    }
  }

  Future<void> write(String key, Map<String, dynamic> payload) async {
    final envelope = jsonEncode({
      '_cached_at': now().toIso8601String(),
      'payload': payload,
    });
    try {
      await _cache.write(key, envelope);
    } on Object {
      // A failed cache write must never fail the request that produced the
      // data. The user got their answer; losing the offline copy is a
      // degradation, not an error.
    }
  }

  Future<void> clear() => _cache.clear();
}

/// Cache keys, kept in one place so a typo cannot silently create a second
/// cache that never gets invalidated.
abstract final class CacheKeys {
  static const bootstrap = 'bootstrap';
  static const profile = 'profile';
  static const preferences = 'preferences';
  static const categories = 'categories';
  static const voices = 'voices';
  static const collections = 'collections';
  static const downloads = 'downloads';
  static const schedules = 'schedules';
}

/// How long cached data may be shown before it is considered stale.
///
/// Staleness does not mean "unusable" — it means the UI should say the value
/// may be out of date. Reference data changes rarely and is cached for longer;
/// anything reflecting entitlement is kept short, because showing a lapsed
/// subscriber as premium is worse than showing a spinner.
abstract final class CacheTtl {
  static const bootstrap = Duration(hours: 1);
  static const profile = Duration(hours: 6);
  static const preferences = Duration(hours: 6);
  static const categories = Duration(days: 1);
  static const voices = Duration(days: 1);
  static const library = Duration(minutes: 30);
}
