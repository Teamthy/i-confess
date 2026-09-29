/// Audio URL service for managing signed audio URLs.
///
/// This service handles fetching, caching, and refreshing signed URLs
/// from the backend for audio playback and downloads.
import 'dart:async';
import 'package:iconfess_api/iconfess_api.dart';

/// Configuration for the audio URL service.
class AudioUrlServiceConfig {
  final Duration urlExpiryBuffer;
  final Duration cacheDuration;
  final int maxCachedUrls;

  const AudioUrlServiceConfig({
    this.urlExpiryBuffer = const Duration(minutes: 5),
    this.cacheDuration = const Duration(hours: 1),
    this.maxCachedUrls = 50,
  });
}

/// Cached signed URL with expiration.
class CachedSignedUrl {
  final String url;
  final DateTime expiresAt;
  final bool isStreamUrl;

  CachedSignedUrl({
    required this.url,
    required this.expiresAt,
    this.isStreamUrl = true,
  });

  /// Check if the URL is still valid.
  bool get isValid => DateTime.now().isBefore(expiresAt);

  /// Check if the URL is about to expire.
  bool isAboutToExpire(Duration buffer) {
    return DateTime.now().add(buffer).isAfter(expiresAt);
  }
}

/// Audio URL service.
class AudioUrlService {
  final ApiClient _client;
  final AudioUrlServiceConfig _config;
  final Map<String, CachedSignedUrl> _urlCache = {};
  final Map<String, DateTime> _requestTimes = {};

  /// Creates an audio URL service.
  AudioUrlService({
    ApiClient? client,
    AudioUrlServiceConfig? config,
  })  : _client = client ?? ApiClient(),
        _config = config ?? const AudioUrlServiceConfig();

  /// Gets a signed streaming URL for an audio asset.
  ///
  /// Returns a cached URL if available and valid, otherwise fetches a new one.
  Future<String> getStreamUrl(String assetId) async {
    // Check cache first
    final cached = _urlCache[assetId];
    if (cached != null && cached.isValid) {
      return cached.url;
    }

    // Fetch new URL
    final url = await _fetchStreamUrl(assetId);
    
    // Cache it
    _cacheUrl(assetId, url, true);
    
    return url;
  }

  /// Gets a signed download URL for an audio asset.
  Future<String> getDownloadUrl(String assetId) async {
    // Check cache first
    final cached = _urlCache['${assetId}_download'];
    if (cached != null && cached.isValid) {
      return cached.url;
    }

    // Fetch new URL
    final url = await _fetchDownloadUrl(assetId);
    
    // Cache it with a different key
    _cacheUrl('${assetId}_download', url, false);
    
    return url;
  }

  /// Fetches a signed streaming URL from the backend.
  Future<String> _fetchStreamUrl(String assetId) async {
    try {
      // Note: The actual endpoint might be different based on the backend API
      // For now, we'll use a placeholder approach
      // In a real implementation, this would call the backend API
      
      // For Phase 1, we assume the backend provides signed URLs
      // through the session or audio endpoints
      final response = await _client.getSessionsById(assetId);
      
      // Extract the signed URL from the response
      // This is a placeholder - actual implementation depends on backend API
      final items = response['items'] as List<dynamic>? ?? [];
      if (items.isNotEmpty) {
        final firstItem = items.first as Map<String, dynamic>;
        final audioUrl = firstItem['audio_url'] as String?;
        if (audioUrl != null && audioUrl.isNotEmpty) {
          return audioUrl;
        }
      }
      
      // Fallback: construct a URL directly
      // In a real implementation, this would call a dedicated endpoint
      throw Exception('Failed to get stream URL: Backend API not yet updated');
    } catch (e) {
      throw Exception('Failed to fetch stream URL: $e');
    }
  }

  /// Fetches a signed download URL from the backend.
  Future<String> _fetchDownloadUrl(String assetId) async {
    try {
      // Similar to stream URL, but for downloads
      // In a real implementation, this would call a dedicated download endpoint
      
      // For now, we'll use the same approach as streaming
      // but with a longer expiry
      final response = await _client.getSessionsById(assetId);
      
      final items = response['items'] as List<dynamic>? ?? [];
      if (items.isNotEmpty) {
        final firstItem = items.first as Map<String, dynamic>;
        // Look for a download URL or use the stream URL
        final downloadUrl = firstItem['download_url'] as String? ?
            firstItem['audio_url'] as String?;
        if (downloadUrl != null && downloadUrl.isNotEmpty) {
          return downloadUrl;
        }
      }
      
      throw Exception('Failed to get download URL: Backend API not yet updated');
    } catch (e) {
      throw Exception('Failed to fetch download URL: $e');
    }
  }

  /// Caches a signed URL.
  void _cacheUrl(String key, String url, bool isStreamUrl) {
    // Parse the expiry from the URL if possible
    // For now, we'll use a default expiry
    final expiresAt = DateTime.now().add(_config.cacheDuration);
    
    _urlCache[key] = CachedSignedUrl(
      url: url,
      expiresAt: expiresAt,
      isStreamUrl: isStreamUrl,
    );
    
    _requestTimes[key] = DateTime.now();
    
    // Clean up old cache entries if we exceed the limit
    if (_urlCache.length > _config.maxCachedUrls) {
      _cleanupCache();
    }
  }

  /// Cleans up expired and least recently used cache entries.
  void _cleanupCache() {
    final now = DateTime.now();
    
    // Remove expired entries
    _urlCache.removeWhere((key, cached) => !cached.isValid);
    _requestTimes.removeWhere((key, time) => !_urlCache.containsKey(key));
    
    // If still over the limit, remove oldest entries
    if (_urlCache.length > _config.maxCachedUrls) {
      final sortedKeys = _requestTimes.keys.toList()
        ..sort((a, b) => _requestTimes[a]!.compareTo(_requestTimes[b]!));
      
      final keysToRemove = sortedKeys.take(
        _urlCache.length - _config.maxCachedUrls,
      );
      
      for (final key in keysToRemove) {
        _urlCache.remove(key);
        _requestTimes.remove(key);
      }
    }
  }

  /// Clears all cached URLs.
  void clearCache() {
    _urlCache.clear();
    _requestTimes.clear();
  }

  /// Clears a specific cached URL.
  void clearCachedUrl(String assetId) {
    _urlCache.remove(assetId);
    _urlCache.remove('${assetId}_download');
    _requestTimes.remove(assetId);
    _requestTimes.remove('${assetId}_download');
  }

  /// Gets the number of cached URLs.
  int get cacheSize => _urlCache.length;
}
