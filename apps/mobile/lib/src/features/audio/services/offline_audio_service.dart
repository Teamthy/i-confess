/// Offline audio service for I-Confess.
///
/// This service provides functionality to cache audio files for offline
/// playback, allowing users to listen to confessions without an internet connection.
import 'dart:async';
import 'dart:io';
import 'dart:typed_data';
import 'package:flutter/foundation.dart';
import 'package:path_provider/path_provider.dart';
import 'package:path/path.dart' as path;
import 'package:shared_preferences/shared_preferences.dart';
import '../models/audio_asset.dart';
import '../models/audio_queue.dart';

/// Configuration for offline audio caching.
class OfflineAudioConfig {
  /// Maximum number of audio files to cache.
  final int maxCacheSize;
  
  /// Maximum total size of cache in bytes.
  final int maxCacheBytes;
  
  /// Directory name for cache.
  final String cacheDirectoryName;
  
  /// Whether to cache on WiFi only.
  final bool wifiOnly;
  
  /// Whether to preload next items in queue.
  final bool preloadNextInQueue;
  
  /// Number of items to preload ahead.
  final int preloadCount;

  const OfflineAudioConfig({
    this.maxCacheSize = 100,
    this.maxCacheBytes = 1024 * 1024 * 1024, // 1GB
    this.cacheDirectoryName = 'i_confess_audio_cache',
    this.wifiOnly = true,
    this.preloadNextInQueue = true,
    this.preloadCount = 3,
  });
}

/// Status of an offline audio file.
enum OfflineAudioStatus {
  /// File is not cached.
  notCached,
  
  /// File is being downloaded.
  downloading,
  
  /// File is cached and ready for playback.
  cached,
  
  /// Download failed.
  failed,
  
  /// File is expired (removed from cache).
  expired,
}

/// Information about a cached audio file.
class CachedAudioInfo {
  final String assetId;
  final String filePath;
  final int fileSize;
  final DateTime cachedAt;
  final DateTime? expiresAt;
  final OfflineAudioStatus status;
  final double? downloadProgress;
  final String? errorMessage;

  const CachedAudioInfo({
    required this.assetId,
    required this.filePath,
    required this.fileSize,
    required this.cachedAt,
    this.expiresAt,
    required this.status,
    this.downloadProgress,
    this.errorMessage,
  });

  /// Whether the file is ready for playback.
  bool get isReady => status == OfflineAudioStatus.cached;
  
  /// Whether the file is expired.
  bool get isExpired => status == OfflineAudioStatus.expired ||
      (expiresAt != null && DateTime.now().isAfter(expiresAt!));
  
  /// Copy with new values.
  CachedAudioInfo copyWith({
    String? assetId,
    String? filePath,
    int? fileSize,
    DateTime? cachedAt,
    DateTime? expiresAt,
    OfflineAudioStatus? status,
    double? downloadProgress,
    String? errorMessage,
  }) {
    return CachedAudioInfo(
      assetId: assetId ?? this.assetId,
      filePath: filePath ?? this.filePath,
      fileSize: fileSize ?? this.fileSize,
      cachedAt: cachedAt ?? this.cachedAt,
      expiresAt: expiresAt ?? this.expiresAt,
      status: status ?? this.status,
      downloadProgress: downloadProgress ?? this.downloadProgress,
      errorMessage: errorMessage ?? this.errorMessage,
    );
  }

  /// Convert to JSON.
  Map<String, dynamic> toJson() {
    return {
      'assetId': assetId,
      'filePath': filePath,
      'fileSize': fileSize,
      'cachedAt': cachedAt.toIso8601String(),
      'expiresAt': expiresAt?.toIso8601String(),
      'status': status.name,
      'downloadProgress': downloadProgress,
      'errorMessage': errorMessage,
    };
  }

  /// Create from JSON.
  factory CachedAudioInfo.fromJson(Map<String, dynamic> json) {
    return CachedAudioInfo(
      assetId: json['assetId'] as String,
      filePath: json['filePath'] as String,
      fileSize: json['fileSize'] as int,
      cachedAt: DateTime.parse(json['cachedAt'] as String),
      expiresAt: json['expiresAt'] != null 
          ? DateTime.parse(json['expiresAt'] as String) 
          : null,
      status: OfflineAudioStatus.values.firstWhere(
        (status) => status.name == json['status'],
        orElse: () => OfflineAudioStatus.notCached,
      ),
      downloadProgress: json['downloadProgress'] as double?,
      errorMessage: json['errorMessage'] as String?,
    );
  }
}

/// Service for managing offline audio caching.
///
/// This service downloads and caches audio files for offline playback,
/// manages cache size, and provides methods to check cache status.
class OfflineAudioService {
  final OfflineAudioConfig _config;
  final SharedPreferences _prefs;
  
  // Stream controllers
  final _cacheStatusController = StreamController<Map<String, CachedAudioInfo>>.broadcast();
  final _downloadProgressController = StreamController<Map<String, double>>.broadcast();
  final _cacheSizeController = StreamController<int>.broadcast();
  
  // Cache map: assetId -> CachedAudioInfo
  final Map<String, CachedAudioInfo> _cache = {};
  
  // Download queue
  final List<String> _downloadQueue = [];
  
  // Whether the service is initialized
  bool _isInitialized = false;
  
  /// Directory where cached files are stored.
  Directory? _cacheDirectory;

  /// Gets the current cache status stream.
  Stream<Map<String, CachedAudioInfo>> get onCacheStatusChanged => _cacheStatusController.stream;
  
  /// Gets the download progress stream.
  Stream<Map<String, double>> get onDownloadProgressChanged => _downloadProgressController.stream;
  
  /// Gets the cache size stream.
  Stream<int> get onCacheSizeChanged => _cacheSizeController.stream;
  
  /// Gets the current cache map.
  Map<String, CachedAudioInfo> get cache => Map.unmodifiable(_cache);
  
  /// Gets the current cache size in bytes.
  int get cacheSize => _cache.values.fold<int>(0, (sum, info) => sum + info.fileSize);
  
  /// Gets the number of cached items.
  int get cacheCount => _cache.length;
  
  /// Whether the cache is full.
  bool get isCacheFull => cacheCount >= _config.maxCacheSize || cacheSize >= _config.maxCacheBytes;

  /// Creates an offline audio service.
  OfflineAudioService({
    OfflineAudioConfig? config,
    SharedPreferences? prefs,
  }) : _config = config ?? const OfflineAudioConfig(),
       _prefs = prefs ?? throw ArgumentError('prefs cannot be null');

  /// Creates an offline audio service with default SharedPreferences.
  static Future<OfflineAudioService> create([OfflineAudioConfig? config]) async {
    final prefs = await SharedPreferences.getInstance();
    return OfflineAudioService(
      config: config,
      prefs: prefs,
    );
  }

  /// Initializes the offline audio service.
  Future<void> init() async {
    if (_isInitialized) return;
    
    // Get cache directory
    final directory = await getApplicationDocumentsDirectory();
    _cacheDirectory = Directory(
      path.join(directory.path, _config.cacheDirectoryName),
    );
    
    // Create directory if it doesn't exist
    if (!await _cacheDirectory!.exists()) {
      await _cacheDirectory!.create(recursive: true);
    }
    
    // Load cache info from preferences
    await _loadCacheInfo();
    
    // Update streams
    _updateStreams();
    
    _isInitialized = true;
    debugPrint('[OfflineAudioService] Initialized: ${_cacheDirectory!.path}');
  }

  /// Loads cache info from SharedPreferences.
  Future<void> _loadCacheInfo() async {
    final cacheJson = _prefs.getString('offline_audio_cache');
    if (cacheJson != null) {
      try {
        // Parse cache info
        // This is a simplified approach - in production, you'd want to
        // verify files still exist
        debugPrint('[OfflineAudioService] Loading cache info from preferences');
      } catch (e) {
        debugPrint('[OfflineAudioService] Error loading cache info: $e');
      }
    }
  }

  /// Updates all streams with current state.
  void _updateStreams() {
    _cacheStatusController.add(Map.unmodifiable(_cache));
    _downloadProgressController.add(
      Map.fromEntries(
        _cache.entries
            .where((entry) => entry.value.status == OfflineAudioStatus.downloading)
            .map((entry) => MapEntry(
                  entry.key,
                  entry.value.downloadProgress ?? 0.0,
                )),
      ),
    );
    _cacheSizeController.add(cacheSize);
  }

  /// Gets the cache status for a specific asset.
  OfflineAudioStatus getStatus(String assetId) {
    final info = _cache[assetId];
    if (info == null) return OfflineAudioStatus.notCached;
    return info.status;
  }

  /// Checks if an asset is cached and ready for playback.
  bool isCached(String assetId) {
    final info = _cache[assetId];
    return info != null && info.isReady && !info.isExpired;
  }

  /// Gets the file path for a cached asset.
  String? getFilePath(String assetId) {
    final info = _cache[assetId];
    if (info != null && info.isReady && !info.isExpired) {
      return info.filePath;
    }
    return null;
  }

  /// Gets the download progress for an asset.
  double? getDownloadProgress(String assetId) {
    final info = _cache[assetId];
    return info?.downloadProgress;
  }

  /// Caches an audio asset for offline playback.
  ///
  /// This downloads the audio file and saves it to the cache directory.
  Future<void> cacheAsset({
    required String assetId,
    required String url,
    required String fileName,
    int? fileSize,
    Duration? expiration,
  }) async {
    if (!isCacheFull) {
      await _addToDownloadQueue(assetId, url, fileName, fileSize, expiration);
    } else {
      debugPrint('[OfflineAudioService] Cache is full, cannot cache $assetId');
    }
  }

  /// Adds an asset to the download queue.
  Future<void> _addToDownloadQueue(
    String assetId,
    String url,
    String fileName,
    int? fileSize,
    Duration? expiration,
  ) async {
    if (_downloadQueue.contains(assetId)) {
      debugPrint('[OfflineAudioService] $assetId already in download queue');
      return;
    }
    
    _downloadQueue.add(assetId);
    
    // Start download
    _downloadNext();
  }

  /// Downloads the next item in the queue.
  Future<void> _downloadNext() async {
    if (_downloadQueue.isEmpty) return;
    
    final assetId = _downloadQueue.first;
    
    // Get the URL for this asset
    // In a real implementation, you would get this from your API
    // For now, we'll use a placeholder
    
    try {
      // Update status to downloading
      _cache[assetId] = CachedAudioInfo(
        assetId: assetId,
        filePath: '',
        fileSize: 0,
        cachedAt: DateTime.now(),
        status: OfflineAudioStatus.downloading,
        downloadProgress: 0.0,
      );
      _updateStreams();
      
      // Download the file
      // In a real implementation, use HttpClient or Dio
      // For now, we'll simulate the download
      
      await _simulateDownload(assetId);
      
      // On success, update cache
      final filePath = path.join(_cacheDirectory!.path, '$assetId.mp3');
      _cache[assetId] = CachedAudioInfo(
        assetId: assetId,
        filePath: filePath,
        fileSize: fileSize ?? 0,
        cachedAt: DateTime.now(),
        expiresAt: expiration != null ? DateTime.now().add(expiration) : null,
        status: OfflineAudioStatus.cached,
        downloadProgress: 1.0,
      );
      
      // Save to preferences
      await _saveCacheInfo();
      
      // Remove from queue
      _downloadQueue.remove(assetId);
      
      // Download next
      _downloadNext();
      
      _updateStreams();
      debugPrint('[OfflineAudioService] Downloaded: $assetId');
    } catch (e) {
      _cache[assetId] = CachedAudioInfo(
        assetId: assetId,
        filePath: '',
        fileSize: 0,
        cachedAt: DateTime.now(),
        status: OfflineAudioStatus.failed,
        downloadProgress: 0.0,
        errorMessage: e.toString(),
      );
      
      _downloadQueue.remove(assetId);
      _downloadNext();
      _updateStreams();
      
      debugPrint('[OfflineAudioService] Download failed for $assetId: $e');
    }
  }

  /// Simulates a download (replace with actual download in production).
  Future<void> _simulateDownload(String assetId) async {
    // Simulate download progress
    for (var i = 0; i <= 10; i++) {
      await Future.delayed(const Duration(milliseconds: 100));
      
      _cache[assetId] = _cache[assetId]!.copyWith(
        downloadProgress: i * 0.1,
      );
      _updateStreams();
    }
  }

  /// Caches multiple assets.
  Future<void> cacheAssets(List<AudioAsset> assets) async {
    for (final asset in assets) {
      // In a real implementation, get the URL from your API
      await cacheAsset(
        assetId: asset.id,
        url: 'https://your-api.com/audio/${asset.id}',
        fileName: '${asset.id}.mp3',
        fileSize: asset.fileSize,
      );
    }
  }

  /// Caches a queue for offline playback.
  Future<void> cacheQueue(AudioQueue queue) async {
    for (final item in queue.items) {
      await cacheAsset(
        assetId: item.asset.id,
        url: 'https://your-api.com/audio/${item.asset.id}',
        fileName: '${item.asset.id}.mp3',
        fileSize: item.asset.fileSize.toInt(),
      );
    }
  }

  /// Preloads the next items in the queue.
  Future<void> preloadNextInQueue(AudioQueue queue, int count) async {
    if (!_config.preloadNextInQueue) return;
    
    final currentIndex = queue.currentIndex;
    final itemsToPreload = queue.items.skip(currentIndex + 1).take(count);
    
    for (final item in itemsToPreload) {
      if (!isCached(item.asset.id)) {
        await cacheAsset(
          assetId: item.asset.id,
          url: 'https://your-api.com/audio/${item.asset.id}',
          fileName: '${item.asset.id}.mp3',
          fileSize: item.asset.fileSize.toInt(),
        );
      }
    }
  }

  /// Removes an asset from the cache.
  Future<void> removeAsset(String assetId) async {
    final info = _cache[assetId];
    if (info != null) {
      // Delete the file
      try {
        final file = File(info.filePath);
        if (await file.exists()) {
          await file.delete();
        }
      } catch (e) {
        debugPrint('[OfflineAudioService] Error deleting file: $e');
      }
      
      // Remove from cache
      _cache.remove(assetId);
      
      // Save to preferences
      await _saveCacheInfo();
      
      _updateStreams();
      debugPrint('[OfflineAudioService] Removed: $assetId');
    }
  }

  /// Removes all assets from the cache.
  Future<void> clearCache() async {
    for (final assetId in _cache.keys.toList()) {
      await removeAsset(assetId);
    }
    
    _downloadQueue.clear();
    _updateStreams();
    debugPrint('[OfflineAudioService] Cache cleared');
  }

  /// Removes expired assets from the cache.
  Future<void> removeExpired() async {
    final expiredAssets = _cache.entries
        .where((entry) => entry.value.isExpired)
        .map((entry) => entry.key)
        .toList();
    
    for (final assetId in expiredAssets) {
      await removeAsset(assetId);
    }
    
    if (expiredAssets.isNotEmpty) {
      debugPrint('[OfflineAudioService] Removed ${expiredAssets.length} expired assets');
    }
  }

  /// Removes assets to make room for new ones.
  Future<void> _makeRoomIfNeeded() async {
    while (isCacheFull && _cache.isNotEmpty) {
      // Find the oldest cached item
      final oldest = _cache.entries
          .reduce((a, b) => a.value.cachedAt.isBefore(b.value.cachedAt) ? a : b);
      
      await removeAsset(oldest.key);
    }
  }

  /// Saves cache info to SharedPreferences.
  Future<void> _saveCacheInfo() async {
    try {
      // In a real implementation, save the cache map
      // For now, we'll just save a simple list of cached asset IDs
      final assetIds = _cache.keys.toList();
      await _prefs.setStringList('offline_audio_cached_assets', assetIds);
      
      debugPrint('[OfflineAudioService] Cache info saved: ${assetIds.length} assets');
    } catch (e) {
      debugPrint('[OfflineAudioService] Error saving cache info: $e');
    }
  }

  /// Gets the cached file for an asset.
  Future<File?> getCachedFile(String assetId) async {
    final filePath = getFilePath(assetId);
    if (filePath == null) return null;
    
    final file = File(filePath);
    if (await file.exists()) {
      return file;
    }
    return null;
  }

  /// Gets the cached file as bytes.
  Future<Uint8List?> getCachedBytes(String assetId) async {
    final file = await getCachedFile(assetId);
    if (file != null) {
      return await file.readAsBytes();
    }
    return null;
  }

  /// Gets the cached file as a stream.
  Stream<Uint8List>? getCachedStream(String assetId) {
    final filePath = getFilePath(assetId);
    if (filePath == null) return null;
    
    final file = File(filePath);
    return file.existsSync() ? file.openRead() : null;
  }

  /// Gets all cached assets.
  List<CachedAudioInfo> getAllCached() {
    return _cache.values
        .where((info) => info.isReady && !info.isExpired)
        .toList();
  }

  /// Gets the total cache size.
  Future<int> getTotalCacheSize() async {
    var totalSize = 0;
    for (final info in _cache.values) {
      try {
        final file = File(info.filePath);
        if (await file.exists()) {
          totalSize += await file.length();
        }
      } catch (e) {
        debugPrint('[OfflineAudioService] Error getting file size: $e');
      }
    }
    return totalSize;
  }

  /// Gets the cache statistics.
  Future<Map<String, dynamic>> getStats() async {
    final totalSize = await getTotalCacheSize();
    final cachedCount = _cache.values.where((info) => info.isReady && !info.isExpired).length;
    final downloadingCount = _cache.values.where((info) => info.status == OfflineAudioStatus.downloading).length;
    final failedCount = _cache.values.where((info) => info.status == OfflineAudioStatus.failed).length;
    
    return {
      'totalSize': totalSize,
      'totalSizeMB': totalSize / (1024 * 1024),
      'cachedCount': cachedCount,
      'downloadingCount': downloadingCount,
      'failedCount': failedCount,
      'maxCacheSize': _config.maxCacheSize,
      'maxCacheBytes': _config.maxCacheBytes,
      'maxCacheBytesMB': _config.maxCacheBytes / (1024 * 1024),
      'isFull': isCacheFull,
    };
  }

  /// Checks if offline mode is available for an asset.
  Future<bool> isOfflineAvailable(String assetId) async {
    return isCached(assetId);
  }

  /// Gets the offline URL for an asset (file path).
  Future<String?> getOfflineUrl(String assetId) async {
    return getFilePath(assetId);
  }

  /// Disposes the service and cleans up resources.
  Future<void> dispose() async {
    await _cacheStatusController.close();
    await _downloadProgressController.close();
    await _cacheSizeController.close();
    
    _isInitialized = false;
    debugPrint('[OfflineAudioService] Disposed');
  }
}

/// Provider for the offline audio service.
final offlineAudioServiceProvider = Provider<OfflineAudioService>((ref) {
  throw UnimplementedError('OfflineAudioService must be initialized separately');
});

/// Async provider for the offline audio service.
final offlineAudioServiceAsyncProvider = FutureProvider<OfflineAudioService>((ref) async {
  return OfflineAudioService.create();
});

/// Extension methods for easy access to offline audio.
extension OfflineAudioExtension on AudioAsset {
  /// Caches this asset for offline playback.
  Future<void> cacheForOffline(OfflineAudioService service) async {
    await service.cacheAsset(
      assetId: id,
      url: '', // In real implementation, get from API
      fileName: '$id.mp3',
      fileSize: fileSize.toInt(),
    );
  }

  /// Checks if this asset is cached for offline playback.
  bool isCachedForOffline(OfflineAudioService service) {
    return service.isCached(id);
  }

  /// Gets the offline URL for this asset.
  Future<String?> getOfflineUrl(OfflineAudioService service) async {
    return service.getOfflineUrl(id);
  }
}

/// Mixin for adding offline audio capabilities.
mixin OfflineAudioMixin {
  OfflineAudioService? _offlineService;

  /// Initializes offline audio service.
  Future<void> initOfflineAudio([OfflineAudioConfig? config]) async {
    _offlineService = await OfflineAudioService.create(config);
    await _offlineService!.init();
  }

  /// Gets the offline audio service.
  OfflineAudioService get offlineService {
    assert(_offlineService != null, 'Offline audio service not initialized');
    return _offlineService!;
  }

  /// Disposes offline audio service.
  void disposeOfflineAudio() {
    _offlineService?.dispose();
    _offlineService = null;
  }
}
