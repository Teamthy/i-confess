import 'dart:async';

import 'api_client.dart';
import 'api_error.dart';
import 'cache.dart';
import 'endpoints.dart';
import 'models.dart';
import 'result.dart';

/// Base behaviour shared by repositories (§57).
///
/// The read policy is the whole point of this class: fetch from the network,
/// fall back to cache when offline, and never present cached data as if it
/// were live. Getting that wrong in each repository separately is how an app
/// ends up silently showing week-old content with no indication.
abstract base class Repository {
  Repository(this.api, this.cache);

  final ApiClient api;
  final JsonCache cache;

  /// Reads a value, preferring the network and falling back to cache.
  ///
  /// [ttl] does not control whether the cache is *used* — it controls whether
  /// a served cache entry is flagged stale. A four-day-old library is still
  /// better than a blank screen on a plane; the user just needs to know.
  Future<Loadable<T>> cachedRead<T>({
    required String key,
    required Duration ttl,
    required Future<Map<String, dynamic>> Function() fetch,
    required T Function(Map<String, dynamic>) decode,
    Map<String, dynamic> Function(Map<String, dynamic>)? cachePayload,
  }) async {
    try {
      final json = await fetch();
      await cache.write(key, cachePayload?.call(json) ?? json);
      return Loadable.loaded(decode(json));
    } on NetworkException catch (e) {
      // Offline: serve the cache if we have one, and say that we did.
      final entry = await cache.read<T>(key, decode);
      if (entry != null) {
        return Loadable.loaded(
          entry.value,
          fromCache: true,
          stale: entry.ageAt(DateTime.now()) > ttl,
        );
      }
      return Loadable.failed(e);
    } on ApiError catch (e) {
      // A server rejection is not an offline condition. Falling back to cache
      // on a 403 would show content the user is no longer entitled to.
      if (e.isServerFault) {
        final entry = await cache.read<T>(key, decode);
        if (entry != null) {
          return Loadable.loaded(entry.value, fromCache: true, stale: true);
        }
      }
      return Loadable.failed(e);
    }
  }

  /// Performs a write, converting failures into a typed result.
  ///
  /// Writes are never optimistically cached. Claiming success before the
  /// server confirms is explicitly called out as wrong (§102), and it is
  /// especially damaging for anything touching entitlement or security.
  Future<WriteResult<T>> write<T>(
    Future<Map<String, dynamic>> Function() action,
    T Function(Map<String, dynamic>) decode,
  ) async {
    try {
      return WriteResult.success(decode(await action()));
    } on ApiException catch (e) {
      return WriteResult.failure(e);
    }
  }
}

/// Sign-in, sign-out and session management.
final class AuthRepository extends Repository {
  AuthRepository(super.api, super.cache);

  /// Signs in. A `mfaRequired` result is a successful credential check that
  /// needs a second factor — not a failure, and the UI must not word it as one.
  Future<SignInOutcome> signIn({
    required String email,
    required String password,
    String? code,
  }) async {
    try {
      final json = await api.postAuthLogin({
        'email': email,
        'password': password,
        if (code != null && code.isNotEmpty) 'code': code,
      });

      if (json['mfa_required'] == true) {
        return const SignInOutcome.mfaRequired();
      }
      final token = json['token'] as String?;
      if (token == null || token.isEmpty) {
        return const SignInOutcome.failed(
          ApiError(status: 500, code: '', message: 'No session was issued.'),
        );
      }
      return SignInOutcome.signedIn(token);
    } on ApiException catch (e) {
      return SignInOutcome.failed(e);
    }
  }

  Future<WriteResult<String>> register({
    required String email,
    required String password,
    String? displayName,
    String? timezone,
  }) =>
      write(
        () => api.postAuthRegister({
          'email': email,
          'password': password,
          if (displayName != null) 'display_name': displayName,
          if (timezone != null) 'timezone': timezone,
        }),
        // A duplicate address returns a neutral 200 with no token, so the
        // endpoint cannot be used to discover who has an account. An empty
        // token here means "check your email", not an error.
        (json) => json['token'] as String? ?? '',
      );

  /// Ends this session. Clears local state even if the call fails: a user who
  /// taps sign-out must end up signed out, and the token is useless locally
  /// once discarded.
  Future<void> signOut() async {
    try {
      await api.postAuthLogout();
    } on ApiException {
      // Deliberately swallowed.
    }
    await cache.clear();
  }

  Future<WriteResult<void>> signOutEverywhere() =>
      write(() => api.postAuthLogoutAll(), (_) {});

  Future<Loadable<List<AuthSession>>> sessions() async {
    try {
      final json = await api.getAuthSessions();
      return Loadable.loaded(parseList(json['data'] ?? json, AuthSession.fromJson));
    } on ApiException catch (e) {
      return Loadable.failed(e);
    }
  }

  Future<WriteResult<void>> revokeSession(String id) =>
      write(() => api.deleteAuthSessionsById(id), (_) {});

  Future<WriteResult<void>> requestPasswordReset(String email) => write(
        () => api.postAuthRequestPasswordReset({'email': email}),
        (_) {},
      );
}

/// The outcome of a sign-in attempt.
sealed class SignInOutcome {
  const SignInOutcome();

  const factory SignInOutcome.signedIn(String token) = SignedIn;
  const factory SignInOutcome.mfaRequired() = MfaRequired;
  const factory SignInOutcome.failed(ApiException error) = SignInFailed;
}

final class SignedIn extends SignInOutcome {
  const SignedIn(this.token);
  final String token;
}

final class MfaRequired extends SignInOutcome {
  const MfaRequired();
}

final class SignInFailed extends SignInOutcome {
  const SignInFailed(this.error);
  final ApiException error;

  /// Offline is not a credential problem. Telling someone their password is
  /// wrong when they are on a train makes them change one that was never wrong.
  bool get isOffline => error is NetworkException;

  bool get isRateLimited => error is ApiError && (error as ApiError).isRateLimited;
}

/// Profile, preferences and interests.
final class ProfileRepository extends Repository {
  ProfileRepository(super.api, super.cache);

  /// Loads everything the app needs to start (§56).
  Future<Loadable<Bootstrap>> bootstrap() => cachedRead(
        key: CacheKeys.bootstrap,
        ttl: CacheTtl.bootstrap,
        fetch: () => api.getMeBootstrap(),
        decode: Bootstrap.fromJson,
      );

  Future<Loadable<Profile>> profile() => cachedRead(
        key: CacheKeys.profile,
        ttl: CacheTtl.profile,
        fetch: () => api.getMeProfile(),
        decode: Profile.fromJson,
      );

  /// Updates the profile. Only supplied fields are sent, so a partial edit
  /// cannot blank a field the screen did not show.
  Future<WriteResult<Profile>> updateProfile({
    String? displayName,
    String? username,
    String? bio,
    String? timezone,
    String? language,
    String? countryCode,
  }) =>
      write(
        () => api.patchMeProfile({
          if (displayName != null) 'display_name': displayName,
          if (username != null) 'username': username,
          if (bio != null) 'bio': bio,
          if (timezone != null) 'timezone': timezone,
          if (language != null) 'language': language,
          if (countryCode != null) 'country_code': countryCode,
        }),
        Profile.fromJson,
      );

  Future<Loadable<Preferences>> preferences() => cachedRead(
        key: CacheKeys.preferences,
        ttl: CacheTtl.preferences,
        fetch: () => api.getMePreferences(),
        decode: Preferences.fromJson,
      );

  Future<WriteResult<Preferences>> updatePreferences(Map<String, dynamic> changes) =>
      write(() => api.patchMePreferences(changes), Preferences.fromJson);

  Future<Loadable<Interests>> interests() async {
    try {
      return Loadable.loaded(Interests.fromJson(await api.getMeInterests()));
    } on ApiException catch (e) {
      return Loadable.failed(e);
    }
  }

  /// Replaces the user's stated interests. Inferred ones are left alone: this
  /// endpoint speaks only for what the user chose (§15).
  Future<WriteResult<Interests>> setInterests(List<String> categoryIds) => write(
        () => api.putMeInterests({'category_ids': categoryIds}),
        Interests.fromJson,
      );
}

/// Categories, voices and sessions.
final class ContentRepository extends Repository {
  ContentRepository(super.api, super.cache);

  Future<Loadable<List<Category>>> categories() => cachedRead(
        key: CacheKeys.categories,
        ttl: CacheTtl.categories,
        fetch: () async => {'data': (await api.getCategories())['data'] ?? []},
        decode: (json) => parseList(json['data'], Category.fromJson),
      );

  Future<Loadable<List<Voice>>> voices() => cachedRead(
        key: CacheKeys.voices,
        ttl: CacheTtl.voices,
        fetch: () async => {'data': (await api.getVoices())['data'] ?? []},
        decode: (json) => parseList(json['data'], Voice.fromJson),
      );

  /// Composes a session. Not cached: the audio URLs are short-lived and
  /// entitlement is re-evaluated server-side on every read, so a cached
  /// session would hand back links that no longer work.
  Future<WriteResult<ListeningSession>> createSession({
    required List<String> categoryIds,
    required int durationSeconds,
    String? voiceId,
  }) =>
      write(
        () => api.postSessions({
          'category_ids': categoryIds,
          'duration_seconds': durationSeconds,
          if (voiceId != null && voiceId.isNotEmpty) 'voice_id': voiceId,
        }),
        ListeningSession.fromJson,
      );

  /// Re-reads a session, which mints fresh signed URLs. Call this rather than
  /// replaying a stored session: the previous links will have expired.
  Future<Loadable<ListeningSession>> session(String id) async {
    try {
      return Loadable.loaded(ListeningSession.fromJson(await api.getSessionsById(id)));
    } on ApiException catch (e) {
      return Loadable.failed(e);
    }
  }
}

/// Collections, downloads and schedules.
final class LibraryRepository extends Repository {
  LibraryRepository(super.api, super.cache);

  Future<Loadable<List<UserCollection>>> collections() => cachedRead(
        key: CacheKeys.collections,
        ttl: CacheTtl.library,
        fetch: () async => {'data': (await api.getMeCollections())['data'] ?? []},
        decode: (json) => parseList(json['data'], UserCollection.fromJson),
      );

  Future<WriteResult<UserCollection>> createCollection(String name) => write(
        () => api.postMeCollections({'name': name}),
        UserCollection.fromJson,
      );

  Future<Loadable<List<Schedule>>> schedules() => cachedRead(
        key: CacheKeys.schedules,
        ttl: CacheTtl.library,
        fetch: () async => {'data': (await api.getSchedules())['data'] ?? []},
        decode: (json) => parseList(json['data'], Schedule.fromJson),
      );

  Future<Loadable<DownloadLibrary>> downloads() => cachedRead(
        key: CacheKeys.downloads,
        ttl: CacheTtl.library,
        fetch: () => api.getMeDownloads(),
        decode: DownloadLibrary.fromJson,
      );

  /// Takes a confession offline. Premium-only, enforced server-side; a 402
  /// here is the upgrade prompt, not an error.
  Future<WriteResult<DownloadTicket>> download(String confessionId, {String? voiceId}) =>
      write(
        () => api.postMeDownloads({
          'confession_id': confessionId,
          if (voiceId != null && voiceId.isNotEmpty) 'voice_id': voiceId,
        }),
        DownloadTicket.fromJson,
      );

  /// Renews an offline licence. Worth calling on app open for anything
  /// expiring soon, so content does not lapse mid-flight.
  Future<WriteResult<DownloadTicket>> renewDownload(String id) =>
      write(() => api.postMeDownloadsByIdRefresh(id), DownloadTicket.fromJson);

  Future<WriteResult<void>> removeDownload(String id) =>
      write(() => api.deleteMeDownloadsById(id), (_) {});
}

/// The offline library plus the plan's limits.
final class DownloadLibrary {
  const DownloadLibrary({
    this.downloads = const [],
    this.limit = 0,
    this.used = 0,
    this.offlineHoursAllowed = 0,
  });

  final List<Download> downloads;
  final int limit;
  final int used;
  final int offlineHoursAllowed;

  bool get atLimit => limit > 0 && used >= limit;

  /// Licences lapsing soon, so the app can renew them before a user boards a
  /// plane and finds their content gone.
  List<Download> expiringWithin(Duration window, {DateTime? now}) => downloads
      .where((d) => d.expiresWithin(window, now: now))
      .toList(growable: false);

  factory DownloadLibrary.fromJson(Map<String, dynamic> json) => DownloadLibrary(
        downloads: parseList(json['downloads'], Download.fromJson),
        limit: (json['limit'] as num?)?.toInt() ?? 0,
        used: (json['used'] as num?)?.toInt() ?? 0,
        offlineHoursAllowed: (json['offline_hours_allowed'] as num?)?.toInt() ?? 0,
      );
}

/// A newly issued download, with the short-lived URL to fetch the bytes.
final class DownloadTicket {
  const DownloadTicket({required this.download, this.downloadUrl = ''});

  final Download download;

  /// Valid for about an hour — long enough to fetch, short enough that a
  /// leaked link is not a lasting problem. Distinct from the licence expiry.
  final String downloadUrl;

  factory DownloadTicket.fromJson(Map<String, dynamic> json) {
    final d = json['download'];
    return DownloadTicket(
      download: Download.fromJson(d is Map<String, dynamic> ? d : const {}),
      downloadUrl: json['download_url'] as String? ?? '',
    );
  }
}
