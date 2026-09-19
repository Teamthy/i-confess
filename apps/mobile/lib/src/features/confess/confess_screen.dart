import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../core/routing/routes.dart';
import '../../core/theme/theme.dart';
import '../../core/theme/tokens.dart';
import '../../core/widgets/screen.dart';
import '../../core/widgets/states.dart';
import 'confess_providers.dart';

/// The builder's first step: what to speak over your life.
///
/// The confess tab is the primary action of the product (§12). It is not a
/// feed and not a library; it is where a listener chooses the areas of life
/// they want to speak Scripture over, then picks a duration and a voice.
///
/// PHASE 22 ships the first step only — category selection — with the chosen
/// ids held locally. PHASE 23 adds duration, voice and the create call. The
/// screen already shows the selected count and routes to the next step, so
/// the flow is navigable even while the next steps are placeholders.
class ConfessScreen extends ConsumerWidget {
  const ConfessScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final categoriesAsync = ref.watch(confessCategoriesProvider);
    final selected = ref.watch(selectedCategoriesProvider);
    final surfaces = AppSurfaces.of(context);

    return AppScaffold(
      title: 'Confess',
      bottom: selected.isNotEmpty
          ? FilledButton(
              key: const ValueKey('btn-continue'),
              onPressed: () {
                // PHASE 23 will read the selected ids and show duration.
                // For now we route to the voices placeholder so the flow has
                // somewhere to go and the test can assert the selection survives.
                context.go(AppRoutes.voices);
              },
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
          categoriesAsync.when(
            loading: () => const ListSkeleton(rows: 4),
            error: (error, _) => ErrorState(
              error: const UserFacingError(
                title: 'Categories did not load',
                message: 'Check your connection and try again.',
                primaryAction: ErrorAction.retry,
                retryable: true,
              ),
              onAction: (_) =>
                  ref.invalidate(confessCategoriesProvider),
            ),
            data: (loadable) {
              final categories = loadable.valueOrNull ?? const [];
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
                        final current =
                            ref.read(selectedCategoriesProvider);
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
