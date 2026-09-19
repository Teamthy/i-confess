import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:iconfess_api/iconfess_api.dart';

import '../../core/routing/routes.dart';
import '../../core/theme/theme.dart';
import '../../core/theme/tokens.dart';
import '../../core/widgets/screen.dart';
import '../../core/widgets/states.dart';
import '../confession/confession_providers.dart';
import 'confess_providers.dart';

/// The builder's first step: what to speak over your life.
///
/// The confess tab is the primary action of the product (§12). It is not a
/// feed and not a library; it is where a listener chooses the areas of life
/// they want to speak Scripture over, then picks a duration and a voice.
///
/// PHASE 23 completes the step: Continue now leads to the duration step, and
/// a confession that arrived via "Build a session with this"
/// (`/confess?confession=<id>`) pre-selects its category and says so, rather
/// than dropping the listener back at an empty slate that quietly forgets
/// where they came from.
class ConfessScreen extends ConsumerWidget {
  const ConfessScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final categoriesAsync = ref.watch(confessCategoriesProvider);
    final selected = ref.watch(selectedCategoriesProvider);
    final surfaces = AppSurfaces.of(context);

    // The confession the detail screen handed over, if any. Read from the
    // route rather than passed in memory: a deep link into the builder
    // carries the same parameter, and a rebuilt widget tree does not lose it.
    final incomingId = GoRouterState.of(context).uri.queryParameters['confession'];
    final incoming = incomingId == null ? null : ref.watch(confessionProvider(incomingId));

    // When the handed-over confession loads, fold its category into the
    // selection. listen, not watch: the mutation happens once, on arrival —
    // a build that wrote state would loop.
    if (incomingId != null) {
      ref.listen(confessionProvider(incomingId), (_, next) {
        final loadable = next.asData?.value;
        final categoryId = loadable?.valueOrNull?.categoryId;
        if (categoryId != null && categoryId.isNotEmpty) {
          final current = ref.read(selectedCategoriesProvider);
          if (!current.contains(categoryId)) {
            ref.read(selectedCategoriesProvider.notifier).state = {...current, categoryId};
          }
        }
      });
    }

    return AppScaffold(
      title: 'Confess',
      bottom: selected.isNotEmpty
          ? FilledButton(
              key: const ValueKey('btn-continue'),
              onPressed: () => context.go(AppRoutes.builderDuration),
              child: Text(
                'Continue with ${selected.length} ${selected.length == 1 ? 'area' : 'areas'}',
              ),
            )
          : null,
      body: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(
            'What do you want to speak over your life today?',
            style: IConfess.heading.copyWith(color: surfaces.textPrimary),
          ),
          const SizedBox(height: IConfess.space2),
          Text(
            'Choose one or more areas. You can change the duration and voice next.',
            style: IConfess.bodySm.copyWith(color: surfaces.textSecondary),
          ),
          const SizedBox(height: IConfess.space6),
          if (incomingId != null)
            incoming.when(
              loading: () => const ListSkeleton(rows: 1),
              error: (error, _) => _HandoffBanner(
                key: const ValueKey('handoff-banner'),
                title: 'Building from a confession',
                subtitle: 'Its category is ready below — add more if you like.',
                onDismiss: () => context.go(AppRoutes.confess),
              ),
              data: (loadable) {
                final confession = loadable.valueOrNull;
                return _HandoffBanner(
                  key: const ValueKey('handoff-banner'),
                  title: confession == null || confession.title.isEmpty
                      ? 'Building from a confession'
                      : 'Building from "${confession.title}"',
                  subtitle: 'Its category is ready below — add more if you like.',
                  onDismiss: () => context.go(AppRoutes.confess),
                );
              },
            ),
          categoriesAsync.when(
            loading: () => const ListSkeleton(rows: 4),
            error: (error, _) => ErrorState(
              error: const UserFacingError(
                title: 'Categories did not load',
                message: 'Check your connection and try again.',
                primaryAction: ErrorAction.retry,
                retryable: true,
              ),
              onAction: (_) => ref.invalidate(confessCategoriesProvider),
            ),
            data: (loadable) {
              final categories = loadable.valueOrNull ?? const <Category>[];
              if (categories.isEmpty) {
                return const EmptyState(
                  title: 'Categories are on the way',
                  message: 'The catalogue is being written.',
                );
              }
              return Wrap(
                spacing: IConfess.space2,
                runSpacing: IConfess.space2,
                children: [
                  for (final cat in categories)
                    FilterChip(
                      key: ValueKey('chip-${cat.id}'),
                      label: Text(cat.name),
                      selected: selected.contains(cat.id),
                      onSelected: (isSelected) {
                        final current = ref.read(selectedCategoriesProvider);
                        final next = Set<String>.from(current);
                        if (isSelected) {
                          next.add(cat.id);
                        } else {
                          next.remove(cat.id);
                        }
                        ref
                            .read(selectedCategoriesProvider.notifier)
                            .state = next;
                      },
                    ),
                ],
              );
            },
          ),
        ],
      ),
    );
  }
}

/// Says where the builder came from, and offers a way back to a clean slate.
class _HandoffBanner extends StatelessWidget {
  const _HandoffBanner({
    super.key,
    required this.title,
    required this.subtitle,
    required this.onDismiss,
  });

  final String title;
  final String subtitle;
  final VoidCallback onDismiss;

  @override
  Widget build(BuildContext context) {
    final surfaces = AppSurfaces.of(context);
    return Container(
      width: double.infinity,
      margin: const EdgeInsets.only(bottom: IConfess.space4),
      padding: const EdgeInsets.all(IConfess.space3),
      decoration: BoxDecoration(
        color: surfaces.surface,
        borderRadius: BorderRadius.circular(IConfess.radiusMd),
        border: Border.all(color: surfaces.border),
      ),
      child: Row(
        children: [
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(title,
                    style: IConfess.body.copyWith(color: surfaces.textPrimary)),
                const SizedBox(height: IConfess.space1),
                Text(subtitle,
                    style:
                        IConfess.bodySm.copyWith(color: surfaces.textSecondary)),
              ],
            ),
          ),
          IconButton(
            key: const ValueKey('btn-dismiss-handoff'),
            tooltip: 'Start from a clean slate',
            onPressed: onDismiss,
            icon: Icon(Icons.close_rounded, size: 18, color: surfaces.textSecondary),
          ),
        ],
      ),
    );
  }
}
