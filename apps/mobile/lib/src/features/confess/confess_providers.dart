import 'package:flutter_riverpod/legacy.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:iconfess_api/iconfess_api.dart';

import '../../core/di/providers.dart';

/// Categories for the builder's first step: what to speak over your life.
///
/// The builder's first step is category selection (§12, confess tab). The
/// categories are server-owned, never hard-coded, and come from the same
/// cache as Explore so a fresh category appears everywhere at once.
final confessCategoriesProvider =
    FutureProvider<Loadable<List<Category>>>((ref) {
  return ref.watch(contentRepositoryProvider).categories();
});

/// The set of category ids the listener has chosen for the next session.
///
/// Held locally until the session is created (PHASE 23). Persisting it
/// server-side before the user has confirmed would create orphaned drafts.
final selectedCategoriesProvider =
    StateProvider<Set<String>>((ref) => <String>{});
