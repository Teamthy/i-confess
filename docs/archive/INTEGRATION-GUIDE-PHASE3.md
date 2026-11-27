# Phase 3 Integration Guide - Step-by-Step

## Overview

This guide provides **detailed, step-by-step instructions** for integrating Phase 3 features (Background Playback & Queue Management) into your I-Confess Flutter application.

**Estimated Time: 30-60 minutes**

---

## Prerequisites

Before starting, ensure you have:
- ✅ Flutter SDK installed (version 3.16+ recommended)
- ✅ Dart SDK installed (version 3.2+)
- ✅ Android Studio or Xcode for platform-specific configuration
- ✅ Physical Android and iOS devices for testing (emulators have limitations)
- ✅ Phase 1 and Phase 2 already integrated

---

## Step 1: Add Dependencies

### 1.1 Update pubspec.yaml

Open `apps/mobile/pubspec.yaml` and add the required dependencies:

```yaml
dependencies:
  flutter:
    sdk: flutter
  
  # Existing dependencies (ensure these are present)
  flutter_riverpod: ^2.4.9
  just_audio: ^0.9.34
  audio_session: ^0.1.16
  http: ^1.1.0
  
  # NEW for Phase 3 - Add these:
  just_audio_background: ^0.0.1-beta.10  # Required for background playback
  shared_preferences: ^2.2.2         # Required for queue persistence
  reorderables: ^0.5.0                # Required for drag-and-drop reordering
```

### 1.2 Run flutter pub get

```bash
cd apps/mobile
flutter pub get
```

**Expected Output:**
```
Running "flutter pub get" in apps/mobile...                       12.5s
```

**Troubleshooting:**
- If you get errors, ensure you're in the correct directory
- Check for typos in the dependency names
- Try `flutter clean` then `flutter pub get`

---

## Step 2: Add Platform Configuration Files

### 2.1 Android Configuration

#### 2.1.1 Copy AndroidManifest.xml

Copy the provided manifest to your Android project:

```bash
# From the repository
cp /home/user/i-confess/android/app/src/main/AndroidManifest.xml \
   /path/to/your/project/android/app/src/main/AndroidManifest.xml
```

**OR manually add these to your existing manifest:**

```xml
<!-- Add these permissions -->
<uses-permission android:name="android.permission.WAKE_LOCK" />
<uses-permission android:name="android.permission.FOREGROUND_SERVICE" />

<!-- Add this service -->
<service 
    android:name="com.baseflow.justaudio_background.JustAudioBackgroundService"
    android:foregroundServiceType="mediaPlayback"
    android:exported="false" />

<!-- Add these manifest placeholders -->
<meta-data
    android:name="com.baseflow.justaudio_background.NOTIFICATION_CHANNEL_ID"
    android:value="i_confess_audio" />
<meta-data
    android:name="com.baseflow.justaudio_background.NOTIFICATION_CHANNEL_NAME"
    android:value="I-Confess Audio" />
```

#### 2.1.2 Copy build.gradle

```bash
cp /home/user/i-confess/android/app/build.gradle \
   /path/to/your/project/android/app/build.gradle
```

**OR manually add this to your existing build.gradle:**

```gradle
android {
    defaultConfig {
        manifestPlaceholders = [
            justAudioBackgroundChannelId: 'i_confess_audio',
            justAudioBackgroundChannelName: 'I-Confess Audio',
        ]
    }
}

dependencies {
    implementation 'com.google.android.exoplayer:exoplayer:2.19.1'
    implementation 'androidx.media:media:1.6.0'
}
```

### 2.2 iOS Configuration

#### 2.2.1 Copy Info.plist

```bash
cp /home/user/i-confess/ios/Runner/Info.plist \
   /path/to/your/project/ios/Runner/Info.plist
```

**OR manually add these to your existing Info.plist:**

```xml
<!-- Add background modes -->
<key>UIBackgroundModes</key>
<array>
    <string>audio</string>
    <string>airplay</string>
</array>

<!-- Add transport security -->
<key>NSAppTransportSecurity</key>
<dict>
    <key>NSAllowsArbitraryLoads</key>
    <true/>
</dict>

<!-- Add microphone permission -->
<key>NSMicrophoneUsageDescription</key>
<string>I-Confess needs access to microphone to record your voice confessions</string>
```

#### 2.2.2 Enable Background Modes in Xcode

1. Open your project in Xcode
2. Select the Runner target
3. Go to **Signing & Capabilities**
4. Click **+ Capability**
5. Add **Background Modes**
6. Check **Audio, AirPlay, and Picture in Picture**

---

## Step 3: Update main.dart

### 3.1 Copy from main_phase3.dart

You have two options:

**Option A: Use main_phase3.dart as your main.dart**
```bash
cp /home/user/i-confess/apps/mobile/lib/main_phase3.dart \
   /path/to/your/project/apps/mobile/lib/main.dart
```

**Option B: Integrate pieces into your existing main.dart**

Copy these parts from `main_phase3.dart`:

#### 3.1.1 Global Variables (add at top)

```dart
// Add these at the top of main.dart
final GlobalKey<NavigatorState> navigatorKey = GlobalKey<NavigatorState>();

// Audio services (will be initialized in main)
AudioPlayer? _globalAudioPlayer;
BackgroundPlaybackService? _backgroundPlaybackService;

// Getters
AudioPlayer get globalAudioPlayer {
  assert(_globalAudioPlayer != null, 'Audio player not initialized');
  return _globalAudioPlayer!;
}

BackgroundPlaybackService get backgroundPlaybackService {
  assert(_backgroundPlaybackService != null, 'Background service not initialized');
  return _backgroundPlaybackService!;
}
```

#### 3.1.2 Initialization Function (add before main())

```dart
// Add this before main()
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
      ),
    );
    
    // Initialize the service
    await _backgroundPlaybackService!.init();
    
    debugPrint('[AudioServices] Initialized successfully');
  } catch (e, stack) {
    debugPrint('[AudioServices] Initialization error: $e');
    debugPrint('[AudioServices] Stack: $stack');
  }
}
```

#### 3.1.3 Update main() Function

```dart
// Update your main() to include audio initialization
Future<void> main() async {
  WidgetsFlutterBinding.ensureInitialized();
  
  // Initialize audio services for Phase 3
  await initAudioServices();
  
  // Your existing initialization code
  // ...
  
  runApp(
    const ProviderScope(
      child: MyApp(),  // Your existing app
    ),
  );
}
```

#### 3.1.4 Wrap App with AudioPlayerWrapper

```dart
// Replace your existing app widget or wrap it
class MyApp extends ConsumerWidget {
  @override
  Widget build(BuildContext context, WidgetRef ref) {
    return AudioPlayerWrapper(
      child: MaterialApp(
        // Your existing MaterialApp configuration
        title: 'I-Confess',
        navigatorKey: navigatorKey,
        // ...
      ),
    );
  }
}

// Add the AudioPlayerWrapper class from main_phase3.dart
```

#### 3.1.5 Add Notification Handler

Add this at the end of main.dart or in your initialization:

```dart
// Initialize notification handlers
NotificationHandler.init();
```

---

## Step 4: Add Queue UI to Player

### 4.1 Import Audio Package

Ensure your player widget has this import:

```dart
import 'package:i_confess/src/features/audio/audio.dart';
```

### 4.2 Add Queue Button

Add the queue button to your player UI:

```dart
// In your player widget's build method
@override
Widget build(BuildContext context, WidgetRef ref) {
  return Column(
    children: [
      // Your existing player UI
      AudioPlayerWidget(
        // ...
      ),
      
      // Add queue controls
      Row(
        mainAxisAlignment: MainAxisAlignment.center,
        children: [
          // Previous button
          IconButton(
            icon: const Icon(Icons.skip_previous),
            onPressed: () {
              final queueController = ref.read(queueControllerProvider.notifier);
              queueController.playPrevious();
            },
          ),
          
          // Play/Pause button
          IconButton(
            icon: Icon(isPlaying ? Icons.pause : Icons.play_arrow),
            onPressed: () async {
              final controller = ref.read(audioPlayerControllerProvider.notifier);
              if (isPlaying) {
                await controller.pause();
              } else {
                await controller.resume();
              }
            },
          ),
          
          // Next button
          IconButton(
            icon: const Icon(Icons.skip_next),
            onPressed: () {
              final queueController = ref.read(queueControllerProvider.notifier);
              queueController.playNext();
            },
          ),
          
          // Queue button
          QueueButton(),
          
          // Queue progress
          QueueProgress(),
        ],
      ),
      
      // Queue controls
      QueueControls(),
    ],
  );
}
```

### 4.3 Add Queue Panel Access

To show the queue panel:

```dart
// Show as bottom sheet
showModalBottomSheet(
  context: context,
  isScrollControlled: true,
  builder: (context) => const QueuePanel(),
);

// Or show as full screen
Navigator.of(context).push(
  MaterialPageRoute(
    builder: (context) => const QueuePanel(fullScreen: true),
  ),
);
```

---

## Step 5: Set Up Queue Persistence

### 5.1 Add to Your App Initialization

In your main.dart or app initialization:

```dart
// Restore queue on app start
Future<void> main() async {
  WidgetsFlutterBinding.ensureInitialized();
  
  await initAudioServices();
  
  // Initialize queue persistence
  final persistence = await QueuePersistenceService.create();
  
  // Restore queue state
  final queueController = QueueNotifier(globalAudioPlayer);
  final (restoredQueue, lastPosition) = await persistence.restoreCurrentState();
  
  if (restoredQueue.isNotEmpty) {
    // Restore the queue
    queueController.restoreFromJson(restoredQueue.toJson());
    
    // If we have a last position, resume from there
    if (lastPosition != null && restoredQueue.currentItem != null) {
      await globalAudioPlayer.seek(lastPosition);
    }
  }
  
  runApp(MyApp());
}
```

### 5.2 Save Queue on Changes

Listen to queue changes and save:

```dart
// In your player widget or controller
final queueController = ref.read(queueControllerProvider.notifier);

// Save when queue changes
ref.listen(queueControllerProvider, (previous, next) {
  final persistence = QueuePersistenceService.create();
  persistence.saveQueue(next.queue);
});
```

---

## Step 6: Test the Integration

### 6.1 Run Unit Tests

```bash
cd apps/mobile
flutter test test/features/audio/
```

**Expected Output:**
```
00:00 +0: All tests passed!
```

### 6.2 Test on Android Device

```bash
flutter run -d <device-id>
```

**Test Cases:**
1. ✅ Play an audio asset
2. ✅ Put app in background - audio should continue
3. ✅ Notification should show with controls
4. ✅ Tap notification play/pause - should control audio
5. ✅ Add items to queue
6. ✅ Play next/previous
7. ✅ Toggle shuffle
8. ✅ Cycle repeat modes
9. ✅ Drag to reorder queue items

### 6.3 Test on iOS Device

```bash
flutter run -d <device-id>
```

**Test Cases:**
1. ✅ Play an audio asset
2. ✅ Put app in background - audio should continue
3. ✅ Lock screen should show controls
4. ✅ Control Center should show controls
5. ✅ All queue operations work

---

## Step 7: Final Adjustments

### 7.1 Customize Notification

Update the notification configuration in main.dart:

```dart
_backgroundPlaybackService = BackgroundPlaybackService(
  player: _globalAudioPlayer!,
  config: const BackgroundPlaybackConfig(
    enabled: true,
    showNotification: true,
    notificationTitle: 'I-Confess',
    notificationSubtitle: 'Listening to confessions',
    notificationImageUrl: 'https://yourdomain.com/icon-512.png',
    showPlayPauseControls: true,
    showSkipControls: true,
    showStopControl: true,
  ),
);
```

### 7.2 Customize Queue Panel

Customize the queue panel appearance:

```dart
QueuePanel(
  fullScreen: false,           // Show as bottom sheet
  barrierDismissible: true,   // Can tap outside to close
  backgroundColor: Colors.grey[900],
  maxHeight: 500,            // Max height for bottom sheet
),
```

### 7.3 Add Queue to App Bar

```dart
AppBar(
  actions: [
    QueueButton(
      icon: Icons.queue_music,
      showCount: true,
    ),
  ],
)
```

---

## Troubleshooting

### Common Issues and Solutions

#### Issue 1: Audio stops when app goes to background

**Cause:** Missing permissions or configuration

**Solution:**
1. Verify `WAKE_LOCK` and `FOREGROUND_SERVICE` permissions in AndroidManifest.xml
2. Verify foreground service declaration in AndroidManifest.xml
3. Verify `UIBackgroundModes` in Info.plist
4. Test on physical device (emulators have issues)

#### Issue 2: Notification not showing

**Cause:** Notification disabled or permissions missing

**Solution:**
1. Check `showNotification: true` in config
2. Verify notification channel in build.gradle
3. Check notification permissions on device
4. Ensure audio is playing before going to background

#### Issue 3: Queue not auto-advancing

**Cause:** Playback event listener not set up

**Solution:**
1. Verify `player.playbackEventStream.listen` is set up
2. Check that `queueController.playNext()` is being called
3. Ensure queue has items when playback completes

#### Issue 4: Drag-and-drop not working

**Cause:** Missing reorderables dependency

**Solution:**
1. Verify `reorderables: ^0.5.0` in pubspec.yaml
2. Run `flutter pub get`
3. Check for errors in console

#### Issue 5: Build errors on Android

**Cause:** Missing or conflicting dependencies

**Solution:**
1. Run `flutter clean`
2. Run `flutter pub get`
3. Check for version conflicts in pubspec.yaml
4. Ensure all dependencies are compatible

---

## Configuration Reference

### Android Configuration Checklist

- [ ] `WAKE_LOCK` permission added
- [ ] `FOREGROUND_SERVICE` permission added
- [ ] Foreground service declared in manifest
- [ ] Manifest placeholders in build.gradle
- [ ] ExoPlayer dependencies in build.gradle

### iOS Configuration Checklist

- [ ] `UIBackgroundModes` with `audio` and `airplay`
- [ ] Background Modes capability in Xcode
- [ ] `NSAppTransportSecurity` allows arbitrary loads
- [ ] Microphone usage description

### Flutter Configuration Checklist

- [ ] `just_audio_background` dependency added
- [ ] `shared_preferences` dependency added
- [ ] `reorderables` dependency added
- [ ] Audio services initialized in main.dart
- [ ] AudioPlayerWrapper added
- [ ] Notification handlers initialized

---

## Complete Code Examples

### Example 1: Basic Player with Queue

```dart
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:i_confess/src/features/audio/audio.dart';

class BasicPlayer extends ConsumerWidget {
  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final playerState = ref.watch(audioPlayerControllerProvider);
    final queue = ref.watch(currentQueueProvider);
    final controller = ref.read(audioPlayerControllerProvider.notifier);
    final queueController = ref.read(queueControllerProvider.notifier);
    
    return Column(
      children: [
        // Current playing
        AudioPlayerWidget(
          assetId: queue.currentItem?.asset.id,
          confessionId: queue.currentItem?.confessionId,
          voiceId: queue.currentItem?.voiceId,
        ),
        
        // Controls
        Row(
          mainAxisAlignment: MainAxisAlignment.center,
          children: [
            IconButton(
              icon: const Icon(Icons.skip_previous),
              onPressed: queueController.playPrevious,
            ),
            IconButton(
              icon: Icon(
                playerState.playerState == PlayerState.playing 
                  ? Icons.pause 
                  : Icons.play_arrow,
              ),
              onPressed: () async {
                if (playerState.playerState == PlayerState.playing) {
                  await controller.pause();
                } else {
                  await controller.resume();
                }
              },
            ),
            IconButton(
              icon: const Icon(Icons.skip_next),
              onPressed: queueController.playNext,
            ),
            QueueButton(),
          ],
        ),
        
        // Queue controls
        QueueControls(),
      ],
    );
  }
}
```

### Example 2: Full Screen Queue

```dart
import 'package:flutter/material.dart';
import 'package:i_confess/src/features/audio/audio.dart';

class QueueScreen extends StatelessWidget {
  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(
        title: const Text('Queue'),
        actions: [
          IconButton(
            icon: const Icon(Icons.clear),
            onPressed: () {
              final queueController = context.read(queueControllerProvider.notifier);
              queueController.clear();
            },
          ),
        ],
      ),
      body: QueuePanel(
        fullScreen: true,
      ),
    );
  }
}
```

### Example 3: Add to Queue from Confession List

```dart
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:i_confess/src/features/audio/audio.dart';

class ConfessionListItem extends ConsumerWidget {
  final String confessionId;
  final String title;
  final String? voiceId;

  const ConfessionListItem({
    required this.confessionId,
    required this.title,
    this.voiceId,
  });

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final queueController = ref.read(queueControllerProvider.notifier);
    
    return ListTile(
      title: Text(title),
      trailing: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          // Play now
          IconButton(
            icon: const Icon(Icons.play_arrow),
            onPressed: () {
              queueController.clear();
              queueController.addConfession(
                confessionId: confessionId,
                voiceId: voiceId,
              );
            },
          ),
          // Add to queue
          IconButton(
            icon: const Icon(Icons.queue_music),
            onPressed: () {
              queueController.addConfession(
                confessionId: confessionId,
                voiceId: voiceId,
              );
              ScaffoldMessenger.of(context).showSnackBar(
                SnackBar(content: Text('Added to queue')),
              );
            },
          ),
        ],
      ),
    );
  }
}
```

---

## Final Checklist

Before deploying to production, verify:

- [ ] All dependencies added to pubspec.yaml
- [ ] AndroidManifest.xml updated with permissions
- [ ] build.gradle updated with manifest placeholders
- [ ] Info.plist updated with background modes
- [ ] Background modes enabled in Xcode
- [ ] Audio services initialized in main.dart
- [ ] AudioPlayerWrapper added
- [ ] Notification handlers initialized
- [ ] Queue button added to player UI
- [ ] Queue persistence set up
- [ ] All tests passing
- [ ] Tested on physical Android device
- [ ] Tested on physical iOS device
- [ ] Background playback works
- [ ] Queue operations work
- [ ] Drag-and-drop reordering works
- [ ] Auto-advance works

---

## Support

If you encounter issues:

1. **Check this guide** for step-by-step instructions
2. **Review PHASE3-GUIDE.md** for API documentation
3. **Review PHASE3-IMPLEMENTATION.md** for detailed implementation
4. **Check the test files** for usage examples
5. **Run flutter doctor** to check your environment

---

## Success! 🎉

Once you've completed all steps:
- ✅ Audio continues playing in background
- ✅ Notification controls work
- ✅ Queue management works
- ✅ Drag-and-drop reordering works
- ✅ Queue persists across app restarts

**Your I-Confess app now has a complete, production-ready audio platform!** 🚀

---

**Document Version:** 1.0.0  
**Last Updated:** 2026-09-29  
**Author:** Arena.ai Agent  
**Phase:** 3 - Background Playback & Queue Management
