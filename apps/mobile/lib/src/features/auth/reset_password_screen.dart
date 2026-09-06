import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:iconfess_api/iconfess_api.dart';

import '../../core/error/error_mapper.dart';
import '../../core/routing/routes.dart';
import 'auth_controller.dart';
import 'validators.dart';
import 'widgets/auth_form.dart';
import 'widgets/auth_form_state.dart';

/// Set a new password with a token from the email.
///
/// The token arrives in a link that opens in a browser, and this app does not
/// register a universal link yet — that is PHASE 19 condition C-3 — so for now
/// the token is entered here by hand. The route still reads `?token=` so the day
/// the deep link is wired, the field is simply prefilled and this screen does not
/// change shape.
///
/// A successful reset ends with the listener signed out *everywhere*, which is
/// the server's decision and not this screen's: whoever held the old password
/// may have been an attacker, so every session is ended (S34). The completion
/// screen says so, because arriving at sign-in after changing a password looks
/// like a bug unless it is explained.
class ResetPasswordScreen extends ConsumerStatefulWidget {
  const ResetPasswordScreen({this.token = '', super.key});

  final String token;

  @override
  ConsumerState<ResetPasswordScreen> createState() =>
      _ResetPasswordScreenState();
}

class _ResetPasswordScreenState extends ConsumerState<ResetPasswordScreen>
    with AuthFormState {
  @override
  AuthSurface get surface => AuthSurface.token;
  late final TextEditingController _token =
      TextEditingController(text: widget.token);
  final _password = TextEditingController();
  final _confirm = TextEditingController();

  final _tokenFocus = FocusNode();
  final _passwordFocus = FocusNode();
  final _confirmFocus = FocusNode();

  String? _tokenError;
  String? _passwordError;
  String? _confirmError;

  @override
  void dispose() {
    _token.dispose();
    _password.dispose();
    _confirm.dispose();
    _tokenFocus.dispose();
    _passwordFocus.dispose();
    _confirmFocus.dispose();
    super.dispose();
  }

  Future<void> _submit() async {
    final tokenError = AuthValidators.token(_token.text);
    final passwordError = AuthValidators.password(_password.text);
    final confirmError = AuthValidators.confirm(_password.text)(_confirm.text);

    setState(() {
      _tokenError = tokenError;
      _passwordError = passwordError;
      _confirmError = confirmError;
    });

    if (tokenError != null || passwordError != null || confirmError != null) {
      markSubmitted();
      failureFeedback();
      return;
    }

    final result = await runGuarded(
      () => ref.read(authControllerProvider.notifier).resetPassword(
            token: AuthValidators.tokenFromPaste(_token.text),
            password: _password.text,
          ),
    );
    if (result == null || !mounted) return;

    if (result is WriteFailure<void>) {
      reportFailure(result.error);
      return;
    }

    context.go('${AppRoutes.completion}?reason=reset');
  }

  @override
  Widget build(BuildContext context) {
    return AutofillGroup(
      child: AuthScaffold(
        title: 'New password',
        onBack: () => context.go(AppRoutes.signIn),
        heading: 'Choose a new password',
        subheading: 'This ends your sign-in on every device, so you’ll need to '
            'sign in again afterwards.',
        body: [
          if (error != null) ErrorBanner(title: error!.title, message: error!.message),
          AuthTextField(
            label: 'Reset code',
            controller: _token,
            focusNode: _tokenFocus,
            nextFocus: _passwordFocus,
            errorText: submitted ? _tokenError : null,
            enabled: !busy,
            helperText: widget.token.isEmpty
                ? 'From the link in the reset email.'
                : 'Taken from the link in your email.',
            onChanged: (_) {
              if (submitted) setState(() => _tokenError = null);
            },
          ),
          AuthPasswordField(
            label: 'New password',
            controller: _password,
            focusNode: _passwordFocus,
            nextFocus: _confirmFocus,
            autofillHints: const [AutofillHints.newPassword],
            helperText: 'At least ${PasswordPolicy.minLength} characters. '
                'Longer is better than more complicated.',
            errorText: submitted ? _passwordError : null,
            enabled: !busy,
          ),
          AuthPasswordField(
            label: 'Confirm new password',
            controller: _confirm,
            focusNode: _confirmFocus,
            onSubmitted: _submit,
            autofillHints: const [AutofillHints.newPassword],
            errorText: submitted ? _confirmError : null,
            enabled: !busy,
            onChanged: (_) {
              if (submitted) setState(() => _confirmError = null);
            },
          ),
        ],
        bottom: PrimaryActionButton(
          label: 'Update password',
          busyLabel: 'Updating',
          busy: busy,
          onPressed: _submit,
        ),
      ),
    );
  }
}
