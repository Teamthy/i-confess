import 'package:flutter/foundation.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../features/activity/activity_screen.dart';
import '../../features/auth/auth_controller.dart';
import '../../features/auth/completion_screen.dart';
import '../../features/auth/forgot_password_screen.dart';
import '../../features/auth/reset_password_screen.dart';
import '../../features/auth/sign_in_screen.dart';
import '../../features/auth/sign_up_screen.dart';
import '../../features/auth/verification_screen.dart';
import '../../features/auth/welcome_screen.dart';
import '../../features/bible/bible_screen.dart';
import '../../features/confess/confess_screen.dart';
import '../../features/confess/duration_screen.dart';
import '../../features/confess/review_screen.dart';
import '../../features/confess/voice_screen.dart';
import '../../features/confession/confession_detail_screen.dart';
import '../../features/community/community_screen.dart';
import '../../features/downloads/downloads_screen.dart';
import '../../features/explore/category_detail_screen.dart';
import '../../features/explore/explore_screen.dart';
import '../../features/home/home_screen.dart';
import '../../features/library/library_screen.dart';
import '../../features/me/me_screen.dart';
import '../../features/onboarding/onboarding_screen.dart';
import '../../features/player/player_screen.dart';
import '../../features/premium/premium_screen.dart';
import '../../features/search/search_screen.dart';
import '../../features/settings/settings_screen.dart';
import '../../features/shell/app_shell.dart';
import '../../features/shell/placeholder_screen.dart';
import '../../features/splash/splash_screen.dart';
import '../../features/templates/templates_screen.dart';
import 'routes.dart';

/// The app's router.
///
/// One router, one redirect rule. Access is decided here rather than in each
/// screen, so a screen cannot be reached by a deep link that its own guard would
/// have refused.
GoRouter createRouter(
  Ref ref, {
  Listenable? refreshListenable,
  String? initialLocation,
}) {
  return GoRouter(
    initialLocation: initialLocation ?? AppRoutes.splash,
    debugLogDiagnostics: false,
    refreshListenable: refreshListenable,
    redirect: (context, state) {
      final loc = state.matchedLocation;
      final auth = ref.read(authControllerProvider);

      if (!auth.isResolved) return null;

      final isAuthFlow = loc == AppRoutes.welcome ||
          loc == AppRoutes.signIn ||
          loc == AppRoutes.signUp ||
          loc == AppRoutes.forgotPassword ||
          loc == AppRoutes.resetPassword ||
          loc == AppRoutes.onboarding;

      if (!auth.isSignedIn) {
        if (AppRoutes.requiresAuth.any(loc.startsWith)) {
          return AppRoutes.welcome;
        }
        if (loc == AppRoutes.splash) return AppRoutes.welcome;
        return null;
      }

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

      // Five destinations
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
                  path: 'community',
                  name: AppRouteNames.community,
                  builder: (context, state) => const CommunityScreen(),
                ),
                GoRoute(
                  path: 'search',
                  name: AppRouteNames.search,
                  builder: (context, state) => const SearchScreen(),
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
              builder: (context, state) => const ConfessScreen(),
              routes: [
                GoRoute(
                  path: 'duration',
                  name: AppRouteNames.builderDuration,
                  builder: (context, state) => const DurationScreen(),
                ),
                GoRoute(
                  path: 'voice',
                  name: AppRouteNames.builderVoice,
                  builder: (context, state) => const VoiceScreen(),
                ),
                GoRoute(
                  path: 'create',
                  name: AppRouteNames.builderCreate,
                  builder: (context, state) => const ReviewScreen(),
                ),
              ],
            ),
          ]),
          StatefulShellBranch(routes: [
            GoRoute(
              path: AppRoutes.activity,
              name: AppRouteNames.activity,
              builder: (context, state) => const ActivityScreen(),
            ),
          ]),
          StatefulShellBranch(routes: [
            GoRoute(
              path: AppRoutes.me,
              name: AppRouteNames.me,
              builder: (context, state) => const MeScreen(),
              routes: [
                GoRoute(
                  path: 'edit',
                  name: AppRouteNames.meEdit,
                  builder: (context, state) => const ProfileEditScreen(),
                ),
                GoRoute(
                  path: 'settings',
                  name: AppRouteNames.meSettings,
                  builder: (context, state) => const SettingsScreen(),
                ),
                GoRoute(
                  path: 'premium',
                  name: AppRouteNames.mePremium,
                  builder: (context, state) => const PremiumScreen(),
                ),
              ],
            ),
          ]),
        ],
      ),

      // Bible reference deep links resolve through the same iCONFESS API as reader navigation.
      GoRoute(
        path: '${AppRoutes.bible}/:translation/:book/:chapter/:verse',
        name: 'bibleVerse',
        builder: (context, state) => BibleScreen(
          initialTranslation: state.pathParameters['translation'],
          initialReference: '${state.pathParameters['book']} ${state.pathParameters['chapter']}:${state.pathParameters['verse']}',
        ),
      ),
      GoRoute(
        path: '${AppRoutes.bible}/:translation/:book/:chapter',
        name: 'bibleChapter',
        builder: (context, state) => BibleScreen(
          initialTranslation: state.pathParameters['translation'],
          initialReference: '${state.pathParameters['book']} ${state.pathParameters['chapter']}',
        ),
      ),
      GoRoute(
        path: '${AppRoutes.bible}/:translation',
        name: 'bibleTranslation',
        builder: (context, state) => BibleScreen(initialTranslation: state.pathParameters['translation']),
      ),
      GoRoute(
        path: AppRoutes.bible,
        name: AppRouteNames.bible,
        builder: (context, state) => const BibleScreen(),
      ),

      // Full-screen flows above shell
      GoRoute(
        path: AppRoutes.player,
        name: AppRouteNames.player,
        builder: (context, state) => const PlaceholderScreen(title: 'Player', body: 'Select a session', immersive: true),
      ),
      GoRoute(
        path: '${AppRoutes.player}/:id',
        name: '${AppRouteNames.player}Detail',
        builder: (context, state) => PlayerScreen(
          sessionId: state.pathParameters['id'] ?? '',
        ),
      ),
      GoRoute(
        path: '${AppRoutes.confession}/:id',
        name: AppRouteNames.confessionDetail,
        builder: (context, state) => ConfessionDetailScreen(
          confessionId: state.pathParameters['id'] ?? '',
        ),
      ),
      GoRoute(
        path: AppRoutes.library,
        name: AppRouteNames.library,
        builder: (context, state) => const LibraryScreen(),
        routes: [
          GoRoute(
            path: 'collection/:id',
            name: AppRouteNames.collectionDetail,
            builder: (context, state) => CollectionDetailScreen(
              collectionId: state.pathParameters['id'] ?? '',
            ),
          ),
        ],
      ),
      GoRoute(
        path: AppRoutes.templates,
        name: AppRouteNames.templates,
        builder: (context, state) => const TemplatesScreen(),
        routes: [
          GoRoute(
            path: ':id',
            name: AppRouteNames.templateDetail,
            builder: (context, state) => TemplateDetailScreen(
              templateId: state.pathParameters['id'] ?? '',
            ),
          ),
        ],
      ),
      GoRoute(
        path: '${AppRoutes.templateShare}/:token',
        name: AppRouteNames.templateShare,
        builder: (context, state) => TemplateShareScreen(
          token: state.pathParameters['token'] ?? '',
        ),
      ),
      GoRoute(
        path: AppRoutes.search,
        name: 'searchRoot',
        builder: (context, state) => const SearchScreen(),
      ),
      GoRoute(
        path: AppRoutes.downloads,
        name: AppRouteNames.downloads,
        builder: (context, state) => const DownloadsScreen(),
      ),
      GoRoute(
        path: AppRoutes.settings,
        name: AppRouteNames.settings,
        builder: (context, state) => const SettingsScreen(),
        routes: [
          GoRoute(
            path: 'preferences',
            name: AppRouteNames.settingsPreferences,
            builder: (context, state) => const PreferencesScreen(),
          ),
          GoRoute(
            path: 'interests',
            name: AppRouteNames.settingsInterests,
            builder: (context, state) => const InterestsScreen(),
          ),
          GoRoute(
            path: 'notifications',
            name: AppRouteNames.settingsNotifications,
            builder: (context, state) => const NotificationsScreen(),
          ),
          GoRoute(
            path: 'devices',
            name: AppRouteNames.settingsDevices,
            builder: (context, state) => const DevicesScreen(),
          ),
          GoRoute(
            path: 'deletion',
            name: AppRouteNames.settingsDeletion,
            builder: (context, state) => const DeletionScreen(),
          ),
        ],
      ),
      GoRoute(
        path: AppRoutes.premium,
        name: AppRouteNames.premium,
        builder: (context, state) => const PremiumScreen(),
      ),
      GoRoute(
        path: AppRoutes.rituals,
        name: AppRouteNames.rituals,
        builder: (context, state) => const TemplatesScreen(),
      ),
      GoRoute(
        path: AppRoutes.saved,
        name: AppRouteNames.saved,
        builder: (context, state) => const LibraryScreen(),
      ),
      GoRoute(
        path: AppRoutes.history,
        name: AppRouteNames.history,
        builder: (context, state) => const ActivityScreen(),
      ),
    ],
  );
}

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
