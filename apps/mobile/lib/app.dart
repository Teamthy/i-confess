import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'src/core/analytics/analytics.dart';
import 'src/core/di/providers.dart';
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
