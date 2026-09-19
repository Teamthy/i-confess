import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:iconfess_api/iconfess_api.dart';

import '../../core/di/providers.dart';

final bootstrapProvider = FutureProvider<Loadable<Bootstrap>>((ref) {
  return ref.watch(profileRepositoryProvider).bootstrap();
});

final profileProvider = FutureProvider<Loadable<Profile>>((ref) {
  return ref.watch(profileRepositoryProvider).profile();
});

final preferencesProvider = FutureProvider<Loadable<Preferences>>((ref) {
  return ref.watch(profileRepositoryProvider).preferences();
});

final interestsProvider = FutureProvider<Loadable<Interests>>((ref) {
  return ref.watch(profileRepositoryProvider).interests();
});

final authSessionsProvider = FutureProvider<Loadable<List<AuthSession>>>((ref) {
  return ref.watch(authRepositoryProvider).sessions();
});
