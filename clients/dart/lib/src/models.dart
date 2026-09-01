/// Domain models.
///
/// Every model parses defensively: a missing or unexpectedly-typed field
/// yields a sensible default rather than throwing. An older app talking to a
/// newer server must degrade, not crash — and the reverse case (a field the
/// server has not shipped yet) is the same problem.
///
/// Nothing here has a `toJson`. These are read models; requests are built
/// explicitly at the call site, so a field cannot be sent back to the server
/// just because it happened to be parsed.
library;

/// Safe accessors. Used everywhere below so the degradation rule is applied
/// consistently rather than remembered case by case.
String _str(Map<String, dynamic> j, String k, [String fallback = '']) {
  final v = j[k];
  return v is String ? v : fallback;
}

int _int(Map<String, dynamic> j, String k, [int fallback = 0]) {
  final v = j[k];
  if (v is int) return v;
  if (v is num) return v.toInt();
  if (v is String) return int.tryParse(v) ?? fallback;
  return fallback;
}

bool _bool(Map<String, dynamic> j, String k, [bool fallback = false]) {
  final v = j[k];
  if (v is bool) return v;
  if (v is String) return v == 'true';
  return fallback;
}

DateTime? _time(Map<String, dynamic> j, String k) {
  final v = j[k];
  return v is String && v.isNotEmpty ? DateTime.tryParse(v) : null;
}

List<Map<String, dynamic>> _list(Object? v) {
  if (v is! List) return const [];
  return v.whereType<Map<String, dynamic>>().toList(growable: false);
}

/// The signed-in account.
final class Account {
  const Account({
    required this.id,
    required this.email,
    this.displayName = '',
    this.timezone = 'UTC',
    this.emailVerified = false,
  });

  final String id;
  final String email;
  final String displayName;
  final String timezone;
  final bool emailVerified;

  factory Account.fromJson(Map<String, dynamic> json) => Account(
        id: _str(json, 'id'),
        email: _str(json, 'email'),
        displayName: _str(json, 'display_name'),
        timezone: _str(json, 'timezone', 'UTC'),
        emailVerified: _bool(json, 'email_verified'),
      );
}

/// Presentation identity, separate from the security record (§5).
final class Profile {
  const Profile({
    this.displayName = '',
    this.username = '',
    this.bio = '',
    this.avatarUrl = '',
    this.timezone = 'UTC',
    this.locale = 'en',
    this.language = 'en',
    this.countryCode = '',
  });

  final String displayName;
  final String username;
  final String bio;
  final String avatarUrl;
  final String timezone;
  final String locale;
  final String language;
  final String countryCode;

  factory Profile.fromJson(Map<String, dynamic> json) => Profile(
        displayName: _str(json, 'display_name'),
        username: _str(json, 'username'),
        bio: _str(json, 'bio'),
        avatarUrl: _str(json, 'avatar_url'),
        timezone: _str(json, 'timezone', 'UTC'),
        locale: _str(json, 'locale', 'en'),
        language: _str(json, 'language', 'en'),
        countryCode: _str(json, 'country_code'),
      );

  Profile copyWith({
    String? displayName,
    String? username,
    String? bio,
    String? avatarUrl,
    String? timezone,
    String? locale,
    String? language,
    String? countryCode,
  }) =>
      Profile(
        displayName: displayName ?? this.displayName,
        username: username ?? this.username,
        bio: bio ?? this.bio,
        avatarUrl: avatarUrl ?? this.avatarUrl,
        timezone: timezone ?? this.timezone,
        locale: locale ?? this.locale,
        language: language ?? this.language,
        countryCode: countryCode ?? this.countryCode,
      );
}

/// Listening and notification settings (§20).
final class Preferences {
  const Preferences({
    this.defaultDuration = 1800,
    this.defaultVoiceId = '',
    this.autoplay = true,
    this.preferredQuality = 'standard',
    this.downloadOverWifi = true,
    this.notificationsEnabled = true,
    this.recommendationsEnabled = true,
    this.personalizationEnabled = true,
    this.language = 'en',
    this.theme = 'system',
  });

  final int defaultDuration;
  final String defaultVoiceId;
  final bool autoplay;
  final String preferredQuality;
  final bool downloadOverWifi;
  final bool notificationsEnabled;
  final bool recommendationsEnabled;
  final bool personalizationEnabled;
  final String language;
  final String theme;

  factory Preferences.fromJson(Map<String, dynamic> json) => Preferences(
        defaultDuration: _int(json, 'default_duration', 1800),
        defaultVoiceId: _str(json, 'default_voice_id'),
        autoplay: _bool(json, 'autoplay', true),
        preferredQuality: _str(json, 'preferred_quality', 'standard'),
        downloadOverWifi: _bool(json, 'download_over_wifi', true),
        notificationsEnabled: _bool(json, 'notifications_enabled', true),
        recommendationsEnabled: _bool(json, 'recommendations_enabled', true),
        personalizationEnabled: _bool(json, 'personalization_enabled', true),
        language: _str(json, 'language', 'en'),
        theme: _str(json, 'theme', 'system'),
      );

  Preferences copyWith({
    int? defaultDuration,
    String? defaultVoiceId,
    bool? autoplay,
    String? preferredQuality,
    bool? downloadOverWifi,
    bool? notificationsEnabled,
    bool? recommendationsEnabled,
    bool? personalizationEnabled,
    String? language,
    String? theme,
  }) =>
      Preferences(
        defaultDuration: defaultDuration ?? this.defaultDuration,
        defaultVoiceId: defaultVoiceId ?? this.defaultVoiceId,
        autoplay: autoplay ?? this.autoplay,
        preferredQuality: preferredQuality ?? this.preferredQuality,
        downloadOverWifi: downloadOverWifi ?? this.downloadOverWifi,
        notificationsEnabled: notificationsEnabled ?? this.notificationsEnabled,
        recommendationsEnabled:
            recommendationsEnabled ?? this.recommendationsEnabled,
        personalizationEnabled:
            personalizationEnabled ?? this.personalizationEnabled,
        language: language ?? this.language,
        theme: theme ?? this.theme,
      );
}

/// What the plan permits. The backend is authoritative (§44, §55); this is a
/// cache of its answer, used only to decide what to *show*. Every gated action
/// is still enforced server-side, so a tampered client gains nothing.
final class Entitlements {
  const Entitlements({
    this.plan = 'free',
    this.premiumVoices = false,
    this.premiumContent = false,
    this.offlineDownloads = false,
    this.personalConfessions = false,
    this.maxSessionSeconds = 900,
    this.maxConcurrentDownloads = 0,
  });

  final String plan;
  final bool premiumVoices;
  final bool premiumContent;
  final bool offlineDownloads;
  final bool personalConfessions;
  final int maxSessionSeconds;
  final int maxConcurrentDownloads;

  bool get isPremium => plan == 'premium';

  factory Entitlements.fromJson(Map<String, dynamic> json) => Entitlements(
        plan: _str(json, 'plan', 'free'),
        premiumVoices: _bool(json, 'premium_voices'),
        premiumContent: _bool(json, 'premium_content'),
        offlineDownloads: _bool(json, 'offline_downloads'),
        personalConfessions: _bool(json, 'personal_confessions'),
        maxSessionSeconds: _int(json, 'max_session_seconds', 900),
        maxConcurrentDownloads: _int(json, 'max_concurrent_downloads'),
      );
}

/// An optional onboarding step (§12). Advisory only — never a gate.
final class CompletionStep {
  const CompletionStep({required this.key, required this.label, required this.done});

  final String key;
  final String label;
  final bool done;

  factory CompletionStep.fromJson(Map<String, dynamic> json) => CompletionStep(
        key: _str(json, 'key'),
        label: _str(json, 'label'),
        done: _bool(json, 'done'),
      );
}

final class ProfileCompletion {
  const ProfileCompletion({this.completed = 0, this.total = 0, this.steps = const []});

  final int completed;
  final int total;
  final List<CompletionStep> steps;

  double get fraction => total == 0 ? 0 : completed / total;

  factory ProfileCompletion.fromJson(Map<String, dynamic> json) =>
      ProfileCompletion(
        completed: _int(json, 'completed'),
        total: _int(json, 'total'),
        steps: _list(json['steps']).map(CompletionStep.fromJson).toList(),
      );
}

/// Everything needed to start the app (§56).
final class Bootstrap {
  const Bootstrap({
    required this.account,
    required this.profile,
    required this.preferences,
    required this.entitlements,
    this.interests = const [],
    this.completion = const ProfileCompletion(),
  });

  final Account account;
  final Profile profile;
  final Preferences preferences;
  final Entitlements entitlements;

  /// Explicitly chosen interests only. Inferences are fetched separately so a
  /// guess is never mistaken for something the user said (§15).
  final List<String> interests;
  final ProfileCompletion completion;

  factory Bootstrap.fromJson(Map<String, dynamic> json) {
    final user = json['user'];
    final profile = json['profile'];
    final prefs = json['preferences'];
    final ents = json['entitlements'];
    final completion = json['profile_completion'];

    return Bootstrap(
      account: Account.fromJson(user is Map<String, dynamic> ? user : const {}),
      profile:
          Profile.fromJson(profile is Map<String, dynamic> ? profile : const {}),
      preferences:
          Preferences.fromJson(prefs is Map<String, dynamic> ? prefs : const {}),
      entitlements:
          Entitlements.fromJson(ents is Map<String, dynamic> ? ents : const {}),
      interests: (json['interests'] is List)
          ? (json['interests'] as List).whereType<String>().toList()
          : const [],
      completion: ProfileCompletion.fromJson(
          completion is Map<String, dynamic> ? completion : const {}),
    );
  }
}

/// A content category (§13). Configured server-side, never hard-coded.
final class Category {
  const Category({
    required this.id,
    required this.name,
    this.description = '',
    this.premium = false,
  });

  final String id;
  final String name;
  final String description;
  final bool premium;

  factory Category.fromJson(Map<String, dynamic> json) => Category(
        id: _str(json, 'id'),
        name: _str(json, 'name'),
        description: _str(json, 'description'),
        premium: _bool(json, 'premium'),
      );
}

final class Voice {
  const Voice({
    required this.id,
    required this.name,
    this.premium = false,
    this.language = 'en',
  });

  final String id;
  final String name;
  final bool premium;
  final String language;

  factory Voice.fromJson(Map<String, dynamic> json) => Voice(
        id: _str(json, 'id'),
        name: _str(json, 'name'),
        premium: _bool(json, 'premium'),
        language: _str(json, 'language', 'en'),
      );
}

/// One track in a session.
///
/// [audioUrl] is a short-lived signed URL, and [locked] marks an item the plan
/// does not cover. The server blanks the URL and sets the flag rather than
/// omitting the item, so the player can show why a gap exists instead of
/// silently skipping (§26).
final class SessionItem {
  const SessionItem({
    required this.confessionId,
    this.title = '',
    this.text = '',
    this.audioUrl = '',
    this.durationSeconds = 0,
    this.position = 0,
    this.locked = false,
    this.lockReason = '',
  });

  final String confessionId;
  final String title;
  final String text;
  final String audioUrl;
  final int durationSeconds;
  final int position;
  final bool locked;
  final String lockReason;

  /// Whether this item can actually be played right now.
  bool get isPlayable => !locked && audioUrl.isNotEmpty;

  factory SessionItem.fromJson(Map<String, dynamic> json) => SessionItem(
        confessionId: _str(json, 'confession_id'),
        title: _str(json, 'title'),
        text: _str(json, 'text'),
        audioUrl: _str(json, 'audio_url'),
        durationSeconds: _int(json, 'duration_seconds'),
        position: _int(json, 'position'),
        locked: _bool(json, 'locked'),
        lockReason: _str(json, 'lock_reason'),
      );
}

final class ListeningSession {
  const ListeningSession({
    required this.id,
    this.type = '',
    this.status = '',
    this.durationSeconds = 0,
    this.voiceId = '',
    this.voiceDowngraded = false,
    this.voiceDowngradeReason = '',
    this.items = const [],
  });

  final String id;
  final String type;
  final String status;
  final int durationSeconds;
  final String voiceId;

  /// The requested voice was not permitted and one was substituted. Surfaced
  /// so a gated experience does not look like a broken one.
  final bool voiceDowngraded;
  final String voiceDowngradeReason;

  final List<SessionItem> items;

  /// Items that can actually be played, in order.
  List<SessionItem> get playable =>
      items.where((i) => i.isPlayable).toList(growable: false);

  bool get hasLockedItems => items.any((i) => i.locked);

  factory ListeningSession.fromJson(Map<String, dynamic> json) =>
      ListeningSession(
        id: _str(json, 'id'),
        type: _str(json, 'type'),
        status: _str(json, 'status'),
        durationSeconds: _int(json, 'duration_seconds'),
        voiceId: _str(json, 'voice_id'),
        voiceDowngraded: _bool(json, 'voice_downgraded'),
        voiceDowngradeReason: _str(json, 'voice_downgrade_reason'),
        items: _list(json['items']).map(SessionItem.fromJson).toList(),
      );
}

/// A recurring routine (§32). [time] is wall-clock in [timezone]; resolving it
/// to an instant is the server's job, because DST rules are not the client's
/// business.
final class Schedule {
  const Schedule({
    required this.id,
    this.label = '',
    this.time = '',
    this.timezone = 'UTC',
    this.daysOfWeek = const [],
    this.durationSeconds = 0,
    this.enabled = true,
  });

  final String id;
  final String label;
  final String time;
  final String timezone;

  /// ISO numbering: 1 = Monday .. 7 = Sunday, matching the server.
  final List<int> daysOfWeek;
  final int durationSeconds;
  final bool enabled;

  factory Schedule.fromJson(Map<String, dynamic> json) => Schedule(
        id: _str(json, 'id'),
        label: _str(json, 'label'),
        time: _str(json, 'time'),
        timezone: _str(json, 'timezone', 'UTC'),
        daysOfWeek: (json['days_of_week'] is List)
            ? (json['days_of_week'] as List).whereType<int>().toList()
            : const [],
        durationSeconds: _int(json, 'duration_seconds'),
        enabled: _bool(json, 'enabled', true),
      );
}

final class UserCollection {
  const UserCollection({
    required this.id,
    required this.name,
    this.description = '',
    this.visibility = 'private',
    this.itemCount = 0,
  });

  final String id;
  final String name;
  final String description;
  final String visibility;
  final int itemCount;

  bool get isPrivate => visibility == 'private';

  factory UserCollection.fromJson(Map<String, dynamic> json) => UserCollection(
        id: _str(json, 'id'),
        name: _str(json, 'name'),
        description: _str(json, 'description'),
        visibility: _str(json, 'visibility', 'private'),
        itemCount: _int(json, 'item_count'),
      );
}

/// An offline licence (§28).
///
/// [expiresAt] is when the right to hold this audio lapses, not when the file
/// is deleted. The client must stop playing at that point; the server cannot
/// reach into the device to enforce it.
final class Download {
  const Download({
    required this.id,
    required this.audioAssetId,
    this.confessionId = '',
    this.title = '',
    this.durationSeconds = 0,
    this.sizeBytes = 0,
    this.status = '',
    this.expiresAt,
    this.expired = false,
  });

  final String id;
  final String audioAssetId;
  final String confessionId;
  final String title;
  final int durationSeconds;
  final int sizeBytes;
  final String status;
  final DateTime? expiresAt;
  final bool expired;

  /// Whether the licence lapses soon enough to warn about. Renewal happens on
  /// app open, so a user who has not opened the app in a while should be told
  /// before their offline content stops working on a flight.
  bool expiresWithin(Duration window, {DateTime? now}) {
    final at = expiresAt;
    if (at == null || expired) return false;
    final ref = now ?? DateTime.now();
    return at.isAfter(ref) && at.isBefore(ref.add(window));
  }

  factory Download.fromJson(Map<String, dynamic> json) => Download(
        id: _str(json, 'id'),
        audioAssetId: _str(json, 'audio_asset_id'),
        confessionId: _str(json, 'confession_id'),
        title: _str(json, 'title'),
        durationSeconds: _int(json, 'duration_seconds'),
        sizeBytes: _int(json, 'size_bytes'),
        status: _str(json, 'status'),
        expiresAt: _time(json, 'expires_at'),
        expired: _bool(json, 'expired'),
      );
}

/// A live sign-in shown on the security screen (§31).
final class AuthSession {
  const AuthSession({
    required this.id,
    this.platform = '',
    this.userAgent = '',
    this.lastUsedAt,
    this.current = false,
  });

  final String id;
  final String platform;
  final String userAgent;
  final DateTime? lastUsedAt;
  final bool current;

  factory AuthSession.fromJson(Map<String, dynamic> json) => AuthSession(
        id: _str(json, 'id'),
        platform: _str(json, 'platform'),
        userAgent: _str(json, 'user_agent'),
        lastUsedAt: _time(json, 'last_used_at'),
        current: _bool(json, 'current'),
      );
}

/// Interests, kept split by provenance (§15).
///
/// The two lists are separate types of claim: one is what the user said, the
/// other is what the system guessed. Merging them in the model would make it
/// impossible for the UI to honour the distinction.
final class Interests {
  const Interests({this.explicit = const [], this.inferred = const []});

  final List<String> explicit;
  final List<String> inferred;

  factory Interests.fromJson(Map<String, dynamic> json) => Interests(
        explicit: _list(json['explicit'])
            .map((e) => _str(e, 'category_id'))
            .where((id) => id.isNotEmpty)
            .toList(),
        inferred: _list(json['inferred'])
            .map((e) => _str(e, 'category_id'))
            .where((id) => id.isNotEmpty)
            .toList(),
      );
}

/// Parses a list endpoint, tolerating both a bare array and a wrapped one.
List<T> parseList<T>(Object? source, T Function(Map<String, dynamic>) fromJson) {
  if (source is List) return _list(source).map(fromJson).toList();
  if (source is Map<String, dynamic>) return _list(source['data']).map(fromJson).toList();
  return const [];
}
