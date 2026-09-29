/// Queue panel widget for displaying and managing the audio queue.
///
/// This widget provides a bottom sheet or full screen view for managing
/// the audio playback queue.
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../models/audio_queue.dart';
import '../controllers/queue_controller.dart';
import '../controllers/audio_player_controller.dart';
import 'audio_player_widget.dart';

/// Queue panel widget that can be shown as a bottom sheet or full screen.
class QueuePanel extends ConsumerStatefulWidget {
  /// Whether to show as a full screen modal.
  final bool fullScreen;
  
  /// Whether the panel can be dismissed by tapping outside.
  final bool barrierDismissible;
  
  /// Background color of the panel.
  final Color? backgroundColor;
  
  /// Maximum height for bottom sheet mode.
  final double? maxHeight;

  const QueuePanel({
    super.key,
    this.fullScreen = false,
    this.barrierDismissible = true,
    this.backgroundColor,
    this.maxHeight,
  });

  @override
  ConsumerState<QueuePanel> createState() => _QueuePanelState();
}

class _QueuePanelState extends ConsumerState<QueuePanel> {
  @override
  Widget build(BuildContext context) {
    final queueState = ref.watch(queueControllerProvider);
    final queue = queueState.queue;
    final controller = ref.read(queueControllerProvider.notifier);
    final audioController = ref.read(audioPlayerControllerProvider.notifier);
    final playerState = ref.watch(audioPlayerControllerProvider);
    
    if (widget.fullScreen) {
      return _buildFullScreen(context, queue, controller, audioController, playerState);
    }
    
    return _buildBottomSheet(context, queue, controller, audioController, playerState);
  }

  Widget _buildFullScreen(
    BuildContext context,
    AudioQueue queue,
    QueueNotifier queueController,
    AudioPlayerNotifier audioController,
    AudioPlayerState playerState,
  ) {
    return Scaffold(
      appBar: AppBar(
        title: const Text('Queue'),
        actions: [
          IconButton(
            icon: const Icon(Icons.clear),
            onPressed: queueController.clear,
            tooltip: 'Clear Queue',
          ),
        ],
      ),
      body: _buildQueueContent(
        context,
        queue,
        queueController,
        audioController,
        playerState,
        isFullScreen: true,
      ),
    );
  }

  Widget _buildBottomSheet(
    BuildContext context,
    AudioQueue queue,
    QueueNotifier queueController,
    AudioPlayerNotifier audioController,
    AudioPlayerState playerState,
  ) {
    return Container(
      decoration: BoxDecoration(
        color: widget.backgroundColor ?? Theme.of(context).cardColor,
        borderRadius: const BorderRadius.vertical(top: Radius.circular(16)),
      ),
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          // Handle
          const SizedBox(height: 8),
          Container(
            width: 40,
            height: 4,
            decoration: BoxDecoration(
              color: Colors.grey.shade400,
              borderRadius: BorderRadius.circular(2),
            ),
          ),
          const SizedBox(height: 8),
          
          // Header
          Padding(
            padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 8),
            child: Row(
              mainAxisAlignment: MainAxisAlignment.spaceBetween,
              children: [
                Text(
                  'Queue (${queue.length})',
                  style: Theme.of(context).textTheme.titleMedium,
                ),
                Row(
                  children: [
                    IconButton(
                      icon: Icon(queue.repeatMode.icon),
                      onPressed: queueController.cycleRepeatMode,
                      tooltip: queue.repeatMode.displayName,
                    ),
                    IconButton(
                      icon: Icon(
                        queue.isShuffled ? Icons.shuffle_on : Icons.shuffle,
                        color: queue.isShuffled 
                            ? Theme.of(context).colorScheme.primary 
                            : null,
                      ),
                      onPressed: queueController.toggleShuffle,
                      tooltip: queue.isShuffled ? 'Shuffle On' : 'Shuffle Off',
                    ),
                    IconButton(
                      icon: const Icon(Icons.clear),
                      onPressed: queueController.clear,
                      tooltip: 'Clear Queue',
                    ),
                  ],
                ),
              ],
            ),
          ),
          
          // Content
          ConstrainedBox(
            constraints: BoxConstraints(
              maxHeight: widget.maxHeight ?? MediaQuery.of(context).size.height * 0.7,
            ),
            child: _buildQueueContent(
              context,
              queue,
              queueController,
              audioController,
              playerState,
              isFullScreen: false,
            ),
          ),
        ],
      ),
    );
  }

  Widget _buildQueueContent(
    BuildContext context,
    AudioQueue queue,
    QueueNotifier queueController,
    AudioPlayerNotifier audioController,
    AudioPlayerState playerState, {
    required bool isFullScreen,
  }) {
    final theme = Theme.of(context);
    final colorScheme = theme.colorScheme;

    if (queue.isEmpty) {
      return Center(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Icon(
              Icons.queue_music,
              size: 48,
              color: colorScheme.onSurface.withOpacity(0.5),
            ),
            const SizedBox(height: 16),
            Text(
              'Queue is empty',
              style: theme.textTheme.bodyLarge?.copyWith(
                color: colorScheme.onSurface.withOpacity(0.5),
              ),
            ),
            const SizedBox(height: 8),
            Text(
              'Add confessions to start listening',
              style: theme.textTheme.bodyMedium?.copyWith(
                color: colorScheme.onSurface.withOpacity(0.3),
              ),
            ),
          ],
        ),
      );
    }

    return Column(
      children: [
        Padding(
          key: const ValueKey('current_player'),
          padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 8),
          child: AudioPlayerWidget(
            compact: true,
            onClose: isFullScreen ? null : () => Navigator.of(context).pop(),
          ),
        ),
        Expanded(
          child: ReorderableListView.builder(
            padding: EdgeInsets.zero,
            itemCount: queue.items.length,
            onReorder: (oldIndex, newIndex) {
              if (newIndex > oldIndex) newIndex -= 1;
              queueController.moveItem(oldIndex, newIndex);
            },
            itemBuilder: (context, index) => _buildQueueItem(
              context,
              queue.items[index],
              index,
              queue.currentIndex == index,
              queueController,
              audioController,
              playerState,
              key: ValueKey(queue.items[index].id),
            ),
          ),
        ),
      ],
    );
  }

  Widget _buildQueueItem(
    BuildContext context,
    AudioQueueItem item,
    int index,
    bool isCurrent,
    QueueNotifier queueController,
    AudioPlayerNotifier audioController,
    AudioPlayerState playerState, {
    required Key key,
  }) {
    final theme = Theme.of(context);
    final colorScheme = theme.colorScheme;
    final isPlaying = isCurrent && playerState.playerState == AudioPlayerPhase.playing;
    
    return Material(
      key: key,
      color: isCurrent 
          ? colorScheme.primaryContainer.withOpacity(0.3)
          : Colors.transparent,
      child: InkWell(
        onTap: () => queueController.playItemAt(index),
        onLongPress: () => _showItemMenu(context, item, index, queueController),
        child: Padding(
          padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 8),
          child: Row(
            children: [
              // Now playing indicator
              if (isCurrent)
                Icon(
                  Icons.volume_up,
                  color: colorScheme.primary,
                  size: 20,
                )
              else
                Text(
                  '${index + 1}',
                  style: theme.textTheme.bodySmall,
                ),
              
              const SizedBox(width: 12),
              
              // Play/pause button for current item
              if (isCurrent)
                IconButton(
                  icon: Icon(
                    isPlaying ? Icons.pause : Icons.play_arrow,
                    size: 20,
                  ),
                  onPressed: () async {
                    if (isPlaying) {
                      await audioController.pause();
                    } else {
                      await audioController.resume();
                    }
                  },
                )
              else
                const SizedBox(width: 36),
              
              // Item info
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      item.title,
                      style: theme.textTheme.bodyMedium?.copyWith(
                        fontWeight: isCurrent ? FontWeight.bold : FontWeight.normal,
                      ),
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                    ),
                    if (item.subtitle != null)
                      Text(
                        item.subtitle!,
                        style: theme.textTheme.bodySmall?.copyWith(
                          color: colorScheme.onSurface.withOpacity(0.6),
                        ),
                        maxLines: 1,
                        overflow: TextOverflow.ellipsis,
                      ),
                  ],
                ),
              ),
              
              // Duration
              Text(
                _formatDuration(item.duration),
                style: theme.textTheme.bodySmall?.copyWith(
                  color: colorScheme.onSurface.withOpacity(0.6),
                ),
              ),
              
              // Played indicator
              if (item.played)
                Icon(
                  Icons.check_circle,
                  color: colorScheme.primary,
                  size: 16,
                ),
              
              // Menu button
              IconButton(
                icon: const Icon(Icons.more_vert, size: 18),
                onPressed: () => _showItemMenu(context, item, index, queueController),
              ),
            ],
          ),
        ),
      ),
    );
  }

  void _showItemMenu(
    BuildContext context,
    AudioQueueItem item,
    int index,
    QueueNotifier queueController,
  ) {
    showModalBottomSheet(
      context: context,
      builder: (context) => SafeArea(
        child: Wrap(
          children: [
            ListTile(
              leading: const Icon(Icons.play_arrow),
              title: const Text('Play Now'),
              onTap: () {
                queueController.playItemAt(index);
                Navigator.of(context).pop();
              },
            ),
            ListTile(
              leading: const Icon(Icons.playlist_play),
              title: const Text('Play Next'),
              onTap: () {
                queueController.removeItemAt(index);
                queueController.addItem(item, playNext: true);
                Navigator.of(context).pop();
              },
            ),
            ListTile(
              leading: const Icon(Icons.delete),
              title: const Text('Remove from Queue'),
              textColor: Colors.red,
              iconColor: Colors.red,
              onTap: () {
                queueController.removeItemAt(index);
                Navigator.of(context).pop();
              },
            ),
            ListTile(
              leading: const Icon(Icons.info_outline),
              title: const Text('Item Info'),
              onTap: () {
                Navigator.of(context).pop();
                _showItemInfo(context, item);
              },
            ),
          ],
        ),
      ),
    );
  }

  void _showItemInfo(BuildContext context, AudioQueueItem item) {
    showDialog(
      context: context,
      builder: (context) => AlertDialog(
        title: Text(item.title),
        content: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text('Confession ID: ${item.confessionId}'),
            const SizedBox(height: 4),
            Text('Voice ID: ${item.voiceId}'),
            const SizedBox(height: 4),
            Text('Asset ID: ${item.asset.id}'),
            const SizedBox(height: 4),
            Text('Duration: ${_formatDuration(item.duration)}'),
            if (item.lastPosition != null)
              Column(
                children: [
                  const SizedBox(height: 4),
                  Text('Last Position: ${_formatDuration(item.lastPosition!)}'),
                ],
              ),
          ],
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(context).pop(),
            child: const Text('Close'),
          ),
        ],
      ),
    );
  }

  String _formatDuration(Duration duration) {
    final minutes = duration.inMinutes;
    final seconds = duration.inSeconds.remainder(60);
    return '${minutes.toString().padLeft(2, '0')}:${seconds.toString().padLeft(2, '0')}';
  }
}

/// Button to show the queue panel.
class QueueButton extends ConsumerWidget {
  /// Icon to display.
  final IconData icon;
  
  /// Label for the button.
  final String? label;
  
  /// Whether to show the item count.
  final bool showCount;
  
  /// Callback when pressed.
  final VoidCallback? onPressed;

  const QueueButton({
    super.key,
    this.icon = Icons.queue_music,
    this.label,
    this.showCount = true,
    this.onPressed,
  });

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final queueLength = ref.watch(queueLengthProvider);
    
    return IconButton(
      icon: Badge(
        label: showCount && queueLength > 0 ? Text(queueLength.toString()) : null,
        child: Icon(icon),
      ),
      onPressed: () {
        onPressed?.call();
        _showQueuePanel(context);
      },
      tooltip: label ?? 'Queue',
    );
  }

  void _showQueuePanel(BuildContext context) {
    showModalBottomSheet(
      context: context,
      isScrollControlled: true,
      useSafeArea: true,
      builder: (context) => const QueuePanel(),
    );
  }
}

/// Queue controls widget for the player.
class QueueControls extends ConsumerWidget {
  /// Whether to show all controls or just essential ones.
  final bool compact;

  const QueueControls({super.key, this.compact = false});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final queue = ref.watch(currentQueueProvider);
    final queueController = ref.read(queueControllerProvider.notifier);
    final theme = Theme.of(context);
    
    if (compact) {
      return Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          IconButton(
            icon: Icon(queue.repeatMode.icon),
            onPressed: queueController.cycleRepeatMode,
            tooltip: queue.repeatMode.displayName,
          ),
          IconButton(
            icon: Icon(
              queue.isShuffled ? Icons.shuffle_on : Icons.shuffle,
              color: queue.isShuffled ? theme.colorScheme.primary : null,
            ),
            onPressed: queueController.toggleShuffle,
            tooltip: queue.isShuffled ? 'Shuffle On' : 'Shuffle Off',
          ),
        ],
      );
    }
    
    return Row(
      mainAxisSize: MainAxisSize.min,
      children: [
        IconButton(
          icon: const Icon(Icons.skip_previous),
          onPressed: queueController.playPrevious,
          tooltip: 'Previous',
        ),
        IconButton(
          icon: Icon(queue.repeatMode.icon),
          onPressed: queueController.cycleRepeatMode,
          tooltip: queue.repeatMode.displayName,
        ),
        IconButton(
          icon: Icon(
            queue.isShuffled ? Icons.shuffle_on : Icons.shuffle,
            color: queue.isShuffled ? theme.colorScheme.primary : null,
          ),
          onPressed: queueController.toggleShuffle,
          tooltip: queue.isShuffled ? 'Shuffle On' : 'Shuffle Off',
        ),
        IconButton(
          icon: const Icon(Icons.skip_next),
          onPressed: queueController.playNext,
          tooltip: 'Next',
        ),
        QueueButton(showCount: false),
      ],
    );
  }
}

/// Queue progress indicator.
class QueueProgress extends ConsumerWidget {
  const QueueProgress({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final queue = ref.watch(currentQueueProvider);
    final currentIndex = queue.currentIndex;
    final length = queue.length;
    
    if (length == 0) return const SizedBox();
    
    return Text(
      '${currentIndex + 1}/$length',
      style: Theme.of(context).textTheme.bodySmall,
    );
  }
}
