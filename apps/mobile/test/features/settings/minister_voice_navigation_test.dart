import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:go_router/go_router.dart';
import 'package:iconfess/src/core/routing/routes.dart';
import 'package:iconfess/src/features/minister_voices/minister_voice_repository.dart';
import 'package:iconfess/src/features/minister_voices/minister_voice_screen.dart';
import 'package:iconfess/src/features/settings/settings_screen.dart';

void main() {
  testWidgets('Settings opens minister voices and back returns to Settings', (tester) async {
    final router = GoRouter(
      initialLocation: '/settings',
      routes: [
        GoRoute(path: '/settings', builder: (_, __) => const SettingsScreen()),
        GoRoute(path: AppRoutes.ministerVoices, builder: (_, __) => const MinisterVoicesScreen()),
      ],
    );
    addTearDown(router.dispose);
    await tester.pumpWidget(ProviderScope(
      overrides: [
        ministerVoiceLibraryProvider.overrideWith(
          (ref) async => const MinisterVoiceLibrary([], 'Synthetic voice disclosure'),
        ),
      ],
      child: MaterialApp.router(routerConfig: router),
    ));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Minister voices'));
    await tester.pumpAndSettle();

    expect(find.text('No minister voices are available right now.'), findsOneWidget);
    expect(find.text('Synthetic voice disclosure'), findsOneWidget);
    await tester.pageBack();
    await tester.pumpAndSettle();
    expect(find.text('Playback & notifications'), findsOneWidget);
  });
}
