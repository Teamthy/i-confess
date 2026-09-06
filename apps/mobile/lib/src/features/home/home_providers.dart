import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:iconfess_api/iconfess_api.dart';

import '../../core/di/providers.dart';

/// The home dashboard's data, split by how it may fail.
///
/// Both reads return the client's [Loadable] rather than throwing, because the
/// home is the first signed-in screen a listener sees and it must degrade one
/// rail at a time: a categories outage should not blank the continue-listening
/// rail, and vice versa. The screen switches on each independently.

/// Categories for the carousel and the quick-start chips.
final homeCategoriesProvider =
    FutureProvider<Loadable<List<Category>>>((ref) {
  return ref.watch(contentRepositoryProvider).categories();
});

/// The listener's sessions, for continue-listening and recent activity.
///
/// One read feeds both rails: they are the same list split by state, and two
/// reads would pay for the same round trip twice on every home visit.
final homeSessionsProvider =
    FutureProvider<Loadable<List<ListeningSession>>>((ref) {
  return ref.watch(contentRepositoryProvider).mySessions();
});

/// Sessions that are still live — playing, paused or interrupted. These are
/// the "continue listening" rail: the listener expects to pick them back up,
/// and `INTERRUPTED` belongs here on purpose because it is an involuntary stop
/// the product promises to recover, not an ending.
final continueListeningSelector = Provider<List<ListeningSession>>((ref) {
  final loadable = ref.watch(homeSessionsProvider).asData?.value;
  final sessions = loadable?.valueOrNull ?? const <ListeningSession>[];
  return sessions
      .where((s) => const {
            'ACTIVE',
            'PAUSED',
            'INTERRUPTED',
            'STARTING',
            'READY',
          }.contains(s.status))
      .toList();
});

/// Sessions that ran to their end — the "recent activity" rail. Terminal states
/// that are not `COMPLETED` (cancelled, expired, failed) are excluded: a
/// listener did not *do* anything by cancelling, and showing it as activity
/// would read as judgement.
final recentActivitySelector = Provider<List<ListeningSession>>((ref) {
  final loadable = ref.watch(homeSessionsProvider).asData?.value;
  final sessions = loadable?.valueOrNull ?? const <ListeningSession>[];
  return sessions.where((s) => s.status == 'COMPLETED').toList();
});
