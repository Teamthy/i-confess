/// Every route the app knows, in one place.
///
/// Paths as constants rather than string literals at call sites: a typo in
/// `context.go('/explore')` is a runtime 404, while a typo in [AppRoutes.explore]
/// is a compile error.
abstract final class AppRoutes {
  static const splash = '/splash';

  // Auth and onboarding
  static const welcome = '/welcome';
  static const onboarding = '/onboarding';
  static const signIn = '/sign-in';
  static const signUp = '/sign-up';
  static const forgotPassword = '/forgot-password';
  static const resetPassword = '/reset-password';
  static const verification = '/verification';

  /// The path the verification email links to.
  ///
  /// Served alongside [verification] rather than instead of it. `internal/email`
  /// builds `https://…/verify-email?token=…`, so this is the path that has to
  /// resolve for a deep link to land on the right screen; the in-app flow uses
  /// [verification] because it carries the address rather than a token.
  static const verifyEmail = '/verify-email';

  /// Where a flow ends: a changed password, a confirmed address. Its own route
  /// so that pressing back cannot resubmit the token that produced it.
  static const completion = '/completion';

  // Primary destinations
  static const home = '/home';
  static const explore = '/explore';
  static const confess = '/confess';
  static const activity = '/activity';
  static const me = '/me';

  // Detail routes
  static const search = '/explore/search';
  static const community = '/explore/community';
  static const category = '/explore/category';
  static const confession = '/confession';
  static const player = '/player';

  /// The builder's steps after category selection, in walk order (§12:
  /// `confess → confess/duration → confess/voice → confess/create → player`).
  /// Paths match the information architecture one-to-one, because a deep
  /// link lands on them verbatim.
  static const builderDuration = '/confess/duration';
  static const builderVoice = '/confess/voice';
  static const builderCreate = '/confess/create';
  static const rituals = '/rituals';
  static const saved = '/saved';
  static const downloads = '/downloads';
  static const history = '/history';
  static const premium = '/premium';
  static const settings = '/settings';
  static const library = '/library';
  static const templates = '/templates';
  static const templateShare = '/t';
  static const meEdit = '/me/edit';
  static const mePremium = '/me/premium';
  static const meSettings = '/me/settings';
  static const settingsPreferences = '/settings/preferences';
  static const settingsInterests = '/settings/interests';
  static const settingsNotifications = '/settings/notifications';
  static const settingsDevices = '/settings/devices';
  static const settingsSecurity = '/settings/security';
  static const settingsDownloads = '/settings/downloads';
  static const settingsDeletion = '/settings/deletion';
  static const settingsExport = '/settings/export';

  /// Builds a category detail path.
  static String categoryDetail(String id) => '$category/$id';

  /// Builds a confession detail path.
  static String confessionDetail(String id) => '$confession/$id';

  static String templateDetail(String id) => '$templates/$id';
  static String collectionDetail(String id) => '$library/collection/$id';
  static String playerWithId(String id) => '$player/$id';

  /// The five bottom-navigation destinations, in display order.
  ///
  /// Index 2 is the confession action, which the shell renders differently: it is
  /// the primary action of the product and the navigation has to say so, but with
  /// emphasis rather than an oversized floating button.
  static const destinations = <String>[home, explore, confess, activity, me];

  /// Which destinations require a session.
  ///
  /// Home and Explore are deliberately not in here: a listener should be able to
  /// see what the product is before being asked to create an account. Requiring
  /// sign-in to browse is the fastest way to lose someone who arrived curious.
  static const requiresAuth = <String>{confess, activity, me, player, rituals, saved, downloads};
}

/// Names used for `goNamed`, kept separate so a path can change without touching
/// call sites.
abstract final class AppRouteNames {
  static const splash = 'splash';
  static const welcome = 'welcome';
  static const onboarding = 'onboarding';
  static const signIn = 'signIn';
  static const signUp = 'signUp';
  static const forgotPassword = 'forgotPassword';
  static const resetPassword = 'resetPassword';
  static const verification = 'verification';
  static const verifyEmail = 'verifyEmail';
  static const completion = 'completion';
  static const home = 'home';
  static const explore = 'explore';
  static const confess = 'confess';
  static const activity = 'activity';
  static const me = 'me';
  static const search = 'search';
  static const community = 'community';
  static const categoryDetail = 'categoryDetail';
  static const confessionDetail = 'confessionDetail';
  static const player = 'player';
  static const builderDuration = 'builderDuration';
  static const builderVoice = 'builderVoice';
  static const builderCreate = 'builderCreate';
  static const rituals = 'rituals';
  static const saved = 'saved';
  static const downloads = 'downloads';
  static const history = 'history';
  static const premium = 'premium';
  static const settings = 'settings';
  static const library = 'library';
  static const templates = 'templates';
  static const templateDetail = 'templateDetail';
  static const templateShare = 'templateShare';
  static const collectionDetail = 'collectionDetail';
  static const meEdit = 'meEdit';
  static const mePremium = 'mePremium';
  static const meSettings = 'meSettings';
  static const settingsPreferences = 'settingsPreferences';
  static const settingsInterests = 'settingsInterests';
  static const settingsNotifications = 'settingsNotifications';
  static const settingsDevices = 'settingsDevices';
  static const settingsSecurity = 'settingsSecurity';
  static const settingsDownloads = 'settingsDownloads';
  static const settingsDeletion = 'settingsDeletion';
  static const settingsExport = 'settingsExport';
}
