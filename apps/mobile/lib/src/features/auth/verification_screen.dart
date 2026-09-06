import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:iconfess_api/iconfess_api.dart';

import '../../core/error/error_mapper.dart';
import '../../core/routing/routes.dart';
import '../../core/theme/theme.dart';
import '../../core/theme/tokens.dart';
import 'auth_controller.dart';
import 'validators.dart';
import 'widgets/auth_form.dart';
import 'widgets/auth_form_state.dart';

/// "Check your email."
///
/// This screen exists because registration cannot finish in one step: the
/// address has to be proven, and the proof is a link in an inbox the app cannot
/// read. What it must not do is dead-end — a listener sitting on a screen with
/// no button is a listener who uninstalls.
///
/// Three ways out, in the order someone will need them:
///
///  1. Open the link in the email. The normal path, and it happens outside the
///     app entirely, which is why the screen has to survive being left and
///     returned to.
///  2. Send the email again, when it did not arrive. Rate-limited locally as
///     well as by the server: `EmailSend` allows four sends per fifteen minutes,
///     and a button that can be tapped twenty times in five seconds just
///     guarantees a 429 the listener then has to read.
///  3. Enter the token by hand, for someone whose mail client is on a desktop or
///     whose link will not open on the phone. The server accepts it; hiding that
///     path would strand them.
///
/// Registration issues a session whether or not the address is confirmed, so the
/// "Continue" action is real and not a bypass: it goes to Home for a listener
/// who is signed in, and to sign-in for one who is not.
class VerificationScreen extends ConsumerStatefulWidget {
  const VerificationScreen({
    this.email = '',
    this.initialToken = '',
    super.key,
  });

  final String email;

  /// A token that arrived with the location, from the link the email sends.
  ///
  /// Prefilled rather than submitted: arriving with a token is not consent to
  /// spend it, and the screen has to show what it is about to send. It also has
  /// to survive being wrong — an expired link lands here exactly like a fresh
  /// one, and the only honest thing is to let the listener press the button and
  /// read the answer.
  final String initialToken;

  /// Local floor between resend attempts. The server's own limit is four per
  /// fifteen minutes; this stops the common case of a tap-fest before it starts.
  static const resendCooldown = Duration(seconds: 30);

  @override
  ConsumerState<VerificationScreen> createState() => _VerificationScreenState();
}

class _VerificationScreenState extends ConsumerState<VerificationScreen>
    with AuthFormState {
  @override
  AuthSurface get surface => AuthSurface.token;
  final _token = TextEditingController();
  final _tokenFocus = FocusNode();

  @override
  void initState() {
    super.initState();
    if (widget.initialToken.isNotEmpty) {
      _token.text = AuthValidators.tokenFromPaste(widget.initialToken);
    }
  }

  String? _tokenError;
  bool _resent = false;
  bool _verified = false;
  int _cooldownSeconds = 0;
  Timer? _cooldownTimer;

  @override
  void dispose() {
    _cooldownTimer?.cancel();
    _token.dispose();
    _tokenFocus.dispose();
    super.dispose();
  }

  void _startCooldown(Duration duration) {
    _cooldownTimer?.cancel();
    setState(() => _cooldownSeconds = duration.inSeconds);
    _cooldownTimer = Timer.periodic(const Duration(seconds: 1), (timer) {
      if (!mounted) {
        timer.cancel();
        return;
      }
      setState(() => _cooldownSeconds -= 1);
      if (_cooldownSeconds <= 0) timer.cancel();
    });
  }

  Future<void> _resend() async {
    if (_cooldownSeconds > 0 || widget.email.isEmpty) return;

    final result = await runGuarded(
      () => ref
          .read(authControllerProvider.notifier)
          .resendVerification(widget.email),
    );
    if (result == null || !mounted) return;

    if (result is WriteFailure<void>) {
      reportFailure(result.error);
      // Honour the server's wait when it sent one; guessing a shorter one just
      // produces another 429.
      final error = result.error;
      final retryAfter =
          error is ApiError ? error.retryAfter : null;
      _startCooldown(retryAfter ?? VerificationScreen.resendCooldown);
      return;
    }

    setState(() => _resent = true);
    _startCooldown(VerificationScreen.resendCooldown);
  }

  Future<void> _verify() async {
    final tokenError = AuthValidators.token(_token.text);
    setState(() => _tokenError = tokenError);
    if (tokenError != null) {
      markSubmitted();
      failureFeedback();
      return;
    }

    final result = await runGuarded(
      () => ref
          .read(authControllerProvider.notifier)
          .verifyEmail(AuthValidators.tokenFromPaste(_token.text)),
    );
    if (result == null || !mounted) return;

    if (result is WriteFailure<void>) {
      reportFailure(result.error);
      return;
    }
    setState(() => _verified = true);
  }

  void _continue() {
    final signedIn = ref.read(authControllerProvider).isSignedIn;
    context.go(signedIn ? AppRoutes.home : AppRoutes.signIn);
  }

  @override
  Widget build(BuildContext context) {
    final surfaces = AppSurfaces.of(context);
    final auth = ref.watch(authControllerProvider);
    final cooling = _cooldownSeconds > 0;

    return AuthScaffold(
      title: 'Confirm your email',
      onBack: auth.isSignedIn ? null : () => context.go(AppRoutes.signIn),
      heading: _verified ? 'Email confirmed' : 'Check your email',
      subheading: _verified
          ? 'Your address is confirmed. You’re ready to build your first session.'
          : widget.email.isEmpty
              ? 'We sent a confirmation link to the address on your account.'
              : 'We sent a confirmation link to ${widget.email}. It expires in '
                  '60 minutes and works once.',
      body: [
        if (error != null) ErrorBanner(title: error!.title, message: error!.message),
        if (_resent && !_verified)
          const SuccessBanner(
            title: 'Sent again',
            message: 'If an account exists for that address, the link is on its '
                'way. Check the spam folder if it does not appear.',
          ),
        if (_verified)
          const SuccessBanner(
            title: 'Confirmed',
            message: 'This address is now verified.',
          ),

        if (!cooling && !_resent && widget.email.isNotEmpty)
          Text(
            'Didn’t arrive? Send it again — four times per fifteen minutes.',
            style: IConfess.bodySm.copyWith(color: surfaces.textSecondary),
          ),

        const SizedBox(height: IConfess.space4),

        // The manual path. Kept below the fold of the explanation because it is
        // the rare case, but present because the alternative is a listener with
        // no way to finish on the device they are holding.
        if (!_verified) ...[
          AuthTextField(
            label: 'Or enter the code from the email',
            controller: _token,
            focusNode: _tokenFocus,
            onSubmitted: _verify,
            errorText: submitted ? _tokenError : null,
            enabled: !busy,
            helperText: 'Copy the code at the end of the confirmation link.',
            onChanged: (_) {
              if (submitted) setState(() => _tokenError = null);
            },
          ),
          PrimaryActionButton(
            label: 'Confirm address',
            busyLabel: 'Confirming',
            busy: busy,
            onPressed: _verify,
          ),
          const SizedBox(height: IConfess.space2),
        ],
      ],
      bottom: Column(
        children: [
          PrimaryActionButton(
            label: auth.isSignedIn ? 'Continue to the app' : 'Go to sign in',
            onPressed: _continue,
          ),
          if (!_verified)
            TextButton(
              // Disabled during the cooldown and when there is no address to
              // send to, rather than tappable and then rejected: a button that
              // does nothing on tap teaches the listener that buttons lie.
              onPressed: (cooling || busy || widget.email.isEmpty) ? null : _resend,
              child: Text(
                cooling
                    ? 'Send again in ${_cooldownSeconds}s'
                    : 'Send the email again',
              ),
            ),
        ],
      ),
    );
  }
}
