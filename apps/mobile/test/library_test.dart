import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:iconfess/app.dart';
import 'package:iconfess/src/core/analytics/analytics.dart';
import 'package:iconfess/src/core/di/providers.dart';
import 'package:iconfess/src/core/routing/router.dart';
import 'package:iconfess/src/core/routing/routes.dart';
import 'package:iconfess/src/features/auth/auth_controller.dart';
import 'package:iconfess/src/features/library/library_providers.dart';
import 'package:iconfess_api/iconfess_api.dart';
import 'package:shared_preferences/shared_preferences.dart';

import 'support/fake_api_client.dart';

/// The library: collections, favourites and the listener's own confessions
/// (PHASE 27).
///
/// Driven through the scripted socket like the other feature tests, so the
/// real repository, the real cache policy and the real models are exercised
/// and only the network is invented.
void main() {
  late ProviderContainer container;
  late FakeApiClient api;
  late InMemoryTokenStore tokens;

  setUp(() {
    SharedPreferences.setMockInitialValues({});
    tokens = InMemoryTokenStore();
    api = FakeApiClient(tokens: tokens);
  });

  Future<void> pumpLibrary(WidgetTester tester, {String? route}) async {
    final prefs = await SharedPreferences.getInstance();
    container = ProviderContainer(
      overrides: [
        sharedPreferencesProvider.overrideWithValue(prefs),
        tokenStoreProvider.overrideWithValue(tokens),
        apiClientProvider.overrideWithValue(api),
        analyticsProvider.overrideWithValue(DebugAnalytics(sink: (_) {})),
        authControllerProvider.overrideWith(() => _SignedIn()),
      ],
    );
    addTearDown(container.dispose);

    await tester.pumpWidget(
      UncontrolledProviderScope(container: container, child: const IConfessApp()),
    );
    await tester.pumpAndSettle();
    container.read(routerProvider).go(route ?? AppRoutes.library);
    await tester.pumpAndSettle();
  }

  /// The library loads all three tabs, so every test has to answer all three
  /// endpoints or an unrelated tab throws and the failure is misattributed.
  void seedEmpty() {
    api.respond('/me/collections', {'data': []});
    api.respond('/me/favorites', {'data': []});
    api.respond('/me/confessions', {'data': []});
  }

  group('collections tab', () {
    testWidgets('lists collections with their counts', (tester) async {
      seedEmpty();
      api.respond('/me/collections', {
        'data': [
          {
            'id': 'col-1',
            'name': 'Morning mercies',
            'description': 'Before the day starts',
            'item_count': 3,
            'visibility': 'private',
          },
          {'id': 'col-2', 'name': 'Evening', 'item_count': 1, 'visibility': 'private'},
        ],
      });
      await pumpLibrary(tester);

      expect(find.text('Morning mercies'), findsOneWidget);
      expect(find.text('Before the day starts'), findsOneWidget);
      expect(find.text('3 confessions'), findsOneWidget);
      // Singular, because "1 confessions" is the kind of detail that makes an
      // app feel unfinished.
      expect(find.text('1 confession'), findsOneWidget);
    });

    testWidgets('a private collection carries no visibility badge',
        (tester) async {
      seedEmpty();
      api.respond('/me/collections', {
        'data': [
          {'id': 'c1', 'name': 'Private one', 'item_count': 0, 'visibility': 'private'},
          {'id': 'c2', 'name': 'Shared one', 'item_count': 0, 'visibility': 'public'},
        ],
      });
      await pumpLibrary(tester);

      // Badging the default would put a privacy label on every row and teach
      // people to ignore it; only the exception is marked.
      expect(find.text('private'), findsNothing);
      expect(find.text('public'), findsOneWidget);
    });

    testWidgets('the empty state offers a way to create one', (tester) async {
      seedEmpty();
      await pumpLibrary(tester);

      expect(find.byKey(const ValueKey('empty-collections')), findsOneWidget);
      expect(find.text('Create a collection'), findsOneWidget);
    });

    testWidgets('creating a collection posts the name and refreshes the list',
        (tester) async {
      seedEmpty();
      await pumpLibrary(tester);

      await tester.tap(find.text('Create a collection'));
      await tester.pumpAndSettle();

      await tester.enterText(
          find.byKey(const ValueKey('field-collection-name')), 'Morning mercies');

      // The list the screen refreshes to must contain the new collection, or
      // the user watches their creation vanish.
      api.respond('/me/collections', {
        'data': [
          {'id': 'new', 'name': 'Morning mercies', 'item_count': 0},
        ],
      });
      await tester.tap(find.byKey(const ValueKey('button-confirm-name')));
      await tester.pumpAndSettle();

      expect(api.bodyOf('/me/collections', method: 'POST'),
          containsPair('name', 'Morning mercies'));
      expect(find.text('Morning mercies'), findsOneWidget);
    });

    testWidgets('an unnamed collection is refused before it reaches the server',
        (tester) async {
      seedEmpty();
      await pumpLibrary(tester);

      await tester.tap(find.text('Create a collection'));
      await tester.pumpAndSettle();
      await tester.tap(find.byKey(const ValueKey('button-confirm-name')));
      await tester.pumpAndSettle();

      expect(find.text('Give it a name.'), findsOneWidget);
      expect(api.callCount('/me/collections'), 1,
          reason: 'only the initial list read; no POST should have been sent');
    });

    testWidgets('a failed load offers a retry rather than a spinner',
        (tester) async {
      seedEmpty();
      api.respondWith('/me/collections',
          const ApiError(status: 500, code: '', message: 'boom'));
      await pumpLibrary(tester);

      expect(find.text('Try again'), findsOneWidget);
    });
  });

  group('favourites tab', () {
    Future<void> openFavorites(WidgetTester tester) async {
      await tester.tap(find.byKey(const ValueKey('tab-favorites')));
      await tester.pumpAndSettle();
    }

    testWidgets('shows resolved titles, not opaque ids', (tester) async {
      seedEmpty();
      api.respond('/me/favorites', {
        'data': [
          {
            'id': 'f1',
            'entity_type': 'confession',
            'entity_id': 'c-abc-123',
            'title': 'I am held by grace',
            'subtitle': 'Peace',
          },
        ],
      });
      await pumpLibrary(tester);
      await openFavorites(tester);

      expect(find.text('I am held by grace'), findsOneWidget);
      expect(find.text('Peace'), findsOneWidget);
      // The id is what this tab displayed before the server learned to
      // hydrate; seeing it again is the regression.
      expect(find.text('c-abc-123'), findsNothing);
    });

    testWidgets('a favourite whose target is gone is shown and can be cleared',
        (tester) async {
      seedEmpty();
      api.respond('/me/favorites', {
        'data': [
          {'id': 'f1', 'entity_type': 'confession', 'entity_id': 'gone', 'missing': true},
        ],
      });
      await pumpLibrary(tester);
      await openFavorites(tester);

      expect(find.text('No longer available'), findsOneWidget);
      // Dropping it would leave an entry the user can see in their export and
      // never remove, so the affordance has to be there.
      expect(find.byKey(const ValueKey('unfavorite-gone')), findsOneWidget);
    });

    testWidgets('unfavouriting sends the entity type of the row', (tester) async {
      seedEmpty();
      api.respond('/me/favorites', {
        'data': [
          {'id': 'f1', 'entity_type': 'category', 'entity_id': 'cat-1', 'title': 'Peace'},
        ],
      });
      await pumpLibrary(tester);
      await openFavorites(tester);

      await tester.tap(find.byKey(const ValueKey('unfavorite-cat-1')));
      await tester.pumpAndSettle();

      final sent = api.bodyOf('/me/favorites', method: 'DELETE');
      // Assuming 'confession' would make a favourited category impossible to
      // remove from the one screen that lists it.
      expect(sent, containsPair('entity_type', 'category'));
      expect(sent, containsPair('entity_id', 'cat-1'));
    });

    testWidgets('the empty state points at somewhere to find content',
        (tester) async {
      seedEmpty();
      await pumpLibrary(tester);
      await openFavorites(tester);

      expect(find.byKey(const ValueKey('empty-favorites')), findsOneWidget);
      expect(find.text('Browse confessions'), findsOneWidget);
    });
  });

  group('my confessions tab', () {
    Future<void> openMine(WidgetTester tester) async {
      await tester.tap(find.byKey(const ValueKey('tab-my-confessions')));
      await tester.pumpAndSettle();
    }

    testWidgets('renders the author\'s text and status', (tester) async {
      seedEmpty();
      api.respond('/me/confessions', {
        'data': [
          {
            'id': 'uc-1',
            'title': 'My own words',
            'text': 'I am kept.',
            'status': 'draft',
            'visibility': 'private',
          },
        ],
      });
      await pumpLibrary(tester);
      await openMine(tester);

      // Decoded as UserConfession. Reading it as the editorial Confession
      // model, which this tab did before PHASE 27, renders an empty row
      // because none of the keys line up.
      expect(find.text('My own words'), findsOneWidget);
      expect(find.text('Draft'), findsOneWidget);
    });

    testWidgets('every moderation status is named in words', (tester) async {
      seedEmpty();
      api.respond('/me/confessions', {
        'data': [
          {'id': '1', 'title': 'A', 'text': 'x', 'status': 'draft'},
          {'id': '2', 'title': 'B', 'text': 'x', 'status': 'submitted'},
          {'id': '3', 'title': 'C', 'text': 'x', 'status': 'published'},
          {'id': '4', 'title': 'D', 'text': 'x', 'status': 'rejected'},
        ],
      });
      await pumpLibrary(tester);
      await openMine(tester);

      // Colour alone never carries the meaning: a status a colour-blind user
      // cannot read is not a status.
      expect(find.text('Draft'), findsOneWidget);
      expect(find.text('In review'), findsOneWidget);
      expect(find.text('Published'), findsOneWidget);
      expect(find.text('Not accepted'), findsOneWidget);
    });

    testWidgets('a rejection shows the reason it was refused', (tester) async {
      seedEmpty();
      api.respond('/me/confessions', {
        'data': [
          {
            'id': 'uc-1',
            'title': 'Refused',
            'text': 'x',
            'status': 'rejected',
            'rejection_reason': 'Please cite the passage you are confessing.',
          },
        ],
      });
      await pumpLibrary(tester);
      await openMine(tester);

      expect(find.text('Please cite the passage you are confessing.'), findsOneWidget);
    });

    testWidgets('only a shareable draft offers to be submitted', (tester) async {
      seedEmpty();
      api.respond('/me/confessions', {
        'data': [
          {'id': 'private', 'title': 'P', 'text': 'x', 'status': 'draft', 'visibility': 'private'},
          {'id': 'shared', 'title': 'S', 'text': 'x', 'status': 'draft', 'visibility': 'public'},
          {'id': 'pending', 'title': 'W', 'text': 'x', 'status': 'submitted', 'visibility': 'public'},
        ],
      });
      await pumpLibrary(tester);
      await openMine(tester);

      // A private note has nothing to moderate and a submitted one is already
      // with a reviewer; offering either is a button that can only fail.
      expect(find.byKey(const ValueKey('submit-private')), findsNothing);
      expect(find.byKey(const ValueKey('submit-pending')), findsNothing);
      expect(find.byKey(const ValueKey('submit-shared')), findsOneWidget);
    });

    testWidgets('submitting says "sent for review", never "published"',
        (tester) async {
      seedEmpty();
      api.respond('/me/confessions', {
        'data': [
          {'id': 'uc-1', 'title': 'S', 'text': 'x', 'status': 'draft', 'visibility': 'public'},
        ],
      });
      api.respond('/me/confessions/uc-1/submit', {'id': 'uc-1', 'status': 'submitted'});
      await pumpLibrary(tester);
      await openMine(tester);

      await tester.tap(find.byKey(const ValueKey('submit-uc-1')));
      await tester.pumpAndSettle();

      // Publication is the moderator's decision. Promising it here would be a
      // promise the client cannot keep.
      expect(find.text('Sent for review'), findsOneWidget);
      expect(api.callCount('/me/confessions/uc-1/submit'), 1);
    });

    testWidgets('the empty state invites a first one', (tester) async {
      seedEmpty();
      await pumpLibrary(tester);
      await openMine(tester);

      expect(find.byKey(const ValueKey('empty-my-confessions')), findsOneWidget);
    });
  });

  group('collection detail', () {
    testWidgets('shows the collection and its items in order', (tester) async {
      seedEmpty();
      api.respond('/me/collections/col-1', {
        'id': 'col-1',
        'name': 'Morning mercies',
        'description': 'Before the day starts',
        'items': [
          {'id': 'i1', 'confession_id': 'c1', 'title': 'I am held', 'position': 0},
          {'id': 'i2', 'confession_id': 'c2', 'title': 'Be still', 'position': 1},
        ],
      });
      await pumpLibrary(tester, route: AppRoutes.collectionDetail('col-1'));

      expect(find.text('Morning mercies'), findsOneWidget);
      expect(find.text('I am held'), findsOneWidget);
      expect(find.text('Be still'), findsOneWidget);
      expect(find.text('2 confessions'), findsOneWidget);
    });

    testWidgets('an item whose confession is gone says so', (tester) async {
      seedEmpty();
      api.respond('/me/collections/col-1', {
        'id': 'col-1',
        'name': 'Partly broken',
        'items': [
          {'id': 'i1', 'confession_id': 'c1', 'title': '', 'position': 0},
        ],
      });
      await pumpLibrary(tester, route: AppRoutes.collectionDetail('col-1'));

      // The server resolves the title by joining confessions; an empty one
      // means the content is gone, and a blank row is uninterpretable.
      expect(find.text('No longer available'), findsOneWidget);
    });

    testWidgets('an empty collection explains how to fill it', (tester) async {
      seedEmpty();
      api.respond('/me/collections/col-1', {
        'id': 'col-1',
        'name': 'Nothing here',
        'items': <Map<String, dynamic>>[],
      });
      await pumpLibrary(tester, route: AppRoutes.collectionDetail('col-1'));

      expect(find.byKey(const ValueKey('empty-collection-items')), findsOneWidget);
    });

    testWidgets('removing an item calls the scoped delete route',
        (tester) async {
      seedEmpty();
      api.respond('/me/collections/col-1', {
        'id': 'col-1',
        'name': 'Morning',
        'items': [
          {'id': 'i1', 'confession_id': 'c1', 'title': 'I am held', 'position': 0},
        ],
      });
      api.respond('/me/collections/col-1/items/c1', {'message': 'item removed'});
      await pumpLibrary(tester, route: AppRoutes.collectionDetail('col-1'));

      await tester.tap(find.byKey(const ValueKey('remove-c1')));
      await tester.pumpAndSettle();

      expect(api.callCount('/me/collections/col-1/items/c1'), 1);
    });

    testWidgets('deleting is confirmed, and says the confessions survive',
        (tester) async {
      seedEmpty();
      api.respond('/me/collections/col-1', {
        'id': 'col-1',
        'name': 'Doomed',
        'items': <Map<String, dynamic>>[],
      });
      await pumpLibrary(tester, route: AppRoutes.collectionDetail('col-1'));

      await tester.tap(find.byKey(const ValueKey('collection-menu')));
      await tester.pumpAndSettle();
      await tester.tap(find.text('Delete collection'));
      await tester.pumpAndSettle();

      expect(find.text('Delete this collection?'), findsOneWidget);
      // Without this sentence a user reasonably fears they are deleting the
      // content itself.
      expect(
        find.text('The confessions in it stay in the library. Only the collection goes.'),
        findsOneWidget,
      );

      // Backing out must not delete anything.
      await tester.tap(find.text('Keep'));
      await tester.pumpAndSettle();
      expect(api.calls.any((c) => c.method == 'DELETE'), isFalse);
    });
  });

  group('offline', () {
    testWidgets('a cached library says it is cached', (tester) async {
      seedEmpty();
      api.respond('/me/collections', {
        'data': [
          {'id': 'col-1', 'name': 'Saved earlier', 'item_count': 2},
        ],
      });
      await pumpLibrary(tester);
      expect(find.text('Saved earlier'), findsOneWidget);

      // Now the network is gone. The repository serves its cache, and §102
      // requires the screen to admit it rather than present it as current.
      api.respondWith('/me/collections', const NetworkException('offline'));
      container.invalidate(libraryCollectionsProvider);
      await tester.pumpAndSettle();

      expect(find.text('Saved earlier'), findsOneWidget);
      expect(
        find.text('Showing your saved copy — you appear to be offline.'),
        findsOneWidget,
      );
    });
  });
}

class _SignedIn extends AuthController {
  @override
  AuthState build() => const AuthState.signedIn(userId: 'u1');
}
