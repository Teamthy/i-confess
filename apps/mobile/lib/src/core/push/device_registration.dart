import 'dart:math';

import 'package:flutter/foundation.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:iconfess_api/iconfess_api.dart';

import '../di/providers.dart';
import '../persistence/persistence.dart';
import 'platform_device_details.dart';

/// DevicePushRegistration manages registering the device and push token with the backend (IC-012).
class DevicePushRegistration {
  const DevicePushRegistration(this._apiClient);

  final ApiClient _apiClient;

  /// Posts the device, its platform and its push token.
  ///
  /// Returns whether the server accepted it. The caller logs the failure and
  /// carries on: a registration that did not reach the server costs the user
  /// reminders until the next attempt, and nothing else in the app depends on
  /// it, so throwing here would turn a transient network fault into a crash.
  Future<bool> registerDevice({
    required String deviceId,
    required String platform,
    required String pushToken,
  }) async {
    try {
      await _apiClient.postMeDevices({
        'device_id': deviceId,
        'platform': platform,
        'push_token': pushToken,
      });
      debugPrint('push: registered device $deviceId on $platform');
      return true;
    } catch (e) {
      debugPrint('push: failed to register device token: $e');
      return false;
    }
  }
}

/// The identity a device reports to the server.
@immutable
class DeviceIdentity {
  const DeviceIdentity({required this.deviceId, required this.platform});

  /// Stable for the life of the installation. Not a hardware serial: it is the
  /// platform's own app-scoped identifier where one exists, and a random value
  /// kept in local storage where it does not.
  final String deviceId;

  /// `ios`, `android` or `web` — the values the server's device table accepts.
  final String platform;
}

/// Resolves this device's identity, once, and remembers it.
///
/// It is persisted rather than recomputed because the underlying identifiers
/// change: Android's ANDROID_ID changes on a factory reset or when the app's
/// signing key changes, and iOS's `identifierForVendor` changes when the last
/// app from a vendor is uninstalled. A device that changed its id overnight
/// would appear to the server as a second device, accumulate a second row, and
/// receive every reminder twice until somebody cleaned up.
class DeviceIdentities {
  DeviceIdentities({required KeyValueStore store, required DeviceDetails details})
      : _store = store,
        _details = details;

  final KeyValueStore _store;
  final DeviceDetails _details;

  static const _deviceIdKey = StoreKeys.pushDeviceId;

  DeviceIdentity? _cached;
  Future<DeviceIdentity>? _loading;

  Future<DeviceIdentity> current() =>
      _loading ??= _resolve().whenComplete(() { _loading = null; });

  Future<DeviceIdentity> _resolve() async {
    final cached = _cached;
    if (cached != null) return cached;

    final stored = await _store.readString(_deviceIdKey);
    if (stored != null && stored.isNotEmpty) {
      return _cached = DeviceIdentity(deviceId: stored, platform: _details.platform);
    }

    final deviceId = await _details.nativeDeviceId() ?? _randomDeviceId();
    await _store.writeString(_deviceIdKey, deviceId);
    return _cached = DeviceIdentity(deviceId: deviceId, platform: _details.platform);
  }

  /// A locally generated identifier, for platforms with nothing better.
  static String _randomDeviceId() {
    final random = Random.secure();
    final bytes = List<int>.generate(16, (_) => random.nextInt(256));
    return bytes.map((b) => b.toRadixString(16).padLeft(2, '0')).join();
  }
}

final deviceIdentitiesProvider = Provider<DeviceIdentities>(
  (ref) => DeviceIdentities(
    store: ref.watch(keyValueStoreProvider),
    details: ref.watch(deviceDetailsProvider),
  ),
);

/// The real [DeviceDetails], reading the platform through `device_info_plus`.
///
/// Lives here rather than in the push transport because the identity is needed
/// by anything that talks about this device, not only by push.
final deviceDetailsProvider = Provider<DeviceDetails>((ref) => const PlatformDeviceDetails());
