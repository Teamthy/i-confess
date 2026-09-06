import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:iconfess_api/iconfess_api.dart';

import '../../core/routing/routes.dart';
import 'auth_controller.dart';
import 'validators.dart';
import 'widgets/auth_form.dart';
import 'widgets/auth_form_state.dart';

/// Sign in, including the second factor.
///
/// The MFA step lives in this screen rather than on a route of its own, because
/// that is what the API does: `/auth/login` takes the code in the same call, and
/// the server issues no intermediate credential. A separate screen would have to
/// carry the password across a navigation boundary to resend it, which is a
/// secret in a route argument — exactly the thing not to do.
///
/// One detail worth stating, because it is a common bug: the password field here
/// is *not* validated against [PasswordPolicy]. That policy governs passwords
/// being set. Someone whose account predates it may have a six-character
/// password that the server will happily accept, and refusing it here would lock
/// them out of an account they are entitled to.
class SignInScreen extends ConsumerStatefulWidget {
  const SignInScreen({super.key});

  @override
  ConsumerState<SignInScreen> createState() => _SignInScreenState();
}

class _SignInScreenState extends ConsumerState<SignInScreen> with AuthFormState {
  final _email = TextEditingController();
  final _password = TextEditingController();
  final _code = TextEditingController();

  final _emailFocus = FocusNode();
  final _passwordFocus = FocusNode();
  final _codeFocus = FocusNode();

  String? _emailError;
  String? _passwordError;
  String? _codeError;

  /// Set when the server asks for the second factor. Credentials were correct;
  /// the sign-in is simply incomplete, and the wording has to reflect that.
  bool _awaitingCode = false;

  @override
  void dispose() {
    _email.dispose();
    _password.dispose();
    _code.dispose();
    _emailFocus.dispose();
    _passwordFocus.dispose();
    _codeFocus.dispose();
    super.dispose();
  }

  Future<void> _submit() async {
    final emailError = _awaitingCode ? null : AuthValidators.email(_email.text);
    final passwordError = _awaitingCode
        ? null
        : (_password.text.isEmpty ? 'Enter your password.' : null);
    final codeError =
        _awaitingCode ? AuthValidators.secondFactor(_code.text) : null;

    setState(() {
      _emailError = emailError;
      _passwordError = passwordError;
      _codeError = codeError;
    });

    if (emailError != null || passwordError != null || codeError != null) {
      markSubmitted();
      failureFeedback();
      return;
    }

    final outcome = await runGuarded(
      () => ref.read(authControllerProvider.notifier).signIn(
            email: _email.text.trim(),
            password: _password.text,
            code: _awaitingCode ? _code.text.trim() : null,
          ),
    );
    if (outcome == null || !mounted) return;

    switch (outcome) {
      case SignedIn():
        context.go(AppRoutes.home);
      case MfaRequired():
        setState(() {
          _awaitingCode = true;
          _codeError = null;
        });
        _codeFocus.requestFocus();
      case SignInFailed(:final error):
        // A wrong code is a field error, not a page-level failure: the email and
        // password were right, and a banner that reads like a failed sign-in
        // sends someone back to change a password that was never wrong.
        if (_awaitingCode && error is ApiError && error.needsSecondFactor) {
          setState(() => _codeError = 'That code isn’t valid. Enter the next one.');
          _code.clear();
          _codeFocus.requestFocus();
          failureFeedback();
        } else {
          reportFailure(error);
        }
    }
  }

  @override
  Widget build(BuildContext context) {
    return AutofillGroup(
      child: AuthScaffold(
        title: 'Sign in',
        onBack: () => context.go(AppRoutes.welcome),
        heading: _awaitingCode ? 'One more step' : 'Welcome back',
        subheading: _awaitingCode
            ? 'Enter the six-digit code from your authenticator app, or one of your recovery codes.'
            : 'Sign in to pick up your sessions, your saved confessions and your history.',
        body: [
          if (error != null)
            ErrorBanner(title: error!.title, message: error!.message),

          if (!_awaitingCode) ...[
            AuthTextField(
              label: 'Email',
              controller: _email,
              focusNode: _emailFocus,
              nextFocus: _passwordFocus,
              keyboardType: TextInputType.emailAddress,
              autofillHints: const [AutofillHints.email],
              errorText: submitted ? _emailError : null,
              enabled: !busy,
              onChanged: (_) => _clearFieldError(() => _emailError = null),
            ),
            AuthPasswordField(
              label: 'Password',
              controller: _password,
              focusNode: _passwordFocus,
              // The password is the last field before the button, so "done" on
              // the keyboard submits — no trip to the bottom of the screen.
              onSubmitted: _submit,
              errorText: submitted ? _passwordError : null,
              autofillHints: const [AutofillHints.password],
              enabled: !busy,
            ),
            Align(
              alignment: Alignment.centerRight,
              child: TextButton(
                onPressed: busy
                    ? null
                    : () => context.go(
                          '${AppRoutes.forgotPassword}?email=${Uri.encodeComponent(_email.text.trim())}',
                        ),
                child: const Text('Forgot your password?'),
              ),
            ),
          ] else ...[
            AuthTextField(
              label: 'Authentication code',
              controller: _code,
              focusNode: _codeFocus,
              onSubmitted: _submit,
              autofillHints: const [AutofillHints.oneTimeCode],
              // Text rather than number: recovery codes are of the form
              // ABCD-EFGH-JKMN, and a numeric keypad would lock out the exact
              // person who has lost their authenticator.
              keyboardType: TextInputType.text,
              textCapitalization: TextCapitalization.characters,
              errorText: submitted ? _codeError : null,
              enabled: !busy,
              onChanged: (_) => _clearFieldError(() => _codeError = null),
            ),
            TextButton(
              onPressed: busy
                  ? null
                  : () => setState(() {
                        _awaitingCode = false;
                        _codeError = null;
                      }),
              child: const Text('Use a different account'),
            ),
          ],
        ],
        bottom: PrimaryActionButton(
          label: _awaitingCode ? 'Verify' : 'Sign in',
          busyLabel: _awaitingCode ? 'Verifying' : 'Signing in',
          busy: busy,
          onPressed: _submit,
        ),
      ),
    );
  }

  /// Clears one field's error as the listener types, once they have been told
  /// about it.
  ///
  /// Only after a submission attempt: clearing an error that was never shown
  /// costs a rebuild per keystroke for nothing.
  void _clearFieldError(void Function() clear) {
    if (!submitted) return;
    setState(clear);
  }
}
