import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:iconfess/src/core/theme/theme.dart';
import 'package:iconfess/src/features/home/home_providers.dart';
import 'package:iconfess/src/features/home/home_screen.dart';
import 'package:iconfess_api/iconfess_api.dart';

void main() {
  testWidgets('renders recommendation reasons, repeat context, and suggested duration', (tester) async {
    const recommendation = Recommendations(
      personalized: true,
      categories: [Category(id: 'healing', name: 'Healing')],
      confessions: [Confession(id: 'peace', title: 'Peace in the storm')],
      listenAgain: [
        ListenAgain(confession: Confession(id: 'courage', title: 'Courage'), times: 3),
      ],
      categoryReasons: {
        'healing': ['interest'],
      },
      confessionReasons: {
        'peace': ['favourite'],
      },
      suggestedDurationSeconds: 600,
    );

    await tester.pumpWidget(
      ProviderScope(
        overrides: [
          homeCategoriesProvider.overrideWith(
            (ref) async => const Loadable<List<Category>>.loaded([]),
          ),
          homeSessionsProvider.overrideWith(
            (ref) async => const Loadable<List<ListeningSession>>.loaded([]),
          ),
          homeRecommendationsProvider.overrideWith(
            (ref) async => const Loadable<Recommendations>.loaded(recommendation),
          ),
        ],
        child: MaterialApp(
          theme: AppTheme.discovery(Brightness.light),
          home: const HomeScreen(),
        ),
      ),
    );
    await tester.pumpAndSettle();

    expect(find.text('Picked for you'), findsOneWidget);
    expect(find.text('Matches an interest you chose'), findsOneWidget);
    expect(find.text('A confession you saved'), findsOneWidget);
    expect(find.text('You returned to this 3 times'), findsOneWidget);
    expect(find.text('Suggested session: 10 minutes'), findsOneWidget);
  });
}
