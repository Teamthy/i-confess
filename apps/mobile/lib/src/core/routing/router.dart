import 'package:flutter/foundation.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../features/auth/auth_controller.dart';
import '../../features/auth/completion_screen.dart';
import '../../features/auth/forgot_password_screen.dart';
import '../../features/auth/reset_password_screen.dart';
import '../../features/auth/sign_in_screen.dart';
import '../../features/auth/sign_up_screen.dart';
import '../../features/auth/verification_screen.dart';
import '../../features/auth/welcome_screen.dart';
import '../../features/explore/category_detail_screen.dart';
import '../../features/explore/explore_screen.dart';
import '../../features/home/home_screen.dart';
import '../../features/onboarding/onboarding_screen.dart';
import '../../features/shell/app_shell.dart';
import '../../features/shell/placeholder_screen.dart';
import '../../features/splash/splash_screen.dart';
import 'routes.dart';

/// The app's router.
///
/// One router, one redirect rule. Access is decided here rather than in each
/// screen, so a screen cannot be reached by a deep link that its own guard would
/// have refused.
/// Builds the router.
///
/// [refreshListenable] is notified when the session changes, which is what makes
/// go_router re-run [redirect] for the location the listener is already on. The
/// alternative — watching the auth provider from the router's own provider — was
/// what PHASE 18 shipped, and it was worse than it looked: every auth transition
/// rebuilt the whole `GoRouter`, threw the navigation stack away and started
/// again at `/splash`. Sign-in survived that by luck, because the redirect sends
/// a signed-in listener to `/home` anyway; registration did not, because it was
/// on its way to the confirmation screen and got redirected to Home instead.
GoRouter createRouter(
  Ref ref, {
  Listenable? refreshListenable,
  String? initialLocation,
}) {
  return GoRouter(
    // Defaults to the splash, which hands off. A platform link handler passes
    // the location it was given instead, so `/verify-email?token=…` opens on the
    // screen that can use the token rather than on the brand screen.
    initialLocation: initialLocation ?? AppRoutes.splash,
    debugLogDiagnostics: false,
    refreshListenable: refreshListenable,
    redirect: (context, state) {
      final loc = state.matchedLocation;
      // Read, not watch. The router outlives any one auth state; watching here
      // would rebuild it, and a rebuilt router forgets where the listener was.
      final auth = ref.read(authControllerProvider);

      // While the keystore is being read, stay where we are. Routing to sign-in
      // during this window would flash it at every signed-in listener on every
      // cold start, which reads as being logged out.
      if (!auth.isResolved) return null;

      // Screens that only make sense for someone without a session.
      //
      // Verification and Completion are deliberately absent. Registration issues
      // a session before the address is confirmed, so a listener who has just
      // created an account is signed in *and* on their way to the confirmation
      // screen; putting it in this list would bounce them to Home mid-flow. Both
      // are harmless to reach at any time — neither grants anything — and
      // neither can be mistaken for a way in.
      final isAuthFlow = loc == AppRoutes.welcome ||
          loc == AppRoutes.signIn ||
          loc == AppRoutes.signUp ||
          loc == AppRoutes.forgotPassword ||
          loc == AppRoutes.resetPassword ||
          loc == AppRoutes.onboarding;

      // `verifyEmail` is absent for the same reason `verification` is: it is
      // where a signed-in listener goes to finish confirming an address they
      // registered with moments ago.

      if (!auth.isSignedIn) {
        // Browsing is allowed before an account exists; the flow is not.
        if (AppRoutes.requiresAuth.any(loc.startsWith)) {
          return AppRoutes.welcome;
        }
        // The splash has nothing to offer a signed-out listener and no way
        // forward, so it must hand off. Without this the app opens on the brand
        // screen and stays there.
        if (loc == AppRoutes.splash) return AppRoutes.welcome;
        return null;
      }

      // Signed in: the auth flow is no longer reachable, and the splash has
      // nothing left to do.
      if (isAuthFlow || loc == AppRoutes.splash) return AppRoutes.home;
      return null;
    },
    routes: [
      GoRoute(
        path: AppRoutes.splash,
        name: AppRouteNames.splash,
        builder: (context, state) => const SplashScreen(),
      ),
      GoRoute(
        path: AppRoutes.welcome,
        name: AppRouteNames.welcome,
        builder: (context, state) => const WelcomeScreen(),
      ),
      GoRoute(
        path: AppRoutes.onboarding,
        name: AppRouteNames.onboarding,
        builder: (context, state) => const OnboardingScreen(),
      ),
      GoRoute(
        path: AppRoutes.signIn,
        name: AppRouteNames.signIn,
        builder: (context, state) => const SignInScreen(),
      ),
      GoRoute(
        path: AppRoutes.signUp,
        name: AppRouteNames.signUp,
        builder: (context, state) => const SignUpScreen(),
      ),
      GoRoute(
        path: AppRoutes.forgotPassword,
        name: AppRouteNames.forgotPassword,
        // The address is carried over from sign-in so it does not have to be
        // retyped. Read from the query string rather than passed in memory: a
        // deep link to this screen is a real path once email links open the app.
        builder: (context, state) => ForgotPasswordScreen(
          initialEmail: state.uri.queryParameters['email'] ?? '',
        ),
      ),
      GoRoute(
        path: AppRoutes.resetPassword,
        name: AppRouteNames.resetPassword,
        builder: (context, state) => ResetPasswordScreen(
          token: state.uri.queryParameters['token'] ?? '',
        ),
      ),
      GoRoute(
        path: AppRoutes.verification,
        name: AppRouteNames.verification,
        builder: (context, state) => VerificationScreen(
          email: state.uri.queryParameters['email'] ?? '',
        ),
      ),
      GoRoute(
        // The path the email links to. A second route to the same screen, not a
        // rename: the in-app flow arrives with an address and no token.
        path: AppRoutes.verifyEmail,
        name: AppRouteNames.verifyEmail,
        builder: (context, state) => VerificationScreen(
          initialToken: state.uri.queryParameters['token'] ?? '',
        ),
      ),
      GoRoute(
        path: AppRoutes.completion,
        name: AppRouteNames.completion,
        builder: (context, state) => CompletionScreen(
          reason: CompletionReason.parse(state.uri.queryParameters['reason']),
        ),
      ),

      // The five destinations. An indexed stack keeps each branch's own
      // navigation history, so leaving Explore to check Activity and coming
      // back returns to the category you were reading rather than the top.
      StatefulShellRoute.indexedStack(
        builder: (context, state, shell) => AppShell(navigationShell: shell),
        branches: [
          StatefulShellBranch(routes: [
            GoRoute(
              path: AppRoutes.home,
              name: AppRouteNames.home,
              builder: (context, state) => const HomeScreen(),
            ),
          ]),
          StatefulShellBranch(routes: [
            GoRoute(
              path: AppRoutes.explore,
              name: AppRouteNames.explore,
              builder: (context, state) => const ExploreScreen(),
              routes: [
                GoRoute(
                  // Search lives on the explore screen; the route exists so a
                  // deep link or future voice action can land on the field.
                  path: 'search',
                  name: AppRouteNames.search,
                  builder: (context, state) => const ExploreScreen(),
                ),
                GoRoute(
                  path: 'category/:id',
                  name: AppRouteNames.categoryDetail,
                  builder: (context, state) => CategoryDetailScreen(
                    categoryId: state.pathParameters['id'] ?? '',
                  ),
                ),
              ],
            ),
          ]),
          StatefulShellBranch(routes: [
            GoRoute(
              path: AppRoutes.confess,
              name: AppRouteNames.confess,
              builder: (context, state) =>
                  const PlaceholderScreen(title: 'Create a session', body: 'PHASE 22'),
            ),
          ]),
          StatefulShellBranch(routes: [
            GoRoute(
              path: AppRoutes.activity,
              name: AppRouteNames.activity,
              builder: (context, state) =>
                  const PlaceholderScreen(title: 'Activity', body: 'PHASE 24'),
            ),
          ]),
          StatefulShellBranch(routes: [
            GoRoute(
              path: AppRoutes.me,
              name: AppRouteNames.me,
              builder: (context, state) =>
                  const PlaceholderScreen(title: 'Me', body: 'PHASE 25'),
              routes: [
                GoRoute(
                  path: 'settings',
                  name: AppRouteNames.settings,
                  builder: (context, state) =>
                      const PlaceholderScreen(title: 'Settings', body: 'PHASE 25'),
                ),
                GoRoute(
                  path: 'premium',
                  name: AppRouteNames.premium,
                  builder: (context, state) =>
                      const PlaceholderScreen(title: 'Premium', body: 'PHASE 30'),
                ),
              ],
            ),
          ]),
        ],
      ),

      // Full-screen flows that sit above the shell: the player is immersive and
      // must not carry the tab bar with it.
      GoRoute(
        path: AppRoutes.player,
        name: AppRouteNames.player,
        builder: (context, state) =>
            const PlaceholderScreen(title: 'Player', body: 'PHASE 23', immersive: true),
      ),
      GoRoute(
        path: '${AppRoutes.confession}/:id',
        name: AppRouteNames.confessionDetail,
        builder: (context, state) => PlaceholderScreen(
          title: 'Confession',
          body: state.pathParameters['id'] ?? '',
        ),
      ),
      GoRoute(
        path: AppRoutes.voices,
        name: AppRouteNames.voices,
        builder: (context, state) =>
            const PlaceholderScreen(title: 'Choose a voice', body: 'PHASE 22'),
      ),
      GoRoute(
        path: AppRoutes.rituals,
        name: AppRouteNames.rituals,
        builder: (context, state) =>
            const PlaceholderScreen(title: 'My ritual', body: 'PHASE 26'),
      ),
      GoRoute(
        path: AppRoutes.saved,
        name: AppRouteNames.saved,
        builder: (context, state) =>
            const PlaceholderScreen(title: 'Saved', body: 'PHASE 24'),
      ),
      GoRoute(
        path: AppRoutes.downloads,
        name: AppRouteNames.downloads,
        builder: (context, state) =>
            const PlaceholderScreen(title: 'Downloads', body: 'PHASE 28'),
      ),
      GoRoute(
        path: AppRoutes.history,
        name: AppRouteNames.history,
        builder: (context, state) =>
            const PlaceholderScreen(title: 'History', body: 'PHASE 24'),
      ),
    ],
  );
}

/// The router as a provider, so tests can override it and screens can watch it.
///
/// Built once. The notifier exists only to tell go_router that the session
/// changed; it carries no state of its own.
final routerProvider = Provider<GoRouter>((ref) {
  final refresh = ValueNotifier<int>(0);
  final subscription = ref.listen<AuthState>(
    authControllerProvider,
    (_, _) => refresh.value++,
  );
  ref.onDispose(() {
    subscription.close();
    refresh.dispose();
  });
  return createRouter(ref, refreshListenable: refresh);
});
