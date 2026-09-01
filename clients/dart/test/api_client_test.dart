import 'dart:convert';
import 'dart:io';

import 'package:iconfess_api/iconfess_api.dart';
import 'package:test/test.dart';

/// Tests run against a real local HTTP server rather than a mocked transport.
///
/// A mock would assert that the client calls a fake the way the test expects,
/// which proves nothing about how it behaves against actual sockets, status
/// codes and headers — the places the bugs are.
void main() {
  late _FakeApi api;
  late ApiClient client;
  late InMemoryTokenStore tokens;

  setUp(() async {
    api = await _FakeApi.start();
    tokens = InMemoryTokenStore();
    client = ApiClient(baseUrl: api.baseUrl, tokens: tokens);
  });

  tearDown(() async {
    client.close();
    await api.stop();
  });

  group('requests', () {
    test('sends the bearer token when signed in', () async {
      await tokens.save('token-1');
      api.respond('/me', 200, {'id': 'u1'});

      await client.get('/me');

      expect(api.lastAuthHeader, equals('Bearer token-1'));
    });

    test('omits the header when signed out', () async {
      api.respond('/categories', 200, {'data': []});

      await client.get('/categories');

      expect(api.lastAuthHeader, isNull);
    });

    test('decodes a bare JSON array under a data key', () async {
      api.respondRaw('/categories', 200, '[{"id":"c1"}]');

      final result = await client.get('/categories');

      // Endpoints that return arrays must not force every caller to handle a
      // different top-level shape.
      expect(result['data'], isA<List<dynamic>>());
    });
  });

  group('errors', () {
    test('maps a 4xx into a typed ApiError with its code', () async {
      await tokens.save('t');
      api.respond('/me/downloads', 402, {
        'error': 'offline downloads are available on Premium',
        'code': 'ENTITLEMENT_REQUIRED',
      });

      final error = await _captureError(() => client.post('/me/downloads', {}));

      expect(error, isA<ApiError>());
      final apiError = error as ApiError;
      expect(apiError.requiresSubscription, isTrue);
      expect(apiError.code, equals(ErrorCodes.entitlementRequired));
    });

    test('surfaces Retry-After so the client waits rather than hammering',
        () async {
      api.respond(
        '/auth/login',
        429,
        {'error': 'too many attempts', 'code': 'AUTH_RATE_LIMITED'},
        headers: {'retry-after': '45'},
      );

      final error = await _captureError(() => client.post('/auth/login', {}));

      final apiError = error as ApiError;
      expect(apiError.isRateLimited, isTrue);
      expect(apiError.retryAfter, equals(const Duration(seconds: 45)));
    });

    test('reports being offline as a network error, not bad credentials',
        () async {
      // The most common client bug: a dropped connection shown as "invalid
      // password" makes people change a password that was never wrong.
      final offline = ApiClient(
        baseUrl: 'http://127.0.0.1:1',
        tokens: InMemoryTokenStore(),
      );
      addTearDown(offline.close);

      final error = await _captureError(() => offline.get('/me'));

      expect(error, isA<NetworkException>());
      expect(error, isNot(isA<ApiError>()));
    });

    test('a malformed body still produces a typed error', () async {
      api.respondRaw('/me', 500, 'not json at all');

      final error = await _captureError(() => client.get('/me'));

      expect((error as ApiError).isServerFault, isTrue);
    });
  });

  group('session refresh', () {
    test('refreshes once on 401 and replays the request', () async {
      await tokens.save('expired');
      api.respondSequence('/me', [
        _Reply(401, {'error': 'unauthorized', 'code': 'AUTH_SESSION_EXPIRED'}),
        _Reply(200, {'id': 'u1'}),
      ]);
      api.respond('/auth/refresh', 200, {'token': 'fresh'});

      final result = await client.get('/me');

      expect(result['id'], equals('u1'));
      expect(await tokens.read(), equals('fresh'),
          reason: 'the rotated token must be persisted');
      expect(api.callCount('/auth/refresh'), equals(1));
    });

    test('collapses concurrent 401s onto a single refresh', () async {
      // This is the important one. Rotation retires the presented session, so
      // firing several refreshes at once makes the later ones replay an
      // already-rotated token. The server correctly reads that as theft and
      // revokes the whole family — signing the user out for doing nothing
      // wrong. A phone waking from sleep issues exactly this burst.
      await tokens.save('expired');
      api.respondSequence('/me', [
        _Reply(401, {'code': 'AUTH_SESSION_EXPIRED'}),
        _Reply(401, {'code': 'AUTH_SESSION_EXPIRED'}),
        _Reply(401, {'code': 'AUTH_SESSION_EXPIRED'}),
        _Reply(200, {'id': 'u1'}),
        _Reply(200, {'id': 'u1'}),
        _Reply(200, {'id': 'u1'}),
      ]);
      api.respond('/auth/refresh', 200, {'token': 'fresh'});

      await Future.wait([
        client.get('/me'),
        client.get('/me'),
        client.get('/me'),
      ]);

      expect(api.callCount('/auth/refresh'), equals(1),
          reason: 'concurrent refreshes would trip server reuse detection');
    });

    test('does not retry forever when the refresh itself fails', () async {
      await tokens.save('dead');
      api.respond('/me', 401, {'code': 'AUTH_SESSION_REVOKED'});
      api.respond('/auth/refresh', 401, {'code': 'AUTH_SESSION_REVOKED'});

      await _captureError(() => client.get('/me'));

      expect(api.callCount('/auth/refresh'), equals(1));
      expect(await tokens.read(), isNull,
          reason: 'an unrecoverable session must be cleared');
    });

    test('reports token reuse distinctly from ordinary expiry', () async {
      AuthLossReason? reason;
      final c = ApiClient(
        baseUrl: api.baseUrl,
        tokens: tokens,
        onAuthenticationLost: (r) => reason = r,
      );
      addTearDown(c.close);

      await tokens.save('stolen');
      api.respond('/me', 401, {'code': 'AUTH_TOKEN_REUSED'});
      api.respond('/auth/refresh', 401, {
        'error': 'you were signed out for security',
        'code': 'AUTH_TOKEN_REUSED',
      });

      await _captureError(() => c.get('/me'));

      // The user should be told their session was ended for a security reason,
      // not silently bounced to a login screen.
      expect(reason, equals(AuthLossReason.tokenReused));
    });

    test('does not attempt refresh when signed out', () async {
      api.respond('/me', 401, {'code': 'AUTH_TOKEN_INVALID'});

      await _captureError(() => client.get('/me'));

      expect(api.callCount('/auth/refresh'), isZero,
          reason: 'no session means nothing to refresh');
    });
  });

  group('generated endpoints', () {
    test('build the right paths and methods', () async {
      await tokens.save('t');
      api.respond('/me/bootstrap', 200, {'user': <String, dynamic>{}});
      api.respond('/me/collections/abc-123', 200, {'id': 'abc-123'});

      await client.getMeBootstrap();
      expect(api.lastPath, equals('/me/bootstrap'));

      // Path parameters must be interpolated, not left as literals.
      await client.getMeCollectionsById('abc-123');
      expect(api.lastPath, equals('/me/collections/abc-123'));
      expect(api.lastMethod, equals('GET'));
    });

    test('send a JSON body on writes', () async {
      await tokens.save('t');
      api.respond('/me/collections', 201, {'id': 'c1'});

      await client.postMeCollections({'name': 'Morning'});

      expect(jsonDecode(api.lastBody!)['name'], equals('Morning'));
      expect(api.lastContentType, contains('application/json'));
    });
  });
}

Future<Object> _captureError(Future<void> Function() action) async {
  try {
    await action();
  } catch (e) {
    return e;
  }
  throw StateError('expected an error but the call succeeded');
}

class _Reply {
  _Reply(this.status, this.body, {this.headers});
  final int status;
  final Object body;
  final Map<String, String>? headers;
}

/// A minimal real HTTP server standing in for the API.
class _FakeApi {
  _FakeApi._(this._server);

  final HttpServer _server;
  final Map<String, List<_Reply>> _queued = {};
  final Map<String, _Reply> _fixed = {};
  final Map<String, int> _calls = {};

  String? lastAuthHeader;
  String? lastPath;
  String? lastMethod;
  String? lastBody;
  String? lastContentType;

  String get baseUrl => 'http://127.0.0.1:${_server.port}';

  static Future<_FakeApi> start() async {
    final server = await HttpServer.bind(InternetAddress.loopbackIPv4, 0);
    final api = _FakeApi._(server);
    api._listen();
    return api;
  }

  void respond(String path, int status, Object body,
          {Map<String, String>? headers}) =>
      _fixed[path] = _Reply(status, body, headers: headers);

  void respondRaw(String path, int status, String raw) =>
      _fixed[path] = _Reply(status, raw);

  void respondSequence(String path, List<_Reply> replies) =>
      _queued[path] = List.of(replies);

  int callCount(String path) => _calls[path] ?? 0;

  void _listen() {
    _server.listen((request) async {
      final path = request.uri.path;
      _calls[path] = (_calls[path] ?? 0) + 1;

      lastPath = path;
      lastMethod = request.method;
      lastAuthHeader = request.headers.value(HttpHeaders.authorizationHeader);
      lastContentType = request.headers.contentType?.mimeType;
      lastBody = await utf8.decoder.bind(request).join();

      final queue = _queued[path];
      final reply = (queue != null && queue.isNotEmpty)
          ? queue.removeAt(0)
          : _fixed[path] ?? _Reply(404, {'error': 'not found'});

      reply.headers?.forEach(request.response.headers.set);
      request.response.statusCode = reply.status;
      request.response.headers.contentType = ContentType.json;
      request.response
          .write(reply.body is String ? reply.body : jsonEncode(reply.body));
      await request.response.close();
    });
  }

  Future<void> stop() => _server.close(force: true);
}
