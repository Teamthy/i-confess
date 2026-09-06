import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:iconfess/app.dart';
import 'package:iconfess/src/core/analytics/analytics.dart';
import 'package:iconfess/src/core/di/providers.dart';
import 'package:iconfess/src/features/auth/auth_controller.dart';
import 'package:iconfess_api/iconfess_api.dart';
import 'package:shared_preferences/shared_preferences.dart';

import 'support/fake_api_client.dart';

/// The home dashboard, driven by the same scripted socket as the auth tests so
/// the real client and repositories stay in the tree.
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

  Future<void> pumpHome(WidgetTester tester) async {
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
    // Cold start: splash -> redirect (signed in) -> home, then let the rails load.
    await tester.pumpAndSettle();
  }

  testWidgets('home shows greeting, primary action and the category rail',
      (tester) async {
    api.respond('/categories', {
      'data': [
        {'id': 'c1', 'name': 'Anxiety', 'description': '', 'premium': false},
        {'id': 'c2', 'name': 'Gratitude', 'description': '', 'premium': false},
      ],
    });
    api.respond('/sessions', {'sessions': []});

    await pumpHome(tester);

    // The greeting headline is time-of-day dependent; the sub-line is not.
    expect(find.text('Take a moment. Speak it aloud.'), findsOneWidget);
    expect(find.text('Set aside a few minutes'), findsOneWidget);
    expect(find.text('Browse by category'), findsOneWidget);
    expect(find.text('Anxiety'), findsOneWidget);
    expect(find.text('Gratitude'), findsOneWidget);

    // Empty library: the conditional rails stay out of the way.
    expect(find.text('Continue listening'), findsNothing);
    expect(find.text('Recent activity'), findsNothing);
  });

  testWidgets('continue listening and recent activity come from session state',
      (tester) async {
    api.respond('/categories', {'data': []});
    api.respond('/sessions', {
      'sessions': [
        {
          'id': 's1',
          'status': 'PAUSED',
          'duration_seconds': 900,
          'items': [
            {'confession_id': 'x1', 'title': 'Peace in the storm'},
          ],
        },
        {
          'id': 's2',
          'status': 'COMPLETED',
          'duration_seconds': 600,
          'items': [
            {'confession_id': 'x2', 'title': 'Evening surrender'},
          ],
        },
        {
          'id': 's3',
          'status': 'CANCELLED',
          'duration_seconds': 300,
          'items': [
            {'confession_id': 'x3', 'title': 'Should not appear'},
          ],
        },
      ],
    });

    await pumpHome(tester);

    expect(find.text('Continue listening'), findsOneWidget);
    expect(find.text('Peace in the storm'), findsOneWidget);
    expect(find.text('Recent activity'), findsOneWidget);
    expect(find.text('Evening surrender'), findsOneWidget);

    // A cancelled session is not activity; showing it would read as judgement.
    expect(find.text('Should not appear'), findsNothing);
  });
}

class _SignedIn extends AuthController {
  @override
  AuthState build() => const AuthState.signedIn(userId: 'u1');
}
