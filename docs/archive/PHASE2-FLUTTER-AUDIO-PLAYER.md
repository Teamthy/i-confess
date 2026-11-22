# Phase 2: Flutter Audio Player Integration - Implementation Summary

## Overview

This document describes the implementation of Phase 2: Flutter Audio Player Integration for the I-Confess application. This phase builds upon Phase 1 (Core Audio Services) by adding a complete audio playback layer in the Flutter mobile application.

## Table of Contents

1. [Architecture](#architecture)
2. [Components](#components)
3. [File Structure](#file-structure)
4. [Integration Points](#integration-points)
5. [Usage Examples](#usage-examples)
6. [Testing](#testing)
7. [Next Steps](#next-steps)

---

## Architecture

The Flutter audio integration follows a clean architecture pattern with the following layers:

```
┌─────────────────────────────────────────────────────────┐
│                    UI Layer (Widgets)                       │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────┐ │
│  │ AudioPlayer │  │ Generation   │  │ Compact Player   │ │
│  │   Widget    │  │  Progress    │  │    Widget        │ │
│  └─────────────┘  └─────────────┘  └─────────────────┘ │
├─────────────────────────────────────────────────────────┤
│                 State Management Layer                     │
│  ┌─────────────────────────────────────────────────────┐ │
│  │ Riverpod Providers & Notifiers                        │ │
│  │ - audioPlayerControllerProvider                       │ │
│  │ - audioGenerationServiceProvider                      │ │
│  │ - audioUrlServiceProvider                             │ │
│  │ - Various state providers                              │ │
│  └─────────────────────────────────────────────────────┘ │
├─────────────────────────────────────────────────────────┤
│                   Service Layer                            │
│  ┌───────────────────┐  ┌─────────────────────────────┐ │
│  │ AudioGeneration   │  │ AudioUrlService              │ │
│  │    Service        │  │                             │ │
│  └───────────────────┘  └─────────────────────────────┘ │
├─────────────────────────────────────────────────────────┤
│                 Playback Layer (Existing)                  │
│  ┌─────────────────────────────────────────────────────┐ │
│  │ AudioPlaybackService (from Phase 1)                   │ │
│  │ - just_audio integration                             │ │
│  │ - audio_session management                           │ │
│  └─────────────────────────────────────────────────────┘ │
├─────────────────────────────────────────────────────────┤
│                 API Client Layer                          │
│  ┌─────────────────────────────────────────────────────┐ │
│  │ Dart API Client (updated in Phase 1)                  │ │
│  │ - Audio generation endpoints                         │ │
│  └─────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────┘
```

---

## Components

### 1. Models (Existing from Phase 1)

Located in: `apps/mobile/lib/src/features/audio/models/`

- **audio_generation_job.dart** - Audio generation job model with status tracking
- **audio_asset.dart** - Audio asset model with metadata
- **audio_generation_request.dart** - Request models for audio generation
- **tts_provider.dart** - TTS provider configuration model

### 2. Services (Existing from Phase 1)

Located in: `apps/mobile/lib/src/features/audio/services/`

- **audio_generation_service.dart** - Service for managing audio generation jobs
- **audio_url_service.dart** - Service for managing signed URLs for audio streaming

### 3. New Controllers

Located in: `apps/mobile/lib/src/features/audio/controllers/`

- **audio_player_controller.dart** - Main controller for audio playback
  - Manages player state (playing, paused, stopped, loading, error)
  - Handles playback controls (play, pause, resume, stop, seek)
  - Manages audio settings (volume, playback speed, mute)
  - Integrates with generation service for generate-and-play workflow
  - Provides formatted display strings for position and duration

### 4. New Providers

Located in: `apps/mobile/lib/src/features/audio/providers/`

- **audio_providers.dart** - Riverpod providers for audio features
  - Service providers (generation, URL)
  - State providers (current job, current asset)
  - Derived state providers (isPlaying, isLoading, positionPercentage, etc.)
  - Family providers for job operations (create, get, retry, cancel)

### 5. New Widgets

Located in: `apps/mobile/lib/src/features/audio/widgets/`

- **audio_player_widget.dart** - Main audio player UI component
  - Full player mode with progress bar and controls
  - Compact mode for inline/minimized display
  - Generation progress indicator
  - Volume and playback speed controls
  - Error state display

### 6. Barrel File

Located in: `apps/mobile/lib/src/features/audio/audio.dart`

- Exports all audio feature components for easy importing

---

## File Structure

```
apps/mobile/lib/src/features/audio/
├── audio.dart                          # Barrel file
├── controllers/
│   └── audio_player_controller.dart   # Main player controller
├── models/                            # Existing from Phase 1
│   ├── audio_asset.dart
│   ├── audio_generation_job.dart
│   ├── audio_generation_request.dart
│   └── tts_provider.dart
├── providers/
│   └── audio_providers.dart            # Riverpod providers
├── services/                          # Existing from Phase 1
│   ├── audio_generation_service.dart
│   └── audio_url_service.dart
└── widgets/
    └── audio_player_widget.dart        # Player UI widgets

apps/mobile/test/features/audio/
└── audio_player_test.dart             # Integration tests
```

---

## Integration Points

### 1. With Existing Playback Infrastructure

The new audio player controller integrates with the existing `AudioPlaybackService` from Phase 1:

```dart
// In audio_player_controller.dart
final AudioPlaybackService _playbackService;

// Uses the existing service for:
- Loading audio from URLs
- Playback control (play, pause, stop, seek)
- Volume and speed control
- Position and duration streaming
- Error handling
```

### 2. With Backend API

The controller integrates with backend services through the existing API client:

```dart
// In audio_generation_service.dart (existing)
- createJob() - Creates new audio generation job
- getJob() - Gets job status
- pollJobUntilComplete() - Polls job until completion
- getJobs() - Lists all jobs
- retryJob() - Retries a failed job
- cancelJob() - Cancels a pending job
- getStats() - Gets generation statistics
- getTtsProviders() - Lists available TTS providers
```

### 3. With Signed URL Service

The controller uses the `AudioUrlService` to get signed URLs for audio streaming:

```dart
// In audio_player_controller.dart
final url = await _urlService.getStreamUrl(assetId);
await _playbackService.load(url);
```

---

## Usage Examples

### Basic Playback

```dart
import 'package:i_confess/src/features/audio/audio.dart';

// Play an audio asset
final controller = ref.read(audioPlayerProvider.notifier);
await controller.playAsset(
  assetId: 'asset_123',
  confessionId: 'confession_456',
  voiceId: 'voice_789',
);
```

### Play a Confession

```dart
// Play a confession (auto-generates if needed)
await controller.playConfession(
  confessionId: 'confession_123',
  voiceId: 'elevenlabs_voice',
);
```

### Generate and Play

```dart
// Generate audio and play immediately
await controller.generateAndPlay(
  confessionId: 'confession_123',
  voiceId: 'elevenlabs_voice',
  provider: 'elevenlabs',
  qualityTier: 'standard',
);
```

### Using the Player Widget

```dart
// Full player
AudioPlayerWidget(
  confessionId: 'confession_123',
  voiceId: 'voice_456',
  onClose: () => Navigator.pop(context),
),

// Compact player
AudioPlayerWidget(
  assetId: 'asset_123',
  compact: true,
),

// Generation progress
AudioGenerationProgress(
  jobId: 'job_123',
  onComplete: (job) {
    // Play the generated audio
    ref.read(audioPlayerProvider.notifier).playAsset(
      assetId: job.audioAssetId!,
    );
  },
),
```

### Using Providers Directly

```dart
// Check if playing
final isPlaying = ref.watch(isPlayingProvider);

// Get current position as percentage
final percentage = ref.watch(positionPercentageProvider);

// Get display strings
final position = ref.watch(displayPositionProvider);
final duration = ref.watch(displayDurationProvider);

// Create a new job
final jobAsync = ref.watch(createAudioJobProvider(request));
```

### Control Playback

```dart
final controller = ref.read(audioPlayerProvider.notifier);

// Basic controls
await controller.play();
await controller.pause();
await controller.stop();
await controller.seek(Duration(seconds: 30));

// Audio settings
await controller.setVolume(0.5);
await controller.setPlaybackSpeed(1.5);
await controller.toggleMute();
```

---

## Testing

### Integration Tests

Located in: `apps/mobile/test/features/audio/audio_player_test.dart`

The test file covers:

1. **Initial State**
   - Verifies initial state is idle
   - Verifies default values

2. **Playback Operations**
   - `playAsset()` updates state correctly
   - `playConfession()` calls playAsset with generated ID
   - `generateAndPlay()` creates job and plays on success
   - All playback controls (pause, resume, stop, seek)

3. **Audio Settings**
   - Volume control
   - Playback speed control
   - Mute toggle

4. **State Management**
   - Position percentage calculation
   - Display string formatting
   - State copying

### Running Tests

```bash
# Run all audio tests
flutter test test/features/audio/

# Run with coverage
flutter test --coverage test/features/audio/
```

---

## State Management

### AudioPlayerState

The main state class contains:

```dart
class AudioPlayerState {
  final String? currentAssetId;        // Currently playing asset ID
  final String? currentConfessionId;    // Currently playing confession ID
  final String? currentVoiceId;         // Currently playing voice ID
  final PlayerState playerState;      // Playback state
  final Duration position;            // Current playback position
  final Duration duration;            // Total duration
  final bool isBuffering;              // Whether audio is buffering
  final String? error;                 // Current error message
  final double volume;                // Current volume (0.0 - 1.0)
  final bool isMuted;                  // Whether audio is muted
  final double playbackSpeed;         // Current playback speed
}
```

### Derived State Providers

For convenience, the following derived state providers are available:

- `isPlayingProvider` - Whether audio is currently playing
- `isLoadingProvider` - Whether audio is loading/buffering
- `isPausedProvider` - Whether audio is paused
- `positionPercentageProvider` - Current position as percentage (0.0 - 1.0)
- `displayPositionProvider` - Formatted position string (MM:SS)
- `displayDurationProvider` - Formatted duration string (MM:SS)
- `audioErrorProvider` - Current error message

---

## Error Handling

The audio player handles errors at multiple levels:

1. **Playback Errors** - Errors from the underlying `just_audio` player
2. **URL Generation Errors** - Errors from the signed URL service
3. **Generation Errors** - Errors from the audio generation service
4. **Network Errors** - Errors from API calls

All errors are captured in the `AudioPlayerState.error` field and can be accessed via `audioErrorProvider`.

### Error Display

```dart
final error = ref.watch(audioErrorProvider);

if (error != null) {
  return Text(
    error,
    style: TextStyle(color: Colors.red),
  );
}
```

---

## Performance Considerations

1. **Stream Subscriptions** - All stream subscriptions are properly disposed
2. **Resource Cleanup** - The controller properly disposes all resources
3. **State Updates** - State updates are minimized to prevent unnecessary rebuilds
4. **Polling** - Generation job polling has configurable intervals and timeouts

---

## Dependencies

### Flutter Packages

- `flutter_riverpod: ^2.4.9` - State management
- `just_audio: ^0.9.34` - Audio playback (existing)
- `audio_session: ^0.1.16` - Audio session management (existing)

### Internal Dependencies

- `src/features/player/audio_playback_service.dart` - Existing playback service
- `src/core/services/api_service.dart` - API client service

---

## Configuration

### Required Configuration

Add the following to your environment configuration:

```dart
// In main.dart or configuration
const String.fromEnvironment('API_BASE_URL');
```

### Android Configuration

Ensure the following permissions are in `AndroidManifest.xml`:

```xml
<uses-permission android:name="android.permission.INTERNET"/>
<uses-permission android:name="android.permission.WAKE_LOCK"/>
```

### iOS Configuration

Ensure the following are in `Info.plist`:

```xml
<key>NSAppTransportSecurity</key>
<dict>
  <key>NSAllowsArbitraryLoads</key>
  <true/>
</dict>
<key>UIBackgroundModes</key>
<array>
  <string>audio</string>
</array>
```

---

## Next Steps

### Phase 3: Advanced Features

1. **Background Playback** - Implement background audio playback
2. **Queue Management** - Add audio queue with shuffle/repeat
3. **Playback History** - Track and display playback history
4. **Bookmarks** - Allow users to bookmark positions in audio
5. **Offline Mode** - Cache audio for offline playback

### Phase 4: UI/UX Enhancements

1. **Custom Styling** - Apply app theme to audio player
2. **Animations** - Add smooth transitions and animations
3. **Accessibility** - Ensure full accessibility support
4. **Localization** - Add support for multiple languages

---

## API Reference

### AudioPlayerNotifier Methods

| Method | Description |
|--------|-------------|
| `playAsset()` | Play an audio asset by ID |
| `playConfession()` | Play a confession by ID |
| `generateAndPlay()` | Generate audio and play immediately |
| `pause()` | Pause playback |
| `resume()` | Resume playback |
| `stop()` | Stop playback and reset |
| `seek()` | Seek to a specific position |
| `setVolume()` | Set playback volume |
| `setPlaybackSpeed()` | Set playback speed |
| `toggleMute()` | Toggle mute on/off |

### AudioPlayerNotifier Getters

| Getter | Description |
|--------|-------------|
| `currentState` | Get the current state |
| `positionPercentage` | Get position as percentage |
| `displayPosition` | Get formatted position string |
| `displayDuration` | Get formatted duration string |

### Providers

| Provider | Type | Description |
|----------|------|-------------|
| `audioPlayerProvider` | StateNotifierProvider | Main player controller |
| `audioGenerationServiceProvider` | Provider | Audio generation service |
| `audioUrlServiceProvider` | Provider | Audio URL service |
| `isPlayingProvider` | Provider | Whether audio is playing |
| `isLoadingProvider` | Provider | Whether audio is loading |
| `isPausedProvider` | Provider | Whether audio is paused |
| `positionPercentageProvider` | Provider | Position as percentage |
| `displayPositionProvider` | Provider | Formatted position |
| `displayDurationProvider` | Provider | Formatted duration |
| `audioErrorProvider` | Provider | Current error |

---

## Troubleshooting

### Common Issues

1. **Audio not playing**
   - Check that the asset ID is valid
   - Verify the signed URL is being generated correctly
   - Ensure the audio file exists and is accessible

2. **Generation stuck**
   - Check the job status via the API
   - Verify the generation service is running
   - Check network connectivity

3. **Playback errors**
   - Check the error message in `audioErrorProvider`
   - Verify audio format is supported
   - Ensure proper permissions are configured

### Debugging

Enable debug logging:

```dart
// In audio_player_controller.dart
print('Playing asset: ${assetId}');
print('URL: $url');
print('Player state: ${state.playerState}');
```

---

## Version History

| Version | Date | Changes |
|---------|------|---------|
| 1.0.0 | 2026-09-29 | Initial implementation |

---

## References

- [Phase 1 Documentation](PHASE1-AUDIO-IMPLEMENTATION.md)
- [Integration Summary](INTEGRATION-SUMMARY.md)
- [Test Results](TEST-RESULTS-PHASE1.md)
- [just_audio Documentation](https://pub.dev/packages/just_audio)
- [Riverpod Documentation](https://riverpod.dev/)
