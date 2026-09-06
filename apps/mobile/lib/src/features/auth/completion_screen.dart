import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../core/routing/routes.dart';
import '../../core/theme/theme.dart';
import '../../core/theme/tokens.dart';
import 'auth_controller.dart';
import 'widgets/auth_form.dart';

/// What a flow ends with.
///
/// Its own route rather than a state on the previous screen for one reason: it
/// cannot be reached by going back. A listener who has just reset a password and
/// presses back should not land on a form that would submit the same token a
/// second time — the server rejects it, and "invalid or expired reset token"
/// after a success is the kind of message that makes people think the change
/// did not take.
///
/// It is also where the one surprising consequence of a reset is explained. The
/// server ends every session when a password is reset (S34), so arriving at
/// sign-in immediately afterwards is correct behaviour that looks like a bug
/// unless it is said out loud.
enum CompletionReason {
  /// A password was just changed.
  reset,

  /// An email address was just confirmed.
  verified;

  /// Parses the route's `reason` parameter.
  ///
  /// Unknown values fall back to [reset] rather than throwing: this screen is
  /// reachable from a link, and a malformed query string must not be a crash.
  static CompletionReason parse(String? raw) =>
      CompletionReason.values.firstWhere(
        (r) => r.name == raw,
        orElse: () => CompletionReason.reset,
      );
}

class CompletionScreen extends ConsumerWidget {
  const CompletionScreen({this.reason = CompletionReason.reset, super.key});

  final CompletionReason reason;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final surfaces = AppSurfaces.of(context);
    final signedIn = ref.watch(authControllerProvider).isSignedIn;

    final isReset = reason == CompletionReason.reset;
    final heading = isReset ? 'Password updated' : 'Email confirmed';
    final body = isReset
        ? 'You can sign in with your new password now. For your protection, '
            'every device that was signed in has been signed out — including this one.'
        : 'Your address is confirmed, so your account is ready to use.';
    final action = isReset || !signedIn ? 'Go to sign in' : 'Continue to the app';

    return Scaffold(
      backgroundColor: Theme.of(context).scaffoldBackgroundColor,
      body: SafeArea(
        child: Padding(
          padding: const EdgeInsets.fromLTRB(
            IConfess.space5,
            IConfess.space9,
            IConfess.space5,
            IConfess.space5,
          ),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Container(
                width: 56,
                height: 56,
                decoration: BoxDecoration(
                  color: surfaces.success.withValues(alpha: 0.12),
                  shape: BoxShape.circle,
                ),
                child: Icon(
                  Icons.check_rounded,
                  color: surfaces.success,
                  size: 28,
                ),
              ),
              const SizedBox(height: IConfess.space6),
              Text(
                heading,
                style: IConfess.heading.copyWith(color: surfaces.textPrimary),
              ),
              const SizedBox(height: IConfess.space4),
              Text(
                body,
                style: IConfess.body.copyWith(
                  color: surfaces.textSecondary,
                  height: 1.6,
                ),
              ),
              const Spacer(),
              PrimaryActionButton(
                label: action,
                onPressed: () => context.go(
                  (isReset || !signedIn) ? AppRoutes.signIn : AppRoutes.home,
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }
}
