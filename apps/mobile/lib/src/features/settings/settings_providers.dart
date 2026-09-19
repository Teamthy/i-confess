import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:iconfess_api/iconfess_api.dart';

import '../../core/di/providers.dart';

final preferencesProvider = FutureProvider<Loadable<Preferences>>((ref) {
  return ref.watch(profileRepositoryProvider).preferences();
});

final notificationPrefsProvider = FutureProvider<Loadable<Map<String, dynamic>>>((ref) async {
  try {
    final json = await ref.watch(apiClientProvider).getMeNotifications();
    return Loadable.loaded(json);
  } catch (e) {
    return Loadable.failed(e as dynamic);
  }
});

final devicesProvider = FutureProvider<Loadable<List<dynamic>>>((ref) async {
  try {
    final json = await ref.watch(apiClientProvider).getMeDevices();
    final data = json['data'] is List ? json['data'] as List : [];
    return Loadable.loaded(data);
  } catch (e) {
    return Loadable.failed(e as dynamic);
  }
});
