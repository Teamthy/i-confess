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
  ///
  /// A successful sign-in persists the token before returning. Until PHASE 19
  /// this method handed the token to the caller and nothing ever wrote it down,
  /// so a listener who signed in was "signed in" for the lifetime of the process
  /// and signed out on the next launch, with every request in between sent
  /// without an Authorization header.
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
      await api.attachSession(token);
      return SignInOutcome.signedIn(token, userId: _userIdOf(json));
    } on ApiException catch (e) {
      return SignInOutcome.failed(e);
    }
  }

  /// Creates an account.
  ///
  /// Three outcomes, not two. The server refuses to confirm whether an address
  /// is already registered (S65): it answers a duplicate exactly as it answers a
  /// fresh signup, minus the session. So "no token" is a success path with its
  /// own screen, not an error, and the UI must not word it as a rejection.
  Future<RegisterOutcome> register({
    required String email,
    required String password,
    String? displayName,
    String? timezone,
  }) async {
    try {
      final json = await api.postAuthRegister({
        'email': email,
        'password': password,
        if (displayName != null && displayName.isNotEmpty)
          'display_name': displayName,
        if (timezone != null && timezone.isNotEmpty) 'timezone': timezone,
      });

      final token = json['token'] as String? ?? '';
      if (token.isEmpty) {
        return RegisterOutcome.checkYourEmail(
          json['message'] as String? ?? 'Check your email to continue.',
        );
      }
      await api.attachSession(token);
      return RegisterOutcome.created(token, userId: _userIdOf(json));
    } on ApiException catch (e) {
      return RegisterOutcome.failed(e);
    }
  }

  /// Confirms an email address with the token from the link that was sent.
  ///
  /// Exposed on the client because the link opens in a browser, and a listener
  /// who cannot open it — or who arrives through the app — still needs a way in.
  /// An invalid or expired token is a 401 from the server, which the UI should
  /// answer with "resend", not with a generic failure.
  Future<WriteResult<void>> verifyEmail(String token) =>
      write(() => api.postAuthVerifyEmail({'token': token}), (_) {});

  /// Asks for the confirmation email to be sent again.
  ///
  /// The server answers identically whether or not the address is known, and
  /// the UI must keep that: showing "resent!" for one address and nothing for
  /// another turns the button into a membership lookup.
  Future<WriteResult<void>> resendVerification(String email) => write(
        () => api.postAuthResendVerification({'email': email}),
        (_) {},
      );

  /// Sets a new password with a reset token.
  ///
  /// The server ends every session belonging to the account when this succeeds,
  /// because whoever held the old password might have been an attacker (S34).
  /// The caller should therefore not expect to be signed in afterwards.
  Future<WriteResult<void>> resetPassword({
    required String token,
    required String password,
  }) =>
      write(
        () => api.postAuthResetPassword({'token': token, 'password': password}),
        (_) {},
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
    await api.clearSession();
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

/// Reads the account id out of a response that carries the user object.
///
/// Sign-in and registration both return `{token, user}`, and the id is what
/// lets analytics attribute events to a person. Absent rather than invented
/// when the server omits it: a wrong id is worse than no id.
String? _userIdOf(Map<String, dynamic> json) {
  final user = json['user'];
  if (user is Map<String, dynamic>) {
    final id = user['id'];
    if (id is String && id.isNotEmpty) return id;
  }
  final flat = json['user_id'];
  return flat is String && flat.isNotEmpty ? flat : null;
}

/// The outcome of a sign-in attempt.
sealed class SignInOutcome {
  const SignInOutcome();

  const factory SignInOutcome.signedIn(String token, {String? userId}) =
      SignedIn;
  const factory SignInOutcome.mfaRequired() = MfaRequired;
  const factory SignInOutcome.failed(ApiException error) = SignInFailed;
}

final class SignedIn extends SignInOutcome {
  const SignedIn(this.token, {this.userId});
  final String token;

  /// The account id, when the response carried the user object. Null is not an
  /// error: it means attribution is unavailable, and a wrong id would be worse.
  final String? userId;
}

/// The outcome of a registration attempt.
///
/// A sealed type rather than a `WriteResult<String>`, because the interesting
/// case is neither success nor failure: an address that already exists returns
/// a 200 with no session, and the UI owes that listener a "check your email"
/// screen rather than an error. Encoding that as an empty string left every
/// caller to know the convention.
sealed class RegisterOutcome {
  const RegisterOutcome();

  const factory RegisterOutcome.created(String token, {String? userId}) =
      AccountCreated;
  const factory RegisterOutcome.checkYourEmail(String message) = CheckYourEmail;
  const factory RegisterOutcome.failed(ApiException error) = RegisterFailed;
}

final class AccountCreated extends RegisterOutcome {
  const AccountCreated(this.token, {this.userId});
  final String token;
  final String? userId;
}

final class CheckYourEmail extends RegisterOutcome {
  const CheckYourEmail(this.message);

  /// The server's own wording. Neutral by construction: it must not differ
  /// between an address that exists and one that does not (S65).
  final String message;
}

final class RegisterFailed extends RegisterOutcome {
  const RegisterFailed(this.error);
  final ApiException error;

  bool get isOffline => error is NetworkException;
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

  /// Published collections — the Explore "featured" rail.
  ///
  /// Distinct cache key from [LibraryRepository.collections] (the listener's own
  /// collections): same word, different data, and a shared key would hand one
  /// surface the other's payload.
  Future<Loadable<List<Collection>>> collections() => cachedRead(
        key: 'published_collections',
        ttl: CacheTtl.categories,
        fetch: () async => {'data': (await api.getCollections())['data'] ?? []},
        decode: (json) => parseList(json['data'], Collection.fromJson),
      );

  /// Confessions in one category. The server returns a bare array, which the
  /// client wraps as `{'data': …}`.
  Future<Loadable<List<Confession>>> categoryConfessions(String id) =>
      cachedRead(
        key: 'catconf:$id',
        ttl: CacheTtl.categories,
        fetch: () async =>
            {'data': (await api.getCategoriesByIdConfessions(id))['data'] ?? []},
        decode: (json) => parseList(json['data'], Confession.fromJson),
      );

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

  /// Dry-runs the engine without creating anything (§5.3). Not cached, for the
  /// same reason [createSession] is not: the answer depends on the caller's
  /// entitlement and favourites at this instant, and a stale "would be 42
  /// minutes" is worse than no answer.
  ///
  /// Returns [WriteResult] rather than [Loadable] because the interesting
  /// failures are rejections, not outages: a 402 says the plan caps the
  /// length, a 422 says no complete set of confessions reaches that length.
  /// The review screen words those differently from "you are offline".
  Future<WriteResult<SessionPreview>> previewSession({
    required List<String> categoryIds,
    required int durationSeconds,
    String? voiceId,
    String? strategy,
  }) =>
      write(
        () => api.postSessionsPreview({
          'category_ids': categoryIds,
          'duration_seconds': durationSeconds,
          if (voiceId != null && voiceId.isNotEmpty) 'voice_id': voiceId,
          if (strategy != null && strategy.isNotEmpty) 'strategy': strategy,
        }),
        SessionPreview.fromJson,
      );

  /// Saves the builder's shape as a reusable template (§5.4).
  Future<WriteResult<SessionTemplate>> createTemplate({
    required String name,
    required List<String> categoryIds,
    String? voiceId,
    String description = '',
  }) =>
      write(
        () => api.postTemplates({
          'name': name,
          'category_ids': categoryIds,
          if (voiceId != null && voiceId.isNotEmpty) 'voice_id': voiceId,
          if (description.isNotEmpty) 'description': description,
        }),
        SessionTemplate.fromJson,
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

  /// Fetches the session queue snapshot with freshly signed URLs and current progress.
  Future<Loadable<SessionQueueResponse>> sessionQueue(String id) async {
    try {
      final json = await api.getSessionsByIdQueue(id);
      return Loadable.loaded(SessionQueueResponse.fromJson(json));
    } on ApiException catch (e) {
      return Loadable.failed(e);
    }
  }

  /// Starts playback for a session.
  Future<WriteResult<ListeningSession>> startSession(String id) =>
      write(() => api.postSessionsByIdStart(id), ListeningSession.fromJson);

  /// Pauses a live session.
  Future<WriteResult<ListeningSession>> pauseSession(String id) =>
      write(() => api.postSessionsByIdPause(id), ListeningSession.fromJson);

  /// Resumes a paused or interrupted session.
  Future<WriteResult<ListeningSession>> resumeSession(String id) =>
      write(() => api.postSessionsByIdResume(id), ListeningSession.fromJson);

  /// Interrupts an active session.
  Future<WriteResult<ListeningSession>> interruptSession(String id) =>
      write(() => api.postSessionsByIdInterrupt(id), ListeningSession.fromJson);

  /// Records playback progress with deterministic conflict resolution.
  Future<WriteResult<SyncProgressResponse>> syncProgress(
    String id, {
    required int positionMs,
    String? queueItemId,
    String? itemStatus,
    String? deviceId,
    String? lastUpdatedAt,
  }) =>
      write(
        () => api.postSessionsByIdProgress(id, {
          'position_ms': positionMs,
          if (queueItemId != null && queueItemId.isNotEmpty)
            'queue_item_id': queueItemId,
          if (itemStatus != null && itemStatus.isNotEmpty)
            'item_status': itemStatus,
          if (deviceId != null && deviceId.isNotEmpty) 'device_id': deviceId,
          if (lastUpdatedAt != null && lastUpdatedAt.isNotEmpty)
            'last_updated_at': lastUpdatedAt,
        }),
        SyncProgressResponse.fromJson,
      );

  /// Skips an item in the queue.
  Future<WriteResult<Map<String, dynamic>>> skipSessionItem(
          String id, String itemId) =>
      write(
        () => api.postSessionsByIdSkip(id, {'item_id': itemId}),
        (json) => json,
      );

  /// Completes the session.
  Future<WriteResult<Map<String, dynamic>>> completeSession(String id) =>
      write(
        () => api.postSessionsByIdComplete(id),
        (json) => json,
      );

  /// The listener's sessions, newest first, for continue-listening and history.
  ///
  /// Not cached, like [createSession]: a session's status changes as it is
  /// played, and a cached list would show "continue" for something already
  /// finished. The read is cheap; staleness is not.
  Future<Loadable<List<ListeningSession>>> mySessions() async {
    try {
      final json = await api.getSessions();
      return Loadable.loaded(parseList(json['sessions'], ListeningSession.fromJson));
    } on ApiException catch (e) {
      return Loadable.failed(e);
    }
  }

  /// One confession with its variants and scriptures.
  ///
  /// Cached briefly: confessions are editorial content and change rarely, but a
  /// stale detail that shows an old title after an edit would read as broken.
  Future<Loadable<Confession>> confession(String id) => cachedRead(
        key: 'confession:$id',
        ttl: CacheTtl.categories,
        fetch: () => api.getConfessionsById(id),
        decode: Confession.fromJson,
      );

  /// Adds a confession to the listener's favourites.
  ///
  /// The server stores favourites as (entity_type, entity_id) so the same table
  /// can hold categories, voices and collections later.
  Future<WriteResult<void>> addFavoriteConfession(String confessionId) =>
      _favoriteWrite(() => api.postMeFavorites({
            'entity_type': 'confession',
            'entity_id': confessionId,
          }));

  /// Removes a confession from favourites.
  Future<WriteResult<void>> removeFavoriteConfession(String confessionId) =>
      _favoriteWrite(() => api.deleteMeFavorites({
            'entity_type': 'confession',
            'entity_id': confessionId,
          }));

  /// Toggling a favourite invalidates the library's cached favourites list.
  ///
  /// The heart lives on the confession screen and the list it changes is a tab
  /// in the library, so without this a user favourites something, opens the
  /// library and does not find it — for up to the cache TTL.
  Future<WriteResult<void>> _favoriteWrite(
      Future<Map<String, dynamic>> Function() action) async {
    final result = await write(action, (_) {});
    if (result.succeeded) {
      await cache.delete(CacheKeys.favorites);
      await cache.delete('${CacheKeys.favorites}:confession');
    }
    return result;
  }

  /// Whether a confession is favourited.
  ///
  /// Matched on (entity_type, entity_id) — the favourite's own row id is
  /// deliberately not consulted. Comparing it against a confession id, which
  /// this did before PHASE 27, is a false positive waiting to happen: both are
  /// UUIDs from the same generator, and the match would silently light up the
  /// heart on the wrong confession.
  ///
  /// Not cached: favouriting is a write that must be reflected immediately,
  /// and the server narrows the list for us.
  Future<Loadable<bool>> isFavorite(String confessionId) async {
    try {
      final json = await api.getMeFavorites(type: 'confession');
      final favorites = parseList(json['data'], Favorite.fromJson);
      return Loadable.loaded(
        favorites.any((f) => f.entityType == 'confession' && f.entityId == confessionId),
      );
    } on ApiException catch (e) {
      return Loadable.failed(e);
    }
  }
}

/// Collections, downloads and schedules.
final class LibraryRepository extends Repository {
  LibraryRepository(super.api, super.cache);

  /// The listener's own collections, with item counts.
  ///
  /// Cached under a key distinct from the editorial `published_collections`:
  /// same word, different data.
  Future<Loadable<List<UserCollection>>> collections() => cachedRead(
        key: CacheKeys.collections,
        ttl: CacheTtl.library,
        fetch: () async => {'data': (await api.getMeCollections())['data'] ?? []},
        decode: (json) => parseList(json['data'], UserCollection.fromJson),
      );

  /// One collection with its items.
  ///
  /// Deliberately uncached. The list is cached because it is what the library
  /// opens on and it survives being a few minutes old; a detail view is opened
  /// to act on — add, remove, reorder — and showing a stale item list there
  /// means the user reorders rows that no longer exist.
  Future<Loadable<UserCollection>> collection(String id) async {
    try {
      return Loadable.loaded(UserCollection.fromJson(await api.getMeCollectionsById(id)));
    } on ApiException catch (e) {
      return Loadable.failed(e);
    }
  }

  /// Creates a collection. Private unless asked otherwise, and the server
  /// enforces that floor independently (§37).
  Future<WriteResult<UserCollection>> createCollection(
    String name, {
    String description = '',
    String? visibility,
    String? coverUrl,
  }) =>
      _invalidatingWrite(
        () => api.postMeCollections({
          'name': name,
          if (description.isNotEmpty) 'description': description,
          if (visibility != null && visibility.isNotEmpty) 'visibility': visibility,
          if (coverUrl != null && coverUrl.isNotEmpty) 'cover_url': coverUrl,
        }),
        UserCollection.fromJson,
      );

  /// Patches a collection. Each parameter is optional, and a null one is
  /// omitted rather than sent; an empty [coverUrl] is sent deliberately, as
  /// the instruction to clear the artwork back to the monogram (G-44).
  Future<WriteResult<UserCollection>> updateCollection(
    String id, {
    String? name,
    String? description,
    String? visibility,
    String? coverUrl,
  }) =>
      _invalidatingWrite(
        () => api.patchMeCollectionsById(id, {
          if (name != null) 'name': name,
          if (description != null) 'description': description,
          if (visibility != null) 'visibility': visibility,
          if (coverUrl != null) 'cover_url': coverUrl,
        }),
        UserCollection.fromJson,
      );

  Future<WriteResult<void>> deleteCollection(String id) =>
      _invalidatingWrite(() => api.deleteMeCollectionsById(id), (_) {});

  /// Adds a confession to a collection. The server answers with the whole
  /// collection, so the caller does not need a second read to refresh.
  Future<WriteResult<UserCollection>> addToCollection(
          String collectionId, String confessionId) =>
      _invalidatingWrite(
        () => api.postMeCollectionsByIdItems(
            collectionId, {'confession_id': confessionId}),
        UserCollection.fromJson,
      );

  Future<WriteResult<void>> removeFromCollection(
          String collectionId, String confessionId) =>
      _invalidatingWrite(
        () => api.deleteMeCollectionsByIdItemsByConfessionId(
            collectionId, confessionId),
        (_) {},
      );

  Future<WriteResult<UserCollection>> reorderCollection(
          String collectionId, List<String> confessionIds) =>
      _invalidatingWrite(
        () => api.patchMeCollectionsByIdReorder(
            collectionId, {'confession_ids': confessionIds}),
        UserCollection.fromJson,
      );

  /// Removes a favourite of any entity type.
  ///
  /// The library lists favourited categories and voices alongside confessions,
  /// so the type has to be carried rather than assumed — hard-coding
  /// 'confession' would make a favourited category impossible to remove from
  /// the one screen that shows it.
  Future<WriteResult<void>> removeFavorite({
    required String entityType,
    required String entityId,
  }) async {
    final result = await write(
      () => api.deleteMeFavorites({
        'entity_type': entityType,
        'entity_id': entityId,
      }),
      (_) {},
    );
    if (result.succeeded) {
      await cache.delete(CacheKeys.favorites);
      await cache.delete('${CacheKeys.favorites}:$entityType');
    }
    return result;
  }

  /// The listener's favourites, newest first, with display names resolved by
  /// the server. [type] narrows to one kind of entity.
  Future<Loadable<List<Favorite>>> favorites({String? type}) => cachedRead(
        key: type == null || type.isEmpty
            ? CacheKeys.favorites
            : '${CacheKeys.favorites}:$type',
        ttl: CacheTtl.library,
        fetch: () async => {
          'data': (await api.getMeFavorites(
                type: type == null || type.isEmpty ? null : type,
              ))['data'] ??
              []
        },
        decode: (json) => parseList(json['data'], Favorite.fromJson),
      );

  /// Confessions the listener wrote, newest first.
  ///
  /// Decoded as [UserConfession], not [Confession]: `/me/confessions` returns
  /// the user's own writing, which shares no field names with editorial
  /// content beyond the id.
  Future<Loadable<List<UserConfession>>> myConfessions() => cachedRead(
        key: CacheKeys.myConfessions,
        ttl: CacheTtl.library,
        fetch: () async => {'data': (await api.getMeConfessions())['data'] ?? []},
        decode: (json) => parseList(json['data'], UserConfession.fromJson),
      );

  /// Offers a draft for moderation review. Publication is the moderator's
  /// decision, so a success here means "submitted", never "published".
  Future<WriteResult<UserConfession>> submitConfession(String id) =>
      _invalidatingWrite(
        () => api.postMeConfessionsByIdSubmit(id),
        UserConfession.fromJson,
      );

  /// Performs a write and drops the library's cached reads.
  ///
  /// Without this a user who creates a collection sees their new collection
  /// appear and then vanish on the next open, because the cached list — up to
  /// 30 minutes old — is served over it. The invalidation runs only on
  /// success: a failed write changed nothing, and throwing away a good cache
  /// because the network refused would make the offline case worse.
  Future<WriteResult<T>> _invalidatingWrite<T>(
    Future<Map<String, dynamic>> Function() action,
    T Function(Map<String, dynamic>) decode,
  ) async {
    final result = await write(action, decode);
    if (result.succeeded) {
      await cache.delete(CacheKeys.collections);
      await cache.delete(CacheKeys.favorites);
      await cache.delete(CacheKeys.myConfessions);
    }
    return result;
  }

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

/// Search across confessions, categories, collections, voices.
final class SearchRepository extends Repository {
  SearchRepository(super.api, super.cache);

  Future<Loadable<List<SearchResult>>> search(String query,
      {List<String>? types, int limit = 20}) async {
    if (query.trim().isEmpty) {
      return const Loadable.loaded(<SearchResult>[]);
    }
    try {
      final json = await api.getSearch(q: query, type: types, limit: limit);
      final results = json['results'] is List
          ? (json['results'] as List)
              .whereType<Map<String, dynamic>>()
              .map(SearchResult.fromJson)
              .toList()
          : <SearchResult>[];
      return Loadable.loaded(results);
    } on ApiException catch (e) {
      return Loadable.failed(e);
    }
  }
}

/// Personalized recommendations.
final class RecommendationsRepository extends Repository {
  RecommendationsRepository(super.api, super.cache);

  Future<Loadable<Recommendations>> recommendations() => cachedRead(
        key: 'recommendations',
        ttl: const Duration(minutes: 10),
        fetch: () => api.getRecommendations(),
        decode: Recommendations.fromJson,
      );
}

/// Templates: list, read, start, delete, share.
final class TemplateRepository extends Repository {
  TemplateRepository(super.api, super.cache);

  Future<Loadable<List<SessionTemplate>>> templates() async {
    try {
      final json = await api.getTemplates();
      return Loadable.loaded(
          parseList(json['templates'] ?? json['data'] ?? json, SessionTemplate.fromJson));
    } on ApiException catch (e) {
      return Loadable.failed(e);
    }
  }

  Future<Loadable<SessionTemplate>> template(String id) async {
    try {
      return Loadable.loaded(SessionTemplate.fromJson(await api.getTemplatesById(id)));
    } on ApiException catch (e) {
      return Loadable.failed(e);
    }
  }

  Future<Loadable<SessionTemplate>> sharedTemplate(String token) async {
    try {
      return Loadable.loaded(SessionTemplate.fromJson(await api.getTByToken(token)));
    } on ApiException catch (e) {
      return Loadable.failed(e);
    }
  }

  Future<WriteResult<ListeningSession>> startTemplate(String id) => write(
        () => api.postTemplatesByIdStart(id),
        ListeningSession.fromJson,
      );

  Future<WriteResult<void>> deleteTemplate(String id) =>
      write(() => api.deleteTemplatesById(id), (_) {});

  Future<WriteResult<SessionTemplate>> updateTemplate(String id,
          {String? name, String? description, bool? isPublic}) =>
      write(
        () => api.patchTemplatesById(id, {
          if (name != null) 'name': name,
          if (description != null) 'description': description,
          if (isPublic != null) 'is_public': isPublic,
        }),
        SessionTemplate.fromJson,
      );
}

/// Community: moderated public posts and published user confessions (G-40).
final class CommunityRepository extends Repository {
  CommunityRepository(super.api, super.cache);

  /// Anonymous, moderation-approved community posts (public).
  Future<Loadable<List<Map<String, dynamic>>>> feed() async {
    try {
      final json = await api.getCommunityFeed();
      final posts = (json['posts'] as List? ?? const [])
          .whereType<Map<String, dynamic>>()
          .map((e) => Map<String, dynamic>.from(e))
          .toList(growable: false);
      return Loadable.loaded(posts);
    } on ApiException catch (e) {
      return Loadable.failed(e);
    }
  }

  /// Published user confessions that are public (public UGC reader, G-40).
  /// Anonymous: the server never returns author identity.
  Future<Loadable<List<UserConfession>>> confessions({int limit = 20}) async {
    try {
      final json = await api.getCommunityConfessions(limit: limit);
      final list = (json['confessions'] as List? ?? const [])
          .whereType<Map<String, dynamic>>()
          .map(UserConfession.fromJson)
          .toList(growable: false);
      return Loadable.loaded(list);
    } on ApiException catch (e) {
      return Loadable.failed(e);
    }
  }

  Future<WriteResult<void>> react(String postId, String reaction) =>
      write(() => api.postCommunityPostsByIdReact(postId, reaction), (_) {});
}

/// Subscription plans, entitlements and trial.
final class SubscriptionRepository extends Repository {
  SubscriptionRepository(super.api, super.cache);

  Future<Loadable<List<Plan>>> plans({String? currency}) async {
    try {
      final json = await api.getSubscriptionsPlans(currency: currency);
      final list = json['plans'] is List
          ? (json['plans'] as List)
              .whereType<Map<String, dynamic>>()
              .map(Plan.fromJson)
              .toList()
          : <Plan>[];
      return Loadable.loaded(list);
    } on ApiException catch (e) {
      return Loadable.failed(e);
    }
  }

  Future<Loadable<Subscription>> subscription() async {
    try {
      return Loadable.loaded(Subscription.fromJson(await api.getSubscription()));
    } on ApiException catch (e) {
      return Loadable.failed(e);
    }
  }

  Future<Loadable<Entitlements>> entitlements() => cachedRead(
        key: 'entitlements',
        ttl: const Duration(minutes: 5),
        fetch: () => api.getEntitlements(),
        decode: Entitlements.fromJson,
      );

  Future<Loadable<List<TrialDay>>> trial() async {
    try {
      final json = await api.getSubscriptionsTrial();
      final journey = json['journey'] is List
          ? (json['journey'] as List)
              .whereType<Map<String, dynamic>>()
              .map(TrialDay.fromJson)
              .toList()
          : <TrialDay>[];
      return Loadable.loaded(journey);
    } on ApiException catch (e) {
      return Loadable.failed(e);
    }
  }

  /// The measured trial: which days were actually completed, and the funnel
  /// behind it.
  ///
  /// Deliberately not cached. A progress display is the evidence the paywall
  /// shows before asking for payment, and a five-minute-old reading would let
  /// it show a day as unfinished that the listener completed a moment ago.
  Future<Loadable<TrialEngagement>> trialEngagement() async {
    try {
      return Loadable.loaded(
          TrialEngagement.fromJson(await api.getSubscriptionsTrialEngagement()));
    } on ApiException catch (e) {
      return Loadable.failed(e);
    }
  }

  Future<WriteResult<TrialStatus>> startTrial() =>
      write(() => api.postSubscriptionsTrial(), TrialStatus.fromJson);

  Future<Loadable<TrialStatus>> trialStatus() async {
    try {
      return Loadable.loaded(
          TrialStatus.fromJson(await api.getSubscriptionsTrialStatus()));
    } on ApiException catch (e) {
      return Loadable.failed(e);
    }
  }

  Future<WriteResult<TrialStatus>> convertTrial() => write(
        () => api.postSubscriptionsTrialConvert(),
        TrialStatus.fromJson,
      );

  Future<WriteResult<Subscription>> verifyReceipt(
          {required String provider, required String receipt}) =>
      write(
        () => api.postSubscriptionsVerify({
          'provider': provider,
          'receipt': receipt,
        }),
        Subscription.fromVerification,
      );
}
