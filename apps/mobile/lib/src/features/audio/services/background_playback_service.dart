/// Background playback service for I-Confess.
///
/// This service enables audio to continue playing when the app is in the background,
/// handles notification controls, and manages audio session state.
import 'dart:async';
import 'dart:ui' as ui;
import 'package:audio_session/audio_session.dart';
import 'package:flutter/material.dart';
import 'package:just_audio/just_audio.dart';
import 'package:just_audio_background/just_audio_background.dart';
import '../models/audio_asset.dart';

/// Configuration for background playback.
class BackgroundPlaybackConfig {
  /// Whether background playback is enabled.
  final bool enabled;
  
  /// Whether to show notification when playing in background.
  final bool showNotification;
  
  /// Title to display in notification.
  final String notificationTitle;
  
  /// Subtitle/artist to display in notification.
  final String? notificationSubtitle;
  
  /// Image URL for notification.
  final String? notificationImageUrl;
  
  /// Whether to show play/pause buttons in notification.
  final bool showPlayPauseControls;
  
  /// Whether to show skip buttons in notification.
  final bool showSkipControls;
  
  /// Whether to show stop button in notification.
  final bool showStopControl;
  
  const BackgroundPlaybackConfig({
    this.enabled = true,
    this.showNotification = true,
    this.notificationTitle = 'I-Confess',
    this.notificationSubtitle,
    this.notificationImageUrl,
    this.showPlayPauseControls = true,
    this.showSkipControls = false,
    this.showStopControl = true,
  });

/// Service for managing background audio playback.
///
/// This service works with the AudioPlaybackService to enable background playback,
/// notification controls, and proper audio session management.
class BackgroundPlaybackService {
  final AudioPlayer _player;
  final BackgroundPlaybackConfig _config;
  
  // Stream controllers for background state
  final _isInBackgroundController = StreamController<bool>.broadcast();
  final _notificationStateController = StreamController<Map<String, dynamic>>.broadcast();
  
  bool _isInitialized = false;
  bool _wasPlayingBeforeBackground = false;
  
  /// Whether the app is currently in the background.
  bool get isInBackground => _isInBackgroundController.valueOrNull ?? false;
  
  /// Stream of background state changes.
  Stream<bool> get onBackgroundStateChanged => _isInBackgroundController.stream;
  
  /// Stream of notification state updates.
  Stream<Map<String, dynamic>> get onNotificationStateChanged => 
      _notificationStateController.stream;

  /// Creates a background playback service.
  BackgroundPlaybackService({
    required AudioPlayer player,
    BackgroundPlaybackConfig? config,
  }) : _player = player,
       _config = config ?? const BackgroundPlaybackConfig();

  /// Initializes the background playback service.
  ///
  /// This should be called during app initialization.
  Future<void> init() async {
    if (_isInitialized) return;
    
    try {
      // Initialize audio session
      await _initAudioSession();
      
      // Initialize background player
      await _initBackgroundPlayer();
      
      // Set up background state listener
      _setupBackgroundStateListener();
      
      // Set up player event handlers
      _setupPlayerHandlers();
      
      _isInitialized = true;
      debugPrint('[BackgroundPlaybackService] Initialized successfully');
    } catch (e) {
      debugPrint('[BackgroundPlaybackService] Initialization error: $e');
      rethrow;
    }
  }

  /// Initializes the audio session for background playback.
  Future<void> _initAudioSession() async {
    final session = await AudioSession.instance;
    
    await session.configure(
      AudioSessionConfiguration(
        avAudioSessionCategory: AVAudioSessionCategory.playback,
        avAudioSessionCategoryOptions: AVAudioSessionCategoryOptions(
          allowBluetooth: true,
          allowBluetoothA2DP: true,
          allowAirPlay: true,
        ),
        avAudioSessionMode: AVAudioSessionMode.defaultMode,
        avAudioSessionRouteSharingPolicy: AVAudioSessionRouteSharingPolicy.defaultPolicy,
        avAudioSessionSetActiveOptions: AVAudioSessionSetActiveOptions.none,
        androidAudioAttributes: const AndroidAudioAttributes(
          contentType: AndroidAudioContentType.speech,
          flags: AndroidAudioFlags.none,
          usage: AndroidAudioUsage.media,
        ),
        androidAudioFocusGainType: AndroidAudioFocusGainType.gain,
        androidWillPauseWhenDucked: false,
      ),
    );
    
    // Set session active
    await session.setActive(true);
    
    debugPrint('[BackgroundPlaybackService] Audio session configured');
  }

  /// Initializes the background player.
  Future<void> _initBackgroundPlayer() async {
    // Configure background audio
    await JustAudioBackground.init(
      androidNotificationChannelId: 'i_confess_audio',
      androidNotificationChannelName: 'I-Confess Audio',
      androidNotificationOngoing: true,
      androidNotificationIcon: 'mipmap/ic_launcher',
      androidShowNotification: _config.showNotification,
      notificationColor: Colors.blue.value,
    );
    
    debugPrint('[BackgroundPlaybackService] Background player initialized');
  }

  /// Sets up the background state listener.
  void _setupBackgroundStateListener() {
    // Use WidgetsBinding to detect app lifecycle changes
    WidgetsBinding.instance.addObserver(
      _AppLifecycleObserver(
        onResume: () {
          _handleAppResumed();
        },
        onPause: () {
          _handleAppPaused();
        },
        onInactive: () {
          _handleAppInactive();
        },
        onDetach: () {
          _handleAppDetached();
        },
      ),
    );
  }

  /// Sets up player event handlers for background playback.
  void _setupPlayerHandlers() {
    _player.playbackEventStream.listen((event) {
      _handlePlaybackEvent(event);
    });
  }

  /// Handles app being resumed (coming to foreground).
  void _handleAppResumed() {
    _isInBackgroundController.add(false);
    debugPrint('[BackgroundPlaybackService] App resumed');
    
    // If we were playing before going to background, we might need to resume
    // Note: With proper background setup, audio should continue automatically
  }

  /// Handles app being paused (going to background).
  void _handleAppPaused() {
    _wasPlayingBeforeBackground = _player.playing;
    _isInBackgroundController.add(true);
    debugPrint('[BackgroundPlaybackService] App paused, wasPlaying: $_wasPlayingBeforeBackground');
  }

  /// Handles app becoming inactive.
  void _handleAppInactive() {
    debugPrint('[BackgroundPlaybackService] App inactive');
  }

  /// Handles app being detached.
  void _handleAppDetached() {
    debugPrint('[BackgroundPlaybackService] App detached');
  }

  /// Handles playback events.
  void _handlePlaybackEvent(PlaybackEvent event) {
    // Update notification state based on playback events
    final state = {
      'playing': _player.playing,
      'position': _player.position.inMilliseconds,
      'duration': _player.duration?.inMilliseconds,
      'currentAssetId': _getCurrentAssetId(),
    };
    
    _notificationStateController.add(state);
    
    // Update notification
    _updateNotification();
  }

  /// Updates the notification with current playback state.
  Future<void> _updateNotification() async {
    if (!_config.showNotification) return;
    
    final currentAssetId = _getCurrentAssetId();
    final isPlaying = _player.playing;
    final position = _player.position;
    final duration = _player.duration ?? Duration.zero;
    
    // Get asset info if available
    final title = _config.notificationTitle;
    final subtitle = _config.notificationSubtitle ?? currentAssetId;
    
    // Calculate progress
    final progress = duration.inMilliseconds > 0 
        ? (position.inMilliseconds / duration.inMilliseconds) * 100 
        : 0;
    
    await JustAudioBackground.setQueue([
      MediaItem(
        id: currentAssetId ?? 'unknown',
        title: title,
        artist: subtitle,
        artUri: _config.notificationImageUrl != null 
            ? Uri.parse(_config.notificationImageUrl!) 
            : null,
      )
    ]);
    
    await JustAudioBackground.setMediaItem(
      MediaItem(
        id: currentAssetId ?? 'unknown',
        title: title,
        artist: subtitle,
        artUri: _config.notificationImageUrl != null 
            ? Uri.parse(_config.notificationImageUrl!) 
            : null,
      ),
    );
    
    await JustAudioBackground.setQueueIndex(0);
    
    // Update playback state
    await JustAudioBackground.setState(
      isPlaying: isPlaying,
      position: position,
      bufferedPosition: position,
      speed: _player.speed,
    );
    
    debugPrint('[BackgroundPlaybackService] Notification updated: $title, playing: $isPlaying');
  }

  /// Gets the current asset ID from the player's current source.
  String? _getCurrentAssetId() {
    // This would be enhanced to track the current asset
    // For now, we'll use a simple approach
    return 'confession_audio';
  }

  /// Enables background playback.
  Future<void> enable() async {
    if (!_config.enabled) return;
    
    await _initAudioSession();
    await _updateNotification();
    
    debugPrint('[BackgroundPlaybackService] Background playback enabled');
  }

  /// Disables background playback.
  Future<void> disable() async {
    final session = await AudioSession.instance;
    await session.setActive(false);
    
    // Clear notification
    await JustAudioBackground.clearMediaItem();
    await JustAudioBackground.clearQueue();
    
    debugPrint('[BackgroundPlaybackService] Background playback disabled');
  }

  /// Updates the notification with new asset information.
  Future<void> updateAssetInfo({
    String? assetId,
    String? title,
    String? subtitle,
    String? imageUrl,
  }) async {
    if (!_config.showNotification) return;
    
    // Update config for this asset
    // Note: In a real implementation, we'd track this per-asset
    
    await _updateNotification();
    
    debugPrint('[BackgroundPlaybackService] Asset info updated: $title');
  }

  /// Updates the notification title.
  Future<void> updateNotificationTitle(String title) async {
    if (!_config.showNotification) return;
    
    // Create new config with updated title
    // This is a simplified approach
    
    await _updateNotification();
  }

  /// Handles a play request from notification.
  Future<void> handleNotificationPlay() async {
    debugPrint('[BackgroundPlaybackService] Notification play requested');
    await _player.play();
    await _updateNotification();
  }

  /// Handles a pause request from notification.
  Future<void> handleNotificationPause() async {
    debugPrint('[BackgroundPlaybackService] Notification pause requested');
    await _player.pause();
    await _updateNotification();
  }

  /// Handles a skip next request from notification.
  Future<void> handleNotificationSkipNext() async {
    debugPrint('[BackgroundPlaybackService] Notification skip next requested');
    // This would integrate with queue management in Phase 3
  }

  /// Handles a skip previous request from notification.
  Future<void> handleNotificationSkipPrevious() async {
    debugPrint('[BackgroundPlaybackService] Notification skip previous requested');
    // This would integrate with queue management in Phase 3
  }

  /// Handles a stop request from notification.
  Future<void> handleNotificationStop() async {
    debugPrint('[BackgroundPlaybackService] Notification stop requested');
    await _player.stop();
    await _updateNotification();
  }

  /// Disposes the background playback service.
  Future<void> dispose() async {
    await _isInBackgroundController.close();
    await _notificationStateController.close();
    
    final session = await AudioSession.instance;
    await session.setActive(false);
    
    await JustAudioBackground.clearMediaItem();
    await JustAudioBackground.clearQueue();
    
    _isInitialized = false;
    debugPrint('[BackgroundPlaybackService] Disposed');
  }
}

/// Observer for app lifecycle changes.
class _AppLifecycleObserver extends WidgetsBindingObserver {
  final VoidCallback? onResume;
  final VoidCallback? onPause;
  final VoidCallback? onInactive;
  final VoidCallback? onDetach;

  _AppLifecycleObserver({
    this.onResume,
    this.onPause,
    this.onInactive,
    this.onDetach,
  });

  @override
  void didChangeAppLifecycleState(AppLifecycleState state) {
    switch (state) {
      case AppLifecycleState.resumed:
        onResume?.call();
        break;
      case AppLifecycleState.paused:
        onPause?.call();
        break;
      case AppLifecycleState.inactive:
        onInactive?.call();
        break;
      case AppLifecycleState.detached:
        onDetach?.call();
        break;
      case AppLifecycleState.hidden:
        // Android only - app is hidden but not paused
        break;
    }
  }
}
