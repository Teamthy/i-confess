import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:iconfess/src/core/theme/theme.dart';
import 'package:iconfess/src/features/auth/welcome_screen.dart';

/// The golden for the screen a listener sees first.
///
/// One screen, on purpose. A golden per screen is a suite that fails whenever
/// anything is restyled, and after the third spurious failure nobody looks at
/// them again. This one exists because the welcome screen is the whole first
/// impression and because it is built almost entirely from theme values: if a
/// token changes and nothing else is touched, no other test in this package
/// notices, and the app still compiles, passes analyse and passes every
/// behavioural test while looking different.
///
/// Regenerate with `flutter test --update-goldens` and look at the diff before
/// committing it. A golden that is updated without being looked at is a
/// snapshot of whatever the code happens to do.
void main() {
  testWidgets('welcome matches the committed golden', (tester) async {
    tester.view.physicalSize = const Size(1170, 2532);
    tester.view.devicePixelRatio = 3.0;
    addTearDown(() {
      tester.view.resetPhysicalSize();
      tester.view.resetDevicePixelRatio();
    });

    await tester.pumpWidget(
      ProviderScope(
        child: MaterialApp(
          home: const WelcomeScreen(),
          // The real theme, not the ambient one. A golden rendered against
          // MaterialApp's default would still pass after every token changed,
          // which is the exact failure this file exists to catch.
          theme: AppTheme.discovery(Brightness.light),
        ),
      ),
    );
    await tester.pump();

    await expectLater(
      find.byType(WelcomeScreen),
      matchesGoldenFile('goldens/welcome.png'),
    );
  });
}
