import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:iconfess/app.dart';
import 'package:iconfess/src/core/analytics/analytics.dart';
import 'package:iconfess/src/core/di/providers.dart';
import 'package:iconfess/src/core/routing/router.dart';
import 'package:iconfess/src/core/persistence/persistence.dart';
import 'package:iconfess/src/core/routing/routes.dart';
import 'package:iconfess/src/features/auth/auth_controller.dart';
import 'package:iconfess/src/features/auth/widgets/auth_form.dart';
import 'package:iconfess_api/iconfess_api.dart';
import 'package:shared_preferences/shared_preferences.dart';

import 'support/fake_api_client.dart';

/// The auth flow, driven the way a listener drives it.
///
/// These are widget tests rather than controller tests on purpose. The things
/// that break in a sign-in flow are the things between the tap and the request:
/// a validation rule that fires before the user has typed, a button that can be
/// pressed twice, an error that arrives with no copy, a keyboard that covers the
/// submit button. A controller test cannot see any of them.
void main() {
  late ProviderContainer container;
  late FakeApiClient api;
  late InMemoryTokenStore tokens;
  late DebugAnalytics analytics;

  // Built before the widget tree, so a test can script a response or seed a
  // token before anything is pumped.
  setUp(() {
    SharedPreferences.setMockInitialValues({});
    tokens = InMemoryTokenStore();
    api = FakeApiClient(tokens: tokens);
    analytics = DebugAnalytics(sink: (_) {});
  });

  Future<void> pumpApp(
    WidgetTester tester, {
    AuthState auth = const AuthState.signedOut(),
    String? initialLocation,
  }) async {
    final prefs = await SharedPreferences.getInstance();

    container = ProviderContainer(
      overrides: [
        sharedPreferencesProvider.overrideWithValue(prefs),
        tokenStoreProvider.overrideWithValue(tokens),
        apiClientProvider.overrideWithValue(api),
        analyticsProvider.overrideWithValue(analytics),
        authControllerProvider.overrideWith(() => _FixedAuth(auth)),
        // A collection-if rather than a passed-in override list: `Override` is
        // not part of flutter_riverpod's public surface, and naming it here
        // would not compile.
        if (initialLocation != null)
          routerProvider.overrideWith(
            (ref) => createRouter(ref, initialLocation: initialLocation),
          ),
      ],
    );
    addTearDown(container.dispose);

    await tester.pumpWidget(
      UncontrolledProviderScope(container: container, child: const IConfessApp()),
    );
    await tester.pumpAndSettle();
  }

  Future<void> goTo(WidgetTester tester, String location) async {
    container.read(routerProvider).go(location);
    await tester.pumpAndSettle();
  }

  Finder field(String label) => find.byKey(ValueKey('field-$label'));

  /// Types into a labelled field, scrolling it into view first.
  ///
  /// Sign-up and reset have more fields than fit in the test window, and a field
  /// below the fold cannot take focus — the text silently goes to whichever
  /// field had it last. Real users scroll; the test has to as well.
  Future<void> enter(WidgetTester tester, String label, String value) async {
    await tester.ensureVisible(field(label));
    await tester.pumpAndSettle();
    await tester.enterText(field(label), value);
  }

  /// The `TextField` inside a labelled field.
  ///
  /// `TextFormField` does not re-expose `autofillHints` or `obscureText`; the
  /// `TextField` it builds is where they live.
  TextField inputOf(WidgetTester tester, String label) => tester.widget<TextField>(
        find.descendant(of: field(label), matching: find.byType(TextField)),
      );

  group('sign in', () {
    testWidgets('a correct password signs in, stores the session and moves on',
        (tester) async {
      api.respond('/auth/login', {
        'token': 'session-token',
        'user': {'id': 'u1', 'email': 'grace@example.com'},
      });
      await pumpApp(tester);
      await goTo(tester, AppRoutes.signIn);

      await enter(tester, 'Email', 'grace@example.com');
      await enter(tester, 'Password', 'correct horse battery');
      await tester.tap(find.widgetWithText(FilledButton, 'Sign in'));
      await tester.pumpAndSettle();

      // The token has to be in the store, not just in memory. PHASE 18 shipped a
      // sign-in that set the state and never wrote the credential, so every
      // request after it went out unauthenticated.
      expect(await tokens.read(), 'session-token');
      expect(find.text('Home'), findsWidgets, reason: 'routing followed the session');
      expect(
        analytics.events.map((e) => e.name),
        contains(AnalyticsEvents.signInSucceeded),
      );
      expect(api.bodyOf('/auth/login'), {
        'email': 'grace@example.com',
        'password': 'correct horse battery',
      });
    });

    testWidgets('a wrong password says so, and does not claim the session ended',
        (tester) async {
      api.respondWith(
        '/auth/login',
        const ApiError(status: 401, code: '', message: 'invalid credentials'),
      );
      await pumpApp(tester);
      await goTo(tester, AppRoutes.signIn);

      await enter(tester, 'Email', 'grace@example.com');
      await enter(tester, 'Password', 'wrong password here');
      await tester.tap(find.widgetWithText(FilledButton, 'Sign in'));
      await tester.pumpAndSettle();

      expect(find.text('That didn’t match'), findsOneWidget);
      expect(find.textContaining('session has ended'), findsNothing);
      expect(find.text('Sign in'), findsWidgets, reason: 'still on the form');
      expect(await tokens.hasSession(), isFalse);
    });

    testWidgets('being offline is not reported as a wrong password',
        (tester) async {
      api.respondWith('/auth/login', const NetworkException('You appear to be offline.'));
      await pumpApp(tester);
      await goTo(tester, AppRoutes.signIn);

      await enter(tester, 'Email', 'grace@example.com');
      await enter(tester, 'Password', 'correct horse battery');
      await tester.tap(find.widgetWithText(FilledButton, 'Sign in'));
      await tester.pumpAndSettle();

      expect(find.text('You’re offline'), findsOneWidget);
      expect(find.textContaining('Nothing was sent'), findsOneWidget,
          reason: 'someone on a train must not be told to change a password '
              'that was never wrong');
    });

    testWidgets('empty fields are refused locally and nothing is sent',
        (tester) async {
      await pumpApp(tester);
      await goTo(tester, AppRoutes.signIn);

      await tester.tap(find.widgetWithText(FilledButton, 'Sign in'));
      await tester.pumpAndSettle();

      expect(find.text('Enter your email address.'), findsOneWidget);
      expect(find.text('Enter your password.'), findsOneWidget);
      expect(api.calls, isEmpty, reason: 'a request that cannot succeed is not sent');
    });

    testWidgets('an existing password is not measured against the signup policy',
        (tester) async {
      // Accounts that predate the policy may hold a short password. Refusing it
      // here would lock someone out of an account the server would accept.
      api.respond('/auth/login', {
        'token': 't',
        'user': {'id': 'u1'},
      });
      await pumpApp(tester);
      await goTo(tester, AppRoutes.signIn);

      await enter(tester, 'Email', 'grace@example.com');
      await enter(tester, 'Password', 'short');
      await tester.tap(find.widgetWithText(FilledButton, 'Sign in'));
      await tester.pumpAndSettle();

      expect(api.callCount('/auth/login'), 1);
      expect(find.textContaining('at least 8 characters'), findsNothing);
    });

    testWidgets('an MFA challenge asks for the code in place, then completes',
        (tester) async {
      api.respond('/auth/login', {'mfa_required': true});
      await pumpApp(tester);
      await goTo(tester, AppRoutes.signIn);

      await enter(tester, 'Email', 'grace@example.com');
      await enter(tester, 'Password', 'correct horse battery');
      await tester.tap(find.widgetWithText(FilledButton, 'Sign in'));
      await tester.pumpAndSettle();

      // Not a failure: the credentials were right, the sign-in is incomplete.
      expect(find.text('That didn’t match'), findsNothing);
      expect(find.text('One more step'), findsOneWidget);
      expect(field('Authentication code'), findsOneWidget);
      expect(await tokens.hasSession(), isFalse,
          reason: 'the second factor is what gates the session (S41)');

      api.respond('/auth/login', {
        'token': 'after-mfa',
        'user': {'id': 'u1'},
      });
      await enter(tester, 'Authentication code', '123456');
      await tester.tap(find.widgetWithText(FilledButton, 'Verify'));
      await tester.pumpAndSettle();

      expect(await tokens.read(), 'after-mfa');
      expect(api.bodyOf('/auth/login'), {
        'email': 'grace@example.com',
        'password': 'correct horse battery',
        'code': '123456',
      });
    });

    testWidgets('a wrong code is a field error, not a failed sign-in',
        (tester) async {
      api.respond('/auth/login', {'mfa_required': true});
      await pumpApp(tester);
      await goTo(tester, AppRoutes.signIn);

      await enter(tester, 'Email', 'grace@example.com');
      await enter(tester, 'Password', 'correct horse battery');
      await tester.tap(find.widgetWithText(FilledButton, 'Sign in'));
      await tester.pumpAndSettle();

      api.respondWith(
        '/auth/login',
        const ApiError(
            status: 401, code: ErrorCodes.mfaCodeInvalid, message: 'that code is not valid'),
      );
      await enter(tester, 'Authentication code', '000000');
      await tester.tap(find.widgetWithText(FilledButton, 'Verify'));
      await tester.pumpAndSettle();

      expect(find.text('That code isn’t valid. Enter the next one.'), findsOneWidget);
      expect(find.text('That didn’t match'), findsNothing,
          reason: 'the password was correct; a page-level failure would send '
              'someone back to change it');
      expect(find.text('One more step'), findsOneWidget);
    });
  });

  group('keyboard behaviour', () {
    testWidgets('the last field submits, the others advance', (tester) async {
      await pumpApp(tester);
      await goTo(tester, AppRoutes.signIn);

      final emailField = inputOf(tester, 'Email');
      final passwordField = inputOf(tester, 'Password');

      expect(emailField.textInputAction, TextInputAction.next);
      expect(passwordField.textInputAction, TextInputAction.done,
          reason: 'on the last field, "done" has to reach the button rather '
              'than sending the listener hunting for it');
      expect(emailField.keyboardType, TextInputType.emailAddress);
    });

    testWidgets('every field carries autofill hints for the password manager',
        (tester) async {
      await pumpApp(tester);
      await goTo(tester, AppRoutes.signIn);

      final email = inputOf(tester, 'Email');
      final password = inputOf(tester, 'Password');

      expect(email.autofillHints, contains(AutofillHints.email));
      expect(password.autofillHints, contains(AutofillHints.password));
      expect(find.byType(AutofillGroup), findsOneWidget,
          reason: 'without the group the manager cannot save the pair together');
    });

    testWidgets('sign-up asks for a new password, not a reused one',
        (tester) async {
      await pumpApp(tester);
      await goTo(tester, AppRoutes.signUp);

      final password = inputOf(tester, 'Password');
      expect(password.autofillHints, contains(AutofillHints.newPassword));
      expect(password.obscureText, isTrue);
    });

    testWidgets('the reveal toggle flips the field and is announced',
        (tester) async {
      await pumpApp(tester);
      await goTo(tester, AppRoutes.signIn);

      expect(inputOf(tester, 'Password').obscureText, isTrue);

      await tester.tap(find.byTooltip('Show password'));
      await tester.pumpAndSettle();

      expect(inputOf(tester, 'Password').obscureText, isFalse);
      expect(find.byTooltip('Hide password'), findsOneWidget);
    });
  });

  group('sign up', () {
    testWidgets('a new account is signed in and sent to confirm its email',
        (tester) async {
      api.respond('/auth/register', {
        'token': 'new-session',
        'user': {'id': 'u9'},
      });
      await pumpApp(tester);
      await goTo(tester, AppRoutes.signUp);

      await enter(tester, 'Name (optional)', 'Grace');
      await enter(tester, 'Email', 'grace@example.com');
      await enter(tester, 'Password', 'correct horse battery');
      await enter(tester, 'Confirm password', 'correct horse battery');
      await tester.tap(find.widgetWithText(FilledButton, 'Create account'));
      await tester.pumpAndSettle();

      expect(find.text('Check your email'), findsOneWidget);
      expect(await tokens.read(), 'new-session');
      expect(analytics.events.map((e) => e.name),
          contains(AnalyticsEvents.signUpSucceeded));
      expect(api.bodyOf('/auth/register')!['display_name'], 'Grace');
      expect(api.bodyOf('/auth/register')!['timezone'], isNotEmpty,
          reason: 'sent so "this evening" means the listener\'s evening');
    });

    testWidgets('an address that already exists looks identical, and grants nothing',
        (tester) async {
      // The server answers a duplicate exactly as it answers a fresh signup,
      // minus the session, so that registration is not a membership oracle
      // (S65). The screen must not undo that with its own wording.
      api.respond('/auth/register', {
        'message': 'Check your email to continue setting up your account.',
        'pending': true,
      });
      await pumpApp(tester);
      await goTo(tester, AppRoutes.signUp);

      await enter(tester, 'Email', 'taken@example.com');
      await enter(tester, 'Password', 'correct horse battery');
      await enter(tester, 'Confirm password', 'correct horse battery');
      await tester.tap(find.widgetWithText(FilledButton, 'Create account'));
      await tester.pumpAndSettle();

      // The same screen, the same copy, and no session: nothing about the
      // response distinguishes it from a fresh signup.
      expect(find.text('Check your email'), findsOneWidget);
      expect(find.byType(ErrorBanner), findsNothing,
          reason: 'a rejection here would tell the caller the address exists, '
              'turning the form into a membership lookup');
      expect(await tokens.hasSession(), isFalse);
      expect(analytics.events.map((e) => e.name),
          isNot(contains(AnalyticsEvents.signUpSucceeded)));
    });

    testWidgets('a breached password is refused before anything is sent',
        (tester) async {
      await pumpApp(tester);
      await goTo(tester, AppRoutes.signUp);

      await enter(tester, 'Email', 'grace@example.com');
      await enter(tester, 'Password', 'password');
      await enter(tester, 'Confirm password', 'password');
      await tester.tap(find.widgetWithText(FilledButton, 'Create account'));
      await tester.pumpAndSettle();

      expect(find.textContaining('known breaches'), findsOneWidget);
      expect(api.calls, isEmpty);
    });

    testWidgets('mismatched confirmation is caught on the device',
        (tester) async {
      await pumpApp(tester);
      await goTo(tester, AppRoutes.signUp);

      await enter(tester, 'Email', 'grace@example.com');
      await enter(tester, 'Password', 'correct horse battery');
      await enter(tester, 'Confirm password', 'correct horse batteri');
      await tester.tap(find.widgetWithText(FilledButton, 'Create account'));
      await tester.pumpAndSettle();

      expect(find.text('The two passwords don’t match.'), findsOneWidget);
      expect(api.calls, isEmpty);
    });

    testWidgets('a rejected registration leaves the form filled in', (tester) async {
      api.respondWith(
        '/auth/register',
        const ApiError(
            status: 400, code: '', message: 'password must be at least 8 characters'),
      );
      await pumpApp(tester);
      await goTo(tester, AppRoutes.signUp);

      await enter(tester, 'Email', 'grace@example.com');
      await enter(tester, 'Password', 'correct horse battery');
      await enter(tester, 'Confirm password', 'correct horse battery');
      await tester.tap(find.widgetWithText(FilledButton, 'Create account'));
      await tester.pumpAndSettle();

      // What the listener typed is still there: a form that clears itself on a
      // failure makes them start again for no reason.
      expect(find.text('That didn’t work'), findsOneWidget);
      expect(find.textContaining('Password must be at least 8 characters'),
          findsOneWidget);
      expect(
        tester.widget<TextFormField>(
          find.descendant(of: field('Email'), matching: find.byType(TextFormField)),
        ).controller!.text,
        'grace@example.com',
      );
    });
  });

  group('forgot password', () {
    testWidgets('the confirmation does not reveal whether the address exists',
        (tester) async {
      api.respond('/auth/request-password-reset', {
        'message': 'If an account exists for that email, a reset link has been sent.',
      });
      await pumpApp(tester);
      await goTo(tester, AppRoutes.forgotPassword);

      await enter(tester, 'Email', 'maybe@example.com');
      await tester.tap(find.widgetWithText(FilledButton, 'Send reset link'));
      await tester.pumpAndSettle();

      expect(find.text('Check your email'), findsOneWidget);
      expect(find.textContaining('If an account exists'), findsOneWidget);
      expect(find.textContaining('we’ve sent you'), findsNothing);
      expect(find.widgetWithText(FilledButton, 'Back to sign in'), findsOneWidget);
    });

    testWidgets('the address is carried over from sign-in', (tester) async {
      await pumpApp(tester);
      await goTo(tester, '${AppRoutes.forgotPassword}?email=grace%40example.com');

      expect(
        tester
            .widget<TextFormField>(
              find.descendant(of: field('Email'), matching: find.byType(TextFormField)),
            )
            .controller!
            .text,
        'grace@example.com',
      );
    });
  });

  group('reset password', () {
    testWidgets('a valid token changes the password and signs the device out',
        (tester) async {
      api.respond('/auth/reset-password', {'message': 'password reset successfully'});
      await tokens.save('a-session-that-must-not-survive');

      await pumpApp(tester);
      await goTo(tester, '${AppRoutes.resetPassword}?token=abc123');

      await enter(tester, 'New password', 'a completely new one');
      await enter(tester, 'Confirm new password', 'a completely new one');
      await tester.tap(find.widgetWithText(FilledButton, 'Update password'));
      await tester.pumpAndSettle();

      expect(find.text('Password updated'), findsOneWidget);
      expect(find.textContaining('every device'), findsOneWidget,
          reason: 'arriving at sign-in after a successful reset looks like a '
              'bug unless it is explained');
      expect(api.bodyOf('/auth/reset-password'), {
        'token': 'abc123',
        'password': 'a completely new one',
      });
      // The server revoked every session; keeping this one would defeat it.
      expect(await tokens.hasSession(), isFalse);
    });

    testWidgets('an expired token is reported and the form stays usable',
        (tester) async {
      api.respondWith(
        '/auth/reset-password',
        const ApiError(
            status: 401, code: '', message: 'invalid or expired reset token'),
      );
      await pumpApp(tester);
      await goTo(tester, AppRoutes.resetPassword);

      await enter(tester, 'Reset code', 'old-token');
      await enter(tester, 'New password', 'a completely new one');
      await enter(tester, 'Confirm new password', 'a completely new one');
      await tester.tap(find.widgetWithText(FilledButton, 'Update password'));
      await tester.pumpAndSettle();

      expect(find.text('That code has expired'), findsOneWidget);
      expect(find.text('Password updated'), findsNothing);
    });
  });

  group('verification', () {
    testWidgets('the address can be confirmed by hand', (tester) async {
      api.respond('/auth/verify-email', {'message': 'email verified'});
      await pumpApp(tester);
      await goTo(tester, '${AppRoutes.verification}?email=grace%40example.com');

      expect(find.textContaining('grace@example.com'), findsOneWidget);

      await enter(tester, 'Or enter the code from the email', 'tok-1');
      await tester.tap(find.widgetWithText(FilledButton, 'Confirm address'));
      await tester.pumpAndSettle();

      expect(find.text('Email confirmed'), findsOneWidget);
      expect(api.bodyOf('/auth/verify-email'), {'token': 'tok-1'});
    });

    testWidgets('resending is rate limited on the device as well as the server',
        (tester) async {
      api.respond('/auth/resend-verification', {'message': 'sent'});
      await pumpApp(tester);
      await goTo(tester, '${AppRoutes.verification}?email=grace%40example.com');

      await tester.tap(find.widgetWithText(TextButton, 'Send the email again'));
      await tester.pump();
      await tester.pump();

      expect(find.text('Sent again'), findsOneWidget);
      expect(find.textContaining('Send again in'), findsOneWidget);
      expect(tester.widget<TextButton>(_resendButton(tester)).onPressed, isNull,
          reason: 'a button that can be tapped twenty times in five seconds just '
              'guarantees a 429 the listener then has to read');

      // Let the cooldown expire so no timer is left running at the end of the test.
      await tester.pump(const Duration(seconds: 31));
      expect(find.widgetWithText(TextButton, 'Send the email again'), findsOneWidget);
    });

    testWidgets('an invalid token is answered with a resend, not a dead end',
        (tester) async {
      api.respondWith(
        '/auth/verify-email',
        const ApiError(
            status: 401, code: '', message: 'invalid or expired verification token'),
      );
      await pumpApp(tester);
      await goTo(tester, '${AppRoutes.verification}?email=grace%40example.com');

      await enter(tester, 'Or enter the code from the email', 'stale');
      await tester.tap(find.widgetWithText(FilledButton, 'Confirm address'));
      await tester.pumpAndSettle();

      expect(find.text('That code has expired'), findsOneWidget);
      expect(find.widgetWithText(TextButton, 'Send the email again'), findsOneWidget,
          reason: 'the way out of an expired token is a new one');
    });
  });

  group('onboarding', () {
    testWidgets('three slides, and the last one creates the account',
        (tester) async {
      await pumpApp(tester);
      await goTo(tester, AppRoutes.onboarding);

      expect(find.text('Say what you need'), findsOneWidget);
      expect(_slideAnnouncement(tester), 'Slide 1 of 3');

      await tester.tap(find.widgetWithText(FilledButton, 'Next'));
      await tester.pumpAndSettle();
      expect(find.text('Set the length'), findsOneWidget);
      expect(_slideAnnouncement(tester), 'Slide 2 of 3');

      await tester.tap(find.widgetWithText(FilledButton, 'Next'));
      await tester.pumpAndSettle();
      expect(find.text('Choose the voice'), findsOneWidget);
      expect(_slideAnnouncement(tester), 'Slide 3 of 3');
      expect(find.widgetWithText(FilledButton, 'Create your account'), findsOneWidget);

      await tester.tap(find.widgetWithText(FilledButton, 'Create your account'));
      await tester.pumpAndSettle();

      expect(find.text('Create your account'), findsOneWidget,
          reason: 'the tour hands off to sign-up');
      expect(analytics.events.map((e) => e.name),
          contains(AnalyticsEvents.onboardingCompleted));
    });

    testWidgets('skipping is recorded, so the tour is not shown twice',
        (tester) async {
      await pumpApp(tester);
      await goTo(tester, AppRoutes.onboarding);

      await tester.tap(find.widgetWithText(TextButton, 'Skip'));
      await tester.pumpAndSettle();

      expect(find.text('Create your account'), findsOneWidget);
      expect(analytics.events.map((e) => e.name),
          contains(AnalyticsEvents.onboardingCompleted),
          reason: 'skipping is a completion, not an abandonment: the listener '
              'has told us they do not want the tour');
      final store = container.read(keyValueStoreProvider);
      expect(await store.readString(StoreKeys.onboardingSeen), 'true');
    });
  });

  group('accessibility', () {
    testWidgets('a failure is announced, not only shown', (tester) async {
      api.respondWith(
        '/auth/login',
        const ApiError(status: 401, code: '', message: 'invalid credentials'),
      );
      await pumpApp(tester);
      await goTo(tester, AppRoutes.signIn);

      await enter(tester, 'Email', 'grace@example.com');
      await enter(tester, 'Password', 'wrong password here');
      await tester.tap(find.widgetWithText(FilledButton, 'Sign in'));
      await tester.pumpAndSettle();

      // Without a live region a screen reader user presses the button, hears
      // nothing, and has no idea the attempt failed.
      final live = tester.widgetList<Semantics>(find.byType(Semantics)).where(
        (s) => s.properties.liveRegion == true,
      );
      expect(live, isNotEmpty);
    });

    testWidgets('every control on the flow meets the 44pt minimum',
        (tester) async {
      await pumpApp(tester);
      await goTo(tester, AppRoutes.signIn);

      final button = tester.getSize(find.widgetWithText(FilledButton, 'Sign in'));
      expect(button.height, greaterThanOrEqualTo(44));

      for (final finder in [
        find.widgetWithText(TextButton, 'Forgot your password?'),
      ]) {
        expect(tester.getSize(finder).height, greaterThanOrEqualTo(44));
      }
    });

    testWidgets('the brand mark is not read out as a heading', (tester) async {
      await pumpApp(tester);
      await goTo(tester, AppRoutes.welcome);

      final excluded = tester.widgetList<ExcludeSemantics>(find.byType(ExcludeSemantics));
      expect(excluded, isNotEmpty,
          reason: 'the wordmark is decoration; the screen supplies the words');
    });
  });

  group('routing around the flow', () {
    testWidgets('a signed-in listener is not bounced off the verification screen',
        (tester) async {
      // Registration issues a session before the address is confirmed, so the
      // listener is signed in *and* mid-flow. Treating verification as part of
      // the auth flow would throw them to Home at exactly the wrong moment.
      await pumpApp(tester, auth: const AuthState.signedIn(userId: 'u1'));
      await goTo(tester, AppRoutes.verification);

      expect(find.text('Check your email'), findsOneWidget);
      expect(find.text('Home'), findsNothing);
    });

    testWidgets('a signed-in listener cannot go back into sign-in',
        (tester) async {
      await pumpApp(tester, auth: const AuthState.signedIn(userId: 'u1'));
      await goTo(tester, AppRoutes.signIn);

      expect(find.text('Welcome back'), findsNothing);
      expect(find.text('Home'), findsWidgets);
    });

    testWidgets('the completion screen is reachable signed out', (tester) async {
      await pumpApp(tester);
      await goTo(tester, '${AppRoutes.completion}?reason=reset');

      expect(find.text('Password updated'), findsOneWidget);
      expect(find.widgetWithText(FilledButton, 'Go to sign in'), findsOneWidget);
    });

    testWidgets('a malformed completion reason does not crash', (tester) async {
      await pumpApp(tester);
      await goTo(tester, '${AppRoutes.completion}?reason=nonsense');

      expect(find.text('Password updated'), findsOneWidget,
          reason: 'this route is reachable from a link; a bad query string must '
              'not be a crash');
    });
  });

  group('email links', () {
    // internal/email builds https://…/verify-email?token=… and
    // https://…/reset-password?token=…, so those are the paths that have to
    // resolve and those are the strings a listener has to hand.

    testWidgets('the emailed path opens verification with its token prefilled',
        (tester) async {
      api.respond('/auth/verify-email', {'message': 'email verified'});
      await pumpApp(tester);
      await goTo(tester, '${AppRoutes.verifyEmail}?token=deep-link-tok');

      final field = inputOf(tester, 'Or enter the code from the email');
      expect(field.controller?.text, 'deep-link-tok',
          reason: 'prefilled, not submitted: arriving with a token is not '
              'consent to spend it, and an expired link has to be shown before '
              'it is used');

      await tester.tap(find.widgetWithText(FilledButton, 'Confirm address'));
      await tester.pumpAndSettle();
      expect(api.bodyOf('/auth/verify-email'), {'token': 'deep-link-tok'});
    });

    testWidgets('pasting the link instead of the token confirms the address',
        (tester) async {
      api.respond('/auth/verify-email', {'message': 'email verified'});
      await pumpApp(tester);
      await goTo(tester, '${AppRoutes.verification}?email=grace%40example.com');

      await enter(
        tester,
        'Or enter the code from the email',
        'https://iconfess.app/verify-email?token=pasted-tok',
      );
      await tester.tap(find.widgetWithText(FilledButton, 'Confirm address'));
      await tester.pumpAndSettle();

      expect(api.bodyOf('/auth/verify-email'), {'token': 'pasted-tok'});
      expect(find.text('Email confirmed'), findsOneWidget);
    });

    testWidgets('a cold start from the emailed link opens on verification',
        (tester) async {
      // The platform handler hands the router the location it was given. Nothing
      // else in these tests covers that, and an unexercised parameter is a
      // parameter that silently stops working.
      api.respond('/auth/verify-email', {'message': 'email verified'});
      await pumpApp(
        tester,
        initialLocation: '${AppRoutes.verifyEmail}?token=cold-start-tok',
      );

      expect(
        inputOf(tester, 'Or enter the code from the email').controller?.text,
        'cold-start-tok',
      );
      expect(find.widgetWithText(FilledButton, 'Create your account'), findsNothing,
          reason: 'the app opened on the link, not on the welcome screen');
    });

    testWidgets('pasting a reset link sets the new password', (tester) async {
      api.respond('/auth/reset-password', {'message': 'password reset successfully'});
      await pumpApp(tester);
      await goTo(tester, '${AppRoutes.resetPassword}?token=ignored');

      await enter(
        tester,
        'Reset code',
        'https://iconfess.app/reset-password?token=pasted-reset',
      );
      await enter(tester, 'New password', 'a completely new one');
      await enter(tester, 'Confirm new password', 'a completely new one');
      await tester.tap(find.widgetWithText(FilledButton, 'Update password'));
      await tester.pumpAndSettle();

      expect(api.bodyOf('/auth/reset-password')!['token'], 'pasted-reset',
          reason: 'the pasted link must not reach the server verbatim');
      expect(find.text('Password updated'), findsOneWidget);
    });
  });
}

/// The resend control, matched by what it says rather than by an exact
/// countdown — the seconds left depend on how the clock was pumped.
Finder _resendButton(WidgetTester tester) => find.byWidgetPredicate(
      (widget) =>
          widget is TextButton &&
          widget.child is Text &&
          (widget.child! as Text).data?.startsWith('Send again in') == true,
    );

/// The pager's spoken position.
///
/// The dots carry their position in a `Semantics` label rather than as text:
/// three unlabelled shapes tell a screen reader user nothing, and adding a
/// visible "1 of 3" would be clutter for everyone else. `find.bySemanticsLabel`
/// needs a semantics handle, so the widget is matched directly.
String? _slideAnnouncement(WidgetTester tester) {
  for (final widget in tester.widgetList<Semantics>(find.byType(Semantics))) {
    final label = widget.properties.label;
    if (label != null && label.startsWith('Slide ')) return label;
  }
  return null;
}

/// An auth controller pinned to a known state, so routing can be tested without
/// a keystore or a network.
class _FixedAuth extends AuthController {
  _FixedAuth(this._initial);
  final AuthState _initial;
  @override
  AuthState build() => _initial;
}
