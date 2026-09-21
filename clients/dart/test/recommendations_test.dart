import 'package:iconfess_api/iconfess_api.dart';
import 'package:test/test.dart';

void main() {
  test('recommendations decode the seven signals as quantities', () {
    final rec = Recommendations.fromJson({
      'categories': [
        {'id': 'cat-healing', 'name': 'Healing', 'slug': 'healing'},
      ],
      'confessions': [
        {'id': 'conf-1', 'category_id': 'cat-healing', 'title': 'Healed'},
      ],
      'personalized': true,
      'listen_again': [
        {
          'confession': {'id': 'conf-1', 'title': 'Healed'},
          'times': 3,
        },
      ],
      'reasons': {
        'categories': {
          'cat-healing': ['listened 3', 'repeated 3'],
        },
        'confessions': {
          'conf-1': ['repeated 3'],
        },
      },
      'suggested_duration_seconds': 600,
      'daypart': 'morning',
      'signals': {
        'present': ['categories_listened_to', 'repeat_listening'],
        'categories_listened_to': 1,
        'completion_rate': 0.75,
        'completion_sample': 4,
        'preferred_daypart': 'morning',
        'typical_duration_seconds': 120,
        'favourites': 1,
        'skips': 1,
        'repeat_listening': 1,
      },
    });

    expect(rec.personalized, isTrue);
    expect(rec.categories.single.id, 'cat-healing');
    expect(rec.listenAgain.single.times, 3,
        reason: 'repeat listening is a count, not a flag');
    expect(rec.listenAgain.single.confession.id, 'conf-1');
    expect(rec.categoryReasons['cat-healing'], ['listened 3', 'repeated 3']);
    expect(rec.confessionReasons['conf-1'], ['repeated 3']);
    expect(rec.suggestedDurationSeconds, 600);
    expect(rec.daypart, 'morning');
    expect(rec.signals.present, contains('repeat_listening'));
    expect(rec.signals.completionRate, 0.75);
    expect(rec.signals.completionSample, 4);
    expect(rec.signals.repeatListening, 1);
    expect(rec.signals.skips, 1);
    expect(rec.signals.typicalDurationSeconds, 120);
  });

  test('a v1 payload without the new fields still decodes', () {
    final rec = Recommendations.fromJson({
      'categories': <Map<String, dynamic>>[],
      'confessions': <Map<String, dynamic>>[],
      'personalized': false,
    });
    expect(rec.personalized, isFalse);
    expect(rec.listenAgain, isEmpty);
    expect(rec.categoryReasons, isEmpty);
    expect(rec.suggestedDurationSeconds, 0);
    expect(rec.signals.present, isEmpty);
  });
}
