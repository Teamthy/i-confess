import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../core/theme/theme.dart';
import '../../core/theme/tokens.dart';
import '../../core/widgets/screen.dart';
import 'player_providers.dart';

/// The immersive player (PHASE 24).
///
/// Full-screen, above the tab bar. Shows the queue as a snapshot (G-1: queue
/// must not change mid-session), current item, progress, controls, and handles
/// locked items by showing upgrade affordance rather than silent skip.
///
/// No real audio engine yet (just_audio commented per D-4); controls drive the
/// state machine and server sync, which is what matters for the lifecycle.
class PlayerScreen extends ConsumerWidget {
  const PlayerScreen({required this.sessionId, super.key});

  final String sessionId;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final sessionAsync = ref.watch(playerSessionProvider(sessionId));
    final playerState = ref.watch(playerControllerProvider);
    final surfaces = AppSurfaces.of(context);

    return AppScaffold(
      immersive: true,
      body: sessionAsync.when(
        loading: () => const Center(child: CircularProgressIndicator()),
        error: (e, _) => Center(
          child: Column(
            mainAxisAlignment: MainAxisAlignment.center,
            children: [
              Text('Could not load session',
                  style: IConfess.subheading.copyWith(color: surfaces.textPrimary)),
              const SizedBox(height: IConfess.space2),
              Text(e.toString(),
                  style: IConfess.bodySm.copyWith(color: surfaces.textSecondary)),
              const SizedBox(height: IConfess.space4),
              FilledButton(
                onPressed: () => ref.invalidate(playerSessionProvider(sessionId)),
                child: const Text('Retry'),
              ),
            ],
          ),
        ),
        data: (loadable) {
          final session = loadable.valueOrNull;
          if (session == null) {
            return Center(
              child: Text('Session not found',
                  style: IConfess.body.copyWith(color: surfaces.textPrimary)),
            );
          }
          final items = session.items;
          final currentIndex = playerState.currentIndex.clamp(0, items.length - 1);
          final currentItem = items.isNotEmpty ? items[currentIndex] : null;

          return Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
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
              const SizedBox(height: IConfess.space6),
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
                          fontFamily: IConfess.fontSerif,
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
                          child: Row(
                            children: [
                              const Icon(Icons.lock_rounded, size: 18),
                              const SizedBox(width: IConfess.space2),
                              Expanded(
                                child: Text(
                                  currentItem.lockReason.isNotEmpty
                                      ? currentItem.lockReason
                                      : 'Premium voice requires subscription',
                                  style: IConfess.bodySm.copyWith(color: surfaces.textPrimary),
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
              // Progress
              Column(
                children: [
                  Slider(
                    value: playerState.positionMs.toDouble().clamp(
                        0, (currentItem?.durationSeconds ?? 1) * 1000.toDouble()),
                    min: 0,
                    max: (currentItem?.durationSeconds ?? 1) * 1000.toDouble(),
                    onChanged: (v) =>
                        ref.read(playerControllerProvider.notifier).seek(v.toInt()),
                  ),
                  Padding(
                    padding: const EdgeInsets.symmetric(horizontal: IConfess.space4),
                    child: Row(
                      mainAxisAlignment: MainAxisAlignment.spaceBetween,
                      children: [
                        Text(
                          _formatMs(playerState.positionMs),
                          style: IConfess.caption.copyWith(color: surfaces.textSecondary),
                        ),
                        Text(
                          _formatMs((currentItem?.durationSeconds ?? 0) * 1000),
                          style: IConfess.caption.copyWith(color: surfaces.textSecondary),
                        ),
                      ],
                    ),
                  ),
                ],
              ),
              const SizedBox(height: IConfess.space4),
              // Controls
              Row(
                mainAxisAlignment: MainAxisAlignment.spaceEvenly,
                children: [
                  IconButton(
                    icon: const Icon(Icons.skip_previous_rounded),
                    iconSize: 36,
                    onPressed: currentIndex > 0
                        ? () => ref.read(playerControllerProvider.notifier).state =
                            playerState.copyWith(currentIndex: currentIndex - 1, positionMs: 0)
                        : null,
                  ),
                  FilledButton(
                    onPressed: () {
                      final ctrl = ref.read(playerControllerProvider.notifier);
                      if (playerState.status == PlayerStatus.playing) {
                        ctrl.pause(sessionId);
                      } else {
                        ctrl.resume(sessionId);
                      }
                    },
                    style: FilledButton.styleFrom(
                      shape: const CircleBorder(),
                      padding: const EdgeInsets.all(IConfess.space5),
                    ),
                    child: Icon(
                      playerState.status == PlayerStatus.playing
                          ? Icons.pause_rounded
                          : Icons.play_arrow_rounded,
                      size: 36,
                    ),
                  ),
                  IconButton(
                    icon: const Icon(Icons.skip_next_rounded),
                    iconSize: 36,
                    onPressed: currentIndex < items.length - 1
                        ? () => ref.read(playerControllerProvider.notifier).skip(sessionId)
                        : () => ref.read(playerControllerProvider.notifier).complete(sessionId),
                  ),
                ],
              ),
              const SizedBox(height: IConfess.space6),
              // Queue peek
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
                      return Container(
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
                            Text(
                              item.title.isNotEmpty ? item.title : 'Item ${idx + 1}',
                              maxLines: 1,
                              overflow: TextOverflow.ellipsis,
                              style: IConfess.bodySm.copyWith(
                                color: isCurrent ? Colors.white : surfaces.textPrimary,
                                fontWeight: isCurrent ? FontWeight.w600 : FontWeight.w400,
                              ),
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
                            if (item.locked)
                              Padding(
                                padding: const EdgeInsets.only(top: IConfess.space1),
                                child: Icon(
                                  Icons.lock_rounded,
                                  size: 12,
                                  color: isCurrent ? Colors.white : surfaces.textSecondary,
                                ),
                              ),
                          ],
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
