import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:iconfess_api/iconfess_api.dart';

import '../../core/di/providers.dart';

final plansProvider = FutureProvider<Loadable<List<Plan>>>((ref) {
  return ref.watch(subscriptionRepositoryProvider).plans();
});

final subscriptionProvider = FutureProvider<Loadable<Subscription>>((ref) {
  return ref.watch(subscriptionRepositoryProvider).subscription();
});

final entitlementsProvider = FutureProvider<Loadable<Entitlements>>((ref) {
  return ref.watch(subscriptionRepositoryProvider).entitlements();
});

final trialProvider = FutureProvider<Loadable<List<TrialDay>>>((ref) {
  return ref.watch(subscriptionRepositoryProvider).trial();
});
