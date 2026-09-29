# Phase 3: Background Playback & Queue Management - Developer Guide

## Overview

This guide covers the Phase 3 implementation of **Background Playback** and **Queue Management** for the I-Confess audio platform.

---

## Quick Start

### 1. Add Required Dependencies

Add to your `pubspec.yaml`:

```yaml
dependencies:
  flutter: sdk: flutter
  flutter_riverpod: ^2.4.9
  just_audio: ^0.9.34
  just_audio_background: ^0.0.1-beta.10  # NEW for background playback
  audio_session: ^0.1.16
  http: ^1.1.0
```

Run:
```bash
flutter pub get
```

### 2. Import the Audio Feature

```dart
import 'package:i_confess/src/features/audio/audio.dart';
```

### 3. Initialize Background Service

In your `main.dart`:

```dart
void main() async {
  WidgetsFlutterBinding.ensureInitialized();
  
  // Initialize audio player
  final player = AudioPlayer();
  
  // Initialize background playback service
  final backgroundService = BackgroundPlaybackService(
    player: player,
    config: const BackgroundPlaybackConfig(
      enabled: true,
      showNotification: true,
      notificationTitle: 'I-Confess',
    ),
  );
  
  await backgroundService.init();
  
  runApp(MyApp());
}
```

### 4. Add Queue Button to Your Player

```dart
// In your player widget
Row(
  children: [
    // ... other controls
    QueueButton(),
    QueueProgress(),
  ],
)
```

### 5. Show Queue Panel

```dart
// Show as bottom sheet
showModalBottomSheet(
  context: context,
  isScrollControlled: true,
  builder: (context) => const QueuePanel(),
);

// Or as full screen
Navigator.of(context).push(
  MaterialPageRoute(
    builder: (context) => const QueuePanel(fullScreen: true),
  ),
);
```

---

## Background Playback

### Configuration

Create a configuration for background playback:

```dart
final config = BackgroundPlaybackConfig(
  enabled: true,                    // Enable background playback
  showNotification: true,          // Show notification when playing
  notificationTitle: 'I-Confess',  // Title in notification
  notificationSubtitle: 'Audio Confession',
  notificationImageUrl: 'https://example.com/icon.png',
  showPlayPauseControls: true,     // Show play/pause buttons
  showSkipControls: true,         // Show skip next/previous
  showStopControl: true,           // Show stop button
);
```

### Platform Configuration

#### Android

Add to `android/app/src/main/AndroidManifest.xml`:

```xml
<!-- Permissions -->
<uses-permission android:name="android.permission.WAKE_LOCK"/>
<uses-permission android:name="android.permission.FOREGROUND_SERVICE"/>

<!-- Foreground Service -->
<service 
    android:name="com.baseflow.justaudio_background.JustAudioBackgroundService"
    android:foregroundServiceType="mediaPlayback"
    android:exported="false"/>
```

Add to `android/app/build.gradle`:

```gradle
android {
    defaultConfig {
        manifestPlaceholders = [
            justAudioBackgroundChannelId: 'i_confess_audio',
            justAudioBackgroundChannelName: 'I-Confess Audio',
        ]
    }
}
```

#### iOS

Add to `ios/Runner/Info.plist`:

```xml
<key>UIBackgroundModes</key>
<array>
  <string>audio</string>
  <string>airplay</string>
</array>

<key>NSAppTransportSecurity</key>
<dict>
  <key>NSAllowsArbitraryLoads</key>
  <true/>
</dict>
```

In Xcode:
1. Open Runner.xcworkspace
2. Select Runner target
3. Go to Signing & Capabilities
4. Click + Capability
5. Add Background Modes
6. Check "Audio, AirPlay, and Picture in Picture"

### Usage

#### Initialize

```dart
final player = AudioPlayer();
final backgroundService = BackgroundPlaybackService(
  player: player,
  config: const BackgroundPlaybackConfig(),
);

await backgroundService.init();
```

#### Enable/Disable

```dart
await backgroundService.enable();
await backgroundService.disable();
```

#### Update Notification

```dart
await backgroundService.updateAssetInfo(
  assetId: 'asset_123',
  title: 'My Confession',
  subtitle: 'Voice: Sarah',
  imageUrl: 'https://example.com/image.png',
);
```

#### Handle Notification Events

The service automatically handles notification events, but you can also call these methods directly:

```dart
await backgroundService.handleNotificationPlay();
await backgroundService.handleNotificationPause();
await backgroundService.handleNotificationSkipNext();
await backgroundService.handleNotificationSkipPrevious();
await backgroundService.handleNotificationStop();
```

### State Management

#### Check Background State

```dart
final isInBackground = backgroundService.isInBackground;
```

#### Listen to State Changes

```dart
backgroundService.onBackgroundStateChanged.listen((isInBackground) {
  print('App is in background: $isInBackground');
});
```

#### Listen to Notification State

```dart
backgroundService.onNotificationStateChanged.listen((state) {
  print('Notification state: $state');
});
```

---

## Queue Management

### Basic Operations

#### Access the Queue Controller

```dart
final queueController = ref.read(queueControllerProvider.notifier);
final queue = ref.watch(currentQueueProvider);
```

#### Add Items

```dart
// Add a queue item
queueController.addItem(myQueueItem);

// Add an asset
queueController.addAsset(myAudioAsset, playNext: true);

// Add a confession (will generate audio if needed)
queueController.addConfession(
  confessionId: 'confession_123',
  voiceId: 'voice_456',
  playNext: false,
);

// Add multiple items
queueController.addItems([item1, item2, item3]);
```

#### Remove Items

```dart
// Remove by index
queueController.removeItemAt(2);

// Remove by ID
queueController.removeItemById('item_123');

// Clear all
queueController.clear();
```

#### Play Items

```dart
// Play item at index
queueController.playItemAt(2);

// Play next
queueController.playNext();

// Play previous
queueController.playPrevious();
```

#### Shuffle and Repeat

```dart
// Toggle shuffle
queueController.toggleShuffle();

// Cycle repeat mode
queueController.cycleRepeatMode();

// Set specific repeat mode
queueController.setRepeatMode(RepeatMode.all);
```

### State Access

#### Access Queue State

```dart
final queue = ref.watch(currentQueueProvider);
final currentItem = ref.watch(currentQueueItemProvider);
final isEmpty = ref.watch(isQueueEmptyProvider);
final isNotEmpty = ref.watch(isQueueNotEmptyProvider);
final isShuffled = ref.watch(isShuffledProvider);
final repeatMode = ref.watch(repeatModeProvider);
final queueLength = ref.watch(queueLengthProvider);
```

#### Check Queue Properties

```dart
final hasItems = queue.isNotEmpty;
final currentIndex = queue.currentIndex;
final hasCurrent = queue.hasCurrentItem;
final totalDuration = queue.totalDuration;
final playedCount = queue.playedCount;
final remainingCount = queue.remainingCount;
```

### UI Components

#### Queue Button

```dart
// Simple queue button with badge
QueueButton(),

// Custom queue button
QueueButton(
  icon: Icons.my_custom_icon,
  label: 'My Queue',
  showCount: true,
  onPressed: () {
    // Custom action
  },
),
```

#### Queue Panel

```dart
// Show as bottom sheet
showModalBottomSheet(
  context: context,
  isScrollControlled: true,
  builder: (context) => const QueuePanel(),
);

// Show as full screen
Navigator.of(context).push(
  MaterialPageRoute(
    builder: (context) => const QueuePanel(fullScreen: true),
  ),
);

// Custom queue panel
QueuePanel(
  fullScreen: false,
  barrierDismissible: true,
  backgroundColor: Colors.blueGrey,
  maxHeight: 400,
),
```

#### Queue Controls

```dart
// Compact controls
QueueControls(compact: true),

// Full controls
QueueControls(compact: false),
```

#### Queue Progress

```dart
// Shows "1/5" for current position in queue
QueueProgress(),
```

### Auto-Advance Setup

To automatically play the next item when playback completes:

```dart
// In your audio player controller or initialization
ref.read(audioPlayerControllerProvider.notifier).state.playerState;

// Or listen to playback events
final player = ref.read(audioPlayerControllerProvider.notifier)._playbackService;
player.playbackEventStream.listen((event) {
  if (event.processingState == ProcessingState.completed) {
    final queueController = ref.read(queueControllerProvider.notifier);
    queueController.playNext();
  }
});
```

---

## Complete Example

### Full Player with Queue Integration

```dart
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:i_confess/src/features/audio/audio.dart';

class FullAudioPlayer extends ConsumerWidget {
  const FullAudioPlayer({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final playerState = ref.watch(audioPlayerControllerProvider);
    final queue = ref.watch(currentQueueProvider);
    final controller = ref.read(audioPlayerControllerProvider.notifier);
    final queueController = ref.read(queueControllerProvider.notifier);

    return Scaffold(
      appBar: AppBar(
        title: const Text('Audio Player'),
        actions: [
          QueueButton(),
        ],
      ),
      body: Column(
        children: [
          // Current playing item
          AudioPlayerWidget(
            assetId: queue.currentItem?.asset.id,
            confessionId: queue.currentItem?.confessionId,
            voiceId: queue.currentItem?.voiceId,
          ),
          
          // Queue progress
          QueueProgress(),
          
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
            ],
          ),
          
          // Queue controls
          QueueControls(),
        ],
      ),
    );
  }
}
```

### Adding Items to Queue

```dart
// Add from a confession list
ListTile(
  title: Text('My Confession'),
  trailing: IconButton(
    icon: const Icon(Icons.add),
    onPressed: () {
      final queueController = ref.read(queueControllerProvider.notifier);
      queueController.addConfession(
        confessionId: 'confession_123',
        voiceId: 'voice_456',
      );
    },
  ),
),

// Add from an asset
IconButton(
  icon: const Icon(Icons.queue_music),
  onPressed: () {
    final queueController = ref.read(queueControllerProvider.notifier);
    queueController.addAsset(myAudioAsset);
  },
),
```

---

## Testing

### Run Tests

```bash
# Run all audio tests
flutter test test/features/audio/

# Run queue tests specifically
flutter test test/features/audio/queue_test.dart

# Run background playback tests
flutter test test/features/audio/background_playback_test.dart

# Run with coverage
flutter test --coverage test/features/audio/
```

### Test Coverage

The test files cover:
- Queue item creation and serialization
- Queue operations (add, remove, reorder)
- Queue state management
- Background playback configuration
- Background playback service initialization

---

## Troubleshooting

### Background Playback Not Working

**Android:**
1. Check that `WAKE_LOCK` and `FOREGROUND_SERVICE` permissions are added
2. Verify foreground service is declared in manifest
3. Check notification channel configuration
4. Test on a physical device (emulators may have issues)

**iOS:**
1. Verify background modes are enabled in Xcode
2. Check Info.plist configuration
3. Ensure audio session is properly configured
4. Test on a physical device

### Queue Not Auto-Advancing

1. Check that playback event listener is set up
2. Verify `playNext()` is being called on completion
3. Check repeat mode settings
4. Ensure queue has items

### Notification Not Showing

1. Check `showNotification` is true in config
2. Verify notification permissions on device
3. Ensure audio is playing
4. Check for errors in logcat (Android) or Xcode console (iOS)

---

## API Reference

### BackgroundPlaybackService

#### Configuration

```dart
BackgroundPlaybackConfig({
  bool enabled = true,
  bool showNotification = true,
  String notificationTitle = 'I-Confess',
  String? notificationSubtitle,
  String? notificationImageUrl,
  bool showPlayPauseControls = true,
  bool showSkipControls = false,
  bool showStopControl = true,
})
```

#### Methods

```dart
Future<void> init()
Future<void> enable()
Future<void> disable()
Future<void> updateAssetInfo({String? assetId, String? title, String? subtitle, String? imageUrl})
Future<void> handleNotificationPlay()
Future<void> handleNotificationPause()
Future<void> handleNotificationSkipNext()
Future<void> handleNotificationSkipPrevious()
Future<void> handleNotificationStop()
void dispose()
```

#### Properties

```dart
bool isInBackground
Stream<bool> onBackgroundStateChanged
Stream<Map<String, dynamic>> onNotificationStateChanged
```

### QueueNotifier

#### Methods

```dart
void addItem(AudioQueueItem item, {bool playNext = false})
void addAsset(AudioAsset asset, {bool playNext = false, String? title})
Future<void> addConfession({required String confessionId, String? voiceId, bool playNext = false})
void addItems(List<AudioQueueItem> items, {bool playNext = false})
void removeItemAt(int index)
void removeItemById(String itemId)
void clear()
void moveItem(int fromIndex, int toIndex)
void playItemAt(int index)
Future<void> playNext()
void playPrevious()
void toggleShuffle()
void cycleRepeatMode()
void setRepeatMode(RepeatMode mode)
void markCurrentAsPlayed()
void updateCurrentPosition(Duration position)
int? getIndexById(String itemId)
AudioQueueItem? getItemById(String itemId)
List<AudioQueueItem> getItemsByConfession(String confessionId)
bool containsConfession(String confessionId)
void replaceAll(List<AudioQueueItem> items, {int startIndex = 0})
Map<String, dynamic> toJson()
void restoreFromJson(Map<String, dynamic> json)
```

### Providers

```dart
queueControllerProvider          // StateNotifierProvider<QueueNotifier, QueueState>
currentQueueProvider             // Provider<AudioQueue>
currentQueueItemProvider         // Provider<AudioQueueItem?>
isQueueEmptyProvider            // Provider<bool>
isQueueNotEmptyProvider          // Provider<bool>
isShuffledProvider                // Provider<bool>
repeatModeProvider                // Provider<RepeatMode>
queueLengthProvider              // Provider<int>
```

### Widgets

```dart
QueuePanel({bool fullScreen = false, bool barrierDismissible = true, Color? backgroundColor, double? maxHeight})
QueueButton({IconData icon = Icons.queue_music, String? label, bool showCount = true, VoidCallback? onPressed})
QueueControls({bool compact = false})
QueueProgress()
```

### Models

```dart
AudioQueueItem
  - String id
  - AudioAsset asset
  - String confessionId
  - String voiceId
  - String title
  - String? subtitle
  - Duration duration
  - int orderIndex
  - bool played
  - Duration? lastPosition

AudioQueue
  - List<AudioQueueItem> items
  - int currentIndex
  - bool isShuffled
  - RepeatMode repeatMode
  - bool isUpdating

RepeatMode
  - none
  - all
  - one
```

---

## Best Practices

### 1. Always Check Queue State

Before performing operations, check if the queue has items:

```dart
if (queue.isNotEmpty) {
  queueController.playNext();
}
```

### 2. Handle Empty Queue

Provide feedback when the queue is empty:

```dart
if (queue.isEmpty) {
  ScaffoldMessenger.of(context).showSnackBar(
    SnackBar(content: Text('Queue is empty')),
  );
}
```

### 3. Save Queue State

Persist the queue state when the app closes:

```dart
// Save on app pause
WidgetsBinding.instance.addObserver(
  _AppLifecycleObserver(
    onPause: () {
      final queueController = ref.read(queueControllerProvider.notifier);
      final json = queueController.toJson();
      // Save to storage
    },
  ),
);
```

### 4. Restore Queue State

Restore the queue when the app resumes:

```dart
// Restore on app resume
final queueController = ref.read(queueControllerProvider.notifier);
final json = await loadQueueFromStorage();
if (json != null) {
  queueController.restoreFromJson(json);
}
```

### 5. Handle Errors

Listen for errors and show appropriate messages:

```dart
final queueState = ref.watch(queueControllerProvider);
if (queueState.error != null) {
  ScaffoldMessenger.of(context).showSnackBar(
    SnackBar(content: Text(queueState.error!)),
  );
}
```

---

## Migration Guide

### From Phase 2 to Phase 3

If you're upgrading from Phase 2:

1. **Add new dependencies**
   ```bash
   flutter pub add just_audio_background
   ```

2. **Add platform configurations**
   - Update AndroidManifest.xml
   - Update Info.plist
   - Configure Xcode

3. **Update imports**
   ```dart
   import 'package:i_confess/src/features/audio/audio.dart';
   // This now includes queue and background playback
   ```

4. **Initialize background service**
   ```dart
   // In main.dart
   final backgroundService = BackgroundPlaybackService(
     player: player,
   );
   await backgroundService.init();
   ```

5. **Add queue button**
   ```dart
   // In your player UI
   QueueButton(),
   ```

6. **Set up auto-advance**
   ```dart
   // Listen to playback completion
   player.playbackEventStream.listen((event) {
     if (event.processingState == ProcessingState.completed) {
       queueController.playNext();
     }
   });
   ```

---

## FAQ

### Q: Does background playback work on all platforms?

A: Background playback works on Android and iOS. On web, it has limited support and depends on the browser. The service will gracefully degrade on unsupported platforms.

### Q: Can I customize the notification appearance?

A: Yes! Use the `BackgroundPlaybackConfig` to customize the notification title, subtitle, image, and which controls to show.

### Q: How do I handle phone call interruptions?

A: The audio session is automatically configured to handle interruptions. When a call comes in, audio will pause. When the call ends, audio will resume (on iOS) or you can manually resume (on Android).

### Q: Can I persist the queue across app restarts?

A: Yes! Use the `toJson()` and `restoreFromJson()` methods to save and restore the queue state.

### Q: How do I prevent duplicate items in the queue?

A: Check if a confession is already in the queue before adding:
  ```dart
  if (!queueController.containsConfession('confession_123')) {
    queueController.addConfession(confessionId: 'confession_123');
  }
  ```

### Q: Can I limit the queue size?

A: Yes! You can check the queue length before adding:
  ```dart
  if (queue.length < maxQueueSize) {
    queueController.addItem(item);
  }
  ```

---

## Support

For issues or questions:
- Check the [Phase 3 Documentation](../../docs/PHASE3-BACKGROUND-AND-QUEUE.md)
- Review the test files for usage examples
- Check the example code in this guide

---

**Last Updated**: 2026-09-29  
**Version**: 1.0.0  
**Phase**: 3 - Background Playback & Queue Management
