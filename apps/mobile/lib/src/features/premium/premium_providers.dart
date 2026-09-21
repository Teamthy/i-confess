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

/// The measured journey: which days were actually completed.
///
/// Kept separate from [trialProvider] on purpose. The journey is the same for
/// everyone and can be cached; the completion count is this listener's
/// progress and is the evidence the paywall shows, so it is fetched fresh.
final trialEngagementProvider = FutureProvider<Loadable<TrialEngagement>>((ref) {
  return ref.watch(subscriptionRepositoryProvider).trialEngagement();
});
