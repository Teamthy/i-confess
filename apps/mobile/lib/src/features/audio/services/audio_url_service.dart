/// Resolves short-lived audio URLs from entitlement-checked session queues.
///
/// Audio assets do not expose storage URLs directly. The server mints playback
/// URLs while returning a particular listener's session queue, so callers must
/// provide a session ID and (when known) a queue item or asset ID.
import 'package:iconfess_api/iconfess_api.dart';

class AudioUrlServiceConfig {
  const AudioUrlServiceConfig({
    this.urlExpiryBuffer = const Duration(minutes: 2),
    this.cacheDuration = const Duration(minutes: 10),
    this.maxCachedUrls = 50,
  });

  final Duration urlExpiryBuffer;
  final Duration cacheDuration;
  final int maxCachedUrls;
}

class CachedSignedUrl {
  const CachedSignedUrl({
    required this.url,
    required this.expiresAt,
    this.isStreamUrl = true,
  });

  final String url;
  final DateTime expiresAt;
  final bool isStreamUrl;

  bool get isValid => DateTime.now().isBefore(expiresAt);

  bool isAboutToExpire(Duration buffer) =>
      !DateTime.now().add(buffer).isBefore(expiresAt);
}

class AudioUrlService {
  AudioUrlService({
    required ApiClient client,
    AudioUrlServiceConfig config = const AudioUrlServiceConfig(),
  })  : _client = client,
        _config = config;

  final ApiClient _client;
  final AudioUrlServiceConfig _config;
  final Map<String, CachedSignedUrl> _urlCache = {};
  final Map<String, DateTime> _requestTimes = {};

  /// Returns a signed URL for an item in an authorized session queue.
  ///
  /// [itemId] may be the session-item ID, audio-asset ID, or confession ID.
  /// If omitted, the first playable queue item is returned.
  Future<String> getStreamUrl(String sessionId, {String? itemId}) async {
    if (sessionId.trim().isEmpty) {
      throw ArgumentError.value(sessionId, 'sessionId', 'must not be empty');
    }
    final cacheKey = '$sessionId:${itemId ?? '*'}';
    final cached = _urlCache[cacheKey];
    if (cached != null &&
        cached.isValid &&
        !cached.isAboutToExpire(_config.urlExpiryBuffer)) {
      _requestTimes[cacheKey] = DateTime.now();
      return cached.url;
    }

    final url = await _fetchStreamUrl(sessionId, itemId: itemId);
    _cacheUrl(cacheKey, url);
    return url;
  }

  Future<String> _fetchStreamUrl(String sessionId, {String? itemId}) async {
    final response = await _client.getSessionsByIdQueue(sessionId);
    final rawItems = response['items'];
    if (rawItems is! List) {
      throw StateError('The session queue did not contain audio items.');
    }

    final items = rawItems.whereType<Map>().map(
      (item) => Map<String, dynamic>.from(item),
    );
    Map<String, dynamic>? selected;
    if (itemId == null || itemId.isEmpty) {
      for (final item in items) {
        if (_audioUrl(item).isNotEmpty && item['locked'] != true) {
          selected = item;
          break;
        }
      }
    } else {
      for (final item in items) {
        if (item['id'] == itemId ||
            item['audio_asset_id'] == itemId ||
            item['confession_id'] == itemId) {
          selected = item;
          break;
        }
      }
    }

    if (selected == null) {
      throw StateError('No matching playable audio item was found in the session.');
    }
    if (selected['locked'] == true) {
      final reason = selected['lock_reason'];
      throw StateError(
        reason is String && reason.isNotEmpty
            ? reason
            : 'This audio requires an eligible subscription.',
      );
    }

    final url = _audioUrl(selected);
    final uri = Uri.tryParse(url);
    if (uri == null || (uri.scheme != 'https' && uri.scheme != 'http')) {
      throw StateError('The session item did not include a signed audio URL.');
    }
    return url;
  }

  String _audioUrl(Map<String, dynamic> item) {
    final value = item['audio_url'];
    return value is String ? value : '';
  }

  void _cacheUrl(String key, String url) {
    final now = DateTime.now();
    final signedExpiry = _signedExpiry(url);
    final configuredExpiry = now.add(_config.cacheDuration);
    final expiresAt = signedExpiry == null || configuredExpiry.isBefore(signedExpiry)
        ? configuredExpiry
        : signedExpiry;
    _urlCache[key] = CachedSignedUrl(url: url, expiresAt: expiresAt);
    _requestTimes[key] = now;
    _cleanupCache();
  }

  DateTime? _signedExpiry(String url) {
    final query = Uri.tryParse(url)?.queryParameters;
    if (query == null) return null;

    // Local and CloudFront signatures use an absolute Unix expiry.
    final unixExpiry = int.tryParse(query['expires'] ?? query['Expires'] ?? '');
    if (unixExpiry != null) {
      return DateTime.fromMillisecondsSinceEpoch(unixExpiry * 1000);
    }

    // S3 V4 presigned URLs encode a signing time and a relative TTL.
    final signedAt = query['X-Amz-Date'];
    final ttlSeconds = int.tryParse(query['X-Amz-Expires'] ?? '');
    if (signedAt != null &&
        signedAt.length >= 16 &&
        signedAt[8] == 'T' &&
        ttlSeconds != null) {
      final iso = '${signedAt.substring(0, 4)}-${signedAt.substring(4, 6)}-'
          '${signedAt.substring(6, 8)}T${signedAt.substring(9, 11)}:'
          '${signedAt.substring(11, 13)}:${signedAt.substring(13, 15)}Z';
      final start = DateTime.tryParse(iso);
      if (start != null) return start.add(Duration(seconds: ttlSeconds));
    }
    return null;
  }

  void _cleanupCache() {
    _urlCache.removeWhere((_, cached) => !cached.isValid);
    _requestTimes.removeWhere((key, _) => !_urlCache.containsKey(key));
    if (_urlCache.length <= _config.maxCachedUrls) return;

    final oldestFirst = _requestTimes.keys.toList()
      ..sort((a, b) => _requestTimes[a]!.compareTo(_requestTimes[b]!));
    final excess = _urlCache.length - _config.maxCachedUrls;
    for (final key in oldestFirst.take(excess)) {
      _urlCache.remove(key);
      _requestTimes.remove(key);
    }
  }

  void clearCache() {
    _urlCache.clear();
    _requestTimes.clear();
  }

  void clearCachedUrl(String sessionId, {String? itemId}) {
    if (itemId != null) {
      final key = '$sessionId:$itemId';
      _urlCache.remove(key);
      _requestTimes.remove(key);
      return;
    }
    final prefix = '$sessionId:';
    _urlCache.removeWhere((key, _) => key.startsWith(prefix));
    _requestTimes.removeWhere((key, _) => key.startsWith(prefix));
  }

  int get cacheSize => _urlCache.length;
}
