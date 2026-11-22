import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:iconfess/src/core/di/providers.dart';
import 'package:iconfess/src/core/theme/theme.dart';
import 'package:iconfess/src/features/home/trial_card.dart';
import 'package:iconfess_api/iconfess_api.dart';

import '../../support/fake_api_client.dart';

void main() {
  late FakeApiClient api;

  setUp(() => api = FakeApiClient(tokens: InMemoryTokenStore()));

  Widget host() => ProviderScope(
        overrides: [apiClientProvider.overrideWithValue(api)],
        child: MaterialApp(
          theme: AppTheme.discovery(Brightness.light),
          home: const Scaffold(body: SingleChildScrollView(child: TrialTodayCard())),
        ),
      );

  testWidgets('hidden when no trial is running', (tester) async {
    api.respond('/subscriptions/trial', {'state': 'ELIGIBLE', 'current_day': 0, 'today': null, 'completed_days': []});
    await tester.pumpWidget(host());
    await tester.pumpAndSettle();
    expect(find.byType(FilledButton), findsNothing);
  });

  testWidgets('Day 2 builds two sessions and offers both', (tester) async {
    api.respond('/subscriptions/trial', {
      'state': 'ACTIVE',
      'current_day': 2,
      'today': {'day': 2, 'title': 'Morning and night', 'description': 'Two short sessions', 'cta': 'Plan your morning and night'},
      'completed_days': [1],
    });
    api.respond('/subscriptions/trial/today', {
      'day': 2,
      'action': 'play',
      'sessions': [
        {'slot': 1, 'label': 'Morning', 'session': {'id': 's1'}},
        {'slot': 2, 'label': 'Evening', 'session': {'id': 's2'}},
      ],
    });
    await tester.pumpWidget(host());
    await tester.pumpAndSettle();

    expect(find.text('Morning and night'), findsOneWidget);
    expect(find.text('TRIAL · DAY 2 OF 7'), findsOneWidget);

    await tester.tap(find.byKey(const ValueKey('trial-start-today')));
    await tester.pumpAndSettle();

    expect(api.callCount('/subscriptions/trial/today'), 1);
    expect(find.text('Morning'), findsOneWidget);
    expect(find.text('Evening'), findsOneWidget);
  });
}
