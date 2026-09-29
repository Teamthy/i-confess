/// Audio player widget for the I-Confess app.
///
/// This widget provides a full-featured audio player UI with playback controls,
/// progress indicator, and audio generation capabilities.
import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../controllers/audio_player_controller.dart';
import '../providers/audio_providers.dart';
import '../models/audio_generation_job.dart';

/// Main audio player widget.
class AudioPlayerWidget extends ConsumerWidget {
  /// The ID of the confession to play.
  final String? confessionId;
  
  /// The ID of the voice to use for playback.
  final String? voiceId;
  
  /// The ID of the audio asset to play.
  final String? assetId;

  /// The immutable content snapshot required to request audio generation.
  final String? contentVersionId;

  /// Server session whose queue granted access to the signed audio URL.
  final String? sessionId;

  /// Optional session-queue item ID to resolve inside [sessionId].
  final String? sessionItemId;
  
  /// Whether to show the full player or a compact version.
  final bool compact;
  
  /// Whether to show generation controls.
  final bool showGenerationControls;
  
  /// Callback when the player is closed.
  final VoidCallback? onClose;

  const AudioPlayerWidget({
    super.key,
    this.confessionId,
    this.voiceId,
    this.assetId,
    this.contentVersionId,
    this.sessionId,
    this.sessionItemId,
    this.compact = false,
    this.showGenerationControls = true,
    this.onClose,
  });

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final playerState = ref.watch(audioPlayerProvider);
    final controller = ref.read(audioPlayerProvider.notifier);
    final isPlaying = ref.watch(isPlayingProvider);
    final isLoading = ref.watch(isLoadingProvider);
    final positionPercentage = ref.watch(positionPercentageProvider);
    final displayPosition = ref.watch(displayPositionProvider);
    final displayDuration = ref.watch(displayDurationProvider);
    final error = ref.watch(audioErrorProvider);

    // Auto-play if we have an asset ID or confession ID
    if (assetId != null && playerState.currentAssetId != assetId) {
      WidgetsBinding.instance.addPostFrameCallback((_) {
        unawaited(
          controller.playAsset(
            assetId: assetId!,
            sessionId: sessionId,
            sessionItemId: sessionItemId,
            confessionId: confessionId,
            voiceId: voiceId,
          ).catchError((Object error) {
            debugPrint('Audio playback failed: $error');
          }),
        );
      });
    } else if (confessionId != null && playerState.currentConfessionId != confessionId) {
      WidgetsBinding.instance.addPostFrameCallback((_) {
        unawaited(
          controller.playConfession(
            confessionId: confessionId!,
            sessionId: sessionId,
            sessionItemId: sessionItemId,
            voiceId: voiceId,
          ).catchError((Object error) {
            debugPrint('Confession playback failed: $error');
          }),
        );
      });
    }

    if (compact) {
      return _buildCompactPlayer(
        context,
        controller,
        isPlaying,
        isLoading,
        displayPosition,
      );
    }

    return _buildFullPlayer(
      context,
      controller,
      playerState,
      isPlaying,
      isLoading,
      positionPercentage,
      displayPosition,
      displayDuration,
      error,
    );
  }

  Widget _buildCompactPlayer(
    BuildContext context,
    AudioPlayerNotifier controller,
    bool isPlaying,
    bool isLoading,
    String displayPosition,
  ) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 8),
      decoration: BoxDecoration(
        color: Theme.of(context).cardColor,
        borderRadius: BorderRadius.circular(8),
        boxShadow: [
          BoxShadow(
            color: Colors.black.withOpacity(0.1),
            blurRadius: 4,
            offset: const Offset(0, 2),
          ),
        ],
      ),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          if (onClose != null)
            IconButton(
              icon: const Icon(Icons.close, size: 20),
              onPressed: onClose,
            ),
          IconButton(
            icon: isLoading
                ? const SizedBox(
                    width: 20,
                    height: 20,
                    child: CircularProgressIndicator(strokeWidth: 2),
                  )
                : Icon(isPlaying ? Icons.pause : Icons.play_arrow),
            onPressed: isLoading ? null : () async {
              if (isPlaying) {
                await controller.pause();
              } else {
                await controller.resume();
              }
            },
          ),
          const SizedBox(width: 8),
          Text(
            displayPosition,
            style: Theme.of(context).textTheme.bodySmall,
          ),
        ],
      ),
    );
  }

  Widget _buildFullPlayer(
    BuildContext context,
    AudioPlayerNotifier controller,
    AudioPlayerState playerState,
    bool isPlaying,
    bool isLoading,
    double positionPercentage,
    String displayPosition,
    String displayDuration,
    String? error,
  ) {
    final theme = Theme.of(context);
    final colorScheme = theme.colorScheme;

    return Column(
      mainAxisSize: MainAxisSize.min,
      children: [
        // Header with close button
        if (onClose != null)
          Row(
            mainAxisAlignment: MainAxisAlignment.end,
            children: [
              IconButton(
                icon: const Icon(Icons.close),
                onPressed: onClose,
              ),
            ],
          ),

        // Error message
        if (error != null)
          Padding(
            padding: const EdgeInsets.only(bottom: 16),
            child: Text(
              error,
              style: theme.textTheme.bodyMedium?.copyWith(
                color: colorScheme.error,
              ),
              textAlign: TextAlign.center,
            ),
          ),

        // Progress bar
        Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            SliderTheme(
              data: SliderTheme.of(context).copyWith(
                trackHeight: 4,
                thumbShape: const RoundSliderThumbShape(enabledThumbRadius: 8),
                overlayShape: const RoundSliderOverlayShape(overlayRadius: 16),
              ),
              child: Slider(
                value: positionPercentage.clamp(0.0, 1.0),
                onChanged: isLoading ? null : (value) async {
                  final duration = playerState.duration;
                  if (duration > Duration.zero) {
                    await controller.seek(Duration(
                      milliseconds: (duration.inMilliseconds * value).round(),
                    ));
                  }
                },
              ),
            ),
            Padding(
              padding: const EdgeInsets.symmetric(horizontal: 16),
              child: Row(
                mainAxisAlignment: MainAxisAlignment.spaceBetween,
                children: [
                  Text(
                    displayPosition,
                    style: theme.textTheme.bodySmall,
                  ),
                  Text(
                    displayDuration,
                    style: theme.textTheme.bodySmall,
                  ),
                ],
              ),
            ),
          ],
        ),

        const SizedBox(height: 24),

        // Main controls
        Row(
          mainAxisAlignment: MainAxisAlignment.center,
          children: [
            // Previous button (placeholder for future functionality)
            IconButton(
              icon: const Icon(Icons.skip_previous, size: 32),
              onPressed: null,
              color: colorScheme.onSurface.withOpacity(0.6),
            ),

            const SizedBox(width: 24),

            // Play/Pause button
            FloatingActionButton(
              onPressed: isLoading ? null : () async {
                if (isPlaying) {
                  await controller.pause();
                } else {
                  await controller.resume();
                }
              },
              backgroundColor: colorScheme.primary,
              foregroundColor: colorScheme.onPrimary,
              child: isLoading
                  ? const CircularProgressIndicator(
                      valueColor: AlwaysStoppedAnimation(Colors.white),
                    )
                  : Icon(isPlaying ? Icons.pause : Icons.play_arrow, size: 32),
            ),

            const SizedBox(width: 24),

            // Next button (placeholder for future functionality)
            IconButton(
              icon: const Icon(Icons.skip_next, size: 32),
              onPressed: null,
              color: colorScheme.onSurface.withOpacity(0.6),
            ),
          ],
        ),

        const SizedBox(height: 24),

        // Additional controls
        if (!compact)
          Row(
            mainAxisAlignment: MainAxisAlignment.center,
            children: [
              // Volume control
              IconButton(
                icon: const Icon(Icons.volume_up),
                onPressed: () {
                  // Show volume slider
                  showModalBottomSheet(
                    context: context,
                    builder: (context) => _buildVolumeSlider(
                      context,
                      controller,
                      playerState.volume,
                    ),
                  );
                },
              ),

              const SizedBox(width: 16),

              // Playback speed
              IconButton(
                icon: const Icon(Icons.speed),
                onPressed: () {
                  // Show speed options
                  showModalBottomSheet(
                    context: context,
                    builder: (context) => _buildSpeedOptions(
                      context,
                      controller,
                      playerState.playbackSpeed,
                    ),
                  );
                },
              ),

              const SizedBox(width: 16),

              // Generation controls
              if (showGenerationControls &&
                  confessionId != null &&
                  contentVersionId != null)
                IconButton(
                  icon: const Icon(Icons.refresh),
                  onPressed: () async {
                    try {
                      await controller.generateAndPlay(
                        confessionId: confessionId!,
                        contentVersionId: contentVersionId,
                        voiceId: voiceId ?? 'default',
                        sessionId: sessionId,
                        sessionItemId: sessionItemId,
                      );
                    } on Object catch (error) {
                      debugPrint('Audio generation failed: $error');
                    }
                  },
                ),
            ],
          ),
      ],
    );
  }

  Widget _buildVolumeSlider(
    BuildContext context,
    AudioPlayerNotifier controller,
    double currentVolume,
  ) {

    return Padding(
      padding: const EdgeInsets.all(16),
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          Text(
            'Volume',
            style: Theme.of(context).textTheme.titleMedium,
          ),
          const SizedBox(height: 16),
          Slider(
            value: currentVolume,
            min: 0.0,
            max: 1.0,
            onChanged: (value) async {
              await controller.setVolume(value);
            },
          ),
          const SizedBox(height: 8),
          Row(
            mainAxisAlignment: MainAxisAlignment.spaceBetween,
            children: [
              IconButton(
                icon: const Icon(Icons.volume_mute),
                onPressed: () async {
                  await controller.setVolume(0.0);
                },
              ),
              IconButton(
                icon: const Icon(Icons.volume_up),
                onPressed: () async {
                  await controller.setVolume(1.0);
                },
              ),
            ],
          ),
        ],
      ),
    );
  }

  Widget _buildSpeedOptions(
    BuildContext context,
    AudioPlayerNotifier controller,
    double currentSpeed,
  ) {
    final speeds = [0.5, 0.75, 1.0, 1.25, 1.5, 2.0];

    return Padding(
      padding: const EdgeInsets.all(16),
      child: Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Text(
            'Playback Speed',
            style: Theme.of(context).textTheme.titleMedium,
            textAlign: TextAlign.center,
          ),
          const SizedBox(height: 16),
          ...speeds.map((speed) => Padding(
            padding: const EdgeInsets.symmetric(vertical: 4),
            child: ListTile(
              title: Text('${speed}x'),
              selected: currentSpeed == speed,
              selectedTileColor: Theme.of(context).colorScheme.primaryContainer,
              onTap: () async {
                await controller.setPlaybackSpeed(speed);
                Navigator.of(context).pop();
              },
            ),
          )).toList(),
        ],
      ),
    );
  }
}

/// Audio generation progress widget.
class AudioGenerationProgress extends ConsumerWidget {
  /// The ID of the generation job.
  final String jobId;

  /// Callback when generation is complete.
  final Function(AudioGenerationJob)? onComplete;

  const AudioGenerationProgress({
    super.key,
    required this.jobId,
    this.onComplete,
  });

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final jobAsync = ref.watch(getAudioJobProvider(jobId));

    return jobAsync.when(
      loading: () => const Center(
        child: CircularProgressIndicator(),
      ),
      error: (error, stack) => Center(
        child: Text('Error: $error'),
      ),
      data: (job) {
        if (job.status == AudioJobStatus.succeeded) {
          WidgetsBinding.instance.addPostFrameCallback((_) {
            onComplete?.call(job);
          });
          return const Center(
            child: Icon(Icons.check_circle, color: Colors.green, size: 48),
          );
        }

        return Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            const CircularProgressIndicator(),
            const SizedBox(height: 16),
            Text(
              'Generating Audio...',
              style: Theme.of(context).textTheme.bodyLarge,
            ),
            const SizedBox(height: 8),
            Text(
              'Status: ${job.status.name}',
              style: Theme.of(context).textTheme.bodySmall,
            ),
            if (job.progress != null)
              Column(
                children: [
                  const SizedBox(height: 8),
                  LinearProgressIndicator(value: job.progress),
                  const SizedBox(height: 4),
                  Text('${(job.progress! * 100).toStringAsFixed(1)}%'),
                ],
              ),
          ],
        );
      },
    );
  }
}
