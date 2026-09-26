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
import 'confession_providers.dart';
import '../library/library_providers.dart';
import '../../core/di/providers.dart';

/// The confession experience — where a listener reads, reflects and acts.
///
/// Shows the full text (short/medium/long), variants, scripture anchors,
/// intensity and tags, plus actions to favourite and to build a session from
/// this confession. It is the screen that turns a category listing into a
/// personal practice.
///
/// The confession itself is public (no auth required to read), but favouriting
/// and building a session require a session. The screen degrades gracefully:
/// a signed-out viewer can read, but the primary actions prompt sign-in.
class ConfessionDetailScreen extends ConsumerStatefulWidget {
  const ConfessionDetailScreen({required this.confessionId, super.key});

  final String confessionId;

  @override
  ConsumerState<ConfessionDetailScreen> createState() =>
      _ConfessionDetailScreenState();
}

class _ConfessionDetailScreenState
    extends ConsumerState<ConfessionDetailScreen> {
  bool _favoriteBusy = false;
  bool? _favoriteOverride;

  @override
  Widget build(BuildContext context) {
    final confessionAsync = ref.watch(confessionProvider(widget.confessionId));
    final favoriteAsync =
        ref.watch(confessionIsFavoriteProvider(widget.confessionId));

    return AppScaffold(
      title: 'Confession',
      body: confessionAsync.when(
        loading: () => const ListSkeleton(rows: 3),
        error: (error, _) => ErrorState(
          error: const UserFacingError(
            title: 'This confession did not load',
            message: 'Check your connection and try again.',
            primaryAction: ErrorAction.retry,
            retryable: true,
          ),
          onAction: (_) =>
              ref.invalidate(confessionProvider(widget.confessionId)),
        ),
        data: (loadable) {
          final confession = loadable.valueOrNull;
          if (confession == null) {
            return const EmptyState(
              title: 'Confession not found',
              message: 'It may have been moved or removed.',
            );
          }

          final isFavLoadable = favoriteAsync.asData?.value;
          final isFav = _favoriteOverride ??
              isFavLoadable?.valueOrNull ??
              false;

          return Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              _Header(
                confession: confession,
                isFavorite: isFav,
                busy: _favoriteBusy,
                onFavoriteToggle: () => _toggleFavorite(isFav),
              ),
              const SizedBox(height: IConfess.space6),
              _Body(confession: confession),
              if (confession.variants.isNotEmpty) ...[
                const SizedBox(height: IConfess.space7),
                _Variants(variants: confession.variants),
              ],
              if (confession.scriptures.isNotEmpty) ...[
                const SizedBox(height: IConfess.space7),
                _Scriptures(scriptures: confession.scriptures),
              ],
              if (confession.tags.isNotEmpty) ...[
                const SizedBox(height: IConfess.space6),
                _Tags(tags: confession.tags),
              ],
              const SizedBox(height: IConfess.space8),
              _Actions(
                confession: confession,
                isFavorite: isFav,
                favoriteBusy: _favoriteBusy,
                onFavoriteToggle: () => _toggleFavorite(isFav),
              ),
            ],
          );
        },
      ),
    );
  }

  Future<void> _toggleFavorite(bool currentlyFav) async {
    if (_favoriteBusy) return;
    setState(() => _favoriteBusy = true);

    final repo = ref.read(contentRepositoryProvider);
    final result = currentlyFav
        ? await repo.removeFavoriteConfession(widget.confessionId)
        : await repo.addFavoriteConfession(widget.confessionId);

    if (!mounted) return;

    result.when(
      success: (_) {
        setState(() {
          _favoriteOverride = !currentlyFav;
          _favoriteBusy = false;
        });
        // Refresh the server-derived favourite state so other screens see it.
        ref.invalidate(confessionIsFavoriteProvider(widget.confessionId));
      },
      failure: (error) {
        setState(() => _favoriteBusy = false);
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(
            content: Text(error is ApiError
                ? error.message
                : 'Could not update favourite. Try again.'),
          ),
        );
      },
    );
  }
}

class _Header extends StatelessWidget {
  const _Header({
    required this.confession,
    required this.isFavorite,
    required this.busy,
    required this.onFavoriteToggle,
  });

  final Confession confession;
  final bool isFavorite;
  final bool busy;
  final VoidCallback onFavoriteToggle;

  @override
  Widget build(BuildContext context) {
    final surfaces = AppSurfaces.of(context);
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Row(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Expanded(
              child: Text(
                confession.title.isNotEmpty
                    ? confession.title
                    : confession.lead,
                style:
                    IConfess.heading.copyWith(color: surfaces.textPrimary),
              ),
            ),
            const SizedBox(width: IConfess.space3),
            _IntensityBadge(level: confession.intensity),
          ],
        ),
        if (confession.author.isNotEmpty) ...[
          const SizedBox(height: IConfess.space2),
          Text(
            'By ${confession.author}',
            style: IConfess.caption.copyWith(color: surfaces.textSecondary),
          ),
        ],
        const SizedBox(height: IConfess.space3),
        Row(
          children: [
            IconButton(
              key: const ValueKey('btn-favorite'),
              icon: Icon(
                isFavorite ? Icons.favorite_rounded : Icons.favorite_border_rounded,
                color: isFavorite ? Colors.redAccent : surfaces.textSecondary,
              ),
              onPressed: busy ? null : onFavoriteToggle,
            ),
            if (busy)
              SizedBox(
                width: 16,
                height: 16,
                child: CircularProgressIndicator(
                  strokeWidth: 2,
                  color: surfaces.textSecondary,
                ),
              ),
            const SizedBox(width: IConfess.space2),
            Text(
              isFavorite ? 'Favourited' : 'Favourite',
              style: IConfess.bodySm.copyWith(color: surfaces.textSecondary),
            ),
          ],
        ),
      ],
    );
  }
}

class _Body extends StatelessWidget {
  const _Body({required this.confession});
  final Confession confession;

  @override
  Widget build(BuildContext context) {
    final surfaces = AppSurfaces.of(context);
    // Show the fullest text, but also surface the shorter variants when they
    // exist — the short text is what a card leads with, and seeing how it
    // expands into the medium and long forms is part of the experience.
    final texts = <_TextBlock>[];
    if (confession.shortText.isNotEmpty) {
      texts.add(_TextBlock(label: 'Declare', text: confession.shortText));
    }
    if (confession.mediumText.isNotEmpty &&
        confession.mediumText != confession.shortText) {
      texts.add(_TextBlock(label: 'Meditate', text: confession.mediumText));
    }
    if (confession.longText.isNotEmpty &&
        confession.longText != confession.mediumText &&
        confession.longText != confession.shortText) {
      texts.add(_TextBlock(label: 'Confess', text: confession.longText));
    }
    if (texts.isEmpty && confession.description.isNotEmpty) {
      texts.add(_TextBlock(label: 'Confession', text: confession.description));
    }

    if (texts.isEmpty) {
      return Text(
        'This confession is being written.',
        style: IConfess.body.copyWith(color: surfaces.textSecondary),
      );
    }

    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        for (var i = 0; i < texts.length; i++) ...[
          if (i > 0) const SizedBox(height: IConfess.space5),
          _TextSection(block: texts[i]),
        ],
      ],
    );
  }
}

class _TextBlock {
  const _TextBlock({required this.label, required this.text});
  final String label;
  final String text;
}

class _TextSection extends StatelessWidget {
  const _TextSection({required this.block});
  final _TextBlock block;

  @override
  Widget build(BuildContext context) {
    final surfaces = AppSurfaces.of(context);
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(
          block.label.toUpperCase(),
          style: IConfess.label.copyWith(color: surfaces.textSecondary),
        ),
        const SizedBox(height: IConfess.space2),
        Text(
          block.text,
          style: IConfess.body.copyWith(color: surfaces.textPrimary, height: 1.6),
        ),
      ],
    );
  }
}

class _Variants extends StatelessWidget {
  const _Variants({required this.variants});
  final List<ConfessionVariant> variants;

  @override
  Widget build(BuildContext context) {
    final surfaces = AppSurfaces.of(context);
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        const SectionHeader('Lengths'),
        Wrap(
          spacing: IConfess.space2,
          runSpacing: IConfess.space2,
          children: [
            for (final v in variants)
              Chip(
                label: Text(
                  v.label.isNotEmpty
                      ? v.label
                      : '${v.durationSeconds}s',
                ),
                backgroundColor: surfaces.surfaceRaised,
              ),
          ],
        ),
      ],
    );
  }
}

class _Scriptures extends StatelessWidget {
  const _Scriptures({required this.scriptures});
  final List<ScriptureRef> scriptures;

  @override
  Widget build(BuildContext context) {
    final surfaces = AppSurfaces.of(context);
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        const SectionHeader('Scripture'),
        for (final s in scriptures) ...[
          Container(
            width: double.infinity,
            padding: const EdgeInsets.all(IConfess.space4),
            decoration: BoxDecoration(
              color: surfaces.surfaceRaised,
              borderRadius: BorderRadius.circular(IConfess.radiusMd),
            ),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Row(
                  children: [
                    Expanded(
                      child: Text(
                        s.reference.isNotEmpty ? s.reference : s.book,
                        style: IConfess.subheading
                            .copyWith(color: surfaces.textPrimary),
                      ),
                    ),
                    if (s.translation.isNotEmpty)
                      Text(
                        s.translation,
                        style: IConfess.caption
                            .copyWith(color: surfaces.textSecondary),
                      ),
                  ],
                ),
                if (s.isDirectQuote)
                  Padding(
                    padding: const EdgeInsets.only(top: IConfess.space1),
                    child: Text(
                      'Direct quote',
                      style: IConfess.caption
                          .copyWith(color: surfaces.textSecondary),
                    ),
                  ),
                if (s.notes.isNotEmpty) ...[
                  const SizedBox(height: IConfess.space2),
                  Text(
                    s.notes,
                    style: IConfess.bodySm
                        .copyWith(color: surfaces.textSecondary),
                  ),
                ],
              ],
            ),
          ),
          const SizedBox(height: IConfess.space3),
        ],
      ],
    );
  }
}

class _Tags extends StatelessWidget {
  const _Tags({required this.tags});
  final List<String> tags;

  @override
  Widget build(BuildContext context) {
    final surfaces = AppSurfaces.of(context);
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        const SectionHeader('Tags'),
        Wrap(
          spacing: IConfess.space2,
          runSpacing: IConfess.space2,
          children: [
            for (final tag in tags)
              Container(
                padding: const EdgeInsets.symmetric(
                  horizontal: IConfess.space3,
                  vertical: IConfess.space1,
                ),
                decoration: BoxDecoration(
                  color: surfaces.surfaceRaised,
                  borderRadius: BorderRadius.circular(IConfess.radiusFull),
                ),
                child: Text(
                  '#$tag',
                  style: IConfess.caption
                      .copyWith(color: surfaces.textSecondary),
                ),
              ),
          ],
        ),
      ],
    );
  }
}

class _Actions extends StatelessWidget {
  const _Actions({
    required this.confession,
    required this.isFavorite,
    required this.favoriteBusy,
    required this.onFavoriteToggle,
  });

  final Confession confession;
  final bool isFavorite;
  final bool favoriteBusy;
  final VoidCallback onFavoriteToggle;

  @override
  Widget build(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        FilledButton.icon(
          key: const ValueKey('btn-build-session'),
          onPressed: () {
            // Into the builder with this confession as the starting point:
            // the confess tab reads the parameter, pre-selects the
            // confession's category and says so on screen.
            context.go('${AppRoutes.confess}?confession=${Uri.encodeComponent(confession.id)}');
          },
          icon: const Icon(Icons.graphic_eq_rounded),
          label: const Text('Build a session with this'),
        ),
        const SizedBox(height: IConfess.space3),
        OutlinedButton.icon(
          key: const ValueKey('btn-toggle-fav'),
          onPressed: favoriteBusy ? null : onFavoriteToggle,
          icon: Icon(isFavorite
              ? Icons.favorite_rounded
              : Icons.favorite_border_rounded),
          label: Text(isFavorite ? 'Remove from favourites' : 'Add to favourites'),
        ),
        const SizedBox(height: IConfess.space3),
        _AddToCollectionButton(confessionId: confession.id),
      ],
    );
  }
}

/// Offers the listener's collections as destinations for this confession
/// (G-43).
///
/// The POST endpoint and the typed client method had existed since the
/// library shipped; what did not exist was any way to reach them from the app.
/// Filing happens where the item is - while reading it - not on the screen
/// the item is filed into, which is why this control lives here rather than
/// only in the collection.
class _AddToCollectionButton extends ConsumerWidget {
  const _AddToCollectionButton({required this.confessionId});

  final String confessionId;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    return OutlinedButton.icon(
      key: const ValueKey('btn-add-to-collection'),
      onPressed: () => showModalBottomSheet<void>(
        context: context,
        isScrollControlled: true,
        builder: (_) => _AddToCollectionSheet(confessionId: confessionId),
      ),
      icon: const Icon(Icons.playlist_add_rounded),
      label: const Text('Add to a collection'),
    );
  }
}

class _AddToCollectionSheet extends ConsumerWidget {
  const _AddToCollectionSheet({required this.confessionId});

  final String confessionId;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final async = ref.watch(libraryCollectionsProvider);
    final surfaces = AppSurfaces.of(context);

    return SafeArea(
      child: Padding(
        padding: const EdgeInsets.fromLTRB(
            IConfess.space4, IConfess.space4, IConfess.space4, IConfess.space2),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            Text('Add to a collection',
                style: IConfess.subheading.copyWith(color: surfaces.textPrimary)),
            const SizedBox(height: IConfess.space3),
            Flexible(
              child: async.when(
                loading: () => const Padding(
                  padding: EdgeInsets.all(IConfess.space6),
                  child: Center(child: CircularProgressIndicator()),
                ),
                // A failure here is usually "there is no session": the sheet
                // says what to do about it instead of spinning or vanishing.
                error: (_, _) => const Padding(
                  padding: EdgeInsets.all(IConfess.space6),
                  child: Text('Sign in to organise your collections.'),
                ),
                data: (loadable) => switch (loadable) {
                  LoadFailed() => const Padding(
                      padding: EdgeInsets.all(IConfess.space6),
                      child: Text('Sign in to organise your collections.'),
                    ),
                  LoadLoaded(:final value) when value.isEmpty => const Padding(
                      padding: EdgeInsets.all(IConfess.space6),
                      child: Text(
                        'No collections yet. Create one in the Library and '
                        'this list fills up.',
                      ),
                    ),
                  LoadLoaded(:final value) => ListView(
                      shrinkWrap: true,
                      children: [
                        for (final collection in value)
                          ListTile(
                            key: ValueKey('pick-collection-${collection.id}'),
                            leading: const Icon(Icons.collections_bookmark_rounded),
                            title: Text(collection.name),
                            subtitle: Text(collection.itemCount == 1
                                ? '1 confession'
                                : '${collection.itemCount} confessions'),
                            onTap: () => _add(context, ref, collection.id),
                          ),
                      ],
                    ),
                  _ => const SizedBox.shrink(),
                },
              ),
            ),
          ],
        ),
      ),
    );
  }

  Future<void> _add(BuildContext context, WidgetRef ref, String collectionId) async {
    final messenger = ScaffoldMessenger.of(context);
    final result =
        await ref.read(libraryActionsProvider).addToCollection(collectionId, confessionId);
    if (context.mounted) Navigator.of(context).pop();
    result.when(
      success: (_) => messenger.showSnackBar(
        const SnackBar(content: Text('Added to collection')),
      ),
      failure: (error) => messenger.showSnackBar(
        SnackBar(content: Text(ErrorMapper.describe(error).message)),
      ),
    );
  }
}

class _IntensityBadge extends StatelessWidget {
  const _IntensityBadge({required this.level});
  final int level;

  @override
  Widget build(BuildContext context) {
    final clamped = level.clamp(0, 5);
    return Container(
      padding: const EdgeInsets.symmetric(
        horizontal: IConfess.space2,
        vertical: IConfess.space1,
      ),
      decoration: BoxDecoration(
        color: AppSurfaces.of(context).surfaceRaised,
        borderRadius: BorderRadius.circular(IConfess.radiusFull),
      ),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          for (var i = 0; i < 5; i++)
            Padding(
              padding: const EdgeInsets.only(left: 2),
              child: Container(
                width: 6,
                height: 6,
                decoration: BoxDecoration(
                  shape: BoxShape.circle,
                  color: i < clamped
                      ? IConfess.colorBrand500
                      : IConfess.colorBrand500.withValues(alpha: 0.2),
                ),
              ),
            ),
        ],
      ),
    );
  }
}
