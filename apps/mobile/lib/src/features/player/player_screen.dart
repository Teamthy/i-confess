import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../core/routing/routes.dart';
import '../../core/theme/theme.dart';
import '../../core/theme/tokens.dart';
import '../../core/widgets/screen.dart';
import 'player_providers.dart';

/// The immersive production player (PHASES 24–26).
///
/// Full-screen, above the tab bar. The Session Engine owns the business lifecycle
/// state. Shows the queue as a snapshot (G-1), current item, progress, controls,
/// and handles locked items with an actionable upgrade affordance.
class PlayerScreen extends ConsumerWidget {
  const PlayerScreen({required this.sessionId, super.key});

  final String sessionId;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final engine = ref.read(sessionEngineProvider(sessionId).notifier);
    final state = ref.watch(sessionEngineProvider(sessionId));

    return AppScaffold(
      immersive: true,
      scrollable: false,
      body: Builder(
        builder: (context) {
          final surfaces =
              Theme.of(context).extension<AppSurfaces>() ?? AppSurfaces.paletteToken;

          // 1. Loading state
          if (state.isLoading) {
            return Center(
              child: Column(
                mainAxisAlignment: MainAxisAlignment.center,
                children: [
                  const CircularProgressIndicator(),
                  const SizedBox(height: IConfess.space4),
                  Text(
                    'Preparing your confession...',
                    style: IConfess.body.copyWith(color: surfaces.textSecondary),
                  ),
                ],
              ),
            );
          }

          // 2. Actionable error state
          if (state.isError && state.items.isEmpty) {
            return Center(
              child: Padding(
                padding: const EdgeInsets.all(IConfess.space6),
                child: Column(
                  mainAxisAlignment: MainAxisAlignment.center,
                  children: [
                    const Icon(Icons.error_outline_rounded, size: 48, color: Colors.redAccent),
                    const SizedBox(height: IConfess.space3),
                    Text(
                      'Could not load session',
                      style: IConfess.subheading.copyWith(color: surfaces.textPrimary),
                    ),
                    const SizedBox(height: IConfess.space2),
                    Text(
                      state.errorMessage.isNotEmpty
                          ? state.errorMessage
                          : 'A network or audio error occurred.',
                      textAlign: TextAlign.center,
                      style: IConfess.bodySm.copyWith(color: surfaces.textSecondary),
                    ),
                    const SizedBox(height: IConfess.space5),
                    Row(
                      mainAxisAlignment: MainAxisAlignment.center,
                      children: [
                        OutlinedButton(
                          onPressed: () => context.pop(),
                          child: const Text('Back'),
                        ),
                        const SizedBox(width: IConfess.space3),
                        FilledButton(
                          onPressed: engine.retry,
                          child: const Text('Retry'),
                        ),
                      ],
                    ),
                  ],
                ),
              ),
            );
          }

          // 3. Empty state
          if (state.items.isEmpty) {
            return Center(
              child: Padding(
                padding: const EdgeInsets.all(IConfess.space6),
                child: Column(
                  mainAxisAlignment: MainAxisAlignment.center,
                  children: [
                    Text(
                      'Session is empty',
                      style: IConfess.subheading.copyWith(color: surfaces.textPrimary),
                    ),
                    const SizedBox(height: IConfess.space3),
                    Text(
                      'No confessions were found in this session.',
                      textAlign: TextAlign.center,
                      style: IConfess.bodySm.copyWith(color: surfaces.textSecondary),
                    ),
                    const SizedBox(height: IConfess.space5),
                    FilledButton(
                      onPressed: () => context.go(AppRoutes.explore),
                      child: const Text('Explore Confessions'),
                    ),
                  ],
                ),
              ),
            );
          }

          // 4. Completed state
          if (state.isCompleted) {
            return Center(
              child: Padding(
                padding: const EdgeInsets.all(IConfess.space6),
                child: Column(
                  mainAxisAlignment: MainAxisAlignment.center,
                  children: [
                    const Icon(Icons.check_circle_rounded, size: 64, color: IConfess.colorBrand500),
                    const SizedBox(height: IConfess.space4),
                    Text(
                      'Session Complete',
                      style: IConfess.heading.copyWith(color: surfaces.textPrimary),
                    ),
                    const SizedBox(height: IConfess.space2),
                    Text(
                      'You have completed speaking God\'s Word over your life today.',
                      textAlign: TextAlign.center,
                      style: IConfess.body.copyWith(color: surfaces.textSecondary),
                    ),
                    const SizedBox(height: IConfess.space3),
                    Text(
                      '${state.completedItemIds.length} of ${state.items.length} confessions spoken',
                      style: IConfess.caption.copyWith(color: IConfess.colorBrand500),
                    ),
                    const SizedBox(height: IConfess.space6),
                    FilledButton(
                      onPressed: () => context.pop(),
                      child: const Text('Done'),
                    ),
                  ],
                ),
              ),
            );
          }

          // 5. Active playback layout
          final items = state.items;
          final currentIndex = state.currentIndex.clamp(0, items.length - 1);
          final currentItem = state.currentItem;
          final durationSec = currentItem?.durationSeconds ?? 0;
          final maxMs = durationSec > 0 ? durationSec * 1000 : 1000;

          return Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              // Top bar: close and queue counter
              Row(
                children: [
                  IconButton(
                    icon: const Icon(Icons.close_rounded),
                    onPressed: () => context.pop(),
                  ),
                  const Spacer(),
                  Text(
                    '${currentIndex + 1} / ${items.length}',
                    style: IConfess.caption.copyWith(color: surfaces.textSecondary),
                  ),
                ],
              ),
              const SizedBox(height: IConfess.space4),

              // Active error banner (if transient error)
              if (state.errorMessage.isNotEmpty) ...[
                Container(
                  width: double.infinity,
                  padding: const EdgeInsets.symmetric(
                    horizontal: IConfess.space4,
                    vertical: IConfess.space2,
                  ),
                  margin: const EdgeInsets.only(bottom: IConfess.space3),
                  decoration: BoxDecoration(
                    color: Colors.amber.withValues(alpha: 0.15),
                    borderRadius: BorderRadius.circular(IConfess.radiusMd),
                  ),
                  child: Row(
                    children: [
                      const Icon(Icons.info_outline_rounded, size: 16, color: Colors.amber),
                      const SizedBox(width: IConfess.space2),
                      Expanded(
                        child: Text(
                          state.errorMessage,
                          style: IConfess.caption.copyWith(color: surfaces.textPrimary),
                        ),
                      ),
                      TextButton(
                        onPressed: engine.retry,
                        child: const Text('Retry'),
                      ),
                    ],
                  ),
                ),
              ],

              // Current confession card
              if (currentItem != null) ...[
                Container(
                  width: double.infinity,
                  padding: const EdgeInsets.all(IConfess.space6),
                  decoration: BoxDecoration(
                    color: surfaces.surfaceRaised,
                    borderRadius: BorderRadius.circular(IConfess.radiusLg),
                  ),
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text(
                        currentItem.title.isNotEmpty ? currentItem.title : 'Confession',
                        style: IConfess.heading.copyWith(color: surfaces.textPrimary),
                      ),
                      const SizedBox(height: IConfess.space3),
                      Text(
                        currentItem.text,
                        style: IConfess.body.copyWith(
                          fontFamily: IConfess.body.fontFamily,
                          color: surfaces.textPrimary,
                          height: 1.6,
                        ),
                      ),
                      if (currentItem.locked) ...[
                        const SizedBox(height: IConfess.space4),
                        Container(
                          padding: const EdgeInsets.all(IConfess.space3),
                          decoration: BoxDecoration(
                            color: IConfess.colorAccentGold.withValues(alpha: 0.15),
                            borderRadius: BorderRadius.circular(IConfess.radiusMd),
                          ),
                          child: Column(
                            crossAxisAlignment: CrossAxisAlignment.start,
                            children: [
                              Row(
                                children: [
                                  const Icon(Icons.lock_rounded, size: 18),
                                  const SizedBox(width: IConfess.space2),
                                  Expanded(
                                    child: Text(
                                      currentItem.lockReason.isNotEmpty
                                          ? currentItem.lockReason
                                          : 'Premium subscription required',
                                      style: IConfess.bodySm.copyWith(color: surfaces.textPrimary),
                                    ),
                                  ),
                                ],
                              ),
                              const SizedBox(height: IConfess.space2),
                              Align(
                                alignment: Alignment.centerRight,
                                child: TextButton(
                                  onPressed: () => context.push(AppRoutes.premium),
                                  child: const Text('Upgrade to Premium'),
                                ),
                              ),
                            ],
                          ),
                        ),
                      ],
                    ],
                  ),
                ),
              ],

              const Spacer(),

              // Progress slider
              Column(
                children: [
                  Slider(
                    value: state.positionMs.toDouble().clamp(0, maxMs.toDouble()),
                    min: 0,
                    max: maxMs.toDouble(),
                    onChanged: (v) => engine.seek(v.toInt()),
                  ),
                  Padding(
                    padding: const EdgeInsets.symmetric(horizontal: IConfess.space4),
                    child: Row(
                      mainAxisAlignment: MainAxisAlignment.spaceBetween,
                      children: [
                        Text(
                          _formatMs(state.positionMs),
                          style: IConfess.caption.copyWith(color: surfaces.textSecondary),
                        ),
                        Text(
                          _formatMs(maxMs),
                          style: IConfess.caption.copyWith(color: surfaces.textSecondary),
                        ),
                      ],
                    ),
                  ),
                ],
              ),

              const SizedBox(height: IConfess.space3),

              // Control buttons: Previous, Play/Pause, Next/Skip
              Row(
                mainAxisAlignment: MainAxisAlignment.spaceEvenly,
                children: [
                  IconButton(
                    icon: const Icon(Icons.skip_previous_rounded),
                    iconSize: 36,
                    onPressed: currentIndex > 0 || state.positionMs > 3000
                        ? engine.previous
                        : null,
                  ),
                  FilledButton(
                    onPressed: () {
                      if (state.isPlaying) {
                        engine.pause();
                      } else {
                        engine.play();
                      }
                    },
                    style: FilledButton.styleFrom(
                      shape: const CircleBorder(),
                      padding: const EdgeInsets.all(IConfess.space5),
                    ),
                    child: Icon(
                      state.isPlaying ? Icons.pause_rounded : Icons.play_arrow_rounded,
                      size: 36,
                    ),
                  ),
                  IconButton(
                    icon: const Icon(Icons.skip_next_rounded),
                    iconSize: 36,
                    onPressed: currentIndex < items.length - 1
                        ? engine.skip
                        : engine.complete,
                  ),
                ],
              ),

              const SizedBox(height: IConfess.space5),

              // Queue peek (horizontal rail)
              if (items.length > 1)
                SizedBox(
                  height: 80,
                  child: ListView.separated(
                    scrollDirection: Axis.horizontal,
                    itemCount: items.length,
                    separatorBuilder: (_, _) => const SizedBox(width: IConfess.space2),
                    itemBuilder: (context, idx) {
                      final item = items[idx];
                      final isCurrent = idx == currentIndex;
                      final isCompleted = state.completedItemIds.contains(item.id);
                      final isSkipped = state.skippedItemIds.contains(item.id);

                      return InkWell(
                        onTap: () => engine.selectQueueItem(idx),
                        borderRadius: BorderRadius.circular(IConfess.radiusMd),
                        child: Container(
                          width: 160,
                          padding: const EdgeInsets.all(IConfess.space3),
                          decoration: BoxDecoration(
                            color: isCurrent ? IConfess.colorBrand500 : surfaces.surfaceRaised,
                            borderRadius: BorderRadius.circular(IConfess.radiusMd),
                            border: isCurrent
                                ? Border.all(color: IConfess.colorBrand700, width: 2)
                                : null,
                          ),
                          child: Column(
                            crossAxisAlignment: CrossAxisAlignment.start,
                            children: [
                              Row(
                                children: [
                                  Expanded(
                                    child: Text(
                                      item.title.isNotEmpty ? item.title : 'Item ${idx + 1}',
                                      maxLines: 1,
                                      overflow: TextOverflow.ellipsis,
                                      style: IConfess.bodySm.copyWith(
                                        color: isCurrent ? Colors.white : surfaces.textPrimary,
                                        fontWeight:
                                            isCurrent ? FontWeight.w600 : FontWeight.w400,
                                      ),
                                    ),
                                  ),
                                  if (isCompleted)
                                    Icon(
                                      Icons.check_circle_rounded,
                                      size: 14,
                                      color: isCurrent ? Colors.white : IConfess.colorBrand500,
                                    )
                                  else if (isSkipped)
                                    Icon(
                                      Icons.skip_next_rounded,
                                      size: 14,
                                      color: isCurrent ? Colors.white70 : surfaces.textSecondary,
                                    )
                                  else if (item.locked)
                                    Icon(
                                      Icons.lock_rounded,
                                      size: 14,
                                      color: isCurrent ? Colors.white : surfaces.textSecondary,
                                    ),
                                ],
                              ),
                              const SizedBox(height: IConfess.space1),
                              Text(
                                '${item.durationSeconds ~/ 60}:${(item.durationSeconds % 60).toString().padLeft(2, '0')}',
                                style: IConfess.caption.copyWith(
                                  color: isCurrent
                                      ? Colors.white.withValues(alpha: 0.85)
                                      : surfaces.textSecondary,
                                ),
                              ),
                            ],
                          ),
                        ),
                      );
                    },
                  ),
                ),
            ],
          );
        },
      ),
    );
  }

  String _formatMs(int ms) {
    final totalSec = ms ~/ 1000;
    final min = totalSec ~/ 60;
    final sec = totalSec % 60;
    return '$min:${sec.toString().padLeft(2, '0')}';
  }
}
