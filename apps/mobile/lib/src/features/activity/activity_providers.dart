import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:iconfess_api/iconfess_api.dart';

import '../../core/di/providers.dart';

/// Activity: history, streak, schedules, continue listening.

final activitySessionsProvider =
    FutureProvider<Loadable<List<ListeningSession>>>((ref) {
  return ref.watch(contentRepositoryProvider).mySessions();
});

final activitySchedulesProvider = FutureProvider<Loadable<List<Schedule>>>((ref) {
  return ref.watch(libraryRepositoryProvider).schedules();
});

/// Streak: count of consecutive days with completed sessions (IC-025).
final streakProvider = Provider<int>((ref) {
  final sessions = ref.watch(activitySessionsProvider).asData?.value.valueOrNull ?? const [];
  if (sessions.isEmpty) return 0;

  // Extract dates of completed sessions
  final completedDates = <DateTime>{};
  for (final s in sessions) {
    if (s.status == 'COMPLETED' && s.createdAt != null) {
      final dt = s.createdAt!.toLocal();
      completedDates.add(DateTime(dt.year, dt.month, dt.day));
    }
  }

  if (completedDates.isEmpty) return 0;

  final now = DateTime.now();
  var checkDate = DateTime(now.year, now.month, now.day);
  var streak = 0;

  // If today is not completed, check if yesterday was completed to keep the streak alive
  if (!completedDates.contains(checkDate)) {
    checkDate = checkDate.subtract(const Duration(days: 1));
  }

  while (completedDates.contains(checkDate)) {
    streak++;
    checkDate = checkDate.subtract(const Duration(days: 1));
  }

  return streak;
});

/// Sessions grouped by status for activity tabs.
final activityGroupedProvider = Provider<Map<String, List<ListeningSession>>>((ref) {
  final sessions = ref.watch(activitySessionsProvider).asData?.value.valueOrNull ?? const [];
  final Map<String, List<ListeningSession>> grouped = {
    'continue': [],
    'completed': [],
    'scheduled': [],
  };
  for (final s in sessions) {
    if (['ACTIVE', 'PAUSED', 'INTERRUPTED', 'READY', 'STARTING'].contains(s.status)) {
      grouped['continue']!.add(s);
    } else if (s.status == 'COMPLETED') {
      grouped['completed']!.add(s);
    }
  }
  return grouped;
});
