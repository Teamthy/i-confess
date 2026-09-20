import 'package:flutter/foundation.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:iconfess_api/iconfess_api.dart';

import '../../features/auth/auth_controller.dart';
import '../di/providers.dart';
import '../routing/routes.dart';
import 'device_registration.dart';
import 'firebase_push_transport.dart';

/// What the app needs from a push transport (IC-012).
///
/// The interface exists so the registration logic — when to ask, what to send,
/// what to do when the token rotates — is testable without Firebase, and so the
/// plugin surface stays in one file. Plugin code is the part that cannot be
/// exercised in `flutter test`: it is a platform channel, and a test binary has
/// no FCM or APNs behind it.
abstract interface class PushTransport {
  /// Prepares the transport. Returns false when push is unavailable on this
  /// device — no Firebase configuration, a platform without messaging, or a
  /// simulator. Unavailable is a normal state, not an error: the app has to run
  /// without push configured, since a developer's checkout has no
  /// `google-services.json` and a build that crashes on launch without one is a
  /// build nobody can run.
  Future<bool> initialize();

  /// Asks the user for permission, returning whether it was granted.
  ///
  /// Only ever called from a user action. A permission prompt at first launch is
  /// the fastest way to a permanent denial, and on iOS a denial cannot be asked
  /// again.
  Future<bool> requestPermission();

  /// The current token, or null when there is none yet.
  Future<String?> token();

  /// Tokens after the first one: APNs and FCM both rotate them, and a stale
  /// token delivers to a device that no longer exists.
  Stream<String> get tokenRefresh;

  /// Deep links opened by tapping a notification, with the payload it carried.
  Stream<PushTap> get opened;
}

/// A notification the user tapped.
@immutable
class PushTap {
  const PushTap({this.deepLink, this.data = const {}});

  /// The `deeplink` the server attached, e.g.
  /// `iconfess://schedules/{id}/start`.
  final String? deepLink;
  final Map<String, String> data;
}

/// How a device registers itself with the server.
///
/// A turn of the app that has not signed in cannot register anything: the
/// endpoint is authenticated and the device belongs to a user. [PushRegistrar]
/// therefore waits for a session and re-registers on every sign-in, which also
/// re-points a device at its new owner after a shared phone changes hands.
class PushRegistrar {
  PushRegistrar({
    required PushTransport transport,
    required DeviceIdentities identities,
    required DevicePushRegistration registration,
  })  : _transport = transport,
        _identities = identities,
        _registration = registration;

  final PushTransport _transport;
  final DeviceIdentities _identities;
  final DevicePushRegistration _registration;

  bool _available = false;
  String? _token;

  /// Whether this device can receive push at all.
  bool get available => _available;

  /// The last token the server was told about, for the settings screen.
  String? get lastToken => _token;

  /// Initialises the transport. Safe to call once per process.
  Future<void> start() async {
    _available = await _transport.initialize();
    if (!_available) {
      debugPrint('push: unavailable on this device - reminders will not arrive');
      return;
    }
    _transport.tokenRefresh.listen((token) {
      if (token.isEmpty) return;
      // A rotated token is a device the old token no longer reaches: the
      // server has to hear about it in the same session that learns it.
      _token = token;
      unawaited(_register(token));
    });
  }

  /// Requests permission and registers, for use from an explicit user action.
  ///
  /// Returns the token that was registered, or null when permission was
  /// refused or push is unavailable. Refusal is not an error: the user said no,
  /// and the app keeps working without reminders.
  Future<String?> enable() async {
    if (!_available) return null;
    if (!await _transport.requestPermission()) {
      debugPrint('push: permission refused - not registering a token');
      return null;
    }
    final token = await _transport.token();
    if (token == null || token.isEmpty) {
      debugPrint('push: permission granted but the device has no token yet');
      return null;
    }
    _token = token;
    await _register(token);
    return token;
  }

  /// Registers the current token without prompting, for a session that already
  /// granted permission.
  Future<void> registerIfPermitted() async {
    if (!_available) return;
    final token = await _transport.token();
    if (token == null || token.isEmpty) return;
    _token = token;
    await _register(token);
  }

  Future<void> _register(String token) async {
    final identity = await _identities.current();
    await _registration.registerDevice(
      deviceId: identity.deviceId,
      platform: identity.platform,
      pushToken: token,
    );
  }
}

/// The push pipeline for the signed-in user.
///
/// Registered when a session exists: an unauthenticated device has nothing to
/// register and no user to attach a token to. The listener is deliberately on
/// the auth state rather than on the launch path, because the app can be launched
/// signed in, signed out, or signed in days later.
final pushRegistrarProvider = Provider<PushRegistrar>((ref) {
  final registrar = PushRegistrar(
    transport: ref.watch(pushTransportProvider),
    identities: ref.watch(deviceIdentitiesProvider),
    registration: DevicePushRegistration(ref.watch(apiClientProvider)),
  );

  var started = false;
  ref.listen<AuthState>(authControllerProvider, (previous, next) {
    if (next.status != AuthStatus.signedIn) return;
    () async {
      if (!started) {
        started = true;
        await registrar.start();
      }
      // Every sign-in re-registers. A token registered by a previous session
      // may belong to a different account on the same handset.
      await registrar.registerIfPermitted();
    }();
  });

  return registrar;
});

/// The schedule id a reminder is about, if it is one of ours.
///
/// The server sends `iconfess://schedules/{id}/start`. Anything else — including
/// the links the stores use (`iconfess://open?...`) — returns null, because a
/// notification is remote input and an app that navigates to whatever path it
/// was handed will eventually be handed a path it did not expect.
@visibleForTesting
String? scheduleIdFromLink(PushTap tap) {
  final link = tap.deepLink ?? tap.data['deeplink'];
  if (link == null || link.isEmpty) return null;
  const scheme = 'iconfess://';
  if (!link.startsWith(scheme)) return null;
  final rest = link.substring(scheme.length);
  if (!rest.startsWith('schedules/') || !rest.endsWith('/start')) return null;
  final id = tap.data['schedule_id'] ?? rest.substring('schedules/'.length, rest.length - '/start'.length);
  return id.isEmpty ? null : id;
}

/// Deep links from tapped notifications, as router paths.
///
/// Tapping a reminder does what the reminder says: it builds the session the
/// schedule describes and opens it. That has to happen through the API
/// (`POST /schedules/{id}/start`), not by navigating straight to a player with a
/// schedule id in it — a schedule is a saved intention, not a session, and only
/// the server can turn one into the other while applying the entitlement rules
/// that are in force at that moment.
final pushDeepLinkProvider = StreamProvider<String>((ref) async* {
  final transport = ref.watch(pushTransportProvider);
  final api = ref.watch(apiClientProvider);

  await for (final tap in transport.opened) {
    final scheduleId = scheduleIdFromLink(tap);
    if (scheduleId == null) {
      debugPrint('push: ignoring a notification link this app does not handle');
      continue;
    }
    try {
      final session = await api.post('/schedules/$scheduleId/start');
      final sessionId = (session['session_id'] ?? session['id'])?.toString();
      if (sessionId == null || sessionId.isEmpty) {
        debugPrint('push: the schedule produced no session to open');
        continue;
      }
      yield AppRoutes.playerWithId(sessionId);
    } catch (error) {
      // The session could not be built: the plan may have lapsed since the
      // reminder was scheduled, or the network is down. Saying nothing is worse
      // than a silent no-op only if the user is left staring at a stale screen,
      // so the fallback is the Activity tab, which shows the schedule and its
      // state.
      debugPrint('push: could not start the scheduled session: $error');
      yield AppRoutes.activity;
    }
  }
});

/// Fire-and-forget, with the error attached to the log rather than swallowed.
void unawaited(Future<void> future) {
  future.catchError((Object error) {
    debugPrint('push: $error');
  });
}
