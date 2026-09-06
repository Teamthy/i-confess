import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:iconfess_api/iconfess_api.dart';

import '../../core/di/providers.dart';

/// Explore's catalogue reads. Each returns the client's [Loadable] so a failed
/// rail degrades on its own instead of blanking the screen.

final exploreCategoriesProvider =
    FutureProvider<Loadable<List<Category>>>((ref) {
  return ref.watch(contentRepositoryProvider).categories();
});

/// Published collections for the featured rail.
final exploreCollectionsProvider =
    FutureProvider<Loadable<List<Collection>>>((ref) {
  return ref.watch(contentRepositoryProvider).collections();
});

/// Confessions in one category, keyed so each detail screen reads its own.
final categoryConfessionsProvider = FutureProvider.family<
    Loadable<List<Confession>>, String>((ref, id) {
  return ref.watch(contentRepositoryProvider).categoryConfessions(id);
});
