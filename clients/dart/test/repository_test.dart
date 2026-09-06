import 'dart:convert';
import 'dart:io';

import 'package:iconfess_api/iconfess_api.dart';
import 'package:test/test.dart';

/// Repository behaviour, tested against a real local HTTP server.
///
/// The interesting cases are the unhappy ones: offline, stale cache, partial
/// entitlement. Those are where an app either degrades gracefully or lies to
/// the user, and they are exactly what a mocked transport cannot exercise.
void main() {
  late _Api api;
  late ApiClient client;
  late InMemoryCache raw;
  late JsonCache cache;
  late InMemoryTokenStore tokens;

  setUp(() async {
    api = await _Api.start();
    tokens = InMemoryTokenStore();
    client = ApiClient(baseUrl: api.baseUrl, tokens: tokens);
    raw = InMemoryCache();
    cache = JsonCache(raw);
  });

  tearDown(() async {
    client.close();
    await api.stop();
  });

  ProfileRepository profileRepo() => ProfileRepository(client, cache);
  LibraryRepository libraryRepo() => LibraryRepository(client, cache);
  AuthRepository authRepo() => AuthRepository(client, cache);

  group('offline reads', () {
    test('serves cache when the network is unavailable, and says so', () async {
      api.respond('/me/profile', 200, {'display_name': 'Grace', 'timezone': 'Africa/Lagos'});

      // Warm the cache while online.
      final online = await profileRepo().profile();
      expect((online as LoadLoaded<Profile>).value.displayName, 'Grace');
      expect(online.fromCache, isFalse);

      // Now go offline entirely.
      final offlineClient = ApiClient(
        baseUrl: 'http://127.0.0.1:1',
        tokens: InMemoryTokenStore(),
      );
      addTearDown(offlineClient.close);

      final offline = await ProfileRepository(offlineClient, cache).profile();

      final loaded = offline as LoadLoaded<Profile>;
      expect(loaded.value.displayName, 'Grace');
      // The flag is the point: presenting cached data as live is how an app
      // shows someone a subscription state that lapsed days ago.
      expect(loaded.fromCache, isTrue);
    });

    test('flags cache older than its TTL as stale', () async {
      // Write a cache entry dated well in the past.
      final old = JsonCache(raw, now: () => DateTime.now().subtract(const Duration(days: 30)));
      await old.write(CacheKeys.profile, {'display_name': 'Stale Grace'});

      final offlineClient = ApiClient(
        baseUrl: 'http://127.0.0.1:1',
        tokens: InMemoryTokenStore(),
      );
      addTearDown(offlineClient.close);

      final result = await ProfileRepository(offlineClient, cache).profile();

      final loaded = result as LoadLoaded<Profile>;
      expect(loaded.value.displayName, 'Stale Grace');
      expect(loaded.stale, isTrue, reason: 'a month-old profile is not fresh');
    });

    test('fails cleanly when offline with no cache', () async {
      final offlineClient = ApiClient(
        baseUrl: 'http://127.0.0.1:1',
        tokens: InMemoryTokenStore(),
      );
      addTearDown(offlineClient.close);

      final result = await ProfileRepository(offlineClient, cache).profile();

      final failed = result as LoadFailed<Profile>;
      expect(failed.error, isA<NetworkException>());
      expect(failed.isRetryable, isTrue, reason: 'coming back online should be retried');
    });

    test('does NOT fall back to cache on a 403', () async {
      api.respond('/me/profile', 200, {'display_name': 'Grace'});
      await profileRepo().profile(); // warm cache

      api.respond('/me/profile', 403, {'error': 'forbidden'});
      final result = await profileRepo().profile();

      // Serving cache on an authorisation failure would show content the user
      // is no longer permitted to see.
      expect(result, isA<LoadFailed<Profile>>());
    });

    test('does fall back to cache on a 5xx', () async {
      api.respond('/me/profile', 200, {'display_name': 'Grace'});
      await profileRepo().profile();

      api.respond('/me/profile', 503, {'error': 'unavailable'});
      final result = await profileRepo().profile();

      // Our outage should not blank the user's screen.
      final loaded = result as LoadLoaded<Profile>;
      expect(loaded.value.displayName, 'Grace');
      expect(loaded.fromCache, isTrue);
    });

    test('a corrupt cache entry is a miss, not a crash', () async {
      await raw.write(CacheKeys.profile, 'not json{{{');
      api.respond('/me/profile', 200, {'display_name': 'Recovered'});

      final result = await profileRepo().profile();

      expect((result as LoadLoaded<Profile>).value.displayName, 'Recovered');
    });
  });

  group('sign-in', () {
    test('an MFA challenge is not reported as a failure', () async {
      api.respond('/auth/login', 200, {'mfa_required': true});

      final outcome = await authRepo().signIn(email: 'a@b.com', password: 'pw');

      expect(outcome, isA<MfaRequired>());
    });

    test('offline is distinguished from bad credentials', () async {
      final offlineClient = ApiClient(
        baseUrl: 'http://127.0.0.1:1',
        tokens: InMemoryTokenStore(),
      );
      addTearDown(offlineClient.close);

      final outcome = await AuthRepository(offlineClient, cache)
          .signIn(email: 'a@b.com', password: 'pw');

      final failed = outcome as SignInFailed;
      expect(failed.isOffline, isTrue,
          reason: 'telling a user on a train that their password is wrong '
              'makes them change one that was never wrong');
    });

    test('rate limiting is surfaced as such', () async {
      api.respond('/auth/login', 429,
          {'error': 'too many attempts', 'code': 'AUTH_RATE_LIMITED'});

      final outcome = await authRepo().signIn(email: 'a@b.com', password: 'pw');

      expect((outcome as SignInFailed).isRateLimited, isTrue);
    });

    test('a successful sign-in persists the session', () async {
      // The regression this pins: sign-in used to hand the token to the caller
      // and nothing wrote it down, so the app was "signed in" for the life of
      // the process and signed out on the next launch - with every request in
      // between sent with no Authorization header at all.
      api.respond('/auth/login', 200, {
        'token': 'session-token',
        'user': {'id': 'u-1', 'email': 'a@b.com'},
      });

      final outcome = await authRepo().signIn(email: 'a@b.com', password: 'pw');

      expect(outcome, isA<SignedIn>());
      expect((outcome as SignedIn).userId, 'u-1');
      expect(await tokens.read(), 'session-token',
          reason: 'a token nobody stored cannot be sent on the next request');
      expect(await client.hasSession(), isTrue);
    });

    test('an MFA challenge stores no session', () async {
      api.respond('/auth/login', 200, {'mfa_required': true});

      await authRepo().signIn(email: 'a@b.com', password: 'pw');

      // A half-finished sign-in must not leave a token behind: the second
      // factor is exactly the thing that gates the session (S41).
      expect(await tokens.hasSession(), isFalse);
    });

    test('a new account is signed in and its session persisted', () async {
      api.respond('/auth/register', 200, {
        'token': 'new-session',
        'user': {'id': 'u-9'},
      });

      final outcome =
          await authRepo().register(email: 'new@b.com', password: 'pw');

      expect(outcome, isA<AccountCreated>());
      expect((outcome as AccountCreated).userId, 'u-9');
      expect(await tokens.read(), 'new-session');
    });

    test('a duplicate registration is not treated as an error', () async {
      // The server answers 200 with no token so the endpoint cannot be used to
      // discover who has an account.
      api.respond('/auth/register', 200, {
        'message': 'Check your email to continue setting up your account.',
        'pending': true,
      });

      final outcome =
          await authRepo().register(email: 'taken@b.com', password: 'pw');

      expect(outcome, isA<CheckYourEmail>(),
          reason: 'no token means "check your email", not a failure');
      expect((outcome as CheckYourEmail).message, contains('Check your email'));
      expect(await tokens.hasSession(), isFalse,
          reason: 'no session was issued, so none may be invented');
    });

    test('registration sends only fields the server accepts', () async {
      api.respond('/auth/register', 200, {'token': 't', 'user': {'id': 'u'}});

      await authRepo().register(email: 'x@y.z', password: 'pw');

      // The server decodes with DisallowUnknownFields, so an extra key is a 400
      // rather than something it quietly ignores.
      final sent = jsonDecode(api.lastBody!) as Map<String, dynamic>;
      expect(sent.keys.toSet(), {'email', 'password'});
    });

    test('a rejected registration leaves no session behind', () async {
      api.respond('/auth/register', 400, {'error': 'password must be at least 8 characters'});

      final outcome =
          await authRepo().register(email: 'x@y.z', password: 'short');

      expect(outcome, isA<RegisterFailed>());
      expect(await tokens.hasSession(), isFalse);
    });

    test('verification, resend and reset send the fields the server reads',
        () async {
      api.respond('/auth/verify-email', 200, {'message': 'email verified'});
      await authRepo().verifyEmail('tok-1');
      expect(jsonDecode(api.lastBody!), {'token': 'tok-1'});

      api.respond('/auth/resend-verification', 200, {'message': 'sent'});
      await authRepo().resendVerification('a@b.com');
      expect(jsonDecode(api.lastBody!), {'email': 'a@b.com'});

      api.respond('/auth/reset-password', 200, {'message': 'password reset'});
      final result = await authRepo().resetPassword(token: 'tok-2', password: 'new-password');
      expect(jsonDecode(api.lastBody!), {'token': 'tok-2', 'password': 'new-password'});
      expect(result.succeeded, isTrue);
    });

    test('an expired reset token is reported, not swallowed', () async {
      api.respond('/auth/reset-password', 401,
          {'error': 'invalid or expired reset token'});

      final result = await authRepo().resetPassword(token: 'old', password: 'new-password');

      final failure = result as WriteFailure<void>;
      expect((failure.error as ApiError).status, 401);
    });

    test('sign-out clears local state even when the call fails', () async {
      api.respond('/me/profile', 200, {'display_name': 'Grace'});
      await profileRepo().profile();
      expect(await raw.read(CacheKeys.profile), isNotNull);

      api.respond('/auth/logout', 500, {'error': 'boom'});
      await authRepo().signOut();

      // A user who taps sign-out must end up signed out.
      expect(await raw.read(CacheKeys.profile), isNull);
      expect(await tokens.hasSession(), isFalse,
          reason: 'a failed revoke must not leave the token on the device');
    });
  });

  group('entitlement', () {
    test('a 402 on download is an upgrade prompt, not a crash', () async {
      api.respond('/me/downloads', 402, {
        'error': 'offline downloads are available on Premium',
        'code': 'ENTITLEMENT_REQUIRED',
      });

      final result = await libraryRepo().download('conf-1');

      final failure = result as WriteFailure<DownloadTicket>;
      expect((failure.error as ApiError).requiresSubscription, isTrue);
    });

    test('locked session items are kept, not hidden', () async {
      api.respond('/sessions', 201, {
        'id': 's1',
        'duration_seconds': 600,
        'voice_downgraded': true,
        'voice_downgrade_reason': 'premium_voice_requires_subscription',
        'items': [
          {'confession_id': 'c1', 'audio_url': '/media/x?sig=a', 'duration_seconds': 300},
          {'confession_id': 'c2', 'audio_url': '', 'locked': true, 'lock_reason': 'premium_voice_requires_subscription'},
        ],
      });

      final result = await ContentRepository(client, cache)
          .createSession(categoryIds: ['cat1'], durationSeconds: 600);

      final session = (result as WriteSuccess<ListeningSession>).value;
      expect(session.items, hasLength(2));
      expect(session.playable, hasLength(1),
          reason: 'only the unlocked item can be played');
      expect(session.hasLockedItems, isTrue,
          reason: 'the UI needs this to show an upgrade prompt instead of a gap');
      // A silently substituted voice looks like a bug; a declared one is a
      // paywall the user can act on.
      expect(session.voiceDowngraded, isTrue);
    });
  });

  test("mySessions lists the listener's sessions without caching them", () async {
    api.respond('/sessions', 200, {
      'sessions': [
        {'id': 'a', 'status': 'PAUSED', 'duration_seconds': 900},
        {'id': 'b', 'status': 'COMPLETED', 'duration_seconds': 600},
      ],
    });

    final repo = ContentRepository(client, cache);
    final loadable = await repo.mySessions();

    final sessions = loadable.valueOrNull!;
    expect(sessions, hasLength(2));
    expect(sessions.map((s) => s.status), ['PAUSED', 'COMPLETED']);

    // A second read must hit the server again: a cached list would show
    // "continue" for a session that has since finished.
    await repo.mySessions();
    await repo.mySessions();
    expect(api.callCount('/sessions'), 3,
        reason: 'session lists are never served from the client cache');
  });

  group('downloads', () {
    test('reports licences expiring soon so they can be renewed', () async {
      final soon = DateTime.now().add(const Duration(hours: 12));
      final later = DateTime.now().add(const Duration(days: 30));

      api.respond('/me/downloads', 200, {
        'limit': 500,
        'used': 2,
        'offline_hours_allowed': 168,
        'downloads': [
          {'id': 'd1', 'audio_asset_id': 'a1', 'expires_at': soon.toIso8601String()},
          {'id': 'd2', 'audio_asset_id': 'a2', 'expires_at': later.toIso8601String()},
        ],
      });

      final result = await libraryRepo().downloads();
      final lib = (result as LoadLoaded<DownloadLibrary>).value;

      final expiring = lib.expiringWithin(const Duration(days: 1));
      expect(expiring.map((d) => d.id), equals(['d1']),
          reason: 'renewing before a flight is the difference between '
              'working offline content and none');
    });

    test('an already-expired licence is not reported as expiring', () async {
      final past = DateTime.now().subtract(const Duration(days: 1));
      api.respond('/me/downloads', 200, {
        'downloads': [
          {'id': 'd1', 'audio_asset_id': 'a1', 'expired': true, 'expires_at': past.toIso8601String()},
        ],
      });

      final lib = ((await libraryRepo().downloads()) as LoadLoaded<DownloadLibrary>).value;

      expect(lib.expiringWithin(const Duration(days: 7)), isEmpty,
          reason: 'expired is a different state from expiring');
    });

    test('surfaces the plan limit', () async {
      api.respond('/me/downloads', 200, {'limit': 2, 'used': 2, 'downloads': []});

      final lib = ((await libraryRepo().downloads()) as LoadLoaded<DownloadLibrary>).value;

      expect(lib.atLimit, isTrue);
    });
  });

  group('writes', () {
    test('a partial profile update sends only the changed fields', () async {
      api.respond('/me/profile', 200, {'display_name': 'Grace'});

      await profileRepo().updateProfile(timezone: 'Africa/Lagos');

      final sent = jsonDecode(api.lastBody!) as Map<String, dynamic>;
      expect(sent.keys, equals({'timezone'}),
          reason: 'sending unmentioned fields would blank them');
    });

    test('an explicitly empty value is still sent', () async {
      api.respond('/me/profile', 200, {});

      await profileRepo().updateProfile(bio: '');

      final sent = jsonDecode(api.lastBody!) as Map<String, dynamic>;
      expect(sent.containsKey('bio'), isTrue,
          reason: 'clearing a bio must be distinguishable from not touching it');
    });

    test('writes are never cached optimistically', () async {
      api.respond('/me/profile', 500, {'error': 'boom'});

      final result = await profileRepo().updateProfile(displayName: 'Nope');

      expect(result.succeeded, isFalse);
      expect(await raw.read(CacheKeys.profile), isNull,
          reason: 'claiming success before the server confirms is how a user '
              'believes a change saved when it did not');
    });
  });

  group('models', () {
    test('tolerate missing and wrongly-typed fields', () {
      // An older app must survive a newer server, and vice versa.
      final profile = Profile.fromJson({'display_name': 42, 'bio': null});
      expect(profile.displayName, isEmpty);
      expect(profile.timezone, 'UTC');

      final session = ListeningSession.fromJson({'items': 'not a list'});
      expect(session.items, isEmpty);

      final ents = Entitlements.fromJson(const {});
      expect(ents.isPremium, isFalse,
          reason: 'an unparseable plan must never default to premium');
    });

    test('keep explicit and inferred interests apart', () {
      final interests = Interests.fromJson({
        'explicit': [
          {'category_id': 'healing'}
        ],
        'inferred': [
          {'category_id': 'finance'}
        ],
      });

      expect(interests.explicit, equals(['healing']));
      expect(interests.inferred, equals(['finance']));
      // Merging them would let the UI present a guess as something the user
      // said, which the product rules forbid.
      expect(interests.explicit, isNot(contains('finance')));
    });
  });
}

class _Api {
  _Api._(this._server);

  final HttpServer _server;
  final Map<String, _Reply> _replies = {};
  final Map<String, int> _counts = {};
  String? lastBody;

  int callCount(String path) => _counts[path] ?? 0;

  String get baseUrl => 'http://127.0.0.1:${_server.port}';

  static Future<_Api> start() async {
    final server = await HttpServer.bind(InternetAddress.loopbackIPv4, 0);
    final api = _Api._(server);
    api._listen();
    return api;
  }

  void respond(String path, int status, Object body) =>
      _replies[path] = _Reply(status, body);

  void _listen() {
    _server.listen((request) async {
      lastBody = await utf8.decoder.bind(request).join();
      _counts[request.uri.path] = (_counts[request.uri.path] ?? 0) + 1;
      final reply = _replies[request.uri.path] ?? _Reply(404, {'error': 'not found'});

      request.response.statusCode = reply.status;
      request.response.headers.contentType = ContentType.json;
      request.response.write(jsonEncode(reply.body));
      await request.response.close();
    });
  }

  Future<void> stop() => _server.close(force: true);
}

class _Reply {
  _Reply(this.status, this.body);
  final int status;
  final Object body;
}
