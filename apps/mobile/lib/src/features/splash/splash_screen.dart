import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../core/theme/theme.dart';
import '../../core/theme/tokens.dart';
import '../auth/widgets/auth_form.dart';

/// The route the app opens on.
///
/// It is a real screen and not a delay, and the distinction matters. A splash
/// that shows a logo for a fixed two seconds is two seconds taken from every
/// launch to advertise the company to someone who already opened the app. This
/// one exists for two reasons that are not marketing:
///
///  1. It is where routing starts, so there is always a destination while the
///     session is being resolved. Deciding nothing is better than flashing
///     sign-in at every signed-in listener.
///  2. It has to hand off. PHASE 18's own test caught the app opening here for a
///     signed-out listener and never leaving, because the only exit was a
///     successful sign-in. The router now moves a signed-out listener to
///     [welcome] and a signed-in one to [home]; this screen never has to decide
///     for itself, and a deep link that lands here cannot dead-end.
///
/// In a release build the native launch screen covers the keystore read, so this
/// is usually visible for a frame. That is the point.
///
/// It does not watch the session itself: [createRouter] does, and rebuilding the
/// router is what moves the listener off this screen.
class SplashScreen extends ConsumerWidget {
  const SplashScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final surfaces = AppSurfaces.of(context);

    return Semantics(
      label: 'I Confess',
      child: Scaffold(
        backgroundColor: Theme.of(context).scaffoldBackgroundColor,
        body: SafeArea(
          child: Padding(
            padding: const EdgeInsets.symmetric(horizontal: IConfess.space6),
            child: Column(
              mainAxisAlignment: MainAxisAlignment.center,
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                const BrandMark(large: true),
                const SizedBox(height: IConfess.space4),
                Text(
                  'Speak. Believe.',
                  style: IConfess.subheading.copyWith(color: surfaces.textSecondary),
                ),
                // Deliberately no progress indicator while the session is
                // unknown. `main` resolves the keystore before `runApp`, so in
                // a release build this screen is on screen for a frame and a
                // spinner would never be seen; and if that read ever did stall,
                // an animation that runs forever on the launch path would keep
                // the GPU busy while the listener waits for nothing. Standing
                // still says the same thing more honestly.
              ],
            ),
          ),
        ),
      ),
    );
  }
}
