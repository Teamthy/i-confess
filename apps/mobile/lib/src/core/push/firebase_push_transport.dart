import 'dart:async';

import 'package:firebase_core/firebase_core.dart';
import 'package:firebase_messaging/firebase_messaging.dart';
import 'package:flutter/foundation.dart';
import 'package:flutter/services.dart';
import 'package:flutter_local_notifications/flutter_local_notifications.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'push_registration.dart';

/// The push transport the app ships with: FCM, which carries both platforms.
///
/// FCM is the transport for Android and, through the Firebase iOS SDK, for APNs
/// as well. The server still speaks to APNs directly for iOS devices (that is
/// what a device registered with `platform: ios` gets), so the two paths are
/// independent: this class obtains and refreshes *tokens*; delivery belongs to
/// the backend.
///
/// Every Firebase call is wrapped. A checkout without `google-services.json` or
/// `GoogleService-Info.plist` — which is every checkout in this repository, since
/// those files carry project credentials and are never committed — must run the
/// app without push rather than fail at launch.
class FirebasePushTransport implements PushTransport {
  FirebasePushTransport({
    FlutterLocalNotificationsPlugin? localNotifications,
    FirebaseMessaging? messaging,
  })  : _local = localNotifications ?? FlutterLocalNotificationsPlugin(),
        _messaging = messaging;

  final FlutterLocalNotificationsPlugin _local;
  FirebaseMessaging? _messaging;

  final _tokens = StreamController<String>.broadcast();
  final _pendingTaps = <PushTap>[];
  late final StreamController<PushTap> _opened = StreamController<PushTap>.broadcast(onListen: () {
    for (final tap in _pendingTaps) { _opened.add(tap); }
    _pendingTaps.clear();
  });

  void _publishTap(PushTap tap) {
    if (_opened.hasListener) {
      _opened.add(tap);
    } else {
      // Native callbacks can arrive before the first frame subscribes.
      if (_pendingTaps.length == 10) _pendingTaps.removeAt(0);
      _pendingTaps.add(tap);
    }
  }
  final _subscriptions = <StreamSubscription<Object?>>[];

  static const _channelId = 'iconfess_schedules';
  static const _channelName = 'Session reminders';
  static const _channelDescription = 'Reminders for the sessions you scheduled.';

  bool _initialized = false;

  @override
  Future<bool> initialize() async {
    if (_initialized) return _messaging != null;
    _initialized = true;

    if (kIsWeb || (defaultTargetPlatform != TargetPlatform.iOS &&
        defaultTargetPlatform != TargetPlatform.android)) {
      // The web build would need a Firebase web app plus a service worker, and
      // neither is configured. Claiming availability would produce a tokenless
      // registration on every page load.
      debugPrint('push: web push is not configured - skipping Firebase');
      return false;
    }

    try {
      await Firebase.initializeApp();
    } catch (error) {
      debugPrint('push: Firebase is not configured ($error) - '
          'run `flutterfire configure` to enable reminders');
      return false;
    }

    if (defaultTargetPlatform == TargetPlatform.iOS) {
      _subscriptions.add(const EventChannel('app.iconfess/push_taps')
          .receiveBroadcastStream().listen((event) {
        if (event is! Map) return;
        final data = event.map((key, value) => MapEntry(key.toString(), value.toString()));
        _publishTap(PushTap(deepLink: data['deeplink'], data: data));
      }, onError: (Object error) { debugPrint('push: APNs tap bridge: $error'); }));
    }
    final messaging = _messaging ??= FirebaseMessaging.instance;
    await _initLocalNotifications();

    _subscriptions.add(messaging.onTokenRefresh.listen((_) async {
      // An iOS device is routed to APNs by the backend. Never register its FCM
      // token as an APNs token, including on refresh.
      final current = await token();
      if (current != null && current.isNotEmpty) _tokens.add(current);
    }));
    _subscriptions.add(FirebaseMessaging.onMessage.listen(_showForeground));
    _subscriptions.add(FirebaseMessaging.onMessageOpenedApp.listen((m) => _publishTap(_tapOf(m))));

    // The notification that launched the app from a terminated state. Reading
    // it here rather than in a widget means the deep link is available before
    // the first frame is routed.
    final launched = await messaging.getInitialMessage();
    if (launched != null) {
      _publishTap(_tapOf(launched));
    }
    return true;
  }

  @override
  Future<bool> requestPermission() async {
    final messaging = _messaging;
    if (messaging == null) return false;
    final settings = await messaging.requestPermission(alert: true, badge: true, sound: true);
    // Provisional means iOS can deliver quietly to the notification centre and
    // the user can promote it later; treating it as a refusal would leave those
    // users unable to turn reminders on without visiting Settings.
    return settings.authorizationStatus == AuthorizationStatus.authorized ||
        settings.authorizationStatus == AuthorizationStatus.provisional;
  }

  @override
  Future<String?> token() async {
    try {
      final messaging = _messaging;
      if (messaging == null) return null;
      final settings = await messaging.getNotificationSettings();
      if (settings.authorizationStatus != AuthorizationStatus.authorized &&
          settings.authorizationStatus != AuthorizationStatus.provisional) return null;
      return defaultTargetPlatform == TargetPlatform.iOS
          ? await messaging.getAPNSToken()
          : await messaging.getToken();
    } catch (error) {
      // APNs registration failing on a simulator or an unprovisioned build is
      // expected; there is simply no token yet.
      debugPrint('push: no token: $error');
      return null;
    }
  }

  @override
  Stream<String> get tokenRefresh => _tokens.stream;

  @override
  Stream<PushTap> get opened => _opened.stream;

  /// Displays a reminder that arrived while the app was in the foreground.
  ///
  /// Android does not show a notification for an FCM message the app is already
  /// handling, so without this a reminder that fires with the app open would be
  /// visible only in the log.
  Future<void> _showForeground(RemoteMessage message) async {
    final title = message.notification?.title ?? message.data['title']?.toString();
    final body = message.notification?.body ?? message.data['body']?.toString();
    if (title == null && body == null) return;

    await _local.show(
      0,
      title,
      body,
      const NotificationDetails(
        android: AndroidNotificationDetails(
          _channelId,
          _channelName,
          channelDescription: _channelDescription,
          importance: Importance.high,
          priority: Priority.high,
        ),
        iOS: DarwinNotificationDetails(),
      ),
      payload: _dataOf(message)['deeplink'],
    );
  }

  Future<void> _initLocalNotifications() async {
    const initialization = InitializationSettings(
      android: AndroidInitializationSettings('@mipmap/ic_launcher'),
      // Permission is requested by the notification that needs it, from an
      // explicit user action - never during startup, where a refusal cannot be
      // undone.
      iOS: DarwinInitializationSettings(
        requestAlertPermission: false,
        requestBadgePermission: false,
        requestSoundPermission: false,
      ),
    );
    await _local.initialize(
      initialization,
      onDidReceiveNotificationResponse: (response) {
        _publishTap(PushTap(deepLink: response.payload));
      },
    );
    final launch = await _local.getNotificationAppLaunchDetails();
    if (launch?.didNotificationLaunchApp == true) {
      _publishTap(PushTap(deepLink: launch!.notificationResponse?.payload));
    }
    await _local
        .resolvePlatformSpecificImplementation<AndroidFlutterLocalNotificationsPlugin>()
        ?.createNotificationChannel(const AndroidNotificationChannel(
          _channelId,
          _channelName,
          description: _channelDescription,
          importance: Importance.high,
        ));
  }

  PushTap _tapOf(RemoteMessage message) {
    final data = _dataOf(message);
    return PushTap(deepLink: data['deeplink'], data: data);
  }

  static Map<String, String> _dataOf(RemoteMessage message) {
    // Direct APNs payloads contain the server's custom fields under `data`;
    // FCM Android exposes the same fields directly in RemoteMessage.data.
    final nested = message.data['data'];
    final data = nested is Map ? nested : message.data;
    return data.map((key, value) => MapEntry(key.toString(), value?.toString() ?? ''));
  }

  /// Releases the listeners. The transport lives for the life of the process,
  /// so this exists for tests and for a future hot-restart path rather than for
  /// normal teardown.
  Future<void> dispose() async {
    for (final subscription in _subscriptions) {
      await subscription.cancel();
    }
    _subscriptions.clear();
    await _tokens.close();
    await _opened.close();
  }
}

/// The transport used by the running app. Tests override this.
final pushTransportProvider = Provider<PushTransport>((ref) {
  final transport = FirebasePushTransport();
  ref.onDispose(() { transport.dispose(); });
  return transport;
});
