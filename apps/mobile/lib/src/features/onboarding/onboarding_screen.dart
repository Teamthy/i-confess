import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../core/routing/routes.dart';
import '../../core/theme/theme.dart';
import '../../core/theme/tokens.dart';
import '../auth/auth_controller.dart';
import '../auth/widgets/auth_form.dart';
import 'onboarding_controller.dart';

/// The three-slide tour, shown once.
///
/// Three slides and no more. Each one answers a question a new listener
/// actually has before they will hand over an email address — what this says,
/// how long it takes, and what it sounds like — and nothing else. An onboarding
/// that explains the product by listing its features is a brochure, and a
/// brochure in a modal is a reason to close the app.
///
/// Skippable, and skipping is recorded as completion. Three slides that can only
/// be escaped by finishing them are not onboarding, they are a toll.
class OnboardingScreen extends ConsumerStatefulWidget {
  const OnboardingScreen({super.key});

  @override
  ConsumerState<OnboardingScreen> createState() => _OnboardingScreenState();
}

class _OnboardingScreenState extends ConsumerState<OnboardingScreen> {
  final _pages = PageController();
  int _index = 0;

  static const _slides = <_Slide>[
    _Slide(
      icon: Icons.auto_stories_rounded,
      title: 'Say what you need',
      body: 'Thirty-nine categories of confession and Scripture — anxiety, '
          'gratitude, forgiveness, strength. Pick where you are today, not '
          'where you think you should be.',
    ),
    _Slide(
      icon: Icons.schedule_rounded,
      title: 'Set the length',
      body: 'Ten minutes before work, or three hours on a Sunday. The session is '
          'planned to fill the time you actually have, and it stops when it is '
          'done.',
    ),
    _Slide(
      icon: Icons.record_voice_over_rounded,
      title: 'Choose the voice',
      body: 'Everything is spoken, so you can listen while you walk, drive or '
          'lie down. If a voice does not suit you, choose another.',
    ),
  ];

  @override
  void dispose() {
    _pages.dispose();
    super.dispose();
  }

  bool get _isLast => _index == _slides.length - 1;

  Future<void> _finish() async {
    await ref.read(onboardingControllerProvider.notifier).complete();
    if (!mounted) return;
    // An existing user who was walked here from Welcome by mistake should be
    // able to sign in from the last slide rather than being pushed into
    // creating a second account.
    final signedIn = ref.read(authControllerProvider).isSignedIn;
    context.go(signedIn ? AppRoutes.home : AppRoutes.signUp);
  }

  Future<void> _skip() async {
    await ref.read(onboardingControllerProvider.notifier).complete();
    if (!mounted) return;
    context.go(AppRoutes.signUp);
  }

  @override
  Widget build(BuildContext context) {
    final surfaces = AppSurfaces.of(context);

    return Scaffold(
      backgroundColor: Theme.of(context).scaffoldBackgroundColor,
      body: SafeArea(
        child: Column(
          children: [
            Align(
              alignment: Alignment.centerRight,
              child: Padding(
                padding: const EdgeInsets.symmetric(horizontal: IConfess.space2),
                child: TextButton(
                  onPressed: _skip,
                  child: const Text('Skip'),
                ),
              ),
            ),
            Expanded(
              child: PageView.builder(
                controller: _pages,
                itemCount: _slides.length,
                onPageChanged: (i) => setState(() => _index = i),
                itemBuilder: (context, i) {
                  final slide = _slides[i];
                  return Padding(
                    padding: const EdgeInsets.fromLTRB(
                      IConfess.space5,
                      IConfess.space9,
                      IConfess.space5,
                      IConfess.space6,
                    ),
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        Container(
                          width: 64,
                          height: 64,
                          decoration: BoxDecoration(
                            color: surfaces.surface,
                            borderRadius: BorderRadius.circular(IConfess.radiusLg),
                            border: Border.all(color: surfaces.border),
                          ),
                          child: Icon(slide.icon, color: surfaces.primary, size: 28),
                        ),
                        const SizedBox(height: IConfess.space7),
                        Text(
                          slide.title,
                          style: IConfess.heading.copyWith(color: surfaces.textPrimary),
                        ),
                        const SizedBox(height: IConfess.space4),
                        Text(
                          slide.body,
                          style: IConfess.body.copyWith(
                            color: surfaces.textSecondary,
                            height: 1.6,
                          ),
                        ),
                      ],
                    ),
                  );
                },
              ),
            ),
            // The dots are announced as one control with a spoken position
            // rather than three unlabelled shapes. A screen reader user swiping
            // through "button, button, button" learns nothing about where they
            // are in the tour.
            Semantics(
              label: 'Slide ${_index + 1} of ${_slides.length}',
              container: true,
              excludeSemantics: true,
              child: Row(
                mainAxisAlignment: MainAxisAlignment.center,
                children: [
                  for (var i = 0; i < _slides.length; i++)
                    AnimatedContainer(
                      duration: IConfess.motionDurationFast,
                      margin: const EdgeInsets.symmetric(horizontal: 4),
                      width: i == _index ? 20 : 8,
                      height: 8,
                      decoration: BoxDecoration(
                        color: i == _index ? surfaces.primary : surfaces.border,
                        borderRadius: BorderRadius.circular(IConfess.radiusFull),
                      ),
                    ),
                ],
              ),
            ),
            Padding(
              padding: const EdgeInsets.fromLTRB(
                IConfess.space5,
                IConfess.space6,
                IConfess.space5,
                IConfess.space5,
              ),
              child: Column(
                children: [
                  PrimaryActionButton(
                    label: _isLast ? 'Create your account' : 'Next',
                    onPressed: _isLast
                        ? _finish
                        : () => _pages.nextPage(
                              duration: IConfess.motionDurationDeliberate,
                              curve: Curves.easeOutCubic,
                            ),
                  ),
                  if (!_isLast)
                    AuthSwitchPrompt(
                      question: 'Already have an account?',
                      actionLabel: 'Sign in',
                      onAction: () => context.go(AppRoutes.signIn),
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

class _Slide {
  const _Slide({required this.icon, required this.title, required this.body});

  final IconData icon;
  final String title;
  final String body;
}
