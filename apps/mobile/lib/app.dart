import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'src/core/analytics/analytics.dart';
import 'src/core/push/push_registration.dart';
import 'src/core/routing/router.dart';
import 'src/core/theme/theme.dart';

/// The application widget.
///
/// Theme and router only — no business logic. Anything that needs state lives in
/// a notifier, so this widget can be rebuilt in a test without side effects.
class IConfessApp extends ConsumerWidget {
  const IConfessApp({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final router = ref.watch(routerProvider);

    // A tap on a reminder opens the session it was about. The path comes from
    // the server's payload and is translated by the push layer before it gets
    // here, so an unrecognised link simply never arrives.
    ref.listen<AsyncValue<String>>(pushDeepLinkProvider, (_, next) {
      final path = next.asData?.value;
      if (path == null) return;
      router.go(path);
    });

    return MaterialApp.router(
      title: 'I CONFESS',
      debugShowCheckedModeBanner: false,
      // Discovery mode follows the system brightness; immersive mode is chosen
      // explicitly by the screens that need it.
      theme: AppTheme.discovery(Brightness.light),
      darkTheme: AppTheme.discovery(Brightness.dark),
      themeMode: ThemeMode.system,
      routerConfig: router,
    );
  }
}

/// Records the cold start.
///
/// Called from `main` rather than from a widget so it fires exactly once per
/// process instead of once per rebuild.
void recordAppOpened(ProviderContainer container) {
  container.read(analyticsProvider).track(AnalyticsEvents.appOpened);
}
