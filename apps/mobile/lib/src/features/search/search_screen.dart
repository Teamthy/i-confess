import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../core/routing/routes.dart';
import '../../core/theme/theme.dart';
import '../../core/theme/tokens.dart';
import '../../core/widgets/screen.dart';
import 'search_providers.dart';

/// Search screen: full-text search across confessions, categories, collections, voices.
///
/// Uses server search endpoint (LIKE-based MVP, future full-text).
class SearchScreen extends ConsumerStatefulWidget {
  const SearchScreen({super.key});

  @override
  ConsumerState<SearchScreen> createState() => _SearchScreenState();
}

class _SearchScreenState extends ConsumerState<SearchScreen> {
  final _controller = TextEditingController();

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final resultsAsync = ref.watch(searchResultsProvider);
    final surfaces = AppSurfaces.of(context);

    return AppScaffold(
      title: 'Search',
      body: Column(
        children: [
          TextField(
            controller: _controller,
            autofocus: true,
            decoration: InputDecoration(
              hintText: 'Search confessions, categories, voices...',
              prefixIcon: const Icon(Icons.search_rounded),
              suffixIcon: _controller.text.isNotEmpty
                  ? IconButton(
                      icon: const Icon(Icons.clear_rounded),
                      onPressed: () {
                        _controller.clear();
                        ref.read(searchQueryProvider.notifier).state = '';
                      },
                    )
                  : null,
              filled: true,
              fillColor: surfaces.surfaceRaised,
              border: OutlineInputBorder(
                borderRadius: BorderRadius.circular(IConfess.radiusFull),
                borderSide: BorderSide.none,
              ),
            ),
            onChanged: (v) => ref.read(searchQueryProvider.notifier).state = v,
          ),
          const SizedBox(height: IConfess.space3),
          // Type chips
          Wrap(
            spacing: IConfess.space2,
            children: [
              for (final type in ['confession', 'category', 'collection', 'voice'])
                FilterChip(
                  label: Text(type),
                  selected: ref.watch(searchTypesProvider).contains(type),
                  onSelected: (selected) {
                    final current = ref.read(searchTypesProvider);
                    if (selected) {
                      ref.read(searchTypesProvider.notifier).state = [...current, type];
                    } else {
                      ref.read(searchTypesProvider.notifier).state =
                          current.where((t) => t != type).toList();
                    }
                  },
                ),
            ],
          ),
          const SizedBox(height: IConfess.space5),
          Expanded(
            child: resultsAsync.when(
              loading: () => const Center(child: CircularProgressIndicator()),
              error: (e, _) => Center(child: Text('Search failed: $e')),
              data: (loadable) {
                final results = loadable.valueOrNull ?? [];
                if (_controller.text.isEmpty) {
                  return Center(
                    child: Column(
                      mainAxisAlignment: MainAxisAlignment.center,
                      children: [
                        Icon(Icons.search_rounded, size: 48, color: surfaces.textSecondary),
                        const SizedBox(height: IConfess.space3),
                        Text('Find what to speak over your life',
                            style: IConfess.body.copyWith(color: surfaces.textPrimary)),
                        Text('Try \"healing\", \"peace\", \"finance\"',
                            style: IConfess.bodySm.copyWith(color: surfaces.textSecondary)),
                      ],
                    ),
                  );
                }
                if (results.isEmpty) {
                  return Center(
                    child: Text('No results for \"${_controller.text}\"',
                        style: IConfess.body.copyWith(color: surfaces.textSecondary)),
                  );
                }
                return ListView.separated(
                  itemCount: results.length,
                  separatorBuilder: (_, _) => const Divider(height: 1),
                  itemBuilder: (context, i) {
                    final r = results[i];
                    return ListTile(
                      leading: _iconForType(r.type),
                      title: Text(r.title),
                      subtitle: Text(r.description, maxLines: 2, overflow: TextOverflow.ellipsis),
                      trailing: Text(r.type, style: IConfess.caption.copyWith(color: surfaces.textSecondary)),
                      onTap: () {
                        if (r.type == 'confession') {
                          context.go(AppRoutes.confessionDetail(r.id));
                        } else if (r.type == 'category') {
                          context.go(AppRoutes.categoryDetail(r.id));
                        }
                      },
                    );
                  },
                );
              },
            ),
          ),
        ],
      ),
    );
  }

  Widget _iconForType(String type) {
    switch (type) {
      case 'confession':
        return const Icon(Icons.menu_book_rounded);
      case 'category':
        return const Icon(Icons.category_rounded);
      case 'collection':
        return const Icon(Icons.collections_rounded);
      case 'voice':
        return const Icon(Icons.record_voice_over_rounded);
      default:
        return const Icon(Icons.search_rounded);
    }
  }
}
