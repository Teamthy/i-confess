import 'package:characters/characters.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:iconfess_api/iconfess_api.dart';

import '../../core/error/error_mapper.dart';
import '../../core/routing/routes.dart';
import '../../core/theme/theme.dart';
import '../../core/theme/tokens.dart';
import '../../core/widgets/screen.dart';
import '../../core/widgets/states.dart';
import 'library_providers.dart';

/// The library: collections, favourites and the listener's own confessions
/// (§35–§37, PHASE 27).
///
/// Three tabs rather than three screens because they answer one question —
/// "what is mine?" — and splitting them across the navigation would make the
/// least-used of the three effectively unreachable.
///
/// Each tab owns its own load. A favourites request that fails leaves
/// collections on screen, because they are separate endpoints and a partial
/// library beats a full-page error.
class LibraryScreen extends ConsumerWidget {
  const LibraryScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final surfaces = AppSurfaces.of(context);

    return DefaultTabController(
      length: 3,
      child: AppScaffold(
        title: 'Library',
        // The tabs own their scrolling. A scrollable scaffold wrapping a
        // TabBarView gives the view unbounded height, which throws rather
        // than rendering.
        scrollable: false,
        body: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            TabBar(
              labelColor: surfaces.primary,
              unselectedLabelColor: surfaces.textSecondary,
              indicatorColor: surfaces.primary,
              labelStyle: IConfess.bodySm.copyWith(fontWeight: FontWeight.w600),
              unselectedLabelStyle: IConfess.bodySm,
              tabs: const [
                Tab(key: ValueKey('tab-collections'), text: 'Collections'),
                Tab(key: ValueKey('tab-favorites'), text: 'Favorites'),
                Tab(key: ValueKey('tab-my-confessions'), text: 'Mine'),
              ],
            ),
            const SizedBox(height: IConfess.space4),
            const Expanded(
              child: TabBarView(
                children: [
                  _CollectionsTab(),
                  _FavoritesTab(),
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

/// Renders one tab's async value through the four states every data surface
/// owes the listener (§58, §100).
///
/// Written once and shared by all three tabs so a new tab cannot quietly ship
/// without an error state — which is how a surface ends up showing a spinner
/// forever when the server returns 500.
class _TabBody<T> extends StatelessWidget {
  const _TabBody({
    required this.async,
    required this.isEmpty,
    required this.empty,
    required this.builder,
    required this.onRetry,
  });

  final AsyncValue<Loadable<T>> async;
  final bool Function(T value) isEmpty;
  final Widget empty;
  final Widget Function(T value, {required bool fromCache}) builder;
  final VoidCallback onRetry;

  @override
  Widget build(BuildContext context) {
    return async.when(
      loading: () => const _LibrarySkeleton(),
      // A provider that threw is a client-side defect, not a mapped API
      // failure; it still has to offer a way forward rather than a red screen.
      error: (error, _) => ErrorState(
        error: error is ApiException
            ? ErrorMapper.describe(error)
            : const UserFacingError(
                title: 'Something went wrong',
                message: 'We could not load your library.',
                primaryAction: ErrorAction.retry,
                retryable: true,
              ),
        onAction: (_) => onRetry(),
      ),
      data: (loadable) => switch (loadable) {
        LoadFailed(:final error) => ErrorState(
            error: ErrorMapper.describe(error),
            onAction: (_) => onRetry(),
          ),
        LoadLoaded(:final value, :final fromCache) =>
          isEmpty(value) ? empty : builder(value, fromCache: fromCache),
        _ => const _LibrarySkeleton(),
      },
    );
  }
}

/// A banner stating that what is on screen came from the cache.
///
/// §102: cached data may be shown, but never presented as current. Silence
/// here is the failure mode where a listener acts on a week-old library.
class _OfflineNotice extends StatelessWidget {
  const _OfflineNotice();

  @override
  Widget build(BuildContext context) {
    final surfaces = AppSurfaces.of(context);
    return Container(
      margin: const EdgeInsets.only(bottom: IConfess.space3),
      padding: const EdgeInsets.symmetric(
        horizontal: IConfess.space3,
        vertical: IConfess.space2,
      ),
      decoration: BoxDecoration(
        color: surfaces.surface,
        borderRadius: BorderRadius.circular(IConfess.radiusSm),
        border: Border.all(color: surfaces.border),
      ),
      child: Row(
        children: [
          Icon(Icons.cloud_off_rounded, size: 14, color: surfaces.textSecondary),
          const SizedBox(width: IConfess.space2),
          Expanded(
            child: Text(
              'Showing your saved copy — you appear to be offline.',
              style: IConfess.caption.copyWith(color: surfaces.textSecondary),
            ),
          ),
        ],
      ),
    );
  }
}

class _LibrarySkeleton extends StatelessWidget {
  const _LibrarySkeleton();

  @override
  Widget build(BuildContext context) {
    return ListView.separated(
      padding: const EdgeInsets.symmetric(vertical: IConfess.space2),
      itemCount: 4,
      separatorBuilder: (_, _) => const SizedBox(height: IConfess.space3),
      itemBuilder: (_, _) => const Skeleton(height: 72, radius: IConfess.radiusMd),
    );
  }
}

// ---------------------------------------------------------------------------
// Collections
// ---------------------------------------------------------------------------

class _CollectionsTab extends ConsumerWidget {
  const _CollectionsTab();

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final async = ref.watch(libraryCollectionsProvider);

    return _TabBody<List<UserCollection>>(
      async: async,
      isEmpty: (v) => v.isEmpty,
      onRetry: () => ref.invalidate(libraryCollectionsProvider),
      empty: EmptyState(
        key: const ValueKey('empty-collections'),
        icon: Icons.collections_bookmark_rounded,
        title: 'No collections yet',
        message: 'Group the confessions you return to — a morning set, '
            'something for hard weeks.',
        actionLabel: 'Create a collection',
        onAction: () => _promptCreate(context, ref),
      ),
      builder: (collections, {required fromCache}) => Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          if (fromCache) const _OfflineNotice(),
          Expanded(
            child: ListView.separated(
              padding: const EdgeInsets.only(bottom: IConfess.space6),
              itemCount: collections.length,
              separatorBuilder: (_, _) => const SizedBox(height: IConfess.space3),
              itemBuilder: (context, i) => _CollectionCard(collection: collections[i]),
            ),
          ),
          // Creating from a full list matters as much as from the empty state:
          // the second collection is the one a user has to hunt for.
          Padding(
            padding: const EdgeInsets.only(bottom: IConfess.space4),
            child: OutlinedButton.icon(
              key: const ValueKey('button-create-collection'),
              onPressed: () => _promptCreate(context, ref),
              icon: const Icon(Icons.add_rounded, size: 18),
              label: const Text('New collection'),
            ),
          ),
        ],
      ),
    );
  }
}

Future<void> _promptCreate(BuildContext context, WidgetRef ref) async {
  final name = await showDialog<String>(
    context: context,
    builder: (context) => const _NameDialog(
      title: 'New collection',
      action: 'Create',
    ),
  );
  if (name == null || name.trim().isEmpty) return;

  final result = await ref.read(libraryActionsProvider).createCollection(name.trim());
  if (!context.mounted) return;
  result.when(
    success: (_) {},
    // The server is the authority on whether the write happened; saying
    // nothing when it refuses is how a user ends up tapping Create twice.
    failure: (error) => ScaffoldMessenger.of(context).showSnackBar(
      SnackBar(content: Text(ErrorMapper.describe(error).message)),
    ),
  );
}

/// A single-field dialog, used for creating and renaming.
///
/// The name limit mirrors the server's 80 characters. Client validation here
/// is a courtesy — the server enforces it regardless — but hitting a 400 for
/// something the field could have said inline is a poor way to learn it.
class _NameDialog extends StatefulWidget {
  const _NameDialog({required this.title, required this.action, this.initial = ''});

  final String title;
  final String action;
  final String initial;

  @override
  State<_NameDialog> createState() => _NameDialogState();
}

class _NameDialogState extends State<_NameDialog> {
  late final TextEditingController _controller =
      TextEditingController(text: widget.initial);
  String? _error;

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  void _submit() {
    final value = _controller.text.trim();
    if (value.isEmpty) {
      setState(() => _error = 'Give it a name.');
      return;
    }
    if (value.length > 80) {
      setState(() => _error = 'Keep it under 80 characters.');
      return;
    }
    Navigator.of(context).pop(value);
  }

  @override
  Widget build(BuildContext context) {
    return AlertDialog(
      title: Text(widget.title),
      content: TextField(
        key: const ValueKey('field-collection-name'),
        controller: _controller,
        autofocus: true,
        maxLength: 80,
        decoration: InputDecoration(hintText: 'Morning mercies', errorText: _error),
        onSubmitted: (_) => _submit(),
      ),
      actions: [
        TextButton(
          onPressed: () => Navigator.of(context).pop(),
          child: const Text('Cancel'),
        ),
        FilledButton(
          key: const ValueKey('button-confirm-name'),
          onPressed: _submit,
          child: Text(widget.action),
        ),
      ],
    );
  }
}

class _CollectionCard extends StatelessWidget {
  const _CollectionCard({required this.collection});

  final UserCollection collection;

  @override
  Widget build(BuildContext context) {
    final surfaces = AppSurfaces.of(context);
    return Material(
      color: surfaces.surfaceRaised,
      borderRadius: BorderRadius.circular(IConfess.radiusMd),
      child: InkWell(
        key: ValueKey('collection-${collection.id}'),
        borderRadius: BorderRadius.circular(IConfess.radiusMd),
        onTap: () => context.go(AppRoutes.collectionDetail(collection.id)),
        child: Padding(
          padding: const EdgeInsets.all(IConfess.space4),
          child: Row(
            children: [
              _CollectionCover(collection: collection),
              const SizedBox(width: IConfess.space4),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    Text(
                      collection.name,
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                      style: IConfess.subheading.copyWith(color: surfaces.textPrimary),
                    ),
                    if (collection.description.isNotEmpty) ...[
                      const SizedBox(height: IConfess.space1),
                      Text(
                        collection.description,
                        maxLines: 1,
                        overflow: TextOverflow.ellipsis,
                        style: IConfess.bodySm.copyWith(color: surfaces.textSecondary),
                      ),
                    ],
                    const SizedBox(height: IConfess.space2),
                    Row(
                      children: [
                        Text(
                          _countLabel(collection.itemCount),
                          style: IConfess.caption.copyWith(color: surfaces.textSecondary),
                        ),
                        // Only a collection the listener has opened up is
                        // labelled. Marking the default private would put a
                        // privacy badge on every row and teach people to
                        // ignore it.
                        if (!collection.isPrivate) ...[
                          const SizedBox(width: IConfess.space2),
                          Icon(
                            collection.visibility == 'public'
                                ? Icons.public_rounded
                                : Icons.link_rounded,
                            size: 13,
                            color: surfaces.textSecondary,
                          ),
                          const SizedBox(width: IConfess.space1),
                          Text(
                            collection.visibility,
                            style: IConfess.caption.copyWith(color: surfaces.textSecondary),
                          ),
                        ],
                      ],
                    ),
                  ],
                ),
              ),
              Icon(Icons.chevron_right_rounded, color: surfaces.textSecondary),
            ],
          ),
        ),
      ),
    );
  }
}

String _countLabel(int count) => count == 1 ? '1 confession' : '$count confessions';

/// Cover art, or a lettered stand-in.
///
/// Most collections will never have artwork. A generated monogram keeps the
/// row's geometry identical whether or not a cover exists, so a list does not
/// visibly reflow as images arrive.
class _CollectionCover extends StatelessWidget {
  const _CollectionCover({required this.collection});

  final UserCollection collection;

  @override
  Widget build(BuildContext context) {
    final surfaces = AppSurfaces.of(context);
    final initial = collection.name.trim().isEmpty
        ? '?'
        : collection.name.trim().characters.first.toUpperCase();

    return Container(
      width: 48,
      height: 48,
      clipBehavior: Clip.antiAlias,
      decoration: BoxDecoration(
        color: IConfess.colorBrand50,
        borderRadius: BorderRadius.circular(IConfess.radiusSm),
        border: Border.all(color: surfaces.border),
      ),
      child: collection.coverUrl.isEmpty
          ? Center(
              child: Text(
                initial,
                style: IConfess.subheading.copyWith(color: IConfess.colorBrand700),
              ),
            )
          : Image.network(
              collection.coverUrl,
              fit: BoxFit.cover,
              // A broken image must not become a broken row.
              errorBuilder: (_, _, _) => Center(
                child: Text(
                  initial,
                  style: IConfess.subheading.copyWith(color: IConfess.colorBrand700),
                ),
              ),
            ),
    );
  }
}

// ---------------------------------------------------------------------------
// Favourites
// ---------------------------------------------------------------------------

class _FavoritesTab extends ConsumerWidget {
  const _FavoritesTab();

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final async = ref.watch(libraryFavoritesProvider);

    return _TabBody<List<Favorite>>(
      async: async,
      isEmpty: (v) => v.isEmpty,
      onRetry: () => ref.invalidate(libraryFavoritesProvider),
      empty: EmptyState(
        key: const ValueKey('empty-favorites'),
        icon: Icons.favorite_border_rounded,
        title: 'Nothing saved yet',
        message: 'Tap the heart on a confession and it will wait for you here.',
        actionLabel: 'Browse confessions',
        onAction: () => context.go(AppRoutes.explore),
      ),
      builder: (favorites, {required fromCache}) => ListView.separated(
        padding: const EdgeInsets.only(bottom: IConfess.space6),
        itemCount: favorites.length + (fromCache ? 1 : 0),
        separatorBuilder: (_, _) => const SizedBox(height: IConfess.space2),
        itemBuilder: (context, i) {
          if (fromCache && i == 0) return const _OfflineNotice();
          final favorite = favorites[i - (fromCache ? 1 : 0)];
          return _FavoriteRow(favorite: favorite);
        },
      ),
    );
  }
}

class _FavoriteRow extends ConsumerWidget {
  const _FavoriteRow({required this.favorite});

  final Favorite favorite;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final surfaces = AppSurfaces.of(context);

    // A favourite whose target is gone is still listed, so it can be cleared,
    // but it must not offer navigation into content that no longer resolves.
    // G-45 closed: every entity kind the favourites tab lists now has a
    // destination. Before this, a favourited session, category or voice
    // rendered and could be removed but did nothing when tapped, because the
    // row only knew how to open confessions.
    final canOpen = !favorite.missing;

    return Material(
      color: surfaces.surfaceRaised,
      borderRadius: BorderRadius.circular(IConfess.radiusMd),
      child: InkWell(
        key: ValueKey('favorite-${favorite.entityId}'),
        borderRadius: BorderRadius.circular(IConfess.radiusMd),
        onTap: canOpen ? () => _openFavorite(context) : null,
        child: Padding(
          padding: const EdgeInsets.symmetric(
            horizontal: IConfess.space4,
            vertical: IConfess.space3,
          ),
          child: Row(
            children: [
              Icon(
                _iconFor(favorite.entityType),
                size: 18,
                color: favorite.missing ? surfaces.textDisabled : surfaces.primary,
              ),
              const SizedBox(width: IConfess.space3),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    Text(
                      favorite.displayTitle,
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                      style: IConfess.body.copyWith(
                        color: favorite.missing
                            ? surfaces.textSecondary
                            : surfaces.textPrimary,
                        fontStyle: favorite.missing ? FontStyle.italic : null,
                      ),
                    ),
                    if (favorite.subtitle.isNotEmpty) ...[
                      const SizedBox(height: IConfess.space1),
                      Text(
                        favorite.subtitle,
                        maxLines: 1,
                        overflow: TextOverflow.ellipsis,
                        style: IConfess.caption.copyWith(color: surfaces.textSecondary),
                      ),
                    ],
                  ],
                ),
              ),
              IconButton(
                key: ValueKey('unfavorite-${favorite.entityId}'),
                // Named for what it does, not for the icon it draws: a screen
                // reader announcing "favorite" on a button that removes one is
                // worse than no label.
                tooltip: 'Remove from favourites',
                icon: const Icon(Icons.favorite_rounded, size: 18),
                color: favorite.missing ? surfaces.textDisabled : surfaces.danger,
                onPressed: () => _unfavorite(context, ref),
              ),
            ],
          ),
        ),
      ),
    );
  }

  Future<void> _unfavorite(BuildContext context, WidgetRef ref) async {
    final messenger = ScaffoldMessenger.of(context);
    final result = await ref.read(libraryActionsProvider).unfavorite(favorite);
    if (!context.mounted) return;
    result.when(
      success: (_) => messenger.showSnackBar(
        const SnackBar(content: Text('Removed from favourites')),
      ),
      failure: (error) => messenger.showSnackBar(
        SnackBar(content: Text(ErrorMapper.describe(error).message)),
      ),
    );
  }

  /// Where a favourite row leads, one route per entity kind.
  ///
  /// Voices have no detail screen of their own — the builder's voice step is
  /// where a voice is chosen and auditioned, so that is the surface a
  /// favourited voice should open, not a dead row.
  void _openFavorite(BuildContext context) {
    final path = switch (favorite.entityType) {
      'confession' => AppRoutes.confessionDetail(favorite.entityId),
      'category' => AppRoutes.categoryDetail(favorite.entityId),
      'session' => AppRoutes.playerWithId(favorite.entityId),
      'voice' => AppRoutes.builderVoice,
      _ => null,
    };
    if (path != null) context.go(path);
  }

  static IconData _iconFor(String entityType) => switch (entityType) {
        'category' => Icons.category_rounded,
        'voice' => Icons.record_voice_over_rounded,
        'session' => Icons.play_circle_outline_rounded,
        _ => Icons.favorite_rounded,
      };
}

// ---------------------------------------------------------------------------
// My confessions
// ---------------------------------------------------------------------------

class _MyConfessionsTab extends ConsumerWidget {
  const _MyConfessionsTab();

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final async = ref.watch(libraryConfessionsProvider);

    return _TabBody<List<UserConfession>>(
      async: async,
      isEmpty: (v) => v.isEmpty,
      onRetry: () => ref.invalidate(libraryConfessionsProvider),
      empty: EmptyState(
        key: const ValueKey('empty-my-confessions'),
        icon: Icons.edit_note_rounded,
        title: 'You have not written one yet',
        message: 'Write a confession in your own words. It stays private '
            'unless you offer it for review.',
        actionLabel: 'Write one',
        onAction: () => context.go(AppRoutes.confess),
      ),
      builder: (confessions, {required fromCache}) => ListView.separated(
        padding: const EdgeInsets.only(bottom: IConfess.space6),
        itemCount: confessions.length + (fromCache ? 1 : 0),
        separatorBuilder: (_, _) => const SizedBox(height: IConfess.space2),
        itemBuilder: (context, i) {
          if (fromCache && i == 0) return const _OfflineNotice();
          return _MyConfessionRow(
            confession: confessions[i - (fromCache ? 1 : 0)],
          );
        },
      ),
    );
  }
}

class _MyConfessionRow extends ConsumerWidget {
  const _MyConfessionRow({required this.confession});

  final UserConfession confession;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final surfaces = AppSurfaces.of(context);

    return Material(
      color: surfaces.surfaceRaised,
      borderRadius: BorderRadius.circular(IConfess.radiusMd),
      child: Padding(
        key: ValueKey('my-confession-${confession.id}'),
        padding: const EdgeInsets.all(IConfess.space4),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Expanded(
                  child: Text(
                    confession.lead,
                    maxLines: 2,
                    overflow: TextOverflow.ellipsis,
                    style: IConfess.body.copyWith(color: surfaces.textPrimary),
                  ),
                ),
                const SizedBox(width: IConfess.space3),
                _StatusChip(status: confession.status),
              ],
            ),
            // A rejection without its reason is the worst version of this
            // screen: the author knows only that they were refused.
            if (confession.isRejected && confession.rejectionReason.isNotEmpty) ...[
              const SizedBox(height: IConfess.space3),
              Container(
                padding: const EdgeInsets.all(IConfess.space3),
                decoration: BoxDecoration(
                  color: surfaces.surface,
                  borderRadius: BorderRadius.circular(IConfess.radiusSm),
                  border: Border.all(color: surfaces.border),
                ),
                child: Text(
                  confession.rejectionReason,
                  style: IConfess.caption.copyWith(color: surfaces.textSecondary),
                ),
              ),
            ],
            if (confession.canSubmit) ...[
              const SizedBox(height: IConfess.space3),
              Align(
                alignment: Alignment.centerLeft,
                child: OutlinedButton(
                  key: ValueKey('submit-${confession.id}'),
                  onPressed: () => _submit(context, ref),
                  child: const Text('Offer for review'),
                ),
              ),
            ],
          ],
        ),
      ),
    );
  }

  Future<void> _submit(BuildContext context, WidgetRef ref) async {
    final messenger = ScaffoldMessenger.of(context);
    final result =
        await ref.read(libraryActionsProvider).submitConfession(confession.id);
    if (!context.mounted) return;
    result.when(
      // Deliberately "sent for review", never "published": the moderator
      // decides, and promising publication here would be a promise the client
      // cannot keep.
      success: (_) => messenger.showSnackBar(
        const SnackBar(content: Text('Sent for review')),
      ),
      failure: (error) => messenger.showSnackBar(
        SnackBar(content: Text(ErrorMapper.describe(error).message)),
      ),
    );
  }
}

/// The moderation state of a personal confession, in words.
///
/// The vocabulary mirrors the server's `user_confessions.status` CHECK
/// constraint. Colour alone never carries the meaning — the label is always
/// present, because a status a colour-blind user cannot read is not a status.
class _StatusChip extends StatelessWidget {
  const _StatusChip({required this.status});

  final String status;

  @override
  Widget build(BuildContext context) {
    final surfaces = AppSurfaces.of(context);
    final (label, color) = switch (status) {
      'draft' => ('Draft', surfaces.textSecondary),
      'submitted' => ('In review', surfaces.warning),
      'approved' => ('Approved', surfaces.success),
      'published' => ('Published', surfaces.success),
      'rejected' => ('Not accepted', surfaces.danger),
      'archived' => ('Archived', surfaces.textDisabled),
      _ => (status, surfaces.textSecondary),
    };

    return Container(
      key: ValueKey('status-$status'),
      padding: const EdgeInsets.symmetric(
        horizontal: IConfess.space2,
        vertical: IConfess.space1,
      ),
      decoration: BoxDecoration(
        color: color.withValues(alpha: 0.12),
        borderRadius: BorderRadius.circular(IConfess.radiusSm),
      ),
      child: Text(label, style: IConfess.label.copyWith(color: color)),
    );
  }
}

// ---------------------------------------------------------------------------
// Collection detail
// ---------------------------------------------------------------------------

/// One collection and its items, in the order the listener arranged them.
class CollectionDetailScreen extends ConsumerWidget {
  const CollectionDetailScreen({required this.collectionId, super.key});

  final String collectionId;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final async = ref.watch(collectionDetailProvider(collectionId));
    final surfaces = AppSurfaces.of(context);

    return AppScaffold(
      title: 'Collection',
      scrollable: false,
      actions: [
        if (async.asData?.value.valueOrNull != null)
          PopupMenuButton<String>(
            key: const ValueKey('collection-menu'),
            onSelected: (value) => switch (value) {
              'rename' => _rename(context, ref, async.asData!.value.valueOrNull!),
              'cover' => _setCover(context, ref, async.asData!.value.valueOrNull!),
              'delete' => _confirmDelete(context, ref),
              _ => null,
            },
            itemBuilder: (_) => const [
              PopupMenuItem(value: 'rename', child: Text('Rename')),
              PopupMenuItem(value: 'cover', child: Text('Set cover image')),
              PopupMenuItem(value: 'delete', child: Text('Delete collection')),
            ],
          ),
      ],
      body: _TabBody<UserCollection>(
        async: async,
        // A collection always exists once loaded; emptiness is about items,
        // handled inside the builder so the header still renders.
        isEmpty: (_) => false,
        empty: const SizedBox.shrink(),
        onRetry: () => ref.invalidate(collectionDetailProvider(collectionId)),
        builder: (collection, {required fromCache}) => Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text(
              collection.name,
              style: IConfess.heading.copyWith(color: surfaces.textPrimary),
            ),
            if (collection.description.isNotEmpty) ...[
              const SizedBox(height: IConfess.space2),
              Text(
                collection.description,
                style: IConfess.bodySm.copyWith(color: surfaces.textSecondary),
              ),
            ],
            const SizedBox(height: IConfess.space2),
            Text(
              _countLabel(collection.itemCount),
              style: IConfess.caption.copyWith(color: surfaces.textSecondary),
            ),
            const SizedBox(height: IConfess.space5),
            Expanded(
              child: collection.items.isEmpty
                  ? EmptyState(
                      key: const ValueKey('empty-collection-items'),
                      icon: Icons.playlist_add_rounded,
                      title: 'Nothing in here yet',
                      message: 'Add a confession from its page to build this '
                          'collection up.',
                      actionLabel: 'Find confessions',
                      onAction: () => context.go(AppRoutes.explore),
                    )
                  // G-43: the order a listener curated is now curatable in
                  // the app. ReorderableListView rather than a custom gesture
                  // because the drag proxy, the list reflow and the a11y
                  // announcements are all already correct here. The default
                  // whole-row drag handles are off: the row itself navigates,
                  // and a tap that both opened the confession and started a
                  // drag would be unusable. A grip icon owns the drag.
                  : ReorderableListView.builder(
                      key: const ValueKey('collection-items'),
                      padding: const EdgeInsets.only(bottom: IConfess.space6),
                      buildDefaultDragHandles: false,
                      itemCount: collection.items.length,
                      onReorder: (oldIndex, newIndex) =>
                          _reorder(context, ref, collection.items, oldIndex, newIndex),
                      itemBuilder: (context, i) => Padding(
                        // ReorderableListView requires the key on the direct
                        // child, and needs no separators - the padding is
                        // what this list had before, kept on the row.
                        key: ValueKey('item-${collection.items[i].confessionId}'),
                        padding: const EdgeInsets.only(bottom: IConfess.space2),
                        child: _CollectionItemRow(
                          collectionId: collectionId,
                          item: collection.items[i],
                          dragIndex: i,
                        ),
                      ),
                    ),
            ),
          ],
        ),
      ),
    );
  }

  Future<void> _rename(
      BuildContext context, WidgetRef ref, UserCollection collection) async {
    final name = await showDialog<String>(
      context: context,
      builder: (_) => _NameDialog(
        title: 'Rename collection',
        action: 'Save',
        initial: collection.name,
      ),
    );
    if (name == null || !context.mounted) return;

    final messenger = ScaffoldMessenger.of(context);
    final result =
        await ref.read(libraryActionsProvider).renameCollection(collectionId, name);
    result.when(
      success: (_) {},
      failure: (error) => messenger.showSnackBar(
        SnackBar(content: Text(ErrorMapper.describe(error).message)),
      ),
    );
  }

  /// Saves a new item order after a drag (G-43).
  ///
  /// The payload is the complete order, not the move: the server rewrites
  /// every position from the array, which keeps a half-applied reorder
  /// impossible. On success the detail view is invalidated rather than kept
  /// at the optimistic order - if a row moved underneath the drag, the server
  /// is right and the screen should show it.
  Future<void> _reorder(BuildContext context, WidgetRef ref,
      List<CollectionItem> items, int oldIndex, int newIndex) async {
    // ReorderableListView hands over the insertion index into the list that
    // still contains the picked row; past the old position it is one too far.
    final target = newIndex > oldIndex ? newIndex - 1 : newIndex;
    final ordered = [...items];
    ordered.insert(target, ordered.removeAt(oldIndex));

    final messenger = ScaffoldMessenger.of(context);
    final result = await ref.read(libraryActionsProvider).reorderCollection(
          collectionId,
          [for (final item in ordered) item.confessionId],
        );
    result.when(
      success: (_) {},
      failure: (error) => messenger.showSnackBar(
        SnackBar(content: Text(ErrorMapper.describe(error).message)),
      ),
    );
  }

  /// Sets, replaces or clears the collection cover (G-44).
  ///
  /// The field is a URL, not a file picker, because the platform has no image
  /// upload route yet; pretending otherwise would end in a picker that
  /// discards what it picks. What this closes is the actual gap: cover_url
  /// was rendered by every row since PHASE 27 and written by nothing, so the
  /// monogram was permanent.
  Future<void> _setCover(BuildContext context, WidgetRef ref, UserCollection collection) async {
    final cover = await showDialog<String>(
      context: context,
      builder: (_) => _CoverUrlDialog(initial: collection.coverUrl),
    );
    if (cover == null || !context.mounted) return;

    final messenger = ScaffoldMessenger.of(context);
    final result = await ref.read(libraryActionsProvider).updateCover(collectionId, cover);
    result.when(
      success: (_) => messenger.showSnackBar(SnackBar(
        content: Text(cover.isEmpty ? 'Cover removed' : 'Cover updated'),
      )),
      failure: (error) => messenger.showSnackBar(
        SnackBar(content: Text(ErrorMapper.describe(error).message)),
      ),
    );
  }

  Future<void> _confirmDelete(BuildContext context, WidgetRef ref) async {
    // Deleting a collection is not recoverable, so it is confirmed. The
    // confessions inside it are untouched, and the dialog says so — otherwise
    // a user reasonably fears they are deleting content.
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (context) => AlertDialog(
        title: const Text('Delete this collection?'),
        content: const Text(
          'The confessions in it stay in the library. Only the collection goes.',
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(context).pop(false),
            child: const Text('Keep'),
          ),
          FilledButton(
            key: const ValueKey('button-confirm-delete'),
            onPressed: () => Navigator.of(context).pop(true),
            child: const Text('Delete'),
          ),
        ],
      ),
    );
    if (confirmed != true || !context.mounted) return;

    final messenger = ScaffoldMessenger.of(context);
    final router = GoRouter.of(context);
    final result = await ref.read(libraryActionsProvider).deleteCollection(collectionId);
    result.when(
      success: (_) => router.go(AppRoutes.library),
      failure: (error) => messenger.showSnackBar(
        SnackBar(content: Text(ErrorMapper.describe(error).message)),
      ),
    );
  }
}

/// A cover-URL field for a collection (G-44).
///
/// Unlike the name dialog, an empty value is a valid submission: it means
/// "remove the cover". The hint says so, because a field that quietly
/// swallows the empty string teaches users that nothing they do here sticks.
class _CoverUrlDialog extends StatefulWidget {
  const _CoverUrlDialog({this.initial = ''});

  final String initial;

  @override
  State<_CoverUrlDialog> createState() => _CoverUrlDialogState();
}

class _CoverUrlDialogState extends State<_CoverUrlDialog> {
  late final TextEditingController _controller =
      TextEditingController(text: widget.initial);
  String? _error;

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  void _submit() {
    final value = _controller.text.trim();
    // Mirrors the server's validCollectionCover; the 400 is still the
    // authority, but a field that can say this inline should.
    if (value.isNotEmpty &&
        !(value.startsWith('https://') || value.startsWith('http://') || value.startsWith('/'))) {
      setState(() => _error = 'Use an http(s) link or a path like /media/…');
      return;
    }
    if (value.length > 2048) {
      setState(() => _error = 'That link is too long (2048 characters at most).');
      return;
    }
    Navigator.of(context).pop(value);
  }

  @override
  Widget build(BuildContext context) {
    return AlertDialog(
      title: const Text('Cover image'),
      content: TextField(
        key: const ValueKey('field-cover-url'),
        controller: _controller,
        autofocus: true,
        keyboardType: TextInputType.url,
        decoration: InputDecoration(
          hintText: 'https://example.org/grace.jpg, or empty to remove',
          errorText: _error,
        ),
        onSubmitted: (_) => _submit(),
      ),
      actions: [
        TextButton(
          onPressed: () => Navigator.of(context).pop(),
          child: const Text('Cancel'),
        ),
        FilledButton(
          key: const ValueKey('button-confirm-cover'),
          onPressed: _submit,
          child: const Text('Save'),
        ),
      ],
    );
  }
}

class _CollectionItemRow extends ConsumerWidget {
  const _CollectionItemRow({
    required this.collectionId,
    required this.item,
    super.key,
    this.dragIndex,
  });

  final String collectionId;
  final CollectionItem item;

  /// The row's position in a ReorderableListView, when it is inside one.
  /// Detail surfaces that list items without reordering pass nothing and get
  /// no grip, because a handle that does nothing is worse than no handle.
  final int? dragIndex;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final surfaces = AppSurfaces.of(context);

    return Material(
      color: surfaces.surfaceRaised,
      borderRadius: BorderRadius.circular(IConfess.radiusMd),
      child: InkWell(
        key: ValueKey('row-${item.confessionId}'),
        borderRadius: BorderRadius.circular(IConfess.radiusMd),
        onTap: item.resolved
            ? () => context.go(AppRoutes.confessionDetail(item.confessionId))
            : null,
        child: Padding(
          padding: const EdgeInsets.symmetric(
            horizontal: IConfess.space4,
            vertical: IConfess.space3,
          ),
          child: Row(
            children: [
              if (dragIndex != null)
                ReorderableDragStartListener(
                  index: dragIndex!,
                  child: Icon(
                    Icons.drag_indicator_rounded,
                    key: ValueKey('drag-${item.confessionId}'),
                    size: 18,
                    color: surfaces.textSecondary,
                    semanticLabel: 'Reorder ${item.resolved ? item.title : 'this row'}',
                  ),
                ),
              if (dragIndex != null) const SizedBox(width: IConfess.space2),
              Expanded(
                child: Text(
                  // An unresolved join means the confession behind this item
                  // is gone. Saying so beats an empty row the user cannot
                  // interpret.
                  item.resolved ? item.title : 'No longer available',
                  maxLines: 2,
                  overflow: TextOverflow.ellipsis,
                  style: IConfess.body.copyWith(
                    color: item.resolved ? surfaces.textPrimary : surfaces.textSecondary,
                    fontStyle: item.resolved ? null : FontStyle.italic,
                  ),
                ),
              ),
              IconButton(
                key: ValueKey('remove-${item.confessionId}'),
                tooltip: 'Remove from collection',
                icon: const Icon(Icons.remove_circle_outline_rounded, size: 18),
                color: surfaces.textSecondary,
                onPressed: () async {
                  final messenger = ScaffoldMessenger.of(context);
                  final result = await ref
                      .read(libraryActionsProvider)
                      .removeFromCollection(collectionId, item.confessionId);
                  result.when(
                    success: (_) {},
                    failure: (error) => messenger.showSnackBar(
                      SnackBar(content: Text(ErrorMapper.describe(error).message)),
                    ),
                  );
                },
              ),
            ],
          ),
        ),
      ),
    );
  }
}
