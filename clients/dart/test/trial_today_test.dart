import 'package:iconfess_api/iconfess_api.dart';
import 'package:test/test.dart';

void main() {
  test('TrialOverview decodes state, today and completed days', () {
    final o = TrialOverview.fromJson({
      'state': 'ACTIVE',
      'current_day': 3,
      'today': {
        'day': 3,
        'title': 'Made for you',
        'cta': 'Open today\'s session',
        'categories': ['peace']
      },
      'completed_days': [1, 2],
    });
    expect(o.running, isTrue);
    expect(o.eligible, isFalse);
    expect(o.currentDay, 3);
    expect(o.today?.title, 'Made for you');
    expect(o.completedDays, [1, 2]);
  });

  test('TrialOverview tolerates a trial that has not started', () {
    final o = TrialOverview.fromJson({
      'state': 'ELIGIBLE',
      'current_day': 0,
      'today': null,
      'completed_days': []
    });
    expect(o.eligible, isTrue);
    expect(o.today, isNull);
  });

  test('TrialTodayResult decodes Day 2 sessions with their labels', () {
    final r = TrialTodayResult.fromJson({
      'day': 2,
      'action': 'play',
      'sessions': [
        {
          'slot': 1,
          'label': 'Morning',
          'session': {'id': 's1', 'title': 'Morning and night · Morning'}
        },
        {
          'slot': 2,
          'label': 'Evening',
          'session': {'id': 's2', 'title': 'Morning and night · Evening'}
        },
      ],
    });
    expect(r.openBuilder, isFalse);
    expect(r.sessions.map((s) => s.sessionId), ['s1', 's2']);
    expect(r.sessions.first.label, 'Morning');
    expect(r.premiumVoiceAvailable, isNull);
  });

  test('TrialTodayResult decodes the Day 6 builder prefill', () {
    final r = TrialTodayResult.fromJson({
      'day': 6,
      'action': 'open_builder',
      'sessions': [],
      'builder': {
        'category_ids': ['c1', 'c2'],
        'category_slugs': ['purpose', 'hope'],
        'duration_seconds': 900
      },
    });
    expect(r.openBuilder, isTrue);
    expect(r.builderCategoryIds, ['c1', 'c2']);
    expect(r.builderDurationSeconds, 900);
    expect(r.sessions, isEmpty);
  });
}
