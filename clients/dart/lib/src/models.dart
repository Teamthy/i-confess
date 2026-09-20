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

/// A published collection — the "featured" rail on Explore.
final class Collection {
  const Collection({
    required this.id,
    this.name = '',
    this.description = '',
    this.premium = false,
  });

  final String id;
  final String name;
  final String description;
  final bool premium;

  factory Collection.fromJson(Map<String, dynamic> json) => Collection(
        id: _str(json, 'id'),
        name: _str(json, 'name'),
        description: _str(json, 'description'),
        premium: _bool(json, 'premium'),
      );
}

/// One length variant of a confession — 30s, 1m, 3m, 5m etc.
final class ConfessionVariant {
  const ConfessionVariant({
    required this.id,
    this.confessionId = '',
    this.label = '',
    this.durationSeconds = 0,
    this.sortOrder = 0,
  });

  final String id;
  final String confessionId;
  final String label;
  final int durationSeconds;
  final int sortOrder;

  factory ConfessionVariant.fromJson(Map<String, dynamic> json) =>
      ConfessionVariant(
        id: _str(json, 'id'),
        confessionId: _str(json, 'confession_id'),
        label: _str(json, 'label'),
        durationSeconds: _int(json, 'duration_seconds'),
        sortOrder: _int(json, 'sort_order'),
      );
}

/// A Scripture anchor for a confession.
final class ScriptureRef {
  const ScriptureRef({
    required this.id,
    this.confessionId = '',
    this.book = '',
    this.chapter = 0,
    this.verse = '',
    this.translation = '',
    this.isDirectQuote = false,
    this.notes = '',
    this.sortOrder = 0,
  });

  final String id;
  final String confessionId;
  final String book;
  final int chapter;
  final String verse;
  final String translation;
  final bool isDirectQuote;
  final String notes;
  final int sortOrder;

  /// Human-readable reference like "John 3:16" or "Psalm 23".
  String get reference {
    if (book.isEmpty) return '';
    if (chapter <= 0) return book;
    if (verse.isEmpty) return '$book $chapter';
    return '$book $chapter:$verse';
  }

  factory ScriptureRef.fromJson(Map<String, dynamic> json) => ScriptureRef(
        id: _str(json, 'id'),
        confessionId: _str(json, 'confession_id'),
        book: _str(json, 'book'),
        chapter: _int(json, 'chapter'),
        verse: _str(json, 'verse'),
        translation: _str(json, 'translation'),
        isDirectQuote: _bool(json, 'is_direct_quote'),
        notes: _str(json, 'notes'),
        sortOrder: _int(json, 'sort_order'),
      );
}

/// A confession as the catalogue publishes it.
///
/// Text comes in three lengths; the browse surfaces use [shortText] and fall
/// back to [description], never the full text — pulling a whole confession into
/// a list row would both overcrowd it and spend bandwidth on words nobody sees.
/// The detail screen uses all three lengths plus variants and scriptures.
final class Confession {
  const Confession({
    required this.id,
    this.categoryId = '',
    this.title = '',
    this.shortText = '',
    this.mediumText = '',
    this.longText = '',
    this.description = '',
    this.intensity = 0,
    this.tags = const [],
    this.language = 'en',
    this.status = '',
    this.author = '',
    this.version = 0,
    this.variants = const [],
    this.scriptures = const [],
  });

  final String id;
  final String categoryId;
  final String title;
  final String shortText;
  final String mediumText;
  final String longText;
  final String description;
  final int intensity;
  final List<String> tags;
  final String language;
  final String status;
  final String author;
  final int version;
  final List<ConfessionVariant> variants;
  final List<ScriptureRef> scriptures;

  /// The line a card leads with when there is no title.
  String get lead => shortText.isNotEmpty ? shortText : description;

  /// The fullest text available, for the detail screen.
  String get fullText {
    if (longText.isNotEmpty) return longText;
    if (mediumText.isNotEmpty) return mediumText;
    if (shortText.isNotEmpty) return shortText;
    return description;
  }

  /// Whether this confession has rich content beyond the short text.
  bool get hasRichContent =>
      mediumText.isNotEmpty ||
      longText.isNotEmpty ||
      scriptures.isNotEmpty ||
      variants.isNotEmpty;

  factory Confession.fromJson(Map<String, dynamic> json) => Confession(
        id: _str(json, 'id'),
        categoryId: _str(json, 'category_id'),
        title: _str(json, 'title'),
        shortText: _str(json, 'short_text'),
        mediumText: _str(json, 'medium_text'),
        longText: _str(json, 'long_text'),
        description: _str(json, 'description'),
        intensity: _int(json, 'intensity'),
        tags: (json['tags'] is List)
            ? (json['tags'] as List).whereType<String>().toList()
            : (json['tags'] is String && (json['tags'] as String).isNotEmpty)
                ? (json['tags'] as String).split(',').map((s) => s.trim()).where((s) => s.isNotEmpty).toList()
                : const [],
        language: _str(json, 'language', 'en'),
        status: _str(json, 'status'),
        author: _str(json, 'author'),
        version: _int(json, 'version'),
        variants: _list(json['variants']).map(ConfessionVariant.fromJson).toList(),
        scriptures: _list(json['scriptures']).map(ScriptureRef.fromJson).toList(),
      );
}

final class Voice {
  const Voice({
    required this.id,
    required this.name,
    this.description = '',
    this.premium = false,
    this.language = 'en',
    this.gender = '',
    this.status = 'active',
  });

  final String id;
  final String name;

  /// One line the voice picker shows under the name, e.g. "Warm, calm
  /// professional narration voice." Optional server-side.
  final String description;
  final bool premium;
  final String language;

  /// Male / female / unset, when the catalogue records it. Display only.
  final String gender;

  /// Catalogue state. The picker must offer only voices that can actually
  /// speak — a voice that is retired or suspended fails the rights gate at
  /// generation time, and selecting it would build a session that cannot
  /// render (§5, "a voice that cannot be licensed must not appear as
  /// selectable"). `ListVoices` returns every row regardless of status, so
  /// the filter is the client's responsibility.
  final String status;

  /// Whether this voice may be offered in the picker.
  bool get isSelectable => status == 'active';

  factory Voice.fromJson(Map<String, dynamic> json) => Voice(
        id: _str(json, 'id'),
        name: _str(json, 'name'),
        description: _str(json, 'description'),
        premium: _bool(json, 'premium'),
        language: _str(json, 'language', 'en'),
        gender: _str(json, 'gender'),
        status: _str(json, 'status', 'active'),
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
    this.createdAt,
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
  final DateTime? createdAt;

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
        createdAt: _time(json, 'created_at'),
      );
}

/// What the engine *would* build, from `POST /sessions/preview` (§5.3).
///
/// The builder shows this before creating anything, so the listener sees the
/// plan the engine chose — including the gap between the length they asked
/// for and the length complete confessions actually add up to — rather than
/// discovering it after the fact. A preview is never persisted server-side,
/// and its audio links are signed for reading, not for playback.
final class SessionPreview {
  const SessionPreview({
    this.targetSeconds = 0,
    this.actualSeconds = 0,
    this.totalItems = 0,
    this.itemsPreview = const [],
    this.voiceId = '',
    this.voiceDowngraded = false,
    this.strategy = '',
    this.display = '',
  });

  /// The length that was requested.
  final int targetSeconds;

  /// The length the engine could actually fill with complete confessions.
  /// A gap between the two is normal and expected; the builder shows why.
  final int actualSeconds;

  /// How many items a real session would carry.
  final int totalItems;

  /// The first few of those items, for a peek without the full queue.
  final List<SessionItem> itemsPreview;

  /// The voice the session would use, after any premium fallback.
  final String voiceId;

  /// The requested voice was not permitted and one was substituted.
  final bool voiceDowngraded;

  /// The packing strategy the engine applied.
  final String strategy;

  /// The server's own one-line summary, e.g. `30 MIN • 12 • Voices`.
  /// Shown as the at-a-glance confirmation; kept verbatim so the server
  /// remains the single author of what a plan is called.
  final String display;

  factory SessionPreview.fromJson(Map<String, dynamic> json) =>
      SessionPreview(
        targetSeconds: _int(json, 'target_seconds'),
        actualSeconds: _int(json, 'actual_seconds'),
        totalItems: _int(json, 'total_items'),
        itemsPreview:
            _list(json['items_preview']).map(SessionItem.fromJson).toList(),
        voiceId: _str(json, 'voice_id'),
        voiceDowngraded: _bool(json, 'voice_downgraded'),
        strategy: _str(json, 'strategy'),
        display: _str(json, 'display'),
      );
}

/// A saved custom session configuration (§5.4), from `POST /templates`.
///
/// A template records *shape* — categories, voice, ordering — never a queue:
/// the session is rebuilt from the live catalogue each time it is started,
/// so a template never replays stale content. The share token is server-minted;
/// the client renders the deep link it is given rather than assembling its own.
final class SessionTemplate {
  const SessionTemplate({
    required this.id,
    this.name = '',
    this.description = '',
    this.categoryIds = const [],
    this.voiceId = '',
    this.isPublic = false,
    this.shareToken = '',
    this.shareUrl = '',
    this.deeplink = '',
  });

  final String id;
  final String name;
  final String description;

  /// What to speak over, in the template's own words.
  final List<String> categoryIds;
  final String voiceId;
  final bool isPublic;

  /// Opaque token for `GET /t/{token}`. The client never parses it.
  final String shareToken;
  final String shareUrl;
  final String deeplink;

  factory SessionTemplate.fromJson(Map<String, dynamic> json) {
    // The create response wraps the row: {template: {...}, share_url, deeplink}.
    // The list endpoints return the bare row. Accept both so the client does
    // not need to know which endpoint answered.
    final t = json['template'] is Map<String, dynamic>
        ? json['template'] as Map<String, dynamic>
        : json;
    return SessionTemplate(
      id: _str(t, 'id'),
      name: _str(t, 'name'),
      description: _str(t, 'description'),
      categoryIds: (t['category_ids'] as List?)?.whereType<String>().toList() ?? const [],
      voiceId: _str(t, 'voice_id'),
      isPublic: _bool(t, 'is_public'),
      shareToken: _str(t, 'share_token'),
      shareUrl: _str(json, 'share_url'),
      deeplink: _str(json, 'deeplink'),
    );
  }
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

/// A search result across confessions, categories, collections, voices.
final class SearchResult {
  const SearchResult({
    required this.id,
    this.type = '',
    this.title = '',
    this.description = '',
    this.imageUrl = '',
    this.score = 0,
  });

  final String id;
  final String type;
  final String title;
  final String description;
  final String imageUrl;
  final double score;

  factory SearchResult.fromJson(Map<String, dynamic> json) => SearchResult(
        id: _str(json, 'id'),
        type: _str(json, 'type'),
        title: _str(json, 'title'),
        description: _str(json, 'description'),
        imageUrl: _str(json, 'image_url'),
        score: (json['score'] is num) ? (json['score'] as num).toDouble() : 0,
      );
}

/// Recommendations payload (deterministic v1).
final class Recommendations {
  const Recommendations({
    this.categories = const [],
    this.confessions = const [],
    this.personalized = false,
  });

  final List<Category> categories;
  final List<Confession> confessions;
  final bool personalized;

  factory Recommendations.fromJson(Map<String, dynamic> json) =>
      Recommendations(
        categories:
            _list(json['categories']).map(Category.fromJson).toList(),
        confessions:
            _list(json['confessions']).map(Confession.fromJson).toList(),
        personalized: _bool(json, 'personalized'),
      );
}

/// Subscription plan with regional pricing.
final class Plan {
  const Plan({
    required this.id,
    this.name = '',
    this.description = '',
    this.interval = 'monthly',
    this.prices = const {},
    this.features = const [],
    this.trialDays = 0,
  });

  final String id;
  final String name;
  final String description;
  final String interval;
  final Map<String, dynamic> prices;
  final List<String> features;
  final int trialDays;

  String priceFor(String currency) {
    final p = prices[currency];
    if (p is Map<String, dynamic>) {
      final amount = p['amount'];
      final curr = p['currency'] ?? currency;
      if (amount != null) return '$curr $amount';
    }
    if (p is num) return '$currency $p';
    return '';
  }

  factory Plan.fromJson(Map<String, dynamic> json) => Plan(
        id: _str(json, 'id'),
        name: _str(json, 'name'),
        description: _str(json, 'description'),
        interval: _str(json, 'interval', 'monthly'),
        prices: json['prices'] is Map<String, dynamic>
            ? json['prices'] as Map<String, dynamic>
            : const {},
        features: (json['features'] is List)
            ? (json['features'] as List).whereType<String>().toList()
            : const [],
        trialDays: _int(json, 'trial_days'),
      );
}

/// Current subscription state.
final class Subscription {
  const Subscription({
    this.plan = 'free',
    this.status = 'active',
    this.active = false,
    this.maxSessionSeconds = 900,
  });

  final String plan;
  final String status;
  final bool active;
  final int maxSessionSeconds;

  bool get isPremium => plan == 'premium' && active;

  /// POST /subscriptions/verify returns a store plan and a server-resolved
  /// entitlement object, not the GET /subscription shape. The store plan alone
  /// must never be used to infer Premium (it can describe an expired purchase).
  factory Subscription.fromVerification(Map<String, dynamic> json) {
    final entitlements = json['entitlements'];
    final premium = json['verified'] == true && entitlements is Map &&
        (entitlements['Plan'] ?? entitlements['plan']) == 'premium';
    return Subscription(
      plan: premium ? 'premium' : 'free',
      status: _str(json, 'state', 'unknown'),
      active: premium,
    );
  }

  factory Subscription.fromJson(Map<String, dynamic> json) => Subscription(
        plan: _str(json, 'plan', 'free'),
        status: _str(json, 'status', 'active'),
        active: _bool(json, 'active'),
        maxSessionSeconds: _int(json, 'max_session_seconds', 900),
      );
}

/// Trial journey day.
final class TrialDay {
  const TrialDay({
    this.day = 0,
    this.title = '',
    this.description = '',
    this.cta = '',
  });

  final int day;
  final String title;
  final String description;
  final String cta;

  factory TrialDay.fromJson(Map<String, dynamic> json) => TrialDay(
        day: _int(json, 'day'),
        title: _str(json, 'title'),
        description: _str(json, 'description'),
        cta: _str(json, 'cta'),
      );
}

/// Parses a list endpoint, tolerating both a bare array and a wrapped one.
List<T> parseList<T>(Object? source, T Function(Map<String, dynamic>) fromJson) {
  if (source is List) return _list(source).map(fromJson).toList();
  if (source is Map<String, dynamic>) return _list(source['data']).map(fromJson).toList();
  return const [];
}
