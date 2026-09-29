/// Bootstrap for just_audio_background.
///
/// just_audio_background is initialized once at application startup. Metadata is
/// attached to AudioSource instances by JustAudioPlaybackService; this package
/// does not expose mutable static queue or notification methods.
import 'package:flutter/material.dart';
import 'package:just_audio_background/just_audio_background.dart';

import '../../../core/theme/tokens.dart';

class BackgroundPlaybackConfig {
  const BackgroundPlaybackConfig({
    this.enabled = true,
    this.showNotification = true,
    this.notificationTitle = 'I-Confess',
    this.notificationSubtitle,
    this.notificationImageUrl,
    this.showPlayPauseControls = true,
    this.showSkipControls = false,
    this.showStopControl = true,
    this.androidNotificationChannelId = 'app.iconfess.audio',
    this.androidNotificationChannelName = 'I-Confess audio',
    this.notificationColor = IConfess.colorBrand500,
  });

  final bool enabled;
  final bool showNotification;
  final String notificationTitle;
  final String? notificationSubtitle;
  final String? notificationImageUrl;
  final bool showPlayPauseControls;
  final bool showSkipControls;
  final bool showStopControl;
  final String androidNotificationChannelId;
  final String androidNotificationChannelName;
  final Color notificationColor;
}

abstract final class BackgroundPlaybackService {
  static Future<void>? _initialization;

  /// Initializes the global background audio handler once, before runApp.
  static Future<void> initialize({
    BackgroundPlaybackConfig config = const BackgroundPlaybackConfig(),
  }) {
    if (!config.enabled || !config.showNotification) return Future<void>.value();
    return _initialization ??= JustAudioBackground.init(
      androidNotificationChannelId: config.androidNotificationChannelId,
      androidNotificationChannelName: config.androidNotificationChannelName,
      androidNotificationOngoing: true,
      notificationColor: config.notificationColor,
    );
  }
}
