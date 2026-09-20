import 'package:flutter_riverpod/legacy.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:iconfess_api/iconfess_api.dart';

import '../../core/di/providers.dart';

/// Search query state.
final searchQueryProvider = StateProvider<String>((ref) => '');

/// Selected search types.
final searchTypesProvider = StateProvider<List<String>>((ref) => ['confession', 'category', 'collection', 'voice']);

/// Search results, debounced by UI.
final searchResultsProvider = FutureProvider<Loadable<List<SearchResult>>>((ref) async {
  final query = ref.watch(searchQueryProvider);
  final types = ref.watch(searchTypesProvider);
  if (query.trim().isEmpty) {
    return const Loadable.loaded(<SearchResult>[]);
  }
  return ref.watch(searchRepositoryProvider).search(query, types: types);
});
