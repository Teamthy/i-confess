import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:iconfess/app.dart';
import 'package:iconfess/src/core/routing/routes.dart';
import 'package:iconfess/src/features/auth/auth_controller.dart';
import 'package:shared_preferences/shared_preferences.dart';

import 'package:iconfess/src/core/di/providers.dart';

/// An auth controller pinned to a known state, so the redirect rules can be
/// tested without a keystore or a network.
class _FixedAuth extends AuthController {
  _FixedAuth(this._initial);
  final AuthState _initial;
  @override
  AuthState build() => _initial;
}

Future<void> pumpApp(WidgetTester tester, AuthState state) async {
  SharedPreferences.setMockInitialValues({});
  final prefs = await SharedPreferences.getInstance();
  await tester.pumpWidget(
    ProviderScope(
      overrides: [
        sharedPreferencesProvider.overrideWithValue(prefs),
        authControllerProvider.overrideWith(() => _FixedAuth(state)),
      ],
      child: const IConfessApp(),
    ),
  );
  await tester.pumpAndSettle();
}

void main() {
  group('route table', () {
    test('declares every operation the product needs', () {
      const required = [
        AppRoutes.splash,
        AppRoutes.welcome,
        AppRoutes.onboarding,
        AppRoutes.signIn,
        AppRoutes.signUp,
        AppRoutes.forgotPassword,
        AppRoutes.resetPassword,
        AppRoutes.verification,
        AppRoutes.home,
        AppRoutes.explore,
        AppRoutes.confess,
        AppRoutes.activity,
        AppRoutes.me,
        AppRoutes.player,
        AppRoutes.voices,
        AppRoutes.rituals,
        AppRoutes.saved,
        AppRoutes.downloads,
        AppRoutes.history,
        AppRoutes.search,
        AppRoutes.category,
        AppRoutes.confession,
        AppRoutes.settings,
        AppRoutes.premium,
      ];
      expect(required.toSet(), hasLength(required.length), reason: 'duplicate paths');
    });

    test('browsing is open but the flow is not', () {
      // Requiring sign-in to browse is the fastest way to lose someone who
      // arrived curious; requiring it to build a session is the point.
      expect(AppRoutes.requiresAuth.contains(AppRoutes.home), isFalse);
      expect(AppRoutes.requiresAuth.contains(AppRoutes.explore), isFalse);
      expect(AppRoutes.requiresAuth.contains(AppRoutes.confess), isTrue);
      expect(AppRoutes.requiresAuth.contains(AppRoutes.activity), isTrue);
      expect(AppRoutes.requiresAuth.contains(AppRoutes.me), isTrue);
      expect(AppRoutes.requiresAuth.contains(AppRoutes.player), isTrue);
    });

    test('there are exactly five destinations, with confess in the centre', () {
      expect(AppRoutes.destinations, hasLength(5));
      expect(AppRoutes.destinations[2], AppRoutes.confess);
    });

    test('detail paths build from their parents', () {
      expect(AppRoutes.categoryDetail('healing'), '/explore/category/healing');
      expect(AppRoutes.confessionDetail('c-1'), '/confession/c-1');
    });
  });

  group('redirects', () {
    testWidgets('a signed-out listener is kept out of the flow', (tester) async {
      await pumpApp(tester, const AuthState.signedOut());

      // The splash hands off to welcome, which is where sign-in and browsing
      // are both offered. Landing on the splash and staying there would be a
      // dead end with no way forward.
      expect(find.text('Create your account'), findsWidgets);
      expect(find.text('Just look around for now'), findsWidgets,
          reason: 'browsing without an account is a supported path, so it has '
              'to be reachable from the first screen');
    });

    testWidgets('a signed-in listener is sent past the auth flow', (tester) async {
      await pumpApp(tester, const AuthState.signedIn(userId: 'u1'));
      expect(find.text('Welcome'), findsNothing);
      expect(find.text('Sign in'), findsNothing);
    });

    testWidgets('while auth is unresolved nothing is decided', (tester) async {
      await pumpApp(tester, const AuthState.unknown());
      // The splash stays up rather than flashing sign-in at a signed-in
      // listener on every cold start.
      expect(find.text('I CONFESS'), findsWidgets);
    });
  });

  group('shell', () {
    testWidgets('shows five destinations with the centre action emphasised',
        (tester) async {
      await pumpApp(tester, const AuthState.signedIn(userId: 'u1'));

      for (final label in ['Home', 'Explore', 'Activity', 'Me']) {
        expect(find.text(label), findsWidgets, reason: '$label tab is missing');
      }
      // The centre action is labelled for what it does, not just "Confess",
      // because it is the primary action and a screen reader user needs the
      // verb. Matched on the widget rather than bySemanticsLabel, which needs
      // the semantics tree enabled to find anything.
      final confessAction = find.byWidgetPredicate(
        (w) => w is Semantics && w.properties.label == 'Create a confession session',
      );
      expect(confessAction, findsOneWidget);
      expect(find.text('Confess'), findsWidgets);

      // Every tab carries a label a screen reader can read.
      for (final label in ['Home', 'Explore', 'Activity', 'Me']) {
        expect(
          find.byWidgetPredicate(
            (w) => w is Semantics && w.properties.label == label,
          ),
          findsOneWidget,
          reason: '$label has no semantic label',
        );
      }
    });

    testWidgets('every tab is at least a 44pt touch target', (tester) async {
      await pumpApp(tester, const AuthState.signedIn(userId: 'u1'));

      // The five tab targets are the only InkWells in the bar. Home contributes
      // its own InkWells to the body, so the search is scoped to the bar by key.
      final targets = find.descendant(
        of: find.byKey(const ValueKey('app-shell-bar')),
        matching: find.byType(InkWell),
      );
      expect(targets, findsNWidgets(5));

      for (var i = 0; i < 5; i++) {
        final size = tester.getSize(targets.at(i));
        expect(size.height, greaterThanOrEqualTo(44),
            reason: 'tab $i target is ${size.height}px tall');
        expect(size.width, greaterThanOrEqualTo(44),
            reason: 'tab $i target is ${size.width}px wide');
      }
    });

    testWidgets('tapping a tab switches branch', (tester) async {
      await pumpApp(tester, const AuthState.signedIn(userId: 'u1'));

      await tester.tap(find.text('Activity').first);
      await tester.pumpAndSettle();
      expect(find.text('PHASE 24'), findsWidgets);
    });
  });
}
