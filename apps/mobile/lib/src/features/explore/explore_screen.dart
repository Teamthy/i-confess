import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:iconfess_api/iconfess_api.dart';

import '../../core/routing/routes.dart';
import '../../core/theme/theme.dart';
import '../../core/theme/tokens.dart';
import '../../core/widgets/screen.dart';
import 'explore_providers.dart';

/// Explore: find something to say.
///
/// Three ways in, matching the brief without overcrowding: a search field that
/// filters the catalogue as you type, a featured rail of published collections,
/// and the full category grid. Search is computed client-side over the
/// categories the server already sent — there is no search endpoint, and
/// fetching every category's confessions to fake one would be thirty-nine calls
/// to answer a keystroke.
class ExploreScreen extends ConsumerStatefulWidget {
  const ExploreScreen({super.key});

  @override
  ConsumerState<ExploreScreen> createState() => _ExploreScreenState();
}

class _ExploreScreenState extends ConsumerState<ExploreScreen> {
  String _query = '';

  @override
  Widget build(BuildContext context) {
    final surfaces = AppSurfaces.of(context);
    final categories = ref.watch(exploreCategoriesProvider);
    final collections = ref.watch(exploreCollectionsProvider);

    final all = categories.asData?.value.valueOrNull ?? const <Category>[];
    final visible = _query.isEmpty
        ? all
        : all
            .where((c) =>
                c.name.toLowerCase().contains(_query.toLowerCase()) ||
                c.description.toLowerCase().contains(_query.toLowerCase()))
            .toList();

    return AppScaffold(
      title: 'Explore',
      body: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          TextField(
            key: const ValueKey('field-explore-search'),
            decoration: InputDecoration(
              hintText: 'Search categories',
              prefixIcon: const Icon(Icons.search_rounded),
              filled: true,
              fillColor: surfaces.surfaceRaised,
              border: OutlineInputBorder(
                borderRadius: BorderRadius.circular(IConfess.radiusFull),
                borderSide: BorderSide.none,
              ),
            ),
            onChanged: (value) => setState(() => _query = value),
          ),
          const SizedBox(height: IConfess.space6),
          _FeaturedRail(state: collections),
          const SizedBox(height: IConfess.space6),
          Text('Categories',
              style: IConfess.subheading.copyWith(color: surfaces.textPrimary)),
          const SizedBox(height: IConfess.space3),
          _CategoryGrid(categories: visible, searching: _query.isNotEmpty),
        ],
      ),
    );
  }
}

/// The horizontal "featured" rail with the card emphasis motion: the card
/// nearest the rail's centre sits slightly larger, and the emphasis eases as
/// the rail scrolls. It is scroll-driven rather than a repeating ticker, so a
/// pump that waits for quiet settles the moment scrolling stops.
class _FeaturedRail extends StatelessWidget {
  const _FeaturedRail({required this.state});
  final AsyncValue<Loadable<List<Collection>>> state;

  @override
  Widget build(BuildContext context) {
    final surfaces = AppSurfaces.of(context);
    return state.when(
      loading: () => const SizedBox.shrink(),
      error: (_, _) => const SizedBox.shrink(),
      data: (loadable) {
        final collections = loadable.valueOrNull ?? const <Collection>[];
        if (collections.isEmpty) return const SizedBox.shrink();
        return Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text('Featured',
                style:
                    IConfess.subheading.copyWith(color: surfaces.textPrimary)),
            const SizedBox(height: IConfess.space3),
            _EmphasisRail<Collection>(
              items: collections,
              width: 200,
              builder: (context, collection, emphasis) {
                return Transform.scale(
                  scale: 0.94 + 0.06 * emphasis,
                  child: _FeaturedCard(collection: collection),
                );
              },
            ),
          ],
        );
      },
    );
  }
}

class _FeaturedCard extends StatelessWidget {
  const _FeaturedCard({required this.collection});
  final Collection collection;

  @override
  Widget build(BuildContext context) {
    return Container(
      width: 200,
      padding: const EdgeInsets.all(IConfess.space4),
      decoration: BoxDecoration(
        gradient: LinearGradient(
          colors: [IConfess.colorBrand500, IConfess.colorBrand700],
          begin: Alignment.topLeft,
          end: Alignment.bottomRight,
        ),
        borderRadius: BorderRadius.circular(IConfess.radiusLg),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        mainAxisAlignment: MainAxisAlignment.spaceBetween,
        children: [
          Text(collection.name,
              maxLines: 2,
              overflow: TextOverflow.ellipsis,
              style: IConfess.subheading.copyWith(color: Colors.white)),
          Text(
            collection.premium ? 'Premium' : 'Free to explore',
            style: IConfess.caption
                .copyWith(color: Colors.white.withValues(alpha: 0.85)),
          ),
        ],
      ),
    );
  }
}

/// A horizontal rail where each card reports how close it is to the centre as
/// a 0..1 emphasis. Scroll notifications drive rebuilds; nothing schedules a
/// frame on its own.
class _EmphasisRail<T> extends StatefulWidget {
  const _EmphasisRail({
    required this.items,
    required this.width,
    required this.builder,
  });

  final List<T> items;
  final double width;
  final Widget Function(BuildContext, T, double emphasis) builder;

  @override
  State<_EmphasisRail<T>> createState() => _EmphasisRailState<T>();
}

class _EmphasisRailState<T> extends State<_EmphasisRail<T>> {
  final _controller = ScrollController();
  double _offset = 0;

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  double _emphasisAt(int index) {
    const gap = IConfess.space3;
    final centre = _offset + 170; // ~half the test viewport width
    final cardCentre = index * (widget.width + gap) + widget.width / 2;
    final distance = (cardCentre - centre).abs();
    final t = 1 - (distance / (widget.width * 1.5)).clamp(0.0, 1.0);
    return t;
  }

  @override
  Widget build(BuildContext context) {
    return NotificationListener<ScrollNotification>(
      onNotification: (notification) {
        if (notification is ScrollUpdateNotification) {
          setState(() => _offset = _controller.hasClients
              ? _controller.offset
              : _offset);
        }
        return false;
      },
      child: SizedBox(
        height: 120,
        child: ListView.separated(
          controller: _controller,
          scrollDirection: Axis.horizontal,
          itemCount: widget.items.length,
          separatorBuilder: (_, _) => const SizedBox(width: IConfess.space3),
          itemBuilder: (context, index) =>
              widget.builder(context, widget.items[index], _emphasisAt(index)),
        ),
      ),
    );
  }
}

class _CategoryGrid extends StatelessWidget {
  const _CategoryGrid({required this.categories, required this.searching});
  final List<Category> categories;
  final bool searching;

  @override
  Widget build(BuildContext context) {
    final surfaces = AppSurfaces.of(context);
    if (categories.isEmpty) {
      return Padding(
        padding: const EdgeInsets.symmetric(vertical: IConfess.space6),
        child: Text(
          searching ? 'Nothing matches that yet.' : 'Categories are on the way.',
          style: IConfess.bodySm.copyWith(color: surfaces.textSecondary),
        ),
      );
    }
    return Wrap(
      spacing: IConfess.space2,
      runSpacing: IConfess.space2,
      children: [
        for (final category in categories)
          ActionChip(
            label: Text(category.name),
            onPressed: () =>
                context.go(AppRoutes.categoryDetail(category.id)),
          ),
      ],
    );
  }
}
