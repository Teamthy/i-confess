import 'package:flutter/foundation.dart';
import 'package:flutter/material.dart';
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

/// Confession detail, driven through the scripted socket.
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

  Future<void> pumpConfession(
    WidgetTester tester, {
    required String confessionId,
  }) async {
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
    container.read(routerProvider).go(AppRoutes.confessionDetail(confessionId));
    await tester.pumpAndSettle();
  }

  Map<String, dynamic> confessionPayload() => {
        'id': 'conf1',
        'category_id': 'c1',
        'title': 'I am healed',
        'short_text': 'By His stripes I am healed',
        'medium_text': 'By His stripes I was healed and I walk in health',
        'long_text':
            'Lord, I confess that by Your stripes I was healed. Sickness has no place in me.',
        'description': 'A confession of healing',
        'intensity': 3,
        'language': 'en',
        'status': 'published',
        'author': 'I CONFESS',
        'tags': ['healing', 'faith'],
        'variants': [
          {'id': 'v1', 'confession_id': 'conf1', 'label': '30s', 'duration_seconds': 30, 'sort_order': 1},
          {'id': 'v2', 'confession_id': 'conf1', 'label': '1m', 'duration_seconds': 60, 'sort_order': 2},
        ],
        'scriptures': [
          {
            'id': 's1',
            'confession_id': 'conf1',
            'book': 'Isaiah',
            'chapter': 53,
            'verse': '5',
            'translation': 'KJV',
            'is_direct_quote': true,
            'notes': 'Healing promise',
            'sort_order': 1,
          },
        ],
      };

  testWidgets('confession detail shows title, texts and scripture', (tester) async {
    api.respond('/confessions/conf1', confessionPayload());
    api.respond('/me/favorites', {'data': []});

    await pumpConfession(tester, confessionId: 'conf1');

    expect(find.text('I am healed'), findsOneWidget);
    expect(find.textContaining('By His stripes I am healed'), findsWidgets);
    expect(find.text('Isaiah 53:5'), findsOneWidget);
    expect(find.text('KJV'), findsOneWidget);
    expect(find.text('30s'), findsOneWidget);
    expect(find.text('1m'), findsOneWidget);
  });

  testWidgets('confession detail shows tags and intensity', (tester) async {
    api.respond('/confessions/conf1', confessionPayload());
    api.respond('/me/favorites', {'data': []});

    await pumpConfession(tester, confessionId: 'conf1');

    expect(find.text('#healing'), findsOneWidget);
    expect(find.text('#faith'), findsOneWidget);
  });

  testWidgets('confession detail has favourite and build session actions', (tester) async {
    api.respond('/confessions/conf1', confessionPayload());
    api.respond('/me/favorites', {'data': []});
    api.respond('/categories', {
      'data': [
        {'id': 'c1', 'name': 'Healing', 'description': '', 'premium': false},
      ],
    });
    api.respond('/sessions', {'sessions': []});

    await pumpConfession(tester, confessionId: 'conf1');

    expect(find.byKey(const ValueKey('btn-build-session')), findsOneWidget);
    expect(find.byKey(const ValueKey('btn-toggle-fav')), findsOneWidget);
    expect(find.byKey(const ValueKey('btn-favorite')), findsOneWidget);
    // G-43: filing into a collection starts here, on the item's own page.
    expect(find.byKey(const ValueKey('btn-add-to-collection')), findsOneWidget);
  });

  testWidgets('add to a collection lists the listener’s collections and posts membership',
      (tester) async {
    api.respond('/confessions/conf1', confessionPayload());
    api.respond('/me/favorites', {'data': []});
    api.respond('/me/collections', {
      'data': [
        {'id': 'col-1', 'name': 'Morning mercies', 'item_count': 2, 'visibility': 'private'},
        {'id': 'col-2', 'name': 'Hard weeks', 'item_count': 0, 'visibility': 'private'},
      ],
    });
    api.respond('/me/collections/col-1/items', {'id': 'col-1', 'name': 'Morning mercies', 'items': []});
    await pumpConfession(tester, confessionId: 'conf1');

    await tester.ensureVisible(find.byKey(const ValueKey('btn-add-to-collection')));
    await tester.tap(find.byKey(const ValueKey('btn-add-to-collection')));
    await tester.pumpAndSettle();

    expect(find.text('Morning mercies'), findsOneWidget);
    expect(find.text('2 confessions'), findsOneWidget);

    await tester.tap(find.byKey(const ValueKey('pick-collection-col-1')));
    await tester.pumpAndSettle();

    expect(api.bodyOf('/me/collections/col-1/items', method: 'POST'),
        containsPair('confession_id', 'conf1'));
    expect(find.text('Added to collection'), findsOneWidget);
  });

  testWidgets('tapping a confession in category opens its detail', (tester) async {
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

    api.respond('/categories', {
      'data': [
        {'id': 'c1', 'name': 'Healing', 'description': '', 'premium': false},
      ],
    });
    api.respond('/collections', {'data': []});
    api.respond('/categories/c1/confessions', {
      'data': [
        {'id': 'conf1', 'title': 'I am healed', 'intensity': 2},
      ],
    });
    api.respond('/confessions/conf1', confessionPayload());
    api.respond('/me/favorites', {'data': []});

    await tester.pumpWidget(
      UncontrolledProviderScope(container: container, child: const IConfessApp()),
    );
    await tester.pumpAndSettle();
    container.read(routerProvider).go(AppRoutes.categoryDetail('c1'));
    await tester.pumpAndSettle();

    expect(find.text('I am healed'), findsOneWidget);

    await tester.tap(find.text('I am healed'));
    await tester.pumpAndSettle();

    // Now on detail screen
    expect(find.text('I am healed'), findsWidgets);
    expect(find.textContaining('By His stripes'), findsWidgets);
  });

  testWidgets('confess tab shows categories and allows selection', (tester) async {
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

    api.respond('/categories', {
      'data': [
        {'id': 'c1', 'name': 'Healing', 'description': '', 'premium': false},
        {'id': 'c2', 'name': 'Faith', 'description': '', 'premium': false},
      ],
    });
    api.respond('/sessions', {'sessions': []});

    await tester.pumpWidget(
      UncontrolledProviderScope(container: container, child: const IConfessApp()),
    );
    await tester.pumpAndSettle();
    container.read(routerProvider).go(AppRoutes.confess);
    await tester.pumpAndSettle();

    expect(find.text('What do you want to speak over your life today?'), findsOneWidget);
    expect(find.text('Healing'), findsOneWidget);
    expect(find.text('Faith'), findsOneWidget);

    await tester.tap(find.text('Healing'));
    await tester.pumpAndSettle();

    expect(find.byKey(const ValueKey('btn-continue')), findsOneWidget);
    expect(find.textContaining('1 area'), findsOneWidget);
  });
}

class _SignedIn extends AuthController {
  @override
  AuthState build() => const AuthState.signedIn(userId: 'u1');
}
