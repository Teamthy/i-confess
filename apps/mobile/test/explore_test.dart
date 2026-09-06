import 'package:flutter/foundation.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:iconfess/app.dart';
import 'package:iconfess/src/core/analytics/analytics.dart';
import 'package:iconfess/src/core/di/providers.dart';
import 'package:iconfess/src/core/routing/router.dart';
import 'package:iconfess/src/core/routing/routes.dart';
import 'package:iconfess/src/features/auth/auth_controller.dart';
import 'package:iconfess_api/iconfess_api.dart';
import 'package:shared_preferences/shared_preferences.dart';

import 'support/fake_api_client.dart';

/// Explore, driven through the scripted socket like the other feature tests.
void main() {
  late ProviderContainer container;
  late FakeApiClient api;
  late InMemoryTokenStore tokens;
  late DebugAnalytics analytics;

  setUp(() {
    SharedPreferences.setMockInitialValues({});
    tokens = InMemoryTokenStore();
    api = FakeApiClient(tokens: tokens);
    analytics = DebugAnalytics(sink: (_) {});
  });

  Future<void> pumpExplore(WidgetTester tester) async {
    final prefs = await SharedPreferences.getInstance();
    container = ProviderContainer(
      overrides: [
        sharedPreferencesProvider.overrideWithValue(prefs),
        tokenStoreProvider.overrideWithValue(tokens),
        apiClientProvider.overrideWithValue(api),
        analyticsProvider.overrideWithValue(analytics),
        authControllerProvider.overrideWith(() => _SignedIn()),
      ],
    );
    addTearDown(container.dispose);

    await tester.pumpWidget(
      UncontrolledProviderScope(container: container, child: const IConfessApp()),
    );
    await tester.pumpAndSettle();
    container.read(routerProvider).go(AppRoutes.explore);
    await tester.pumpAndSettle();
  }

  void seedCatalogue() {
    api.respond('/categories', {
      'data': [
        {'id': 'c1', 'name': 'Anxiety', 'description': 'peace', 'premium': false},
        {'id': 'c2', 'name': 'Gratitude', 'description': 'thanks', 'premium': false},
      ],
    });
    api.respond('/collections', {
      'data': [
        {'id': 'col1', 'name': 'Morning mercies', 'premium': false},
      ],
    });
  }

  testWidgets('explore shows search, featured rail and the category grid',
      (tester) async {
    seedCatalogue();
    await pumpExplore(tester);

    expect(find.byKey(const ValueKey('field-explore-search')), findsOneWidget);
    expect(find.text('Featured'), findsOneWidget);
    expect(find.text('Morning mercies'), findsOneWidget);
    expect(find.text('Anxiety'), findsOneWidget);
    expect(find.text('Gratitude'), findsOneWidget);
  });

  testWidgets('search filters the category grid as you type', (tester) async {
    seedCatalogue();
    await pumpExplore(tester);

    await tester.enterText(
        find.byKey(const ValueKey('field-explore-search')), 'anx');
    await tester.pumpAndSettle();

    expect(find.text('Anxiety'), findsOneWidget);
    expect(find.text('Gratitude'), findsNothing,
        reason: 'a live filter must remove non-matching categories');

    await tester.enterText(
        find.byKey(const ValueKey('field-explore-search')), 'zzz');
    await tester.pumpAndSettle();
    expect(find.text('Nothing matches that yet.'), findsOneWidget);
  });

  testWidgets('a category opens its confessions', (tester) async {
    seedCatalogue();
    api.respond('/categories/c1/confessions', {
      'data': [
        {'id': 'x1', 'title': 'Peace in the storm', 'intensity': 2},
        {'id': 'x2', 'title': 'Be still', 'intensity': 4},
      ],
    });
    await pumpExplore(tester);

    container.read(routerProvider).go(AppRoutes.categoryDetail('c1'));
    await tester.pumpAndSettle();

    expect(find.text('Peace in the storm'), findsOneWidget);
    expect(find.text('Be still'), findsOneWidget);
  });
}

class _SignedIn extends AuthController {
  @override
  AuthState build() => const AuthState.signedIn(userId: 'u1');
}
