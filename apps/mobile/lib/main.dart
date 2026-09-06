import 'package:flutter/foundation.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:shared_preferences/shared_preferences.dart';

import 'app.dart';
import 'src/core/di/providers.dart';
import 'src/features/auth/auth_controller.dart';

Future<void> main() async {
  WidgetsFlutterBinding.ensureInitialized();

  // Fail loudly in debug rather than rendering a red screen for every framework
  // exception; in release the error still reaches the zone handler.
  FlutterError.onError = (details) {
    FlutterError.presentError(details);
    if (kDebugMode) debugPrint(details.exceptionAsString());
  };

  final prefs = await SharedPreferences.getInstance();

  final container = ProviderContainer(
    overrides: [sharedPreferencesProvider.overrideWithValue(prefs)],
  );

  // Resolve the session before the first frame.
  //
  // Doing this inside the widget tree would show the sign-in screen for a frame
  // to every signed-in listener on every cold start, which reads as having been
  // logged out. The read is a single keystore lookup, so the cost is a few
  // milliseconds of native splash rather than a visible flash.
  await container.read(authControllerProvider.notifier).restore();
  recordAppOpened(container);

  runApp(UncontrolledProviderScope(container: container, child: const IConfessApp()));
}
