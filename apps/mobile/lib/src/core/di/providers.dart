import 'package:flutter/foundation.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:iconfess_api/iconfess_api.dart';
import 'package:shared_preferences/shared_preferences.dart';

import '../../features/auth/auth_controller.dart';
import '../analytics/analytics.dart';
import '../persistence/persistence.dart';

/// Composition root.
///
/// Everything a feature needs is resolved here, so a widget never constructs a
/// client, a cache or a store. That is what makes a screen testable: overriding
/// one provider replaces the whole dependency chain behind it.
///
/// Nothing in this file reads `SharedPreferences` or the keystore at import
/// time. Startup work happens inside the providers, so constructing the
/// [ProviderScope] in a test does not touch a platform channel.

/// The API base URL. Overridden per environment and in tests.
final apiBaseUrlProvider = Provider<String>((ref) {
  const fromEnv = String.fromEnvironment('ICONFESS_API_URL');
  return fromEnv.isEmpty ? 'https://api.i-confess.app' : fromEnv;
});

/// Non-secret storage. Async because SharedPreferences hands back a future.
final sharedPreferencesProvider = Provider<SharedPreferences>((ref) {
  throw UnimplementedError(
    'sharedPreferencesProvider must be overridden at startup with the resolved instance',
  );
});

final keyValueStoreProvider = Provider<KeyValueStore>(
  (ref) => PreferencesKeyValueStore(ref.watch(sharedPreferencesProvider)),
);

/// The keystore. Overridden in tests with an in-memory [SecureStorage].
final secureStorageProvider = Provider<SecureStorage>((ref) => KeystoreSecureStorage());

/// Where the bearer token lives.
///
/// [SecureTokenStore] is the client's own wrapper: it caches the token in memory
/// so a screen issuing parallel calls does not pay for a Keychain read each
/// time, and it fails closed — a locked keystore after a reboot reports
/// "signed out" rather than crashing the app on launch.
final tokenStoreProvider = Provider<TokenStore>((ref) {
  final store = SecureTokenStore(
    ref.watch(secureStorageProvider),
    onStorageFailure: (error) => debugPrint('keystore unavailable: $error'),
  );
  ref.onDispose(store.clear);
  return store;
});

/// Reports that the session was lost server-side, so the app can clear local
/// state and let the router's redirect handle it — rather than leaving the
/// listener staring at requests that all return 401.
final authLossListenerProvider = Provider<void Function(AuthLossReason reason)>((ref) {
  return (reason) {
    debugPrint('auth lost: $reason');
    ref.read(authControllerProvider.notifier).onAuthenticationLost(reason);
  };
});

final apiClientProvider = Provider<ApiClient>((ref) {
  return ApiClient(
    baseUrl: ref.watch(apiBaseUrlProvider),
    tokens: ref.watch(tokenStoreProvider),
    onAuthenticationLost: ref.watch(authLossListenerProvider),
  );
});

/// The client's own JSON cache. Kept in memory: it is a latency optimisation for
/// a session, not a persistence layer, and rebuilding it on launch is cheaper
/// than invalidating it correctly across a token change.
final jsonCacheProvider = Provider<JsonCache>((ref) => JsonCache(InMemoryCache()));

final authRepositoryProvider = Provider<AuthRepository>(
  (ref) => AuthRepository(ref.watch(apiClientProvider), ref.watch(jsonCacheProvider)),
);

final profileRepositoryProvider = Provider<ProfileRepository>(
  (ref) => ProfileRepository(ref.watch(apiClientProvider), ref.watch(jsonCacheProvider)),
);

final contentRepositoryProvider = Provider<ContentRepository>(
  (ref) => ContentRepository(ref.watch(apiClientProvider), ref.watch(jsonCacheProvider)),
);

final libraryRepositoryProvider = Provider<LibraryRepository>(
  (ref) => LibraryRepository(ref.watch(apiClientProvider), ref.watch(jsonCacheProvider)),
);

/// Analytics. Debug builds get the recording implementation; release gets a
/// no-op until a transport is chosen.
///
/// The release default is deliberately inert rather than the debug one: shipping
/// a `debugPrint` per event in a release build is both a performance cost and a
/// leak surface, and silently discarding events is honest about an integration
/// that has not been wired yet.
final analyticsProvider = Provider<Analytics>((ref) {
  return kDebugMode ? DebugAnalytics() : const NoopAnalytics();
});
