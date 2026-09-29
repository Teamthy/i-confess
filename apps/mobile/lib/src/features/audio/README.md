# Audio Feature

> Complete audio playback and generation integration for I-Confess

## Overview

This package provides a comprehensive audio solution for the I-Confess mobile application, including:

- Audio playback with full controls
- Audio generation and job management
- Signed URL handling for secure streaming
- Riverpod-based state management
- Customizable UI widgets

## Installation

This feature is already integrated into the I-Confess mobile app. No additional installation is required.

## Quick Start

### Import the package

```dart
import 'package:i_confess/src/features/audio/audio.dart';
```

### Play an audio asset

```dart
// Using the widget
AudioPlayerWidget(
  assetId: 'asset_123',
  confessionId: 'confession_456',
  voiceId: 'voice_789',
),

// Or programmatically
final controller = ref.read(audioPlayerProvider.notifier);
await controller.playAsset(
  assetId: 'asset_123',
  confessionId: 'confession_456',
  voiceId: 'voice_789',
);
```

### Generate and play audio

```dart
final controller = ref.read(audioPlayerProvider.notifier);
await controller.generateAndPlay(
  confessionId: 'confession_123',
  voiceId: 'elevenlabs_voice',
  provider: 'elevenlabs',
  qualityTier: 'premium',
);
```

## Features

### Playback
- ✅ Play/pause/resume/stop
- ✅ Seek to position
- ✅ Volume control
- ✅ Playback speed control
- ✅ Mute toggle
- ✅ Progress tracking

### Generation
- ✅ Create generation jobs
- ✅ Poll job status
- ✅ Retry failed jobs
- ✅ Cancel pending jobs
- ✅ Batch operations

### UI
- ✅ Full player widget
- ✅ Compact player widget
- ✅ Generation progress indicator
- ✅ Customizable styling

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│                    UI Layer (Widgets)                       │
│  audio_player_widget.dart                                 │
├─────────────────────────────────────────────────────────┤
│                 State Management Layer                     │
│  audio_providers.dart                                    │
├─────────────────────────────────────────────────────────┤
│                   Controller Layer                          │
│  audio_player_controller.dart                            │
├─────────────────────────────────────────────────────────┤
│                   Service Layer                            │
│  audio_generation_service.dart                           │
│  audio_url_service.dart                                  │
├─────────────────────────────────────────────────────────┤
│                 Playback Layer                            │
│  ../player/audio_playback_service.dart                   │
└─────────────────────────────────────────────────────────┘
```

## API Reference

### AudioPlayerNotifier

Main controller for audio playback.

#### Properties

```dart
AudioPlayerState state
bool isPlaying
bool isLoading
bool isPaused
double positionPercentage
String displayPosition
String displayDuration
String? error
```

#### Methods

```dart
// Playback
Future<void> playAsset({required String assetId, String? confessionId, String? voiceId, Duration? initialPosition})
Future<void> playConfession({required String confessionId, String? voiceId, Duration? initialPosition})
Future<void> generateAndPlay({required String confessionId, String? contentVersionId, String? variantId, required String voiceId, String provider, String qualityTier})

// Controls
Future<void> pause()
Future<void> resume()
Future<void> stop()
Future<void> seek(Duration position)

// Settings
Future<void> setVolume(double volume)
Future<void> setPlaybackSpeed(double speed)
Future<void> toggleMute()
```

### Providers

#### Service Providers

```dart
audioPlayerProvider                    // Main player controller
audioGenerationServiceProvider        // Audio generation service
audioUrlServiceProvider               // Audio URL service
```

#### State Providers

```dart
isPlayingProvider                      // bool - whether playing
isLoadingProvider                      // bool - whether loading
isPausedProvider                       // bool - whether paused
positionPercentageProvider             // double - position as %
displayPositionProvider                // String - formatted position
displayDurationProvider                // String - formatted duration
audioErrorProvider                     // String? - current error
```

#### Data Providers

```dart
currentAudioJobProvider                // AudioGenerationJob? - current job
currentAudioAssetProvider              // AudioAsset? - current asset
ttsProvidersProvider                   // List<TtsProvider> - TTS providers
audioJobsProvider                      // List<AudioGenerationJob> - all jobs
audioGenerationStatsProvider          // Map<String, dynamic> - stats
```

#### Action Providers

```dart
createAudioJobProvider(request)         // Create a new job
getAudioJobProvider(jobId)             // Get a job by ID
retryAudioJobProvider(jobId)           // Retry a job
cancelAudioJobProvider(jobId)          // Cancel a job
createBatchAudioJobsProvider(request)  // Create batch jobs
```

## Widgets

### AudioPlayerWidget

Full-featured audio player UI.

```dart
AudioPlayerWidget({
  Key? key,
  String? assetId,
  String? confessionId,
  String? voiceId,
  bool compact = false,
  bool showGenerationControls = true,
  VoidCallback? onClose,
})
```

**Parameters:**
- `assetId` - Audio asset ID to play
- `confessionId` - Confession ID to play
- `voiceId` - Voice ID to use
- `compact` - Whether to show compact UI
- `showGenerationControls` - Whether to show generation controls
- `onClose` - Callback when player is closed

### AudioGenerationProgress

Shows generation progress for a job.

```dart
AudioGenerationProgress({
  Key? key,
  required String jobId,
  Function(AudioGenerationJob)? onComplete,
})
```

**Parameters:**
- `jobId` - Job ID to track
- `onComplete` - Callback when generation completes

## Models

### AudioGenerationJob

Represents an audio generation job.

```dart
class AudioGenerationJob {
  final String id;
  final String confessionId;
  final String voiceId;
  final AudioJobStatus status;
  final String? audioAssetId;
  final double? progress;
  final String? errorMessage;
  final DateTime createdAt;
  final DateTime updatedAt;
}
```

**Status Values:**
- `queued` - Job is waiting to start
- `processing` - Job is being processed
- `succeeded` - Job completed successfully
- `failed` - Job failed
- `cancelled` - Job was cancelled
- `pending` - Job is pending

### AudioAsset

Represents an audio asset.

```dart
class AudioAsset {
  final String id;
  final String confessionId;
  final String voiceId;
  final AudioAssetStatus status;
  final double duration;
  final int fileSize;
  final String storagePath;
  final DateTime createdAt;
}
```

**Status Values:**
- `pending` - Asset is pending
- `processing` - Asset is being processed
- `ready` - Asset is ready for playback
- `failed` - Asset processing failed

### AudioGenerationRequest

Request to generate audio.

```dart
class AudioGenerationRequest {
  final String confessionId;
  final String? contentVersionId;
  final String? variantId;
  final String voiceId;
  final String provider;
  final String qualityTier;
}
```

### TtsProvider

TTS provider configuration.

```dart
class TtsProvider {
  final String id;
  final String name;
  final bool isActive;
  final List<String> qualityTiers;
  final Map<String, dynamic> config;
}
```

## Services

### AudioGenerationService

Manages audio generation jobs.

```dart
class AudioGenerationService {
  Future<AudioGenerationJob> createJob(AudioGenerationRequest request)
  Future<AudioGenerationJob> getJob(String jobId)
  Future<List<AudioGenerationJob>> getJobs()
  Future<AudioGenerationJob> retryJob(String jobId)
  Future<AudioGenerationJob> cancelJob(String jobId)
  Future<Map<String, dynamic>> getStats()
  Future<List<TtsProvider>> getTtsProviders()
  Future<List<AudioGenerationJob>> createBatchJobs(BatchAudioGenerationRequest request)
  Future<AudioGenerationJob> pollJobUntilComplete({required String jobId, int interval, int timeout})
}
```

### AudioUrlService

Manages signed URLs for audio streaming.

```dart
class AudioUrlService {
  String getStreamUrl(String assetId)
  String getDownloadUrl(String assetId)
  String getSignedUrl(String assetId, int expirationSeconds)
  String getSignedUrlWithOptions(String assetId, int expirationSeconds, Map<String, dynamic> options)
  Future<void> invalidateUrl(String assetId)
}
```

## Testing

Run the tests:

```bash
# Run all audio tests
flutter test test/features/audio/

# Run with coverage
flutter test --coverage test/features/audio/
```

## Examples

### Basic Playback

```dart
import 'package:i_confess/src/features/audio/audio.dart';

class MyAudioPage extends ConsumerWidget {
  @override
  Widget build(BuildContext context, WidgetRef ref) {
    return Scaffold(
      appBar: AppBar(title: Text('Audio Player')),
      body: Center(
        child: AudioPlayerWidget(
          confessionId: 'confession_123',
          voiceId: 'voice_456',
        ),
      ),
    );
  }
}
```

### Advanced Usage

```dart
class AdvancedAudioPage extends ConsumerWidget {
  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final controller = ref.read(audioPlayerProvider.notifier);
    final isPlaying = ref.watch(isPlayingProvider);
    final position = ref.watch(displayPositionProvider);
    final duration = ref.watch(displayDurationProvider);
    
    return Scaffold(
      appBar: AppBar(title: Text('Advanced Player')),
      body: Column(
        children: [
          Text('$position / $duration'),
          Slider(
            value: ref.watch(positionPercentageProvider),
            onChanged: (value) {
              final totalDuration = controller.state.duration;
              controller.seek(Duration(
                milliseconds: (totalDuration.inMilliseconds * value).round(),
              ));
            },
          ),
          Row(
            mainAxisAlignment: MainAxisAlignment.center,
            children: [
              IconButton(
                icon: Icon(Icons.skip_previous),
                onPressed: () {},
              ),
              IconButton(
                icon: Icon(isPlaying ? Icons.pause : Icons.play_arrow),
                onPressed: () async {
                  if (isPlaying) {
                    await controller.pause();
                  } else {
                    await controller.resume();
                  }
                },
              ),
              IconButton(
                icon: Icon(Icons.skip_next),
                onPressed: () {},
              ),
            ],
          ),
        ],
      ),
    );
  }
}
```

### Generation and Play

```dart
class GenerateAndPlayPage extends ConsumerWidget {
  final String confessionId;
  final String voiceId;

  const GenerateAndPlayPage({
    super.key,
    required this.confessionId,
    required this.voiceId,
  });

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final controller = ref.read(audioPlayerProvider.notifier);
    
    return Scaffold(
      appBar: AppBar(title: Text('Generate & Play')),
      body: Center(
        child: ElevatedButton(
          onPressed: () async {
            await controller.generateAndPlay(
              confessionId: confessionId,
              voiceId: voiceId,
              provider: 'elevenlabs',
              qualityTier: 'premium',
            );
          },
          child: Text('Generate and Play'),
        ),
      ),
    );
  }
}
```

## Troubleshooting

### Common Issues

1. **Audio not playing**
   - Check that the asset ID is valid
   - Verify the signed URL is being generated
   - Ensure network connectivity

2. **Generation stuck**
   - Check job status via API
   - Verify generation service is running
   - Check error messages

3. **Playback errors**
   - Check `audioErrorProvider` for error messages
   - Verify audio format is supported
   - Ensure proper permissions

### Debugging

Enable debug logging:

```dart
// In your code
final error = ref.watch(audioErrorProvider);
if (error != null) {
  print('Audio Error: $error');
}

// Or use the logger
import 'package:logging/logging.dart';
final logger = Logger('AudioPlayer');
logger.info('Playing asset: ${assetId}');
```

## Configuration

### Environment

```dart
// In main.dart or configuration
const String.fromEnvironment('API_BASE_URL');
```

### Android

Add to `AndroidManifest.xml`:

```xml
<uses-permission android:name="android.permission.INTERNET"/>
<uses-permission android:name="android.permission.WAKE_LOCK"/>
```

### iOS

Add to `Info.plist`:

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

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Run tests: `flutter test test/features/audio/`
5. Submit a pull request

## License

This code is part of the I-Confess application and is proprietary.

## Support

For issues or questions:
- Check the [documentation](../docs/PHASE2-FLUTTER-AUDIO-PLAYER.md)
- Review the [implementation summary](../docs/PHASE2-IMPLEMENTATION-SUMMARY.md)
- Contact the development team
