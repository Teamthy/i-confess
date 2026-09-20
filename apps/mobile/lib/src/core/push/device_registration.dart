import 'package:flutter/foundation.dart';
import 'package:iconfess_api/iconfess_api.dart';

/// DevicePushRegistration manages registering the device and push token with the backend (IC-012).
class DevicePushRegistration {
  const DevicePushRegistration(this._apiClient);

  final ApiClient _apiClient;

  Future<void> registerDevice({
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
    } catch (e) {
      debugPrint('push: failed to register device token: $e');
    }
  }
}
