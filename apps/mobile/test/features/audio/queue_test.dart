/// Unit tests for the queue management system.
///
/// These tests verify the functionality of the audio queue model and controller.
import 'package:flutter_test/flutter_test.dart';
import 'package:iconfess/src/features/audio/models/audio_queue.dart';
import 'package:iconfess/src/features/audio/models/audio_asset.dart';

void main() {
  group('AudioQueueItem', () {
    test('create from minimal info', () {
      final item = AudioQueueItem.minimal(
        assetId: 'asset_123',
        confessionId: 'confession_456',
        voiceId: 'voice_789',
        title: 'Test Confession',
        duration: Duration(minutes: 2, seconds: 30),
      );

      expect(item.id, isNotNull);
      expect(item.id, startsWith('asset_123_'));
      expect(item.confessionId, 'confession_456');
      expect(item.voiceId, 'voice_789');
      expect(item.title, 'Test Confession');
      expect(item.duration, Duration(minutes: 2, seconds: 30));
      expect(item.played, false);
      expect(item.lastPosition, isNull);
    });

    test('create from asset', () {
      final asset = AudioAsset(
        id: 'asset_123',
        confessionId: 'confession_456',
        voiceId: 'voice_789',
        status: AudioAssetStatus.ready,
        durationSeconds: 151,
        sizeBytes: 1024000,
        createdAt: DateTime.now().toIso8601String(),
        updatedAt: DateTime.now().toIso8601String(),
      );

      final item = AudioQueueItem.fromAsset(
        asset: asset,
        title: 'Custom Title',
        subtitle: 'Custom Subtitle',
        orderIndex: 5,
      );

      expect(item.asset.id, 'asset_123');
      expect(item.confessionId, 'confession_456');
      expect(item.voiceId, 'voice_789');
      expect(item.title, 'Custom Title');
      expect(item.subtitle, 'Custom Subtitle');
      expect(item.duration, const Duration(seconds: 151));
      expect(item.orderIndex, 5);
    });

    test('copyWith creates new instance', () {
      final item = AudioQueueItem.minimal(
        assetId: 'asset_123',
        confessionId: 'confession_456',
        voiceId: 'voice_789',
        title: 'Original Title',
        duration: Duration(minutes: 2),
      );

      final newItem = item.copyWith(
        title: 'New Title',
        played: true,
        lastPosition: Duration(seconds: 30),
      );

      expect(newItem.title, 'New Title');
      expect(newItem.played, true);
      expect(newItem.lastPosition, Duration(seconds: 30));
      expect(item.title, 'Original Title'); // Original unchanged
      expect(item.played, false); // Original unchanged
    });

    test('toJson and fromJson', () {
      final original = AudioQueueItem.minimal(
        assetId: 'asset_123',
        confessionId: 'confession_456',
        voiceId: 'voice_789',
        title: 'Test Title',
        subtitle: 'Test Subtitle',
        duration: Duration(minutes: 2, seconds: 30),
        orderIndex: 3,
      );

      final json = original.toJson();
      final restored = AudioQueueItem.fromJson(json);

      expect(restored.id, original.id);
      expect(restored.asset.id, original.asset.id);
      expect(restored.confessionId, original.confessionId);
      expect(restored.voiceId, original.voiceId);
      expect(restored.title, original.title);
      expect(restored.subtitle, original.subtitle);
      expect(restored.duration, original.duration);
      expect(restored.orderIndex, original.orderIndex);
    });

    test('equality based on id', () {
      final item1 = AudioQueueItem.minimal(
        assetId: 'asset_123',
        confessionId: 'confession_1',
        voiceId: 'voice_1',
        title: 'Title 1',
        duration: Duration(minutes: 1),
      );

      final item2 = item1.copyWith(title: 'Title 2');

      final item3 = AudioQueueItem.minimal(
        assetId: 'asset_456',
        confessionId: 'confession_3',
        voiceId: 'voice_3',
        title: 'Title 3',
        duration: Duration(minutes: 3),
      );

      expect(item1 == item2, true); // Same ID
      expect(item1 == item3, false); // Different ID
    });

    test('toString includes relevant info', () {
      final item = AudioQueueItem.minimal(
        assetId: 'asset_123',
        confessionId: 'confession_456',
        voiceId: 'voice_789',
        title: 'Test Title',
        duration: Duration(minutes: 2),
      );

      final str = item.toString();
      expect(str, contains('AudioQueueItem'));
      expect(str, contains('asset_123'));
      expect(str, contains('Test Title'));
    });
  });

  group('AudioQueue', () {
    test('empty queue', () {
      const queue = AudioQueue();

      expect(queue.isEmpty, true);
      expect(queue.isNotEmpty, false);
      expect(queue.length, 0);
      expect(queue.currentItem, isNull);
      expect(queue.nextItem, isNull);
      expect(queue.previousItem, isNull);
      expect(queue.totalDuration, Duration.zero);
    });

    test('queue with items', () {
      final items = [
        AudioQueueItem.minimal(
          assetId: 'asset_1',
          confessionId: 'confession_1',
          voiceId: 'voice_1',
          title: 'Item 1',
          duration: Duration(minutes: 2),
        ),
        AudioQueueItem.minimal(
          assetId: 'asset_2',
          confessionId: 'confession_2',
          voiceId: 'voice_2',
          title: 'Item 2',
          duration: Duration(minutes: 3),
        ),
        AudioQueueItem.minimal(
          assetId: 'asset_3',
          confessionId: 'confession_3',
          voiceId: 'voice_3',
          title: 'Item 3',
          duration: Duration(minutes: 4),
        ),
      ];

      final queue = AudioQueue(
        items: items,
        currentIndex: 1,
      );

      expect(queue.isEmpty, false);
      expect(queue.isNotEmpty, true);
      expect(queue.length, 3);
      expect(queue.currentIndex, 1);
      expect(queue.currentItem, items[1]);
      expect(queue.totalDuration, Duration(minutes: 9));
    });

    test('next item in normal mode', () {
      final items = [
        AudioQueueItem.minimal(
          assetId: 'asset_1',
          confessionId: 'confession_1',
          voiceId: 'voice_1',
          title: 'Item 1',
          duration: Duration(minutes: 1),
        ),
        AudioQueueItem.minimal(
          assetId: 'asset_2',
          confessionId: 'confession_2',
          voiceId: 'voice_2',
          title: 'Item 2',
          duration: Duration(minutes: 1),
        ),
        AudioQueueItem.minimal(
          assetId: 'asset_3',
          confessionId: 'confession_3',
          voiceId: 'voice_3',
          title: 'Item 3',
          duration: Duration(minutes: 1),
        ),
      ];

      // At index 0, next should be index 1
      var queue = AudioQueue(items: items, currentIndex: 0);
      expect(queue.nextItem, items[1]);

      // At index 1, next should be index 2
      queue = AudioQueue(items: items, currentIndex: 1);
      expect(queue.nextItem, items[2]);

      // At index 2 (last), next should be null (no repeat)
      queue = AudioQueue(items: items, currentIndex: 2);
      expect(queue.nextItem, isNull);

      // At index 2 with repeat all, next should be index 0
      queue = AudioQueue(
        items: items,
        currentIndex: 2,
        repeatMode: RepeatMode.all,
      );
      expect(queue.nextItem, items[0]);
    });

    test('previous item in normal mode', () {
      final items = [
        AudioQueueItem.minimal(
          assetId: 'asset_1',
          confessionId: 'confession_1',
          voiceId: 'voice_1',
          title: 'Item 1',
          duration: Duration(minutes: 1),
        ),
        AudioQueueItem.minimal(
          assetId: 'asset_2',
          confessionId: 'confession_2',
          voiceId: 'voice_2',
          title: 'Item 2',
          duration: Duration(minutes: 1),
        ),
        AudioQueueItem.minimal(
          assetId: 'asset_3',
          confessionId: 'confession_3',
          voiceId: 'voice_3',
          title: 'Item 3',
          duration: Duration(minutes: 1),
        ),
      ];

      // At index 2, previous should be index 1
      var queue = AudioQueue(items: items, currentIndex: 2);
      expect(queue.previousItem, items[1]);

      // At index 1, previous should be index 0
      queue = AudioQueue(items: items, currentIndex: 1);
      expect(queue.previousItem, items[0]);

      // At index 0 (first), previous should be null (no repeat)
      queue = AudioQueue(items: items, currentIndex: 0);
      expect(queue.previousItem, isNull);

      // At index 0 with repeat all, previous should be index 2
      queue = AudioQueue(
        items: items,
        currentIndex: 0,
        repeatMode: RepeatMode.all,
      );
      expect(queue.previousItem, items[2]);
    });

    test('next item in shuffle mode', () {
      final items = [
        AudioQueueItem.minimal(
          assetId: 'asset_1',
          confessionId: 'confession_1',
          voiceId: 'voice_1',
          title: 'Item 1',
          duration: Duration(minutes: 1),
          played: false,
        ),
        AudioQueueItem.minimal(
          assetId: 'asset_2',
          confessionId: 'confession_2',
          voiceId: 'voice_2',
          title: 'Item 2',
          duration: Duration(minutes: 1),
          played: false,
        ),
        AudioQueueItem.minimal(
          assetId: 'asset_3',
          confessionId: 'confession_3',
          voiceId: 'voice_3',
          title: 'Item 3',
          duration: Duration(minutes: 1),
          played: true, // This one is played
        ),
      ];

      // In shuffle mode, next should be a random unplayed item
      final queue = AudioQueue(
        items: items,
        currentIndex: 0,
        isShuffled: true,
      );

      final nextItem = queue.nextItem;
      expect(nextItem, isNotNull);
      // Should be one of the unplayed items (0 or 1)
      expect([items[0], items[1]], contains(nextItem));
    });

    test('copyWith creates new instance', () {
      final items = [
        AudioQueueItem.minimal(
          assetId: 'asset_1',
          confessionId: 'confession_1',
          voiceId: 'voice_1',
          title: 'Item 1',
          duration: Duration(minutes: 1),
        ),
      ];

      const queue = AudioQueue(
        items: items,
        currentIndex: 0,
        isShuffled: false,
        repeatMode: RepeatMode.none,
      );

      final newQueue = queue.copyWith(
        currentIndex: 1,
        isShuffled: true,
        repeatMode: RepeatMode.all,
      );

      expect(newQueue.currentIndex, 1);
      expect(newQueue.isShuffled, true);
      expect(newQueue.repeatMode, RepeatMode.all);
      expect(queue.currentIndex, 0); // Original unchanged
      expect(queue.isShuffled, false); // Original unchanged
    });

    test('toJson and fromJson', () {
      final items = [
        AudioQueueItem.minimal(
          assetId: 'asset_1',
          confessionId: 'confession_1',
          voiceId: 'voice_1',
          title: 'Item 1',
          duration: Duration(minutes: 2),
        ),
        AudioQueueItem.minimal(
          assetId: 'asset_2',
          confessionId: 'confession_2',
          voiceId: 'voice_2',
          title: 'Item 2',
          duration: Duration(minutes: 3),
        ),
      ];

      const original = AudioQueue(
        items: items,
        currentIndex: 1,
        isShuffled: true,
        repeatMode: RepeatMode.all,
      );

      final json = original.toJson();
      final restored = AudioQueue.fromJson(json);

      expect(restored.items.length, original.items.length);
      expect(restored.currentIndex, original.currentIndex);
      expect(restored.isShuffled, original.isShuffled);
      expect(restored.repeatMode, original.repeatMode);
    });

    test('hasCurrentItem returns correct value', () {
      final items = [
        AudioQueueItem.minimal(
          assetId: 'asset_1',
          confessionId: 'confession_1',
          voiceId: 'voice_1',
          title: 'Item 1',
          duration: Duration(minutes: 1),
        ),
      ];

      // Valid index
      var queue = AudioQueue(items: items, currentIndex: 0);
      expect(queue.hasCurrentItem, true);

      // Index out of bounds (negative)
      queue = AudioQueue(items: items, currentIndex: -1);
      expect(queue.hasCurrentItem, false);

      // Index out of bounds (too large)
      queue = AudioQueue(items: items, currentIndex: 10);
      expect(queue.hasCurrentItem, false);

      // Empty queue
      queue = AudioQueue(items: [], currentIndex: 0);
      expect(queue.hasCurrentItem, false);
    });

    test('playedCount and remainingCount', () {
      final items = [
        AudioQueueItem.minimal(
          assetId: 'asset_1',
          confessionId: 'confession_1',
          voiceId: 'voice_1',
          title: 'Item 1',
          duration: Duration(minutes: 1),
          played: true,
        ),
        AudioQueueItem.minimal(
          assetId: 'asset_2',
          confessionId: 'confession_2',
          voiceId: 'voice_2',
          title: 'Item 2',
          duration: Duration(minutes: 1),
          played: false,
        ),
        AudioQueueItem.minimal(
          assetId: 'asset_3',
          confessionId: 'confession_3',
          voiceId: 'voice_3',
          title: 'Item 3',
          duration: Duration(minutes: 1),
          played: true,
        ),
        AudioQueueItem.minimal(
          assetId: 'asset_4',
          confessionId: 'confession_4',
          voiceId: 'voice_4',
          title: 'Item 4',
          duration: Duration(minutes: 1),
          played: false,
        ),
      ];

      final queue = AudioQueue(items: items);

      expect(queue.playedCount, 2);
      expect(queue.remainingCount, 2);
    });
  });

  group('RepeatMode', () {
    test('values', () {
      expect(RepeatMode.values.length, 3);
      expect(RepeatMode.values, contains(RepeatMode.none));
      expect(RepeatMode.values, contains(RepeatMode.all));
      expect(RepeatMode.values, contains(RepeatMode.one));
    });

    test('next cycles correctly', () {
      expect(RepeatMode.none.next, RepeatMode.all);
      expect(RepeatMode.all.next, RepeatMode.one);
      expect(RepeatMode.one.next, RepeatMode.none);
    });

    test('displayName', () {
      expect(RepeatMode.none.displayName, 'No Repeat');
      expect(RepeatMode.all.displayName, 'Repeat All');
      expect(RepeatMode.one.displayName, 'Repeat One');
    });

    test('icon', () {
      expect(RepeatMode.none.icon, Icons.repeat);
      expect(RepeatMode.all.icon, Icons.repeat);
      expect(RepeatMode.one.icon, Icons.repeat_one);
    });
  });

  group('RepeatModeExtension', () {
    test('next cycles through all modes', () {
      var mode = RepeatMode.none;
      mode = mode.next;
      expect(mode, RepeatMode.all);
      
      mode = mode.next;
      expect(mode, RepeatMode.one);
      
      mode = mode.next;
      expect(mode, RepeatMode.none);
    });
  });
}
