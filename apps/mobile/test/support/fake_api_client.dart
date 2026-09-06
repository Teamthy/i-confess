import 'package:iconfess_api/iconfess_api.dart';

/// A scripted API client for widget tests.
///
/// `ApiClient` is subclassed rather than replaced with a fake `AuthRepository`
/// for two reasons. First, `AuthRepository` is a `final class` and cannot be
/// implemented. Second — and more importantly — a fake repository would test the
/// app against a copy of the client's behaviour, and the defect PHASE 19 exists
/// to fix (a sign-in that never stored its token) lived *in* the client. Keeping
/// the real `AuthRepository` and the real `ApiClient` in the tree means these
/// tests exercise the path that ships, and only the socket is invented.
///
/// The alternative used by `clients/dart`'s own tests is a real local HTTP
/// server. That is the better tool there, and unavailable here: `flutter_test`
/// installs an `HttpOverrides` that refuses network access inside `testWidgets`,
/// so any attempt to open a socket fails the test before the assertion is
/// reached.
class FakeApiClient extends ApiClient {
  FakeApiClient({required super.tokens})
      : super(baseUrl: 'http://localhost:0');

  /// What each path answers with: a JSON body, or an [ApiException] to throw.
  final Map<String, Object> responses = {};

  /// Every call, in order, so a test can assert what was actually sent.
  final List<FakeCall> calls = [];

  void respond(String path, Map<String, dynamic> body) => responses[path] = body;

  void respondWith(String path, ApiException error) => responses[path] = error;

  /// The body of the last call to [path], as decoded JSON.
  Map<String, dynamic>? bodyOf(String path) {
    for (final call in calls.reversed) {
      if (call.path == path) return call.body;
    }
    return null;
  }

  int callCount(String path) => calls.where((c) => c.path == path).length;

  @override
  Future<Map<String, dynamic>> post(String path, [Object? body]) =>
      _answer('POST', path, body);

  @override
  Future<Map<String, dynamic>> get(String path, {Map<String, String>? query}) =>
      _answer('GET', path, null);

  @override
  Future<Map<String, dynamic>> patch(String path, [Object? body]) =>
      _answer('PATCH', path, body);

  @override
  Future<Map<String, dynamic>> put(String path, [Object? body]) =>
      _answer('PUT', path, body);

  @override
  Future<Map<String, dynamic>> delete(String path, [Object? body]) =>
      _answer('DELETE', path, body);

  Future<Map<String, dynamic>> _answer(String method, String path, Object? body) async {
    calls.add(FakeCall(method, path, body is Map<String, dynamic> ? body : null));
    final reply = responses[path];
    if (reply == null) {
      throw ApiError(status: 404, code: '', message: 'no response scripted for $path');
    }
    if (reply is ApiException) throw reply;
    return Map<String, dynamic>.from(reply as Map);
  }
}

class FakeCall {
  const FakeCall(this.method, this.path, this.body);

  final String method;
  final String path;
  final Map<String, dynamic>? body;

  @override
  String toString() => '$method $path ${body ?? ''}';
}
