import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:iconfess_api/iconfess_api.dart';

import '../../core/routing/routes.dart';
import '../../core/theme/theme.dart';
import '../../core/theme/tokens.dart';
import '../../core/widgets/screen.dart';
import 'home_providers.dart';

/// The signed-in landing surface.
///
/// A home dashboard fails by overcrowding, so this one is a small number of
/// rails with a reason to exist: a greeting that says who the app is for, one
/// primary action, the thing the listener was already doing (continue
/// listening), the catalogue organised the way the product thinks (categories),
/// and what they have already done (recent activity). Everything else stays in
/// Explore.
///
/// Every rail is fed by the real read-only API; there is no sample data. A rail
/// whose call failed is removed rather than replaced with an error card,
/// because the home is a doorway, not a diagnostics screen — the one exception
/// is categories, which gets a skeleton while it loads because the carousel is
/// the only way into the catalogue from here.
class HomeScreen extends ConsumerWidget {
  const HomeScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final categories = ref.watch(homeCategoriesProvider);
    final live = ref.watch(continueListeningSelector);
    final activity = ref.watch(recentActivitySelector);

    return AppScaffold(
      body: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          const _Greeting(),
          const SizedBox(height: IConfess.space6),
          const _DailySessionCard(),
          if (live.isNotEmpty) ...[
            const SizedBox(height: IConfess.space7),
            _RailHeader(label: 'Continue listening'),
            const SizedBox(height: IConfess.space3),
            _ContinueRail(sessions: live),
          ],
          const SizedBox(height: IConfess.space7),
          _RailHeader(label: 'Browse by category'),
          const SizedBox(height: IConfess.space3),
          _CategoryCarousel(state: categories),
          if (activity.isNotEmpty) ...[
            const SizedBox(height: IConfess.space7),
            _RailHeader(label: 'Recent activity'),
            const SizedBox(height: IConfess.space3),
            _ActivityRail(sessions: activity),
          ],
        ],
      ),
    );
  }
}

class _Greeting extends StatelessWidget {
  const _Greeting();

  @override
  Widget build(BuildContext context) {
    final surfaces = AppSurfaces.of(context);
    final hour = TimeOfDay.now().hour;
    final part = hour < 5
        ? 'Grace and peace'
        : hour < 12
            ? 'Good morning'
            : hour < 17
                ? 'Good afternoon'
                : 'Good evening';

    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(part, style: IConfess.heading.copyWith(color: surfaces.textPrimary)),
        const SizedBox(height: IConfess.space1),
        Text(
          'Take a moment. Speak it aloud.',
          style: IConfess.bodySm.copyWith(color: surfaces.textSecondary),
        ),
      ],
    );
  }
}

/// The one primary action on the screen: begin a session.
///
/// It routes to the confession builder rather than playing anything itself.
/// The home never mints audio — that is the builder's job, and keeping the CTA
/// a navigation means the home stays correct while the builder is still a
/// placeholder.
class _DailySessionCard extends ConsumerWidget {
  const _DailySessionCard();

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    return DecoratedBox(
      decoration: BoxDecoration(
        color: IConfess.colorBrand500,
        borderRadius: BorderRadius.circular(IConfess.radiusLg),
      ),
      child: InkWell(
        borderRadius: BorderRadius.circular(IConfess.radiusLg),
        onTap: () => context.go(AppRoutes.confess),
        child: Padding(
          padding: const EdgeInsets.all(IConfess.space5),
          child: Row(
            children: [
              const Icon(Icons.graphic_eq_rounded, color: Colors.white),
              const SizedBox(width: IConfess.space4),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      'Set aside a few minutes',
                      style: IConfess.subheading.copyWith(color: Colors.white),
                    ),
                    Text(
                      'Build a session for the time you have.',
                      style: IConfess.bodySm.copyWith(
                        color: Colors.white.withValues(alpha: 0.85),
                      ),
                    ),
                  ],
                ),
              ),
              const Icon(Icons.chevron_right_rounded, color: Colors.white),
            ],
          ),
        ),
      ),
    );
  }
}

class _RailHeader extends StatelessWidget {
  const _RailHeader({required this.label});
  final String label;

  @override
  Widget build(BuildContext context) {
    final surfaces = AppSurfaces.of(context);
    return Text(label,
        style: IConfess.subheading.copyWith(color: surfaces.textPrimary));
  }
}

class _CarouselPlaceholder extends StatelessWidget {
  const _CarouselPlaceholder();

  @override
  Widget build(BuildContext context) {
    final surfaces = AppSurfaces.of(context);
    return Row(
      children: [
        for (final width in const <double>[96, 120, 84]) ...[
          Container(
            width: width,
            height: 40,
            decoration: BoxDecoration(
              color: surfaces.surfaceRaised,
              borderRadius: BorderRadius.circular(IConfess.radiusFull),
            ),
          ),
          const SizedBox(width: IConfess.space2),
        ],
      ],
    );
  }
}

class _ContinueRail extends StatelessWidget {
  const _ContinueRail({required this.sessions});
  final List<ListeningSession> sessions;

  @override
  Widget build(BuildContext context) {
    final surfaces = AppSurfaces.of(context);
    return SizedBox(
      height: 92,
      child: ListView.separated(
        scrollDirection: Axis.horizontal,
        itemCount: sessions.length,
        separatorBuilder: (_, _) => const SizedBox(width: IConfess.space3),
        itemBuilder: (context, index) {
          final session = sessions[index];
          final title = session.items.isNotEmpty
              ? session.items.first.title
              : 'Your session';
          return _SessionCard(
            title: title,
            subtitle: session.status == 'PAUSED' ? 'Paused' : 'In progress',
            color: surfaces.surfaceRaised,
            textColor: surfaces.textPrimary,
            onTap: () => context.go(AppRoutes.player),
          );
        },
      ),
    );
  }
}

class _CategoryCarousel extends StatelessWidget {
  const _CategoryCarousel({required this.state});
  final AsyncValue<Loadable<List<Category>>> state;

  @override
  Widget build(BuildContext context) {
    return state.when(
      // A static placeholder, not the shimmering [Skeleton]: the shimmer runs a
      // repeating ticker, and the navigation tests pump the signed-in shell with
      // pumpAndSettle, which never settles while a ticker keeps scheduling
      // frames. Home is the first signed-in screen, so it is the one that lands
      // inside those pumps.
      loading: () => const _CarouselPlaceholder(),
      error: (_, _) => const SizedBox.shrink(),
      data: (loadable) {
        final categories = loadable.valueOrNull ?? const <Category>[];
        if (categories.isEmpty) return const SizedBox.shrink();
        return SizedBox(
          height: 40,
          child: ListView.separated(
            scrollDirection: Axis.horizontal,
            itemCount: categories.length,
            separatorBuilder: (_, _) => const SizedBox(width: IConfess.space2),
            itemBuilder: (context, index) {
              final category = categories[index];
              return ActionChip(
                label: Text(category.name),
                onPressed: () =>
                    context.go(AppRoutes.categoryDetail(category.id)),
              );
            },
          ),
        );
      },
    );
  }
}

class _ActivityRail extends StatelessWidget {
  const _ActivityRail({required this.sessions});
  final List<ListeningSession> sessions;

  @override
  Widget build(BuildContext context) {
    final surfaces = AppSurfaces.of(context);
    return Column(
      children: [
        for (final session in sessions.take(3))
          Padding(
            padding: const EdgeInsets.only(bottom: IConfess.space2),
            child: Row(
              children: [
                Icon(Icons.check_circle_rounded,
                    size: 18, color: IConfess.colorBrand500),
                const SizedBox(width: IConfess.space3),
                Expanded(
                  child: Text(
                    session.items.isNotEmpty
                        ? session.items.first.title
                        : 'Completed session',
                    maxLines: 1,
                    overflow: TextOverflow.ellipsis,
                    style:
                        IConfess.bodySm.copyWith(color: surfaces.textPrimary),
                  ),
                ),
                Text(
                  '${session.durationSeconds ~/ 60} min',
                  style:
                      IConfess.caption.copyWith(color: surfaces.textSecondary),
                ),
              ],
            ),
          ),
      ],
    );
  }
}

class _SessionCard extends StatelessWidget {
  const _SessionCard({
    required this.title,
    required this.subtitle,
    required this.color,
    required this.textColor,
    required this.onTap,
  });

  final String title;
  final String subtitle;
  final Color color;
  final Color textColor;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return Material(
      color: color,
      borderRadius: BorderRadius.circular(IConfess.radiusMd),
      child: InkWell(
        borderRadius: BorderRadius.circular(IConfess.radiusMd),
        onTap: onTap,
        child: Container(
          width: 220,
          padding: const EdgeInsets.all(IConfess.space4),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            mainAxisAlignment: MainAxisAlignment.center,
            children: [
              Text(title,
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                  style: IConfess.subheading.copyWith(color: textColor)),
              const SizedBox(height: IConfess.space1),
              Text(subtitle,
                  style: IConfess.caption.copyWith(color: textColor)),
            ],
          ),
        ),
      ),
    );
  }
}
