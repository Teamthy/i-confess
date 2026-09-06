import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../features/auth/auth_controller.dart';
import '../../features/shell/app_shell.dart';
import '../../features/shell/placeholder_screen.dart';
import 'routes.dart';

/// The app's router.
///
/// One router, one redirect rule. Access is decided here rather than in each
/// screen, so a screen cannot be reached by a deep link that its own guard would
/// have refused.
GoRouter createRouter(Ref ref) {
  final auth = ref.watch(authControllerProvider);

  return GoRouter(
    initialLocation: AppRoutes.splash,
    debugLogDiagnostics: false,
    redirect: (context, state) {
      final loc = state.matchedLocation;

      // While the keystore is being read, stay where we are. Routing to sign-in
      // during this window would flash it at every signed-in listener on every
      // cold start, which reads as being logged out.
      if (!auth.isResolved) return null;

      final isAuthFlow = loc == AppRoutes.welcome ||
          loc == AppRoutes.signIn ||
          loc == AppRoutes.signUp ||
          loc == AppRoutes.forgotPassword ||
          loc == AppRoutes.resetPassword ||
          loc == AppRoutes.verification ||
          loc == AppRoutes.onboarding;

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
        builder: (context, state) => const PlaceholderScreen(
          title: 'I CONFESS',
          body: 'Speak. Believe.',
          immersive: true,
        ),
      ),
      GoRoute(
        path: AppRoutes.welcome,
        name: AppRouteNames.welcome,
        builder: (context, state) =>
            const PlaceholderScreen(title: 'Welcome', body: 'PHASE 19'),
      ),
      GoRoute(
        path: AppRoutes.onboarding,
        name: AppRouteNames.onboarding,
        builder: (context, state) =>
            const PlaceholderScreen(title: 'Onboarding', body: 'PHASE 19'),
      ),
      GoRoute(
        path: AppRoutes.signIn,
        name: AppRouteNames.signIn,
        builder: (context, state) =>
            const PlaceholderScreen(title: 'Sign in', body: 'PHASE 19'),
      ),
      GoRoute(
        path: AppRoutes.signUp,
        name: AppRouteNames.signUp,
        builder: (context, state) =>
            const PlaceholderScreen(title: 'Create account', body: 'PHASE 19'),
      ),
      GoRoute(
        path: AppRoutes.forgotPassword,
        name: AppRouteNames.forgotPassword,
        builder: (context, state) =>
            const PlaceholderScreen(title: 'Forgot password', body: 'PHASE 19'),
      ),
      GoRoute(
        path: AppRoutes.resetPassword,
        name: AppRouteNames.resetPassword,
        builder: (context, state) =>
            const PlaceholderScreen(title: 'Reset password', body: 'PHASE 19'),
      ),
      GoRoute(
        path: AppRoutes.verification,
        name: AppRouteNames.verification,
        builder: (context, state) =>
            const PlaceholderScreen(title: 'Verify your email', body: 'PHASE 19'),
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
              builder: (context, state) =>
                  const PlaceholderScreen(title: 'Home', body: 'PHASE 20'),
            ),
          ]),
          StatefulShellBranch(routes: [
            GoRoute(
              path: AppRoutes.explore,
              name: AppRouteNames.explore,
              builder: (context, state) =>
                  const PlaceholderScreen(title: 'Explore', body: 'PHASE 21'),
              routes: [
                GoRoute(
                  path: 'search',
                  name: AppRouteNames.search,
                  builder: (context, state) =>
                      const PlaceholderScreen(title: 'Search', body: 'PHASE 21'),
                ),
                GoRoute(
                  path: 'category/:id',
                  name: AppRouteNames.categoryDetail,
                  builder: (context, state) => PlaceholderScreen(
                    title: 'Category',
                    body: state.pathParameters['id'] ?? '',
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
final routerProvider = Provider<GoRouter>(createRouter);
