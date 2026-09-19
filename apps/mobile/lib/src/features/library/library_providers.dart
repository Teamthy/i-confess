import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:iconfess_api/iconfess_api.dart';

import '../../core/di/providers.dart';

final libraryCollectionsProvider = FutureProvider<Loadable<List<UserCollection>>>((ref) {
  return ref.watch(libraryRepositoryProvider).collections();
});

final libraryFavoritesProvider = FutureProvider<Loadable<List<dynamic>>>((ref) async {
  try {
    final json = await ref.watch(apiClientProvider).getMeFavorites();
    final data = json['data'] is List ? json['data'] as List : [];
    return Loadable.loaded(data);
  } catch (e) {
    return Loadable.failed(e as dynamic);
  }
});

final libraryConfessionsProvider = FutureProvider<Loadable<List<Confession>>>((ref) async {
  try {
    final json = await ref.watch(apiClientProvider).getMeConfessions();
    final list = json['data'] is List
        ? (json['data'] as List).whereType<Map<String, dynamic>>().map(Confession.fromJson).toList()
        : <Confession>[];
    return Loadable.loaded(list);
  } catch (e) {
    return Loadable.failed(e as dynamic);
  }
});

final collectionDetailProvider = FutureProvider.family<Loadable<Map<String, dynamic>>, String>((ref, id) async {
  try {
    final json = await ref.watch(apiClientProvider).getMeCollectionsById(id);
    return Loadable.loaded(json);
  } catch (e) {
    return Loadable.failed(e as dynamic);
  }
});
