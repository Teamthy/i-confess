import 'dart:io';

import 'package:device_info_plus/device_info_plus.dart';
import 'package:flutter/foundation.dart';



/// [DeviceDetails] backed by `device_info_plus`.
///
/// Every read is defensive. These are platform channels: on a simulator, in a
/// preview build, or after an OS upgrade that renamed a field, they can throw —
/// and a device that cannot name itself must still be able to register a push
/// token, because the alternative is no reminders at all for that user.
class PlatformDeviceDetails implements DeviceDetails {
  const PlatformDeviceDetails();

  @override
  String get platform {
    if (kIsWeb) return 'web';
    if (Platform.isIOS) return 'ios';
    if (Platform.isAndroid) return 'android';
    return 'unknown';
  }

  @override
  Future<String?> nativeDeviceId() async {
    if (kIsWeb) return null;
    final info = DeviceInfoPlugin();
    try {
      if (Platform.isIOS) {
        // identifierForVendor is app-scoped and stable while any app from the
        // same vendor is installed - which is what the App Store guidelines
        // permit, and enough to key a device row.
        return (await info.iosInfo).identifierForVendor;
      }
      if (Platform.isAndroid) {
        // AndroidDeviceInfo.id is a build identifier shared by many phones,
        // not ANDROID_ID. Use the persisted random installation id instead.
        return null;
      }
    } catch (error) {
      debugPrint('push: could not read the platform device id: $error');
    }
    return null;
  }
}

/// The platform facts a device identity is built from.
///
/// Behind an interface because the real implementation reads a plugin, and a
/// plugin is a platform channel that a unit test cannot answer.
abstract interface class DeviceDetails {
  /// The platform's app-scoped device identifier, or null when it has none or
  /// cannot be read.
  Future<String?> nativeDeviceId();

  /// The platform name the server expects.
  String get platform;
}
