import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../core/routing/routes.dart';
import 'auth_controller.dart';
import 'validators.dart';
import 'widgets/auth_form.dart';
import 'widgets/auth_form_state.dart';
import 'package:iconfess_api/iconfess_api.dart';

/// Create an account.
///
/// Both non-failure outcomes end on the verification screen, and that is not
/// sloppiness — it is the server's design. `/auth/register` answers a duplicate
/// address exactly as it answers a new one, minus the session, so that the
/// endpoint cannot be used to find out who has an account (S65). A screen that
/// said "that email is taken" would undo a deliberate security decision, so this
/// one does not: it shows the same next step either way and lets the difference
/// be whether the listener is already signed in.
class SignUpScreen extends ConsumerStatefulWidget {
  const SignUpScreen({super.key});

  @override
  ConsumerState<SignUpScreen> createState() => _SignUpScreenState();
}

class _SignUpScreenState extends ConsumerState<SignUpScreen> with AuthFormState {
  final _name = TextEditingController();
  final _email = TextEditingController();
  final _password = TextEditingController();
  final _confirm = TextEditingController();

  final _nameFocus = FocusNode();
  final _emailFocus = FocusNode();
  final _passwordFocus = FocusNode();
  final _confirmFocus = FocusNode();

  String? _nameError;
  String? _emailError;
  String? _passwordError;
  String? _confirmError;

  @override
  void dispose() {
    _name.dispose();
    _email.dispose();
    _password.dispose();
    _confirm.dispose();
    _nameFocus.dispose();
    _emailFocus.dispose();
    _passwordFocus.dispose();
    _confirmFocus.dispose();
    super.dispose();
  }

  Future<void> _submit() async {
    final nameError = AuthValidators.displayName(_name.text);
    final emailError = AuthValidators.email(_email.text);
    final passwordError = AuthValidators.password(_password.text);
    // Validated against what is in the field right now, not against a snapshot:
    // editing the first password after typing the second is the common case.
    final confirmError = AuthValidators.confirm(_password.text)(_confirm.text);

    setState(() {
      _nameError = nameError;
      _emailError = emailError;
      _passwordError = passwordError;
      _confirmError = confirmError;
    });

    if (nameError != null ||
        emailError != null ||
        passwordError != null ||
        confirmError != null) {
      markSubmitted();
      failureFeedback();
      return;
    }

    final email = _email.text.trim();
    final outcome = await runGuarded(
      () => ref.read(authControllerProvider.notifier).register(
            email: email,
            password: _password.text,
            displayName: _name.text.trim(),
            // Sent so scheduled sessions and "this evening" mean the listener's
            // evening rather than the server's.
            timezone: DateTime.now().timeZoneName,
          ),
    );
    if (outcome == null || !mounted) return;

    switch (outcome) {
      case AccountCreated():
      case CheckYourEmail():
        // Both go to the same place on purpose. See the class comment.
        context.go('${AppRoutes.verification}?email=${Uri.encodeComponent(email)}');
      case RegisterFailed(:final error):
        reportFailure(error);
    }
  }

  @override
  Widget build(BuildContext context) {
    return AutofillGroup(
      child: AuthScaffold(
        title: 'Create account',
        onBack: () => context.go(AppRoutes.onboarding),
        heading: 'Create your account',
        subheading: 'Your sessions, your saved confessions and your history — kept '
            'when you move to another phone.',
        body: [
          if (error != null) ErrorBanner(title: error!.title, message: error!.message),
          AuthTextField(
            label: 'Name (optional)',
            controller: _name,
            focusNode: _nameFocus,
            nextFocus: _emailFocus,
            textCapitalization: TextCapitalization.words,
            autofillHints: const [AutofillHints.name],
            errorText: submitted ? _nameError : null,
            enabled: !busy,
            onChanged: (_) => _clear(() => _nameError = null),
          ),
          AuthTextField(
            label: 'Email',
            controller: _email,
            focusNode: _emailFocus,
            nextFocus: _passwordFocus,
            keyboardType: TextInputType.emailAddress,
            autofillHints: const [AutofillHints.email],
            errorText: submitted ? _emailError : null,
            enabled: !busy,
            onChanged: (_) => _clear(() => _emailError = null),
          ),
          AuthPasswordField(
            label: 'Password',
            controller: _password,
            focusNode: _passwordFocus,
            nextFocus: _confirmFocus,
            autofillHints: const [AutofillHints.newPassword],
            // The policy, stated once, in the words the server would use if the
            // client stayed quiet. NIST's guidance is length over composition,
            // so there is no "must contain a symbol" here — that rule makes
            // people append "1!" to the word they were already using.
            helperText: 'At least ${PasswordPolicy.minLength} characters. '
                'Longer is better than more complicated.',
            errorText: submitted ? _passwordError : null,
            enabled: !busy,
          ),
          AuthPasswordField(
            label: 'Confirm password',
            controller: _confirm,
            focusNode: _confirmFocus,
            onSubmitted: _submit,
            autofillHints: const [AutofillHints.newPassword],
            errorText: submitted ? _confirmError : null,
            enabled: !busy,
            onChanged: (_) => _clear(() => _confirmError = null),
          ),
        ],
        bottom: Column(
          children: [
            PrimaryActionButton(
              label: 'Create account',
              busyLabel: 'Creating',
              busy: busy,
              onPressed: _submit,
            ),
            AuthSwitchPrompt(
              question: 'Already have an account?',
              actionLabel: 'Sign in',
              onAction: () => context.go(AppRoutes.signIn),
            ),
          ],
        ),
      ),
    );
  }

  void _clear(void Function() clear) {
    if (!submitted) return;
    setState(clear);
  }
}
