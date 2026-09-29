import 'package:flutter_test/flutter_test.dart';
import 'package:iconfess/src/features/audio/services/background_playback_service.dart';
import 'package:iconfess/src/features/player/audio_playback_service.dart';

void main() {
  group('BackgroundPlaybackConfig', () {
    test('provides app-specific notification defaults', () {
      const config = BackgroundPlaybackConfig();

      expect(config.enabled, isTrue);
      expect(config.androidNotificationChannelId, 'app.iconfess.audio');
      expect(config.androidNotificationChannelName, 'I-Confess audio');
    });

    test('supports a custom Android channel', () {
      const config = BackgroundPlaybackConfig(
        androidNotificationChannelId: 'app.iconfess.audio.test',
        androidNotificationChannelName: 'Test audio',
      );

      expect(config.androidNotificationChannelId, 'app.iconfess.audio.test');
      expect(config.androidNotificationChannelName, 'Test audio');
    });
  });

  test('disabled background audio does not touch platform channels', () async {
    await BackgroundPlaybackService.initialize(
      config: const BackgroundPlaybackConfig(enabled: false),
    );
  });

  test('playback metadata has notification-ready identifying fields', () {
    const metadata = PlaybackMediaMetadata(
      id: 'session-item-1',
      title: 'Morning confession',
      artist: 'I-Confess',
    );

    expect(metadata.id, 'session-item-1');
    expect(metadata.title, 'Morning confession');
    expect(metadata.artist, 'I-Confess');
  });
}
