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

/// The §36 record: where this account stands in its trial (G-3).
///
/// Separate from `trialProvider` on purpose. The journey list is the same
/// seven days for everyone and is cacheable; the lifecycle answer is
/// per-account and time-sensitive — "expiring" that arrived five minutes
/// stale would be a lie about the only deadline the paywall shows.
final trialLifecycleProvider = FutureProvider<Loadable<Trial>>((ref) {
  return ref.watch(subscriptionRepositoryProvider).trialLifecycle();
});
