import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:iconfess_api/iconfess_api.dart';

import '../../core/error/error_mapper.dart';
import '../../core/theme/theme.dart';
import '../../core/theme/tokens.dart';
import '../../core/widgets/screen.dart';
import '../../core/widgets/states.dart';
import 'explore_providers.dart';

/// One category's confessions.
///
/// The list leads with each confession's shortest usable line and its
/// intensity, which is what a listener actually chooses between. The full text
/// stays server-side until the confession experience (PHASE 22) needs it.
class CategoryDetailScreen extends ConsumerWidget {
  const CategoryDetailScreen({required this.categoryId, super.key});

  final String categoryId;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final confessions =
        ref.watch(categoryConfessionsProvider(categoryId));

    return AppScaffold(
      title: 'Category',
      body: confessions.when(
        loading: () => const ListSkeleton(rows: 4),
        error: (error, _) => ErrorState(
          error: const UserFacingError(
            title: 'The confessions did not load',
            message: 'Check your connection and try again.',
            primaryAction: ErrorAction.retry,
            retryable: true,
          ),
          onAction: (_) =>
              ref.invalidate(categoryConfessionsProvider(categoryId)),
        ),
        data: (loadable) {
          final list = loadable.valueOrNull ?? const <Confession>[];
          if (list.isEmpty) {
            return EmptyState(
              title: 'Nothing here yet',
              message: 'This category is being written.',
            );
          }
          return Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              for (final confession in list) ...[
                _ConfessionRow(confession: confession),
                const SizedBox(height: IConfess.space3),
              ],
            ],
          );
        },
      ),
    );
  }
}

class _ConfessionRow extends StatelessWidget {
  const _ConfessionRow({required this.confession});
  final Confession confession;

  @override
  Widget build(BuildContext context) {
    final surfaces = AppSurfaces.of(context);
    return DecoratedBox(
      decoration: BoxDecoration(
        color: surfaces.surfaceRaised,
        borderRadius: BorderRadius.circular(IConfess.radiusMd),
      ),
      child: Padding(
        padding: const EdgeInsets.all(IConfess.space4),
        child: Row(
          children: [
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    confession.title.isNotEmpty
                        ? confession.title
                        : confession.lead,
                    maxLines: 2,
                    overflow: TextOverflow.ellipsis,
                    style:
                        IConfess.subheading.copyWith(color: surfaces.textPrimary),
                  ),
                  if (confession.title.isNotEmpty &&
                      confession.lead.isNotEmpty)
                    Text(
                      confession.lead,
                      maxLines: 2,
                      overflow: TextOverflow.ellipsis,
                      style: IConfess.bodySm
                          .copyWith(color: surfaces.textSecondary),
                    ),
                ],
              ),
            ),
            const SizedBox(width: IConfess.space3),
            _IntensityDots(level: confession.intensity),
          ],
        ),
      ),
    );
  }
}

/// Intensity as filled dots, because a number beside a confession reads as a
/// score, and these are not scores.
class _IntensityDots extends StatelessWidget {
  const _IntensityDots({required this.level});
  final int level;

  @override
  Widget build(BuildContext context) {
    final clamped = level.clamp(0, 5);
    return Row(
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
    );
  }
}
