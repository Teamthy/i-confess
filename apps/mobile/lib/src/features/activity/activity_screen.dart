import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../core/routing/routes.dart';
import '../../core/theme/theme.dart';
import '../../core/theme/tokens.dart';
import '../../core/widgets/screen.dart';
import 'activity_providers.dart';
import '../settings/settings_screen.dart' show EnableDeviceRemindersTile;

/// Activity screen: history, streak, schedules.
///
/// Tab bar with Continue, Completed, Schedules. Shows streak at top.
class ActivityScreen extends ConsumerWidget {
  const ActivityScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final sessionsAsync = ref.watch(activitySessionsProvider);
    final schedulesAsync = ref.watch(activitySchedulesProvider);
    final streak = ref.watch(streakProvider);
    final surfaces = AppSurfaces.of(context);

    return AppScaffold(
      title: 'Activity',
      body: DefaultTabController(
        length: 3,
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            // Streak card
            Container(
              width: double.infinity,
              padding: const EdgeInsets.all(IConfess.space5),
              decoration: BoxDecoration(
                color: IConfess.colorBrand50,
                borderRadius: BorderRadius.circular(IConfess.radiusLg),
                border: Border.all(color: IConfess.colorBrand200),
              ),
              child: Row(
                children: [
                  Container(
                    padding: const EdgeInsets.all(IConfess.space3),
                    decoration: BoxDecoration(
                      color: IConfess.colorBrand500,
                      borderRadius: BorderRadius.circular(IConfess.radiusFull),
                    ),
                    child: const Icon(Icons.local_fire_department_rounded, color: Colors.white),
                  ),
                  const SizedBox(width: IConfess.space4),
                  Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text('$streak day streak',
                          style: IConfess.subheading.copyWith(color: surfaces.textPrimary)),
                      Text('Keep speaking life daily',
                          style: IConfess.bodySm.copyWith(color: surfaces.textSecondary)),
                    ],
                  ),
                  const Spacer(),
                  Text('🔥', style: TextStyle(fontSize: 24)),
                ],
              ),
            ),
            const SizedBox(height: IConfess.space5),
            TabBar(
              labelColor: IConfess.colorBrand600,
              unselectedLabelColor: surfaces.textSecondary,
              indicatorColor: IConfess.colorBrand600,
              tabs: const [
                Tab(text: 'Continue'),
                Tab(text: 'History'),
                Tab(text: 'Schedules'),
              ],
            ),
            const SizedBox(height: IConfess.space3),
            const EnableDeviceRemindersTile(),
            Expanded(
              child: TabBarView(
                children: [
                  _ContinueTab(sessionsAsync: sessionsAsync),
                  _HistoryTab(sessionsAsync: sessionsAsync),
                  _SchedulesTab(schedulesAsync: schedulesAsync),
                ],
              ),
            ),
          ],
        ),
      ),
    );
  }
}

class _ContinueTab extends StatelessWidget {
  const _ContinueTab({required this.sessionsAsync});
  final AsyncValue sessionsAsync;

  @override
  Widget build(BuildContext context) {
    final surfaces = AppSurfaces.of(context);
    return sessionsAsync.when(
      loading: () => const Center(child: CircularProgressIndicator()),
      error: (e, _) => Center(child: Text('Could not load: $e')),
      data: (loadable) {
        final sessions = loadable.valueOrNull ?? [];
        final continueSessions = sessions.where((s) => ['ACTIVE', 'PAUSED', 'INTERRUPTED', 'READY', 'STARTING'].contains(s.status)).toList();
        if (continueSessions.isEmpty) {
          return Center(
            child: Column(
              mainAxisAlignment: MainAxisAlignment.center,
              children: [
                Icon(Icons.play_circle_outline_rounded, size: 48, color: surfaces.textSecondary),
                const SizedBox(height: IConfess.space3),
                Text('No sessions in progress',
                    style: IConfess.body.copyWith(color: surfaces.textPrimary)),
                const SizedBox(height: IConfess.space2),
                Text('Build a session to start',
                    style: IConfess.bodySm.copyWith(color: surfaces.textSecondary)),
                const SizedBox(height: IConfess.space4),
                FilledButton(
                  onPressed: () => context.go(AppRoutes.confess),
                  child: const Text('Build a session'),
                ),
              ],
            ),
          );
        }
        return ListView.separated(
          itemCount: continueSessions.length,
          separatorBuilder: (_, _) => const SizedBox(height: IConfess.space2),
          itemBuilder: (context, i) {
            final s = continueSessions[i];
            return _SessionTile(session: s, isContinue: true);
          },
        );
      },
    );
  }
}

class _HistoryTab extends StatelessWidget {
  const _HistoryTab({required this.sessionsAsync});
  final AsyncValue sessionsAsync;

  @override
  Widget build(BuildContext context) {
    return sessionsAsync.when(
      loading: () => const Center(child: CircularProgressIndicator()),
      error: (e, _) => Center(child: Text('Could not load: $e')),
      data: (loadable) {
        final sessions = loadable.valueOrNull ?? [];
        final completed = sessions.where((s) => s.status == 'COMPLETED').toList();
        if (completed.isEmpty) {
          return Center(child: Text('No history yet'));
        }
        return ListView.separated(
          itemCount: completed.length,
          separatorBuilder: (_, _) => const SizedBox(height: IConfess.space2),
          itemBuilder: (context, i) => _SessionTile(session: completed[i]),
        );
      },
    );
  }
}

class _SchedulesTab extends StatelessWidget {
  const _SchedulesTab({required this.schedulesAsync});
  final AsyncValue schedulesAsync;

  @override
  Widget build(BuildContext context) {
    final surfaces = AppSurfaces.of(context);
    return schedulesAsync.when(
      loading: () => const Center(child: CircularProgressIndicator()),
      error: (e, _) => Center(child: Text('Could not load: $e')),
      data: (loadable) {
        final schedules = loadable.valueOrNull ?? [];
        if (schedules.isEmpty) {
          return Center(
            child: Column(
              mainAxisAlignment: MainAxisAlignment.center,
              children: [
                Icon(Icons.schedule_rounded, size: 48, color: surfaces.textSecondary),
                const SizedBox(height: IConfess.space3),
                Text('No schedules',
                    style: IConfess.body.copyWith(color: surfaces.textPrimary)),
                Text('Set a daily reminder',
                    style: IConfess.bodySm.copyWith(color: surfaces.textSecondary)),
              ],
            ),
          );
        }
        return ListView.separated(
          itemCount: schedules.length,
          separatorBuilder: (_, _) => const SizedBox(height: IConfess.space2),
          itemBuilder: (context, i) {
            final sched = schedules[i];
            return ListTile(
              title: Text(sched.label.isNotEmpty ? sched.label : 'Daily ritual'),
              subtitle: Text('${sched.time} • ${sched.daysOfWeek.length} days'),
              trailing: Switch(
                value: sched.enabled,
                onChanged: (_) {},
              ),
            );
          },
        );
      },
    );
  }
}

class _SessionTile extends StatelessWidget {
  const _SessionTile({required this.session, this.isContinue = false});
  final dynamic session;
  final bool isContinue;

  @override
  Widget build(BuildContext context) {
    final surfaces = AppSurfaces.of(context);
    return Material(
      color: surfaces.surfaceRaised,
      borderRadius: BorderRadius.circular(IConfess.radiusMd),
      child: InkWell(
        borderRadius: BorderRadius.circular(IConfess.radiusMd),
        onTap: () => context.go(AppRoutes.player),
        child: Padding(
          padding: const EdgeInsets.all(IConfess.space4),
          child: Row(
            children: [
              Container(
                width: 48,
                height: 48,
                decoration: BoxDecoration(
                  color: isContinue ? IConfess.colorBrand500 : surfaces.surface,
                  borderRadius: BorderRadius.circular(IConfess.radiusMd),
                ),
                child: Icon(
                  isContinue ? Icons.play_arrow_rounded : Icons.check_rounded,
                  color: isContinue ? Colors.white : IConfess.colorBrand600,
                ),
              ),
              const SizedBox(width: IConfess.space3),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      session.items.isNotEmpty ? session.items.first.title : 'Session',
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                      style: IConfess.body.copyWith(color: surfaces.textPrimary),
                    ),
                    Text(
                      '${session.durationSeconds ~/ 60} min • ${session.status}',
                      style: IConfess.caption.copyWith(color: surfaces.textSecondary),
                    ),
                  ],
                ),
              ),
              const Icon(Icons.chevron_right_rounded),
            ],
          ),
        ),
      ),
    );
  }
}
