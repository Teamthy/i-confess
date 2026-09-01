import 'dart:async';
import 'dart:convert';
import 'dart:io';

import 'api_error.dart';
import 'token_store.dart';

/// Transport for the i-confess API.
///
/// Three responsibilities live here and nowhere else in the app:
///
///  1. Attaching credentials, so no screen touches a token.
///  2. Refreshing a session exactly once when several requests fail together —
///     a phone waking from sleep fires many calls at once, and refreshing per
///     call would rotate the session repeatedly and trip the server's reuse
///     detection, signing the user out.
///  3. Turning HTTP failures into typed errors the UI can branch on.
class ApiClient {
  ApiClient({
    required this.baseUrl,
    required TokenStore tokens,
    HttpClient? httpClient,
    this.onAuthenticationLost,
  })  : _tokens = tokens,
        _http = httpClient ?? HttpClient() {
    _http.connectionTimeout = const Duration(seconds: 10);
  }

  final String baseUrl;
  final TokenStore _tokens;
  final HttpClient _http;

  /// Called when the session cannot be recovered and the user must sign in.
  ///
  /// A callback rather than an exception so a background refresh failing does
  /// not surface as an error on whatever screen happens to be open.
  final void Function(AuthLossReason reason)? onAuthenticationLost;

  /// Guards refresh so concurrent 401s share one attempt.
  Future<bool>? _refreshInFlight;

  /// Timeout for a single request. Deliberately short: a user on a train would
  /// rather see "no connection" than a spinner that never resolves.
  static const _requestTimeout = Duration(seconds: 20);

  Future<Map<String, dynamic>> get(String path, {Map<String, String>? query}) =>
      _send('GET', path, query: query);

  Future<Map<String, dynamic>> post(String path, [Object? body]) =>
      _send('POST', path, body: body);

  Future<Map<String, dynamic>> patch(String path, [Object? body]) =>
      _send('PATCH', path, body: body);

  Future<Map<String, dynamic>> put(String path, [Object? body]) =>
      _send('PUT', path, body: body);

  Future<Map<String, dynamic>> delete(String path, [Object? body]) =>
      _send('DELETE', path, body: body);

  /// Sends a request, refreshing and retrying once on a 401.
  Future<Map<String, dynamic>> _send(
    String method,
    String path, {
    Object? body,
    Map<String, String>? query,
    bool allowRetry = true,
  }) async {
    final response = await _raw(method, path, body: body, query: query);

    // A 401 on an authenticated call means the access token has aged out.
    // Refresh once, then replay. `allowRetry` stops an infinite loop when the
    // refresh itself returns 401.
    if (response.status == 401 && allowRetry && await _tokens.hasSession()) {
      final refreshed = await _refreshOnce();
      if (refreshed) {
        return _send(method, path, body: body, query: query, allowRetry: false);
      }
    }

    return _decode(response);
  }

  /// Refreshes the session, collapsing concurrent callers onto one attempt.
  ///
  /// This is the important part. Rotation retires the old session, so two
  /// simultaneous refreshes would make the second present an already-rotated
  /// token — which the server correctly treats as theft and responds to by
  /// revoking every session in the family.
  Future<bool> _refreshOnce() {
    return _refreshInFlight ??= _doRefresh().whenComplete(() {
      _refreshInFlight = null;
    });
  }

  Future<bool> _doRefresh() async {
    final response = await _raw('POST', '/auth/refresh');

    if (response.status == 200) {
      final token = response.json['token'] as String?;
      if (token != null && token.isNotEmpty) {
        await _tokens.save(token);
        return true;
      }
    }

    // Distinguish theft from ordinary expiry: the user deserves to know their
    // session was ended for a security reason rather than silently timing out.
    final code = response.json['code'] as String? ?? '';
    await _tokens.clear();
    onAuthenticationLost?.call(
      code == ErrorCodes.tokenReused
          ? AuthLossReason.tokenReused
          : AuthLossReason.expired,
    );
    return false;
  }

  Future<_RawResponse> _raw(
    String method,
    String path, {
    Object? body,
    Map<String, String>? query,
  }) async {
    var uri = Uri.parse('$baseUrl$path');
    if (query != null && query.isNotEmpty) {
      uri = uri.replace(queryParameters: {...uri.queryParameters, ...query});
    }

    try {
      final request = await _http.openUrl(method, uri).timeout(_requestTimeout);
      request.headers.set(HttpHeaders.acceptHeader, 'application/json');

      final token = await _tokens.read();
      if (token != null && token.isNotEmpty) {
        request.headers.set(HttpHeaders.authorizationHeader, 'Bearer $token');
      }
      if (body != null) {
        request.headers.contentType = ContentType.json;
        request.write(jsonEncode(body));
      }

      final response = await request.close().timeout(_requestTimeout);
      final text = await response.transform(utf8.decoder).join();

      Map<String, dynamic> parsed;
      try {
        final decoded = text.isEmpty ? <String, dynamic>{} : jsonDecode(text);
        // Some endpoints return a bare array; wrap it so callers have one shape.
        parsed = decoded is Map<String, dynamic>
            ? decoded
            : <String, dynamic>{'data': decoded};
      } on FormatException {
        parsed = <String, dynamic>{};
      }

      return _RawResponse(
        status: response.statusCode,
        json: parsed,
        retryAfter: _parseRetryAfter(response.headers.value('retry-after')),
      );
    } on TimeoutException {
      throw const NetworkException('The server took too long to respond.');
    } on SocketException catch (e) {
      throw NetworkException('You appear to be offline.', cause: e);
    } on HttpException catch (e) {
      throw NetworkException('The connection failed.', cause: e);
    }
  }

  Map<String, dynamic> _decode(_RawResponse response) {
    if (response.status >= 200 && response.status < 300) {
      return response.json;
    }
    throw ApiError(
      status: response.status,
      code: response.json['code'] as String? ?? '',
      message: response.json['error'] as String? ?? 'Request failed.',
      retryAfter: response.retryAfter,
    );
  }

  static Duration? _parseRetryAfter(String? header) {
    if (header == null) return null;
    final seconds = int.tryParse(header.trim());
    return seconds == null ? null : Duration(seconds: seconds);
  }

  void close() => _http.close(force: true);
}

/// Why the session ended, so the UI can explain rather than just eject.
enum AuthLossReason {
  /// The session aged out. Ordinary; sign in again.
  expired,

  /// A token was replayed, which means it was copied. The user should be told
  /// plainly and prompted to change their password.
  tokenReused,
}

class _RawResponse {
  const _RawResponse({
    required this.status,
    required this.json,
    this.retryAfter,
  });

  final int status;
  final Map<String, dynamic> json;
  final Duration? retryAfter;
}
