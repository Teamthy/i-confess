import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:iconfess_api/iconfess_api.dart';

import '../../core/di/providers.dart';
import '../../core/routing/routes.dart';
import '../../core/theme/tokens.dart';

/// The listener's trial right now. Not cached: it moves every day.
final trialOverviewProvider = FutureProvider<Loadable<TrialOverview>>((ref) {
  return ref.watch(subscriptionRepositoryProvider).trialOverview();
});

/// Today's step of the seven-day trial (G-48).
///
/// Guided, not automatic: nothing is built until the listener taps, and then
/// the server builds what the day declares. One session opens the player;
/// Day 2's two sessions become two buttons; Day 6 opens the builder. Hidden
/// when no trial is running, so it never shows to a paying listener.
class TrialTodayCard extends ConsumerStatefulWidget {
  const TrialTodayCard({super.key});

  @override
  ConsumerState<TrialTodayCard> createState() => _TrialTodayCardState();
}

class _TrialTodayCardState extends ConsumerState<TrialTodayCard> {
  bool _busy = false;
  List<TrialTodaySession> _sessions = const [];

  Future<void> _startToday() async {
    setState(() => _busy = true);
    final result = await ref
        .read(subscriptionRepositoryProvider)
        .startTrialToday();
    if (!mounted) return;
    setState(() => _busy = false);
    result.when(
      success: (today) {
        if (today.openBuilder) {
          context.go(AppRoutes.confess);
          return;
        }
        if (today.premiumVoiceAvailable == false) {
          ScaffoldMessenger.of(context).showSnackBar(
            const SnackBar(
              content: Text(
                'No Premium voice is ready yet — today uses the standard voice.',
              ),
            ),
          );
        }
        if (today.sessions.length == 1) {
          context.go(AppRoutes.playerWithId(today.sessions.first.sessionId));
        } else {
          setState(() => _sessions = today.sessions);
        }
      },
      failure: (error) =>
          ScaffoldMessenger.of(context)
              .showSnackBar(SnackBar(content: Text(error.message))),
    );
  }

  @override
  Widget build(BuildContext context) {
    final overview = ref.watch(trialOverviewProvider).asData?.value.valueOrNull;
    final today = overview?.today;
    if (overview == null || !overview.running || today == null) {
      return const SizedBox.shrink();
    }
    const onCard = IConfess.colorNeutral0;
    return Padding(
      // The card owns its spacing, so nothing is left behind when it hides.
      padding: const EdgeInsets.only(bottom: IConfess.space4),
      child: Semantics(
        container: true,
        label: 'Trial, day ${overview.currentDay} of 7: ${today.title}',
        child: Container(
          width: double.infinity,
          padding: const EdgeInsets.all(IConfess.space5),
          decoration: BoxDecoration(
            color: IConfess.colorBrand800,
            borderRadius: BorderRadius.circular(IConfess.radiusXl),
          ),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(
                'TRIAL · DAY ${overview.currentDay} OF 7',
                style: IConfess.label.copyWith(
                  color: IConfess.colorBrand200,
                  letterSpacing: 1.4,
                ),
              ),
              const SizedBox(height: IConfess.space2),
              Text(
                today.title,
                style: IConfess.subheading.copyWith(color: onCard),
              ),
              const SizedBox(height: IConfess.space1),
              Text(
                today.description,
                style: IConfess.bodySm.copyWith(color: IConfess.colorBrand100),
              ),
              const SizedBox(height: IConfess.space3),
              ExcludeSemantics(
                child: Row(
                  children: [
                    for (var d = 1; d <= 7; d++)
                      Padding(
                        padding: const EdgeInsets.only(right: IConfess.space1),
                        child: _DayDot(
                          day: d,
                          done: overview.completedDays.contains(d),
                          today: d == overview.currentDay,
                        ),
                      ),
                  ],
                ),
              ),
              const SizedBox(height: IConfess.space4),
              if (_sessions.length > 1)
                Wrap(
                  spacing: IConfess.space2,
                  runSpacing: IConfess.space2,
                  children: [
                    for (final s in _sessions)
                      FilledButton.icon(
                        onPressed: () =>
                            context.go(AppRoutes.playerWithId(s.sessionId)),
                        icon: const Icon(Icons.play_arrow_rounded),
                        label: Text(s.label.isNotEmpty ? s.label : s.title),
                      ),
                  ],
                )
              else
                FilledButton(
                  key: const ValueKey('trial-start-today'),
                  onPressed: _busy ? null : _startToday,
                  child: Text(_busy ? 'Preparing…' : today.cta),
                ),
            ],
          ),
        ),
      ),
    );
  }
}

class _DayDot extends StatelessWidget {
  const _DayDot({required this.day, required this.done, required this.today});

  final int day;
  final bool done;
  final bool today;

  @override
  Widget build(BuildContext context) {
    return Container(
      width: 26,
      height: 26,
      alignment: Alignment.center,
      decoration: BoxDecoration(
        shape: BoxShape.circle,
        color: done ? IConfess.colorBrand100 : null,
        border: Border.all(
          color: today ? IConfess.colorNeutral0 : IConfess.colorBrand400,
          width: today ? 2 : 1,
        ),
      ),
      child: done
          ? const Icon(
              Icons.check_rounded,
              size: 14,
              color: IConfess.colorBrand800,
            )
          : Text(
              '$day',
              style: IConfess.caption.copyWith(color: IConfess.colorBrand100),
            ),
    );
  }
}
