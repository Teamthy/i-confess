import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../core/theme/theme.dart';
import '../../core/theme/tokens.dart';
import '../../core/widgets/screen.dart';
import 'library_providers.dart';

/// Library: collections, favorites, personal confessions.
class LibraryScreen extends ConsumerWidget {
  const LibraryScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final collectionsAsync = ref.watch(libraryCollectionsProvider);
    final favoritesAsync = ref.watch(libraryFavoritesProvider);
    final surfaces = AppSurfaces.of(context);

    return AppScaffold(
      title: 'Library',
      body: DefaultTabController(
        length: 3,
        child: Column(
          children: [
            TabBar(
              labelColor: IConfess.colorBrand600,
              unselectedLabelColor: surfaces.textSecondary,
              indicatorColor: IConfess.colorBrand600,
              tabs: const [
                Tab(text: 'Collections'),
                Tab(text: 'Favorites'),
                Tab(text: 'My Confessions'),
              ],
            ),
            const SizedBox(height: IConfess.space3),
            Expanded(
              child: TabBarView(
                children: [
                  _CollectionsTab(async: collectionsAsync),
                  _FavoritesTab(async: favoritesAsync),
                  _MyConfessionsTab(),
                ],
              ),
            ),
          ],
        ),
      ),
    );
  }
}

class _CollectionsTab extends StatelessWidget {
  const _CollectionsTab({required this.async});
  final AsyncValue async;

  @override
  Widget build(BuildContext context) {
    return async.when(
      loading: () => const Center(child: CircularProgressIndicator()),
      error: (e, _) => Center(child: Text('Failed: $e')),
      data: (loadable) {
        final collections = loadable.valueOrNull ?? [];
        if (collections.isEmpty) {
          return Center(
            child: Column(
              mainAxisAlignment: MainAxisAlignment.center,
              children: [
                Icon(Icons.collections_bookmark_rounded, size: 48, color: AppSurfaces.of(context).textSecondary),
                const SizedBox(height: IConfess.space3),
                Text('No collections yet'),
                const SizedBox(height: IConfess.space2),
                FilledButton(
                  onPressed: () {},
                  child: const Text('Create collection'),
                ),
              ],
            ),
          );
        }
        return ListView.separated(
          itemCount: collections.length,
          separatorBuilder: (_, _) => const SizedBox(height: IConfess.space2),
          itemBuilder: (context, i) {
            final c = collections[i] as dynamic;
            final name = c.name ?? 'Collection';
            return ListTile(
              title: Text(name),
              subtitle: Text('${c.itemCount ?? 0} items • ${c.visibility ?? 'private'}'),
              trailing: const Icon(Icons.chevron_right_rounded),
              onTap: () => context.go('/library/collection/${c.id}'),
            );
          },
        );
      },
    );
  }
}

class _FavoritesTab extends StatelessWidget {
  const _FavoritesTab({required this.async});
  final AsyncValue async;

  @override
  Widget build(BuildContext context) {
    return async.when(
      loading: () => const Center(child: CircularProgressIndicator()),
      error: (e, _) => Center(child: Text('Failed: $e')),
      data: (loadable) {
        final favs = loadable.valueOrNull ?? [];
        if (favs.isEmpty) {
          return Center(child: Text('No favorites yet — tap ♥ on a confession'));
        }
        return ListView.builder(
          itemCount: favs.length,
          itemBuilder: (context, i) {
            final f = favs[i] as Map;
            return ListTile(
              leading: const Icon(Icons.favorite_rounded, color: Colors.red),
              title: Text(f['entity_id']?.toString() ?? 'Favorite'),
              subtitle: Text(f['entity_type']?.toString() ?? ''),
            );
          },
        );
      },
    );
  }
}

class _MyConfessionsTab extends ConsumerWidget {
  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final async = ref.watch(libraryConfessionsProvider);
    return async.when(
      loading: () => const Center(child: CircularProgressIndicator()),
      error: (e, _) => Center(child: Text('Failed: $e')),
      data: (loadable) {
        final confs = loadable.valueOrNull ?? [];
        if (confs.isEmpty) {
          return Center(
            child: Column(
              mainAxisAlignment: MainAxisAlignment.center,
              children: [
                Icon(Icons.edit_note_rounded, size: 48, color: AppSurfaces.of(context).textSecondary),
                const SizedBox(height: IConfess.space3),
                Text('No personal confessions'),
                Text('Write your own', style: IConfess.bodySm.copyWith(color: AppSurfaces.of(context).textSecondary)),
              ],
            ),
          );
        }
        return ListView.builder(
          itemCount: confs.length,
          itemBuilder: (context, i) {
            final c = confs[i];
            return ListTile(
              title: Text(c.title.isNotEmpty ? c.title : c.shortText),
              subtitle: Text(c.description, maxLines: 2, overflow: TextOverflow.ellipsis),
            );
          },
        );
      },
    );
  }
}

class CollectionDetailScreen extends ConsumerWidget {
  const CollectionDetailScreen({required this.collectionId, super.key});
  final String collectionId;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final async = ref.watch(collectionDetailProvider(collectionId));
    final surfaces = AppSurfaces.of(context);
    return AppScaffold(
      title: 'Collection',
      body: async.when(
        loading: () => const Center(child: CircularProgressIndicator()),
        error: (e, _) => Center(child: Text('Failed: $e')),
        data: (loadable) {
          final data = loadable.valueOrNull ?? {};
          return Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(data['name']?.toString() ?? 'Collection',
                  style: IConfess.heading.copyWith(color: surfaces.textPrimary)),
              const SizedBox(height: IConfess.space2),
              Text(data['description']?.toString() ?? '',
                  style: IConfess.bodySm.copyWith(color: surfaces.textSecondary)),
              const SizedBox(height: IConfess.space5),
              Text('Items', style: IConfess.subheading.copyWith(color: surfaces.textPrimary)),
              const SizedBox(height: IConfess.space3),
              Expanded(
                child: (data['items'] is List && (data['items'] as List).isNotEmpty)
                    ? ListView.builder(
                        itemCount: (data['items'] as List).length,
                        itemBuilder: (context, i) {
                          final item = (data['items'] as List)[i];
                          return ListTile(
                            title: Text(item is Map ? item['title']?.toString() ?? 'Item' : 'Item'),
                          );
                        },
                      )
                    : Center(child: Text('No items in this collection')),
              ),
            ],
          );
        },
      ),
    );
  }
}
