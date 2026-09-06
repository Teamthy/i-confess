import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:iconfess_api/iconfess_api.dart';

import '../../core/routing/routes.dart';
import 'auth_controller.dart';
import 'validators.dart';
import 'widgets/auth_form.dart';
import 'widgets/auth_form_state.dart';

/// Ask for a reset link.
///
/// The confirmation is deliberately uninformative, and that is the requirement
/// rather than a compromise: `/auth/request-password-reset` answers an unknown
/// address exactly as it answers a known one (S33, S65). A screen that said
/// "we've sent you a link" for one address and "no account found" for another
/// would turn a password form into a membership lookup, and would undo a
/// decision the server made on purpose.
///
/// So the success copy carries the server's own hedge. It reads slightly less
/// confident than "sent!", which is the correct trade: an account-enumeration
/// oracle is worse than one cautious sentence.
class ForgotPasswordScreen extends ConsumerStatefulWidget {
  const ForgotPasswordScreen({this.initialEmail = '', super.key});

  /// Prefilled from the sign-in screen, so a listener who realises they have
  /// forgotten their password does not have to retype the address they were
  /// already looking at.
  final String initialEmail;

  @override
  ConsumerState<ForgotPasswordScreen> createState() =>
      _ForgotPasswordScreenState();
}

class _ForgotPasswordScreenState extends ConsumerState<ForgotPasswordScreen>
    with AuthFormState {
  late final TextEditingController _email =
      TextEditingController(text: widget.initialEmail);
  final _emailFocus = FocusNode();
  String? _emailError;
  bool _sent = false;

  @override
  void dispose() {
    _email.dispose();
    _emailFocus.dispose();
    super.dispose();
  }

  Future<void> _submit() async {
    final emailError = AuthValidators.email(_email.text);
    setState(() => _emailError = emailError);
    if (emailError != null) {
      markSubmitted();
      failureFeedback();
      return;
    }

    final result = await runGuarded(
      () => ref
          .read(authControllerProvider.notifier)
          .requestPasswordReset(_email.text.trim()),
    );
    if (result == null || !mounted) return;

    if (result is WriteFailure<void>) {
      reportFailure(result.error);
      return;
    }
    setState(() => _sent = true);
  }

  @override
  Widget build(BuildContext context) {
    return AuthScaffold(
      title: 'Reset password',
      onBack: () => context.go(AppRoutes.signIn),
      heading: _sent ? 'Check your email' : 'Forgot your password?',
      subheading: _sent
          ? 'The link expires in 30 minutes and works once. If it does not arrive, '
              'check the spam folder before requesting another.'
          : 'Enter the address on your account and we’ll send a link to set a new password.',
      body: [
        if (error != null) ErrorBanner(title: error!.title, message: error!.message),
        if (_sent)
          const SuccessBanner(
            title: 'Request received',
            message: 'If an account exists for that address, a reset link is on '
                'its way. Nothing else is needed here.',
          )
        else
          AuthTextField(
            label: 'Email',
            controller: _email,
            focusNode: _emailFocus,
            keyboardType: TextInputType.emailAddress,
            autofillHints: const [AutofillHints.email],
            errorText: submitted ? _emailError : null,
            enabled: !busy,
            onSubmitted: _submit,
            onChanged: (_) {
              if (submitted) setState(() => _emailError = null);
            },
          ),
      ],
      bottom: _sent
          ? PrimaryActionButton(
              label: 'Back to sign in',
              onPressed: () => context.go(AppRoutes.signIn),
            )
          : PrimaryActionButton(
              label: 'Send reset link',
              busyLabel: 'Sending',
              busy: busy,
              onPressed: _submit,
            ),
    );
  }
}
