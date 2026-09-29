/// Main entry point for I-Confess with Audio Platform Phase 3 integration.
///
/// This file demonstrates how to integrate Phase 3 features (Background Playback
/// and Queue Management) into the I-Confess application.
///
/// To use this integration:
/// 1. Copy the relevant parts to your main.dart
/// 2. Or rename this file to main.dart
/// 3. Ensure all dependencies are in pubspec.yaml
/// 4. Configure Android and iOS as described in PHASE3-GUIDE.md
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:just_audio/just_audio.dart';
import 'package:just_audio_background/just_audio_background.dart';
import 'package:audio_session/audio_session.dart';

// Import the audio feature
import 'src/features/audio/audio.dart';
import 'src/core/di/providers.dart';
import 'src/core/routing/router.dart';
import 'src/core/theme/theme.dart';
import 'app.dart';

/// Global audio player instance for background playback.
/// This should be a singleton that persists across the app lifecycle.
final GlobalKey<NavigatorState> navigatorKey = GlobalKey<NavigatorState>();

// Audio services (will be initialized in main)
AudioPlayer? _globalAudioPlayer;
BackgroundPlaybackService? _backgroundPlaybackService;

/// Get the global audio player.
AudioPlayer get globalAudioPlayer {
  assert(_globalAudioPlayer != null, 'Audio player not initialized. Call initAudioServices() first.');
  return _globalAudioPlayer!;
}

/// Get the background playback service.
BackgroundPlaybackService get backgroundPlaybackService {
  assert(_backgroundPlaybackService != null, 'Background service not initialized. Call initAudioServices() first.');
  return _backgroundPlaybackService!;
}

/// Initialize audio services for Phase 3.
///
/// This should be called during app initialization before runApp().
Future<void> initAudioServices() async {
  try {
    // Initialize audio player
    _globalAudioPlayer = AudioPlayer();
    
    // Initialize background playback service
    _backgroundPlaybackService = BackgroundPlaybackService(
      player: _globalAudioPlayer!,
      config: const BackgroundPlaybackConfig(
        enabled: true,
        showNotification: true,
        notificationTitle: 'I-Confess',
        notificationSubtitle: 'Audio Confession',
        // Uncomment and set your app icon URL
        // notificationImageUrl: 'https://yourdomain.com/icon.png',
        showPlayPauseControls: true,
        showSkipControls: true,
        showStopControl: true,
      ),
    );
    
    // Initialize the service
    await _backgroundPlaybackService!.init();
    
    debugPrint('[AudioServices] Initialized successfully');
  } catch (e, stack) {
    debugPrint('[AudioServices] Initialization error: $e');
    debugPrint('[AudioServices] Stack: $stack');
    // Continue without audio services in debug mode
    // In production, you might want to show an error
  }
}

/// Clean up audio services.
///
/// Call this when the app is terminating.
Future<void> cleanupAudioServices() async {
  try {
    await _backgroundPlaybackService?.dispose();
    await _globalAudioPlayer?.dispose();
    _globalAudioPlayer = null;
    _backgroundPlaybackService = null;
    debugPrint('[AudioServices] Cleaned up successfully');
  } catch (e) {
    debugPrint('[AudioServices] Cleanup error: $e');
  }
}

/// Main function with Phase 3 integration.
Future<void> main() async {
  // Ensure Flutter binding is initialized
  WidgetsFlutterBinding.ensureInitialized();
  
  // Initialize audio services for Phase 3
  await initAudioServices();
  
  // Set up error handling
  FlutterError.onError = (details) {
    FlutterError.presentError(details);
    debugPrint('[FlutterError] ${details.exception}');
  };
  
  // Set preferred orientations
  await SystemChrome.setPreferredOrientations([
    DeviceOrientation.portraitUp,
    DeviceOrientation.portraitDown,
  ]);
  
  // Set system UI overlay style
  SystemChrome.setSystemUIOverlayStyle(
    const SystemUiOverlayStyle(
      statusBarColor: Colors.transparent,
      statusBarIconBrightness: Brightness.dark,
      systemNavigationBarColor: Colors.white,
      systemNavigationBarIconBrightness: Brightness.dark,
    ),
  );
  
  // Run the app
  runApp(
    const ProviderScope(
      child: AudioPlatformApp(),
    ),
  );
}

/// Main app widget with audio platform integration.
class AudioPlatformApp extends ConsumerWidget {
  const AudioPlatformApp({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    // Listen to background state changes
    final backgroundService = _backgroundPlaybackService;
    if (backgroundService != null) {
      backgroundService.onBackgroundStateChanged.listen((isInBackground) {
        debugPrint('[AudioPlatform] App in background: $isInBackground');
      });
    }
    
    return MaterialApp(
      title: 'I-Confess',
      debugShowCheckedModeBanner: false,
      theme: AppTheme.light,
      darkTheme: AppTheme.dark,
      themeMode: ThemeMode.system,
      navigatorKey: navigatorKey,
      onGenerateRoute: AppRouter.onGenerateRoute,
      initialRoute: AppRouter.splash,
      builder: (context, child) {
        // Wrap with audio player controller
        return AudioPlayerWrapper(
          child: child!,
        );
      },
    );
  }
}

/// Wrapper widget that provides audio player functionality to the app.
class AudioPlayerWrapper extends ConsumerWidget {
  final Widget child;

  const AudioPlayerWrapper({super.key, required this.child});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    // Get the audio player controller
    final controller = ref.watch(audioPlayerControllerProvider.notifier);
    final queueController = ref.watch(queueControllerProvider.notifier);
    
    // Set up auto-advance when playback completes
    final player = controller._playbackService;
    
    // Listen for playback completion
    player.playbackEventStream.listen((event) {
      if (event.processingState == ProcessingState.completed) {
        debugPrint('[AudioPlayerWrapper] Playback completed, advancing to next');
        queueController.playNext();
      }
    });
    
    // Update notification when queue or playback state changes
    final playerState = ref.watch(audioPlayerControllerProvider);
    final queue = ref.watch(currentQueueProvider);
    
    // When current item changes, update notification
    if (queue.currentItem != null && backgroundPlaybackService.isInBackground) {
      backgroundPlaybackService.updateAssetInfo(
        assetId: queue.currentItem!.asset.id,
        title: queue.currentItem!.title,
        subtitle: queue.currentItem!.subtitle,
      );
    }
    
    return child;
  }
}

/// Helper class for app lifecycle management with audio.
class AppLifecycleObserver extends WidgetsBindingObserver {
  final Function(bool) onBackgroundChanged;

  AppLifecycleObserver({required this.onBackgroundChanged});

  @override
  void didChangeAppLifecycleState(AppLifecycleState state) {
    final isInBackground = state == AppLifecycleState.paused ||
                         state == AppLifecycleState.inactive ||
                         state == AppLifecycleState.detached;
    
    onBackgroundChanged(isInBackground);
    
    switch (state) {
      case AppLifecycleState.resumed:
        debugPrint('[AppLifecycle] App resumed');
        break;
      case AppLifecycleState.paused:
        debugPrint('[AppLifecycle] App paused');
        break;
      case AppLifecycleState.inactive:
        debugPrint('[AppLifecycle] App inactive');
        break;
      case AppLifecycleState.detached:
        debugPrint('[AppLifecycle] App detached');
        break;
      case AppLifecycleState.hidden:
        debugPrint('[AppLifecycle] App hidden');
        break;
    }
  }
}

/// Extension for easy access to audio services from anywhere in the app.
class AudioServices {
  /// Get the global audio player controller.
  static AudioPlayerNotifier get playerController {
    // This assumes the widget tree has a ProviderScope
    // In a real app, you might want to use a service locator
    // or pass the controller explicitly
    throw UnimplementedError('Use ref.read(audioPlayerControllerProvider.notifier) instead');
  }
  
  /// Get the global queue controller.
  static QueueNotifier get queueController {
    throw UnimplementedError('Use ref.read(queueControllerProvider.notifier) instead');
  }
  
  /// Get the global background playback service.
  static BackgroundPlaybackService get backgroundService {
    return backgroundPlaybackService;
  }
}

/// Utility functions for audio operations.
class AudioUtils {
  /// Add a confession to the queue and optionally play it.
  static Future<void> addAndPlayConfession({
    required String confessionId,
    String? voiceId,
    bool playNow = false,
  }) async {
    final queueController = AudioServices.queueController;
    
    if (playNow) {
      // Clear queue and play this confession
      queueController.clear();
      queueController.addConfession(
        confessionId: confessionId,
        voiceId: voiceId,
      );
    } else {
      // Add to queue
      queueController.addConfession(
        confessionId: confessionId,
        voiceId: voiceId,
        playNext: false,
      );
    }
  }
  
  /// Play a confession immediately.
  static Future<void> playConfession({
    required String confessionId,
    String? voiceId,
  }) async {
    final queueController = AudioServices.queueController;
    final playerController = AudioServices.playerController;
    
    // Clear queue and add this confession
    queueController.clear();
    queueController.addConfession(
      confessionId: confessionId,
      voiceId: voiceId,
    );
    
    // Play the first item
    if (queueController.currentItem != null) {
      await playerController.playAsset(
        assetId: queueController.currentItem!.asset.id,
        confessionId: confessionId,
        voiceId: voiceId,
      );
    }
  }
  
  /// Toggle play/pause.
  static Future<void> togglePlayPause() async {
    final playerController = AudioServices.playerController;
    final playerState = playerController.state;
    
    if (playerState.playerState == PlayerState.playing) {
      await playerController.pause();
    } else {
      await playerController.resume();
    }
  }
  
  /// Skip to next item.
  static Future<void> skipNext() async {
    final queueController = AudioServices.queueController;
    await queueController.playNext();
  }
  
  /// Skip to previous item.
  static Future<void> skipPrevious() async {
    final queueController = AudioServices.queueController;
    queueController.playPrevious();
  }
}

/// Notification handler for background audio controls.
///
/// This class handles playback control events from the notification.
class NotificationHandler {
  /// Initialize notification handlers.
  static void init() {
    // Set up handlers for notification controls
    JustAudioBackground.setOnPlay((_) {
      debugPrint('[NotificationHandler] Play requested');
      _handlePlay();
    });
    
    JustAudioBackground.setOnPause((_) {
      debugPrint('[NotificationHandler] Pause requested');
      _handlePause();
    });
    
    JustAudioBackground.setOnSkipNext((_) {
      debugPrint('[NotificationHandler] Skip next requested');
      _handleSkipNext();
    });
    
    JustAudioBackground.setOnSkipPrevious((_) {
      debugPrint('[NotificationHandler] Skip previous requested');
      _handleSkipPrevious();
    });
    
    JustAudioBackground.setOnStop((_) {
      debugPrint('[NotificationHandler] Stop requested');
      _handleStop();
    });
    
    JustAudioBackground.setOnSeek((_, position) {
      debugPrint('[NotificationHandler] Seek requested: $position');
      _handleSeek(position);
    });
  }
  
  static Future<void> _handlePlay() async {
    try {
      final playerController = globalAudioPlayer;
      await playerController.play();
      
      // Update notification
      await backgroundPlaybackService.updateAssetInfo(
        assetId: 'current',
        title: 'Playing',
      );
    } catch (e) {
      debugPrint('[NotificationHandler] Play error: $e');
    }
  }
  
  static Future<void> _handlePause() async {
    try {
      final playerController = globalAudioPlayer;
      await playerController.pause();
      
      // Update notification
      await backgroundPlaybackService.updateAssetInfo(
        assetId: 'current',
        title: 'Paused',
      );
    } catch (e) {
      debugPrint('[NotificationHandler] Pause error: $e');
    }
  }
  
  static Future<void> _handleSkipNext() async {
    try {
      final queueController = _getQueueController();
      await queueController.playNext();
    } catch (e) {
      debugPrint('[NotificationHandler] Skip next error: $e');
    }
  }
  
  static Future<void> _handleSkipPrevious() async {
    try {
      final queueController = _getQueueController();
      queueController.playPrevious();
    } catch (e) {
      debugPrint('[NotificationHandler] Skip previous error: $e');
    }
  }
  
  static Future<void> _handleStop() async {
    try {
      final playerController = globalAudioPlayer;
      await playerController.stop();
      
      // Clear notification
      await backgroundPlaybackService.disable();
    } catch (e) {
      debugPrint('[NotificationHandler] Stop error: $e');
    }
  }
  
  static Future<void> _handleSeek(Duration position) async {
    try {
      final playerController = globalAudioPlayer;
      await playerController.seek(position);
    } catch (e) {
      debugPrint('[NotificationHandler] Seek error: $e');
    }
  }
  
  static QueueNotifier _getQueueController() {
    // In a real implementation, you would get this from Riverpod
    // This is a placeholder
    throw UnimplementedError('Queue controller not available');
  }
}
