import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:iconfess_api/iconfess_api.dart';

import '../../core/di/providers.dart';

/// One confession with its variants and scriptures, keyed by id.
///
/// Returns the client's [Loadable] so the screen can degrade independently:
/// a failed detail does not blank the category that linked to it.
final confessionProvider =
    FutureProvider.family<Loadable<Confession>, String>((ref, id) {
  return ref.watch(contentRepositoryProvider).confession(id);
});

/// Whether the confession is favourited.
///
/// A separate provider so the favourite button can update without reloading the
/// whole confession body. The list is small and not cached beyond this call.
final confessionIsFavoriteProvider =
    FutureProvider.family<Loadable<bool>, String>((ref, id) {
  return ref.watch(contentRepositoryProvider).isFavorite(id);
});
