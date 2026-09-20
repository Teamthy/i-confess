import 'package:iconfess/src/core/di/providers.dart';
import 'package:iconfess/src/core/push/platform_device_details.dart';
import 'dart:async';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter/material.dart';
import 'package:iconfess/src/features/auth/auth_controller.dart';
import 'package:iconfess/src/features/settings/settings_screen.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:iconfess/src/core/persistence/persistence.dart';
import 'package:iconfess/src/core/push/device_registration.dart';
import 'package:iconfess/src/core/push/firebase_push_transport.dart';
import 'package:iconfess/src/core/push/push_registration.dart';
import 'package:iconfess_api/iconfess_api.dart';

import 'support/fake_api_client.dart';

/// IC-012: the client registers a device and a push token.
///
/// The audit's finding was not that the registration code was wrong; it was that
/// nothing ever called it (`rg -n 'postMeDevices' apps/mobile/lib` found no
/// matches), so the server logged "no registered devices" for every reminder and
/// the product's habit loop could not close. These tests exercise the wiring
/// that was missing: permission, the token, the request body, and what happens
/// when any of the three is not there.

/// A transport that answers whatever the test wants and nothing else.
class FakePushTransport implements PushTransport {
  FakePushTransport({this.available = true, this.permission = true});

  bool available;
  bool permission;
  String? currentToken;

  final _refreshes = StreamController<String>.broadcast();
  final _opened = StreamController<PushTap>.broadcast();

  int permissionRequests = 0;
  int initializations = 0;

  @override
  Future<bool> initialize() async {
    initializations++;
    return available;
  }

  @override
  Future<bool> requestPermission() async {
    permissionRequests++;
    return permission;
  }

  @override
  Future<String?> token() async => currentToken;

  @override
  Stream<String> get tokenRefresh => _refreshes.stream;

  @override
  Stream<PushTap> get opened => _opened.stream;

  /// Simulates the provider rotating the token.
  void rotate(String token) => _refreshes.add(token);

  /// Simulates a tap on a delivered notification.
  void tap(PushTap tap) => _opened.add(tap);

  Future<void> dispose() async {
    await _refreshes.close();
    await _opened.close();
  }
}

class FakeDeviceDetails implements DeviceDetails {
  FakeDeviceDetails({this.nativeId = 'native-1', this.platform = 'ios'});

  final String? nativeId;
  @override
  final String platform;

  @override
  Future<String?> nativeDeviceId() async => nativeId;
}

class SignedInAuth extends AuthController {
  @override
  AuthState build() => const AuthState.signedIn(userId: 'user-1');
}

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  late InMemoryTokenStore tokens;
  late FakeApiClient api;
  late FakePushTransport transport;
  late InMemoryKeyValueStore store;

  PushRegistrar registrarWith(FakeDeviceDetails details) => PushRegistrar(
        transport: transport,
        identities: DeviceIdentities(store: store, details: details),
        registration: DevicePushRegistration(api),
      );

  setUp(() {
    tokens = InMemoryTokenStore();
    api = FakeApiClient(tokens: tokens);
    api.respond('/me/devices', {'data': <String, dynamic>{}});
    transport = FakePushTransport();
    store = InMemoryKeyValueStore();
  });

  tearDown(() => transport.dispose());

  test('enabling registers the device, its platform and its token', () async {
    transport.currentToken = 'fcm-token-1';
    final registrar = registrarWith(FakeDeviceDetails());

    final token = await registrar.enable();

    expect(token, 'fcm-token-1');
    expect(api.callCount('/me/devices'), 1);
    final body = api.bodyOf('/me/devices')!;
    expect(body['device_id'], 'native-1');
    expect(body['platform'], 'ios');
    expect(body['push_token'], 'fcm-token-1');
  });

  // A permission prompt is raised from a user action, and a refusal is an
  // answer. Registering a token anyway would tell the server to send reminders
  // to a device that has already said it does not want them.
  test('a refused permission registers nothing', () async {
    transport.permission = false;
    transport.currentToken = 'fcm-token-1';
    final registrar = registrarWith(FakeDeviceDetails());

    final token = await registrar.enable();

    expect(token, isNull);
    expect(api.callCount('/me/devices'), 0);
  });

  // A checkout without firebase configuration, or a simulator with no APNs,
  // must not produce a tokenless registration or an exception at launch.
  test('an unavailable transport is not an error', () async {
    transport.available = false;
    final registrar = registrarWith(FakeDeviceDetails());

    await registrar.start();
    await registrar.registerIfPermitted();

    expect(registrar.available, isFalse);
    expect(transport.permissionRequests, 0, reason: 'permission was requested with no transport');
    expect(api.callCount('/me/devices'), 0);
  });

  test('start initialises the transport once and registers nothing by itself', () async {
    transport.currentToken = 'fcm-token-1';
    final registrar = registrarWith(FakeDeviceDetails());

    await registrar.start();
    await registrar.start();

    expect(transport.initializations, 1);
    expect(transport.permissionRequests, 0,
        reason: 'start() prompted for permission; that belongs to an explicit user action');
    expect(api.callCount('/me/devices'), 0);
  });

  // Both stores rotate tokens. A device that keeps the old one is a device the
  // server can no longer reach, and it fails silently - which is the worst kind
  // of failure for a reminder.
  test('a rotated token is registered again', () async {
    transport.currentToken = 'fcm-token-1';
    final registrar = registrarWith(FakeDeviceDetails());
    await registrar.start();
    await registrar.registerIfPermitted();

    transport.rotate('fcm-token-2');
    await pumpEventQueue();

    expect(api.callCount('/me/devices'), 2);
    expect(api.bodyOf('/me/devices')!['push_token'], 'fcm-token-2');
  });

  // The identifier is persisted: a device that renamed itself overnight would
  // appear to the server as a second device and be reminded twice.
  test('the device id is stable across a platform id change', () async {
    transport.currentToken = 'fcm-token-1';
    final first = registrarWith(FakeDeviceDetails(nativeId: 'native-1'));
    await first.enable();
    final registered = api.bodyOf('/me/devices')!['device_id'];

    // The same local storage, a platform reporting something else - an Android
    // ANDROID_ID change after a reinstall is the real case.
    final second = PushRegistrar(
      transport: transport,
      identities: DeviceIdentities(store: store, details: FakeDeviceDetails(nativeId: 'native-2')),
      registration: DevicePushRegistration(api),
    );
    await second.registerIfPermitted();

    expect(api.bodyOf('/me/devices')!['device_id'], registered);
  });

  test('a platform with no native id still yields a stable device id', () async {
    transport.currentToken = 'fcm-token-1';
    final registrar = registrarWith(FakeDeviceDetails(nativeId: null));

    await registrar.enable();
    final first = api.bodyOf('/me/devices')!['device_id'];

    await registrar.registerIfPermitted();

    expect(first, isNotNull);
    expect(first.toString(), isNotEmpty);
    expect(api.bodyOf('/me/devices')!['device_id'], first);
  });

  // A registration that did not reach the server costs reminders until the next
  // attempt and nothing else: nothing in the app awaits it, so throwing would
  // turn a dropped connection into a crash.
  test('concurrent identity reads share one installation id', () async {
    final identities = DeviceIdentities(store: store, details: FakeDeviceDetails(nativeId: null));
    final values = await Future.wait([identities.current(), identities.current()]);
    expect(values.first.deviceId, values.last.deviceId);
  });

  test('a failed registration is reported, not thrown', () async {
    transport.currentToken = 'fcm-token-1';
    api.respondWith('/me/devices', const ApiError(status: 500, code: 'INTERNAL', message: 'boom'));

    final token = await registrarWith(FakeDeviceDetails()).enable();

    expect(token, isNull, reason: 'a failed upload must not report reminders enabled');
  });

  test('a session restored before push setup registers without another auth event', () async {
    transport.currentToken = 'apns-current';
    final container = ProviderContainer(overrides: [
      authControllerProvider.overrideWith(SignedInAuth.new),
      pushTransportProvider.overrideWithValue(transport),
      apiClientProvider.overrideWithValue(api),
      keyValueStoreProvider.overrideWithValue(store),
      deviceDetailsProvider.overrideWithValue(FakeDeviceDetails()),
    ]);
    addTearDown(container.dispose);
    container.read(pushRegistrarProvider);
    await pumpEventQueue();
    expect(api.callCount('/me/devices'), 1);
    expect(transport.permissionRequests, 0);
  });

  test('token rotation after sign-out does not register a device', () async {
    var signedIn = true;
    final registrar = PushRegistrar(
      transport: transport,
      identities: DeviceIdentities(store: store, details: FakeDeviceDetails()),
      registration: DevicePushRegistration(api),
      isSignedIn: () => signedIn,
    );
    addTearDown(registrar.dispose);
    await registrar.start();
    signedIn = false;
    transport.rotate('rotated-after-logout');
    await pumpEventQueue();
    expect(api.callCount('/me/devices'), 0);
  });

  testWidgets('reminder tile asks permission, registers and saves the server preference', (tester) async {
    transport.currentToken = 'apns-current';
    api.respond('/me/notifications', {});
    final registrar = registrarWith(FakeDeviceDetails());
    addTearDown(registrar.dispose);
    await tester.pumpWidget(ProviderScope(overrides: [
      pushRegistrarProvider.overrideWithValue(registrar),
      apiClientProvider.overrideWithValue(api),
    ], child: const MaterialApp(home: NotificationsScreen())));
    await tester.pumpAndSettle();
    await tester.tap(find.text('This device'));
    await tester.pumpAndSettle();
    expect(transport.permissionRequests, 1);
    expect(api.bodyOf('/me/devices')!['push_token'], 'apns-current');
    expect(api.bodyOf('/me/notifications', method: 'PATCH')!['scheduled_sessions'], true);
    expect(find.text('Reminders enabled on this device.'), findsOneWidget);
  });

  group('notification deep links', () {
    test('a schedule reminder names the schedule it is about', () {
      expect(
        scheduleIdFromLink(const PushTap(
          deepLink: 'iconfess://schedules/sch-7/start',
          data: {'schedule_id': 'sch-7'},
        )),
        'sch-7',
      );
      // The id in the link is usable on its own, which matters because a
      // payload assembled by hand in the console has no data map.
      expect(
        scheduleIdFromLink(const PushTap(deepLink: 'iconfess://schedules/sch-9/start')),
        'sch-9',
      );
    });

    test('an unfamiliar link is dropped rather than guessed at', () {
      expect(scheduleIdFromLink(const PushTap(deepLink: 'https://example.com/evil')), isNull);
      expect(scheduleIdFromLink(const PushTap(deepLink: 'iconfess://open/whatever')), isNull);
      expect(scheduleIdFromLink(const PushTap()), isNull);
      expect(scheduleIdFromLink(const PushTap(deepLink: 'iconfess://schedules/../start')), isNull);
      expect(scheduleIdFromLink(const PushTap(deepLink: 'iconfess://schedules/a%2Fb/start')), isNull);
      expect(scheduleIdFromLink(const PushTap(deepLink: 'iconfess://schedules/a/start',
          data: {'schedule_id': 'other'})), isNull);
    });

    // Tapping opens the session the schedule builds. It has to ask the server,
    // because only the server can turn a saved intention into a session under
    // the entitlement rules in force at that moment.
    test('a reminder starts the session through the API', () async {
      api.respond('/schedules/sch-7/start', {'session_id': 'sess-1'});
      transport.currentToken = 'fcm-token-1';

      final container = ProviderContainer(overrides: [
        pushTransportProvider.overrideWithValue(transport),
        apiClientProvider.overrideWithValue(api),
      ]);
      addTearDown(container.dispose);

      final first = container.listen(pushDeepLinkProvider, (_, _) {});
      addTearDown(first.close);

      await pumpEventQueue();
      transport.tap(const PushTap(
        deepLink: 'iconfess://schedules/sch-7/start',
        data: {'schedule_id': 'sch-7'},
      ));

      final path = await container.read(pushDeepLinkProvider.future);
      expect(path, '/player/sess-1');
      expect(api.callCount('/schedules/sch-7/start'), 1);
    });

    // A plan that lapsed between scheduling and delivery must not leave the
    // user on a screen with nothing on it.
    test('a reminder that cannot start falls back to Activity', () async {
      api.respondWith(
        '/schedules/sch-7/start',
        const ApiError(status: 402, code: 'ENTITLEMENT_REQUIRED', message: 'plan limit'),
      );

      final container = ProviderContainer(overrides: [
        pushTransportProvider.overrideWithValue(transport),
        apiClientProvider.overrideWithValue(api),
      ]);
      addTearDown(container.dispose);
      final first = container.listen(pushDeepLinkProvider, (_, _) {});
      addTearDown(first.close);

      await pumpEventQueue();
      transport.tap(const PushTap(deepLink: 'iconfess://schedules/sch-7/start'));

      expect(await container.read(pushDeepLinkProvider.future), '/activity');
    });
  });
}
