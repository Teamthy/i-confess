import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../core/routing/routes.dart';
import '../../core/theme/theme.dart';
import '../../core/theme/tokens.dart';
import 'widgets/auth_form.dart';

/// The first screen a new listener sees.
///
/// Three ways forward, because there are three kinds of person arriving here and
/// they are not interchangeable:
///
///  - someone who has never used the product, who needs the tour;
///  - someone who already has an account, for whom the tour is an obstacle;
///  - someone who wants to look before committing anything, who must be able to.
///
/// The third is a deliberate product decision recorded in [AppRoutes.requiresAuth]:
/// Home and Explore are open. Requiring an account to see what the product is
/// loses the curious, and a listener who cannot hear the catalogue cannot be
/// told what it sounds like.
class WelcomeScreen extends ConsumerWidget {
  const WelcomeScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final surfaces = AppSurfaces.of(context);

    return Scaffold(
      backgroundColor: Theme.of(context).scaffoldBackgroundColor,
      body: SafeArea(
        child: Column(
          children: [
            Expanded(
              child: SingleChildScrollView(
                padding: const EdgeInsets.fromLTRB(
                  IConfess.space5,
                  IConfess.space9,
                  IConfess.space5,
                  IConfess.space6,
                ),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    const BrandMark(large: true),
                    const SizedBox(height: IConfess.space8),
                    Text(
                      'Speak it. Believe it.',
                      style: IConfess.heading.copyWith(color: surfaces.textPrimary),
                    ),
                    const SizedBox(height: IConfess.space4),
                    Text(
                      'Confessions, Scripture meditation and prayer — spoken aloud, '
                      'in the voice you choose, for as long as you have.',
                      style: IConfess.body.copyWith(
                        color: surfaces.textSecondary,
                        height: 1.6,
                      ),
                    ),
                    const SizedBox(height: IConfess.space8),
                    const _WelcomePoint(
                      icon: Icons.auto_stories_rounded,
                      text: 'Thirty-nine categories, from anxiety to gratitude.',
                    ),
                    const _WelcomePoint(
                      icon: Icons.schedule_rounded,
                      text: 'Sessions planned for the time you actually have.',
                    ),
                    const _WelcomePoint(
                      icon: Icons.record_voice_over_rounded,
                      text: 'Spoken, not scrolled — listen while you do anything else.',
                    ),
                  ],
                ),
              ),
            ),
            Padding(
              padding: const EdgeInsets.fromLTRB(
                IConfess.space5,
                IConfess.space3,
                IConfess.space5,
                IConfess.space5,
              ),
              child: Column(
                children: [
                  PrimaryActionButton(
                    label: 'Create your account',
                    onPressed: () => context.go(AppRoutes.onboarding),
                  ),
                  const SizedBox(height: IConfess.space2),
                  SizedBox(
                    width: double.infinity,
                    child: OutlinedButton(
                      onPressed: () => context.go(AppRoutes.signIn),
                      child: const Text('I already have an account'),
                    ),
                  ),
                  // Browsing is a real path, so it is a real button — but it is
                  // the third one, because an account is what makes a session
                  // possible and the app should say what it wants.
                  Center(
                    child: TextButton(
                      onPressed: () => context.go(AppRoutes.home),
                      child: const Text('Just look around for now'),
                    ),
                  ),
                ],
              ),
            ),
          ],
        ),
      ),
    );
  }
}

class _WelcomePoint extends StatelessWidget {
  const _WelcomePoint({required this.icon, required this.text});

  final IconData icon;
  final String text;

  @override
  Widget build(BuildContext context) {
    final surfaces = AppSurfaces.of(context);
    return Padding(
      padding: const EdgeInsets.only(bottom: IConfess.space4),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Icon(icon, size: 20, color: surfaces.primary),
          const SizedBox(width: IConfess.space3),
          Expanded(
            child: Text(
              text,
              style: IConfess.bodySm.copyWith(
                color: surfaces.textPrimary,
                height: 1.5,
              ),
            ),
          ),
        ],
      ),
    );
  }
}
