# Phase 3: Background Playback & Queue Management - Implementation Guide

## Overview

Phase 3 builds upon Phase 1 and Phase 2 by adding **Background Playback** and **Queue Management** capabilities to the I-Confess audio platform. These are the most requested features that will significantly enhance the user experience.

---

## Table of Contents

1. [Background Playback](#background-playback)
2. [Queue Management](#queue-management)
3. [File Structure](#file-structure)
4. [Implementation Details](#implementation-details)
5. [Integration Guide](#integration-guide)
6. [Testing](#testing)
7. [Configuration](#configuration)
8. [Usage Examples](#usage-examples)
9. [Next Steps](#next-steps)

---

## Background Playback

### Features

✅ **Continuous Playback**
- Audio continues playing when app goes to background
- Works on both Android and iOS
- Handles phone call interruptions

✅ **Notification Controls**
- Play/Pause button in notification
- Skip next/previous buttons (optional)
- Stop button
- Progress display

✅ **Lock Screen Controls**
- Play/Pause on lock screen
- Skip controls on lock screen
- Album art display

✅ **Audio Session Management**
- Proper audio session configuration
- Handles interruptions (calls, alarms, etc.)
- Background audio mode

### Implementation

#### Files Created

1. **`apps/mobile/lib/src/features/audio/services/background_playback_service.dart`**
   - Main background playback service
   - Audio session management
   - Notification control handling
   - Background state tracking

#### Key Classes

```dart
class BackgroundPlaybackConfig {
  // Configuration options for background playback
  final bool enabled;
  final bool showNotification;
  final String notificationTitle;
  final String? notificationSubtitle;
  final String? notificationImageUrl;
  final bool showPlayPauseControls;
  final bool showSkipControls;
  final bool showStopControl;
}

class BackgroundPlaybackService {
  // Main service class
  Future<void> init();
  Future<void> enable();
  Future<void> disable();
  Future<void> updateAssetInfo(...);
  Future<void> handleNotificationPlay();
  Future<void> handleNotificationPause();
  Future<void> handleNotificationSkipNext();
  Future<void> handleNotificationSkipPrevious();
  Future<void> handleNotificationStop();
  void dispose();
}
```

### Android Configuration

Add to `android/app/src/main/AndroidManifest.xml`:

```xml
<uses-permission android:name="android.permission.WAKE_LOCK"/>
<uses-permission android:name="android.permission.FOREGROUND_SERVICE"/>

<service 
    android:name="com.baseflow.justaudio_background.JustAudioBackgroundService"
    android:foregroundServiceType="mediaPlayback"
    android:exported="false"/>
```

### iOS Configuration

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

Also enable Background Modes in Xcode:
1. Open Runner.xcworkspace in Xcode
2. Select the Runner target
3. Go to Signing & Capabilities
4. Click + Capability
5. Add Background Modes
6. Check "Audio, AirPlay, and Picture in Picture"

### Integration with Audio Player

```dart
// In your app initialization
final player = AudioPlayer();
final backgroundService = BackgroundPlaybackService(
  player: player,
  config: BackgroundPlaybackConfig(
    enabled: true,
    showNotification: true,
    notificationTitle: 'I-Confess',
  ),
);

await backgroundService.init();

// When playing audio
final controller = ref.read(audioPlayerControllerProvider.notifier);
await controller.playAsset(
  assetId: 'asset_123',
  confessionId: 'confession_456',
  voiceId: 'voice_789',
);

// Update notification with asset info
await backgroundService.updateAssetInfo(
  assetId: 'asset_123',
  title: 'My Confession',
  subtitle: 'Voice: Sarah',
);
```

---

## Queue Management

### Features

✅ **Queue Operations**
- Add items to queue
- Remove items from queue
- Reorder items (drag and drop)
- Clear entire queue

✅ **Playback Modes**
- Normal playback (sequential)
- Shuffle mode
- Repeat modes (none, all, one)

✅ **Queue State**
- Current item tracking
- Played item tracking
- Last position tracking
- Queue persistence

✅ **UI Components**
- Queue panel (bottom sheet)
- Queue button with badge
- Queue controls
- Queue progress indicator

### Implementation

#### Files Created

1. **`apps/mobile/lib/src/features/audio/models/audio_queue.dart`**
   - Queue item model
   - Queue model
   - Repeat mode enum

2. **`apps/mobile/lib/src/features/audio/controllers/queue_controller.dart`**
   - Queue notifier
   - Queue state
   - All queue operations

3. **`apps/mobile/lib/src/features/audio/widgets/queue_panel.dart`**
   - Queue panel widget
   - Queue button
   - Queue controls
   - Queue progress

#### Key Classes

```dart
class AudioQueueItem {
  // Represents an item in the queue
  final String id;
  final AudioAsset asset;
  final String confessionId;
  final String voiceId;
  final String title;
  final String? subtitle;
  final Duration duration;
  final int orderIndex;
  final bool played;
  final Duration? lastPosition;
}

class AudioQueue {
  // Represents the entire queue
  final List<AudioQueueItem> items;
  final int currentIndex;
  final bool isShuffled;
  final RepeatMode repeatMode;
  final bool isUpdating;
}

enum RepeatMode {
  none,   // No repeat
  all,    // Repeat all
  one,    // Repeat one
}

class QueueNotifier extends StateNotifier<QueueState> {
  // Manages queue state
  void addItem(AudioQueueItem item, {bool playNext = false});
  void addAsset(AudioAsset asset, {bool playNext = false});
  void removeItemAt(int index);
  void removeItemById(String itemId);
  void clear();
  void moveItem(int fromIndex, int toIndex);
  void playItemAt(int index);
  Future<void> playNext();
  void playPrevious();
  void toggleShuffle();
  void cycleRepeatMode();
  void setRepeatMode(RepeatMode mode);
  void markCurrentAsPlayed();
  void updateCurrentPosition(Duration position);
}
```

### Integration with Audio Player

```dart
// The queue controller integrates with the audio player controller
// When an item is played from the queue, it calls:
final audioController = ref.read(audioPlayerControllerProvider.notifier);
await audioController.playAsset(
  assetId: item.asset.id,
  confessionId: item.confessionId,
  voiceId: item.voiceId,
  initialPosition: item.lastPosition,
);

// When playback completes, automatically play next
final playerState = ref.watch(audioPlayerControllerProvider);
if (playerState.playerState == PlayerState.completed) {
  final queueController = ref.read(queueControllerProvider.notifier);
  await queueController.playNext();
}
```

---

## File Structure

```
apps/mobile/lib/src/features/audio/
├── audio.dart                              # Barrel file (updated)
├── controllers/
│   ├── audio_player_controller.dart       # Existing
│   └── queue_controller.dart              # NEW
├── models/
│   ├── audio_asset.dart                    # Existing
│   ├── audio_generation_job.dart          # Existing
│   ├── audio_generation_request.dart      # Existing
│   ├── audio_queue.dart                   # NEW
│   └── tts_provider.dart                  # Existing
├── providers/
│   └── audio_providers.dart                # Existing (updated)
├── services/
│   ├── audio_generation_service.dart      # Existing
│   ├── audio_url_service.dart              # Existing
│   └── background_playback_service.dart    # NEW
└── widgets/
    ├── audio_player_widget.dart            # Existing
    └── queue_panel.dart                    # NEW

apps/mobile/test/features/audio/
├── audio_player_test.dart                 # Existing
├── audio_services_test.dart               # Existing
└── queue_test.dart                        # TO BE CREATED
```

---

## Implementation Details

### Background Playback Flow

```
1. User presses play
   ↓
2. AudioPlayerController.playAsset() called
   ↓
3. AudioPlaybackService loads and plays audio
   ↓
4. BackgroundPlaybackService updates notification
   ↓
5. User puts app in background
   ↓
6. Audio continues playing (handled by just_audio_background)
   ↓
7. Notification shows playback controls
   ↓
8. User taps notification play/pause
   ↓
9. BackgroundPlaybackService handles the event
   ↓
10. AudioPlaybackService plays/pauses
```

### Queue Management Flow

```
1. User adds confession to queue
   ↓
2. QueueNotifier.addConfession() called
   ↓
3. New AudioQueueItem created
   ↓
4. Item added to queue
   ↓
5. If queue was empty, auto-play the item
   ↓
6. Queue state updated
   ↓
7. UI updates via Riverpod
   ↓
8. User can see queue in panel
```

### Auto-Advance Flow

```
1. Audio playback completes
   ↓
2. AudioPlayerController detects PlayerState.completed
   ↓
3. QueueNotifier.playNext() called
   ↓
4. Next item determined based on:
   - Normal mode: next in sequence
   - Shuffle mode: random unplayed item
   - Repeat all: first item if at end
   ↓
5. AudioPlayerController.playAsset() called for next item
   ↓
6. Playback continues
```

---

## Integration Guide

### Step 1: Add Dependencies

Ensure these dependencies are in your `pubspec.yaml`:

```yaml
dependencies:
  flutter:
    sdk: flutter
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

### Step 2: Update Android Configuration

Add to `android/app/build.gradle`:

```gradle
android {
    defaultConfig {
        // Add this for foreground service
        manifestPlaceholders = [
            justAudioBackgroundChannelId: 'i_confess_audio',
            justAudioBackgroundChannelName: 'I-Confess Audio',
        ]
    }
}
```

### Step 3: Update iOS Configuration

Add to `ios/Runner/Info.plist`:

```xml
<key>UIBackgroundModes</key>
<array>
  <string>audio</string>
  <string>airplay</string>
</array>
```

### Step 4: Initialize Background Service

In your app initialization (e.g., `main.dart`):

```dart
import 'package:i_confess/src/features/audio/audio.dart';

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
  
  runApp(MyApp(player: player));
}
```

### Step 5: Integrate Queue with Player

Update your audio player controller to handle queue auto-advance:

```dart
// In audio_player_controller.dart
// Add this to the init() method:
_player.playbackEventStream.listen((event) {
  if (event.processingState == ProcessingState.completed) {
    // Playback completed, advance to next
    final queueController = ref.read(queueControllerProvider.notifier);
    queueController.playNext();
  }
});
```

### Step 6: Add Queue Button to Player

```dart
// In your player UI
Row(
  children: [
    // ... other controls
    QueueButton(),
    QueueProgress(),
  ],
)
```

---

## Testing

### Background Playback Tests

```dart
// Test cases to implement:
1. Audio continues playing when app goes to background
2. Notification shows correct information
3. Notification controls work (play/pause/stop)
4. Audio session is properly configured
5. Interruptions are handled correctly
```

### Queue Management Tests

```dart
// Test cases to implement:
1. Add item to queue
2. Add multiple items to queue
3. Remove item from queue
4. Reorder items in queue
5. Clear queue
6. Play next/previous
7. Toggle shuffle
8. Cycle repeat modes
9. Auto-advance on completion
10. Resume from last position
```

### Running Tests

```bash
# Run all audio tests
flutter test test/features/audio/

# Run with coverage
flutter test --coverage test/features/audio/
```

---

## Configuration

### Background Playback Configuration

```dart
final config = BackgroundPlaybackConfig(
  enabled: true,
  showNotification: true,
  notificationTitle: 'I-Confess',
  notificationSubtitle: 'Audio Confession',
  notificationImageUrl: 'https://example.com/icon.png',
  showPlayPauseControls: true,
  showSkipControls: true,
  showStopControl: true,
);
```

### Queue Configuration

The queue controller can be configured through Riverpod providers:

```dart
// Access queue state
final queue = ref.watch(currentQueueProvider);
final queueLength = ref.watch(queueLengthProvider);
final isShuffled = ref.watch(isShuffledProvider);
final repeatMode = ref.watch(repeatModeProvider);
```

---

## Usage Examples

### Basic Queue Usage

```dart
import 'package:i_confess/src/features/audio/audio.dart';

// Add a confession to the queue
final queueController = ref.read(queueControllerProvider.notifier);
queueController.addConfession(
  confessionId: 'confession_123',
  voiceId: 'voice_456',
);

// Add an asset to the queue
queueController.addAsset(
  asset: myAudioAsset,
  playNext: true, // Play this next
);

// Play a specific item
queueController.playItemAt(2);

// Control playback
queueController.playNext();
queueController.playPrevious();
queueController.toggleShuffle();
queueController.cycleRepeatMode();
```

### Show Queue Panel

```dart
// Show queue as bottom sheet
showModalBottomSheet(
  context: context,
  isScrollControlled: true,
  builder: (context) => const QueuePanel(),
);

// Show queue as full screen
Navigator.of(context).push(
  MaterialPageRoute(
    builder: (context) => const QueuePanel(fullScreen: true),
  ),
);
```

### Queue Button in App Bar

```dart
AppBar(
  actions: [
    QueueButton(),
  ],
)
```

### Queue Controls in Player

```dart
Row(
  children: [
    IconButton(
      icon: Icon(queue.repeatMode.icon),
      onPressed: queueController.cycleRepeatMode,
    ),
    IconButton(
      icon: Icon(queue.isShuffled ? Icons.shuffle_on : Icons.shuffle),
      onPressed: queueController.toggleShuffle,
    ),
    QueueProgress(),
  ],
)
```

---

## Platform-Specific Notes

### Android

1. **Foreground Service**: Required for background playback on Android 8+
2. **Notification Channel**: Must be created for the notification
3. **Permissions**: WAKE_LOCK and FOREGROUND_SERVICE required
4. **Testing**: Test on multiple Android versions (8.0+)

### iOS

1. **Background Modes**: Must be enabled in Xcode
2. **Info.plist**: Must include UIBackgroundModes
3. **Audio Session**: Properly configured for background audio
4. **Testing**: Test on multiple iOS versions (12.0+)

### Web

Note: Background playback has limited support on web. The service will gracefully degrade.

---

## Troubleshooting

### Common Issues

1. **Audio stops when app goes to background**
   - Check Android foreground service configuration
   - Check iOS background modes
   - Verify audio session is active

2. **Notification not showing**
   - Check notification channel creation
   - Verify showNotification is true
   - Check Android notification permissions

3. **Notification controls not working**
   - Verify notification is properly configured
   - Check background service initialization
   - Test on physical device (emulators may have issues)

4. **Queue not auto-advancing**
   - Check playback event listener
   - Verify playNext() is being called
   - Check repeat mode settings

### Debugging

Enable debug logging:

```dart
// In background_playback_service.dart
debugPrint = (String? message, {int wrapWidth}) {
  print('[BackgroundPlayback] $message');
};

// In queue_controller.dart
debugPrint = (String? message, {int wrapWidth}) {
  print('[QueueController] $message');
};
```

Check logs for:
- Audio session state changes
- Background state changes
- Notification updates
- Queue operations

---

## Next Steps

### Phase 3 Completion Checklist

- [x] Create BackgroundPlaybackService
- [x] Create AudioQueue model
- [x] Create QueueNotifier
- [x] Create QueuePanel widget
- [x] Create QueueButton widget
- [x] Create QueueControls widget
- [ ] Add background playback tests
- [ ] Add queue management tests
- [ ] Update audio player to integrate with queue
- [ ] Test on Android and iOS
- [ ] Document API

### Phase 4 Features

After completing Phase 3, consider:

1. **Playback History** - Track listening history
2. **Bookmarks** - Save positions in audio
3. **Offline Mode** - Cache audio for offline playback
4. **Crossfade** - Smooth transitions between tracks
5. **Equalizer** - Audio equalization controls

---

## API Reference

### BackgroundPlaybackService

| Method | Description |
|--------|-------------|
| `init()` | Initialize the service |
| `enable()` | Enable background playback |
| `disable()` | Disable background playback |
| `updateAssetInfo()` | Update notification with asset info |
| `handleNotificationPlay()` | Handle play from notification |
| `handleNotificationPause()` | Handle pause from notification |
| `handleNotificationSkipNext()` | Handle skip next from notification |
| `handleNotificationSkipPrevious()` | Handle skip previous from notification |
| `handleNotificationStop()` | Handle stop from notification |
| `dispose()` | Clean up resources |

### QueueNotifier

| Method | Description |
|--------|-------------|
| `addItem()` | Add an item to the queue |
| `addAsset()` | Add an asset to the queue |
| `addConfession()` | Add a confession to the queue |
| `removeItemAt()` | Remove item at index |
| `removeItemById()` | Remove item by ID |
| `clear()` | Clear the entire queue |
| `moveItem()` | Move item from one index to another |
| `playItemAt()` | Play item at specific index |
| `playNext()` | Play the next item |
| `playPrevious()` | Play the previous item |
| `toggleShuffle()` | Toggle shuffle mode |
| `cycleRepeatMode()` | Cycle to next repeat mode |
| `setRepeatMode()` | Set specific repeat mode |
| `markCurrentAsPlayed()` | Mark current item as played |
| `updateCurrentPosition()` | Update last position of current item |

### Providers

| Provider | Type | Description |
|----------|------|-------------|
| `queueControllerProvider` | StateNotifierProvider | Queue controller |
| `currentQueueProvider` | Provider | Current queue |
| `currentQueueItemProvider` | Provider | Current queue item |
| `isQueueEmptyProvider` | Provider | Whether queue is empty |
| `isQueueNotEmptyProvider` | Provider | Whether queue is not empty |
| `isShuffledProvider` | Provider | Whether shuffle is enabled |
| `repeatModeProvider` | Provider | Current repeat mode |
| `queueLengthProvider` | Provider | Queue length |

---

## Conclusion

Phase 3 adds **Background Playback** and **Queue Management** - two of the most important features for a great audio experience. These features will allow users to:

- Listen to confessions while using other apps
- Control playback from notifications and lock screen
- Create playlists of confessions
- Shuffle and repeat their queue

With Phase 3 complete, the I-Confess audio platform will be on par with major audio apps in terms of core functionality.

---

**Status**: Implementation in progress  
**Next**: Complete tests and final integration  
**Target Completion**: This week  
**Priority**: High

---

## Appendices

### Appendix A: Required Dependencies

```yaml
dependencies:
  flutter: sdk: flutter
  flutter_riverpod: ^2.4.9
  just_audio: ^0.9.34
  just_audio_background: ^0.0.1-beta.10
  audio_session: ^0.1.16
  http: ^1.1.0
```

### Appendix B: Platform Configuration Checklist

**Android:**
- [ ] Add WAKE_LOCK permission
- [ ] Add FOREGROUND_SERVICE permission
- [ ] Add foreground service to manifest
- [ ] Configure notification channel
- [ ] Test on Android 8.0+

**iOS:**
- [ ] Add background modes capability
- [ ] Update Info.plist
- [ ] Configure audio session
- [ ] Test on iOS 12.0+

### Appendix C: Test Plan

**Unit Tests:**
- [ ] BackgroundPlaybackService initialization
- [ ] BackgroundPlaybackService notification updates
- [ ] QueueNotifier add/remove operations
- [ ] QueueNotifier play next/previous
- [ ] QueueNotifier shuffle and repeat

**Integration Tests:**
- [ ] Background playback on Android
- [ ] Background playback on iOS
- [ ] Notification controls
- [ ] Queue auto-advance
- [ ] Queue persistence

**UI Tests:**
- [ ] Queue panel display
- [ ] Queue button functionality
- [ ] Queue controls
- [ ] Queue item interactions
