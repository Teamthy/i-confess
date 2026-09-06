import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import '../../../core/theme/theme.dart';
import '../../../core/theme/tokens.dart';

/// The building blocks shared by every auth screen.
///
/// These exist because the alternative is five screens each with their own text
/// field, their own error colour and their own idea of what a busy button looks
/// like — which is how a product ends up looking assembled from templates
/// rather than designed. Section 12's tokens are what they are built from, so
/// the agreement is enforced rather than intended.

/// The container for a form screen.
///
/// Three decisions live here and nowhere else:
///
///  1. The content scrolls, so a keyboard that covers half the screen still
///     leaves the focused field reachable. A fixed-height form is unusable on a
///     small phone with the keyboard up, which is most of this flow.
///  2. Tapping outside a field dismisses the keyboard. Without it, the only way
///     to see the button under the keyboard is to press back, which on Android
///     leaves the screen.
///  3. Scrolling dismisses the keyboard too, for the same reason.
class AuthScaffold extends StatelessWidget {
  const AuthScaffold({
    required this.body,
    this.title,
    this.heading,
    this.subheading,
    this.bottom,
    this.onBack,
    this.immersive = false,
    super.key,
  });

  final List<Widget> body;
  final String? title;
  final String? heading;
  final String? subheading;

  /// Pinned below the content: the primary action, so it stays under the thumb
  /// instead of scrolling out of reach.
  final Widget? bottom;

  /// When set, shows a back affordance. Null means there is nowhere to go back
  /// to, and a back button that pops to an empty stack is a dead end.
  final VoidCallback? onBack;

  final bool immersive;

  @override
  Widget build(BuildContext context) {
    final theme = immersive ? AppTheme.immersive() : Theme.of(context);

    return Theme(
      data: theme,
      child: Builder(
        builder: (context) {
          final surfaces = AppSurfaces.of(context);
          return GestureDetector(
            // `opaque: false` so this does not swallow taps meant for children.
            behavior: HitTestBehavior.translucent,
            onTap: () => _dismissKeyboard(context),
            child: Scaffold(
              backgroundColor: theme.scaffoldBackgroundColor,
              appBar: (title != null || onBack != null)
                  ? AppBar(
                      title: title == null ? null : Text(title!),
                      automaticallyImplyLeading: false,
                      leading: onBack == null
                          ? null
                          : IconButton(
                              onPressed: onBack,
                              icon: const Icon(Icons.arrow_back_rounded),
                              tooltip: 'Back',
                            ),
                    )
                  : null,
              body: SafeArea(
                top: title == null && onBack == null,
                bottom: bottom == null,
                child: Column(
                  children: [
                    Expanded(
                      child: NotificationListener<ScrollStartNotification>(
                        onNotification: (_) {
                          _dismissKeyboard(context);
                          return false;
                        },
                        child: SingleChildScrollView(
                          keyboardDismissBehavior:
                              ScrollViewKeyboardDismissBehavior.onDrag,
                          padding: const EdgeInsets.fromLTRB(
                            IConfess.space5,
                            IConfess.space6,
                            IConfess.space5,
                            IConfess.space7,
                          ),
                          child: Column(
                            crossAxisAlignment: CrossAxisAlignment.start,
                            children: [
                              if (heading != null)
                                Text(
                                  heading!,
                                  style: IConfess.heading
                                      .copyWith(color: surfaces.textPrimary),
                                ),
                              if (subheading != null) ...[
                                const SizedBox(height: IConfess.space3),
                                Text(
                                  subheading!,
                                  style: IConfess.body.copyWith(
                                    color: surfaces.textSecondary,
                                    height: 1.55,
                                  ),
                                ),
                              ],
                              if (heading != null || subheading != null)
                                const SizedBox(height: IConfess.space7),
                              ...body,
                            ],
                          ),
                        ),
                      ),
                    ),
                    if (bottom != null)
                      Padding(
                        padding: const EdgeInsets.fromLTRB(
                          IConfess.space5,
                          IConfess.space3,
                          IConfess.space5,
                          IConfess.space4,
                        ),
                        child: bottom!,
                      ),
                  ],
                ),
              ),
            ),
          );
        },
      ),
    );
  }

  static void _dismissKeyboard(BuildContext context) =>
      FocusScope.of(context).unfocus();
}

/// A labelled text field with its error text and keyboard behaviour in one.
///
/// The label is always present — never a hint alone. A placeholder that
/// disappears on focus leaves a field with nothing identifying it, which fails
/// both the person scanning the form and the screen reader.
class AuthTextField extends StatelessWidget {
  const AuthTextField({
    required this.label,
    required this.controller,
    this.focusNode,
    this.nextFocus,
    this.onSubmitted,
    this.errorText,
    this.hint,
    this.obscure = false,
    this.keyboardType,
    this.autofillHints,
    this.textCapitalization = TextCapitalization.none,
    this.enabled = true,
    this.maxLength,
    this.helperText,
    this.maxLines = 1,
    this.onChanged,
    super.key,
  });

  final String label;
  final TextEditingController controller;
  final FocusNode? focusNode;

  /// Where "next" on the keyboard goes. Null means this is the last field, and
  /// the action becomes "done".
  final FocusNode? nextFocus;

  /// Called on submit. On the last field this is the same as pressing the
  /// button, which is what makes the form usable without lifting a thumb to the
  /// bottom of the screen.
  final VoidCallback? onSubmitted;

  final String? errorText;
  final String? hint;
  final bool obscure;
  final TextInputType? keyboardType;
  final Iterable<String>? autofillHints;
  final TextCapitalization textCapitalization;
  final bool enabled;
  final int? maxLength;
  final String? helperText;
  final int maxLines;
  final ValueChanged<String>? onChanged;

  @override
  Widget build(BuildContext context) {
    final isLast = nextFocus == null;
    return Padding(
      // A stable identity for the field, so it survives a rebuild of the list it
      // sits in and can be found by its label rather than by its position.
      key: key ?? ValueKey('field-$label'),
      padding: const EdgeInsets.only(bottom: IConfess.space4),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(
            label,
            style: IConfess.label.copyWith(color: AppSurfaces.of(context).textPrimary),
          ),
          const SizedBox(height: IConfess.space2),
          TextFormField(
            controller: controller,
            focusNode: focusNode,
            obscureText: obscure,
            keyboardType: keyboardType,
            autofillHints: autofillHints,
            textCapitalization: textCapitalization,
            enabled: enabled,
            maxLength: maxLength,
            maxLines: obscure ? 1 : maxLines,
            onChanged: onChanged,
            // The input's own error slot: it colours the border, moves the
            // layout predictably, and — unlike a manually added Text — is
            // associated with the field for assistive technology.
            decoration: InputDecoration(
              hintText: hint,
              helperText: helperText,
              errorText: errorText,
            ),
            textInputAction: isLast
                ? TextInputAction.done
                : TextInputAction.next,
            onFieldSubmitted: (_) {
              if (nextFocus != null) {
                nextFocus!.requestFocus();
              } else {
                FocusScope.of(context).unfocus();
                onSubmitted?.call();
              }
            },
          ),
        ],
      ),
    );
  }
}

/// A password field with a show/hide toggle.
///
/// The toggle is a separate widget rather than a `suffixIcon` flag on
/// [AuthTextField] because it owns state, and a stateless field that has to
/// remember whether it is revealed is a stateful field with extra steps.
class AuthPasswordField extends StatefulWidget {
  const AuthPasswordField({
    required this.label,
    required this.controller,
    this.focusNode,
    this.nextFocus,
    this.onSubmitted,
    this.errorText,
    this.helperText,
    this.autofillHints = const [AutofillHints.password],
    this.enabled = true,
    this.onChanged,
    super.key,
  });

  final String label;
  final TextEditingController controller;
  final FocusNode? focusNode;
  final FocusNode? nextFocus;
  final VoidCallback? onSubmitted;
  final String? errorText;
  final String? helperText;
  final Iterable<String> autofillHints;
  final bool enabled;
  final ValueChanged<String>? onChanged;

  @override
  State<AuthPasswordField> createState() => _AuthPasswordFieldState();
}

class _AuthPasswordFieldState extends State<AuthPasswordField> {
  bool _revealed = false;

  @override
  Widget build(BuildContext context) {
    final surfaces = AppSurfaces.of(context);
    final isLast = widget.nextFocus == null;

    return Padding(
      key: widget.key ?? ValueKey('field-${widget.label}'),
      padding: const EdgeInsets.only(bottom: IConfess.space4),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(
            widget.label,
            style: IConfess.label.copyWith(color: surfaces.textPrimary),
          ),
          const SizedBox(height: IConfess.space2),
          TextFormField(
            controller: widget.controller,
            focusNode: widget.focusNode,
            obscureText: !_revealed,
            enabled: widget.enabled,
            autofillHints: widget.autofillHints,
            onChanged: widget.onChanged,
            keyboardType: TextInputType.visiblePassword,
            decoration: InputDecoration(
              errorText: widget.errorText,
              helperText: widget.helperText,
              suffixIcon: IconButton(
                onPressed: () => setState(() => _revealed = !_revealed),
                icon: Icon(
                  _revealed
                      ? Icons.visibility_off_rounded
                      : Icons.visibility_rounded,
                ),
                // Announced as a state change, not just a tap: "show password"
                // pressed with nothing further said leaves a screen reader user
                // unsure whether anything happened.
                tooltip: _revealed ? 'Hide password' : 'Show password',
              ),
            ),
            textInputAction: isLast ? TextInputAction.done : TextInputAction.next,
            onFieldSubmitted: (_) {
              if (widget.nextFocus != null) {
                widget.nextFocus!.requestFocus();
              } else {
                FocusScope.of(context).unfocus();
                widget.onSubmitted?.call();
              }
            },
          ),
        ],
      ),
    );
  }
}

/// The primary action.
///
/// Busy is shown in the button rather than as a full-screen overlay: the form
/// stays readable, the button stays where the thumb already is, and a second
/// tap is impossible because the button is disabled. A modal spinner over a
/// form is how people end up submitting twice.
class PrimaryActionButton extends StatelessWidget {
  const PrimaryActionButton({
    required this.label,
    required this.onPressed,
    this.busy = false,
    this.busyLabel,
    super.key,
  });

  final String label;
  final VoidCallback? onPressed;
  final bool busy;

  /// What the button reads while working. Defaults to [label], which is the
  /// right answer for most of this flow: "Sign in" becoming "Signing in" needs
  /// no extra copy.
  final String? busyLabel;

  @override
  Widget build(BuildContext context) {
    return SizedBox(
      width: double.infinity,
      child: FilledButton(
        onPressed: busy ? null : onPressed,
        child: busy
            ? Row(
                mainAxisAlignment: MainAxisAlignment.center,
                children: [
                  const SizedBox(
                    width: 18,
                    height: 18,
                    child: CircularProgressIndicator(strokeWidth: 2),
                  ),
                  const SizedBox(width: IConfess.space3),
                  Text(busyLabel ?? label),
                ],
              )
            : Text(label),
      ),
    );
  }
}

/// An inline error, announced to assistive technology as it appears.
///
/// `liveRegion: true` is the part that is easy to forget and impossible to see
/// in a screenshot: without it, a screen reader user presses "Sign in", hears
/// nothing, and has no idea the attempt failed.
class ErrorBanner extends StatelessWidget {
  const ErrorBanner({
    required this.message,
    this.title,
    this.onAction,
    this.actionLabel,
    super.key,
  });

  final String message;
  final String? title;
  final VoidCallback? onAction;
  final String? actionLabel;

  @override
  Widget build(BuildContext context) {
    final surfaces = AppSurfaces.of(context);
    return Semantics(
      liveRegion: true,
      container: true,
      label: title == null ? message : '$title. $message',
      child: Container(
        width: double.infinity,
        margin: const EdgeInsets.only(bottom: IConfess.space4),
        padding: const EdgeInsets.all(IConfess.space4),
        decoration: BoxDecoration(
          color: surfaces.danger.withValues(alpha: 0.08),
          borderRadius: BorderRadius.circular(IConfess.radiusMd),
          border: Border.all(color: surfaces.danger.withValues(alpha: 0.4)),
        ),
        child: Row(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Icon(Icons.error_outline_rounded, color: surfaces.danger, size: 20),
            const SizedBox(width: IConfess.space3),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  if (title != null)
                    Text(
                      title!,
                      style: IConfess.bodySm.copyWith(
                        color: surfaces.textPrimary,
                        fontWeight: FontWeight.w600,
                      ),
                    ),
                  if (title != null) const SizedBox(height: IConfess.space1),
                  Text(
                    message,
                    style: IConfess.bodySm.copyWith(
                      color: surfaces.textPrimary,
                      height: 1.5,
                    ),
                  ),
                  if (onAction != null && actionLabel != null)
                    Align(
                      alignment: Alignment.centerLeft,
                      child: TextButton(
                        onPressed: onAction,
                        child: Text(actionLabel!),
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

/// A confirmation that is the visual opposite of [ErrorBanner].
///
/// Same live-region behaviour, because a success that is only visible is also a
/// success a screen reader user misses.
class SuccessBanner extends StatelessWidget {
  const SuccessBanner({required this.message, this.title, super.key});

  final String message;
  final String? title;

  @override
  Widget build(BuildContext context) {
    final surfaces = AppSurfaces.of(context);
    return Semantics(
      liveRegion: true,
      container: true,
      label: title == null ? message : '$title. $message',
      child: Container(
        width: double.infinity,
        margin: const EdgeInsets.only(bottom: IConfess.space4),
        padding: const EdgeInsets.all(IConfess.space4),
        decoration: BoxDecoration(
          color: surfaces.success.withValues(alpha: 0.10),
          borderRadius: BorderRadius.circular(IConfess.radiusMd),
          border: Border.all(color: surfaces.success.withValues(alpha: 0.45)),
        ),
        child: Row(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Icon(Icons.check_circle_outline_rounded,
                color: surfaces.success, size: 20),
            const SizedBox(width: IConfess.space3),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  if (title != null)
                    Text(
                      title!,
                      style: IConfess.bodySm.copyWith(
                        color: surfaces.textPrimary,
                        fontWeight: FontWeight.w600,
                      ),
                    ),
                  if (title != null) const SizedBox(height: IConfess.space1),
                  Text(
                    message,
                    style: IConfess.bodySm.copyWith(
                      color: surfaces.textPrimary,
                      height: 1.5,
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

/// "Already have an account? Sign in."
class AuthSwitchPrompt extends StatelessWidget {
  const AuthSwitchPrompt({
    required this.question,
    required this.actionLabel,
    required this.onAction,
    super.key,
  });

  final String question;
  final String actionLabel;
  final VoidCallback onAction;

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.only(top: IConfess.space5),
      child: Row(
        mainAxisAlignment: MainAxisAlignment.center,
        children: [
          Text(
            question,
            style: IConfess.bodySm.copyWith(
              color: AppSurfaces.of(context).textSecondary,
            ),
          ),
          TextButton(onPressed: onAction, child: Text(actionLabel)),
        ],
      ),
    );
  }
}

/// The wordmark used on the splash and the welcome screen.
///
/// `excludeSemantics` because it says "I CONFESS" in type that a screen reader
/// would otherwise read as a heading of no meaning; the surrounding screen
/// provides the real label.
class BrandMark extends StatelessWidget {
  const BrandMark({this.large = false, super.key});

  final bool large;

  @override
  Widget build(BuildContext context) {
    final surfaces = AppSurfaces.of(context);
    return ExcludeSemantics(
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Container(
            width: large ? 56 : 40,
            height: large ? 56 : 40,
            decoration: BoxDecoration(
              color: surfaces.primary,
              borderRadius: BorderRadius.circular(IConfess.radiusMd),
            ),
            alignment: Alignment.center,
            child: Icon(
              Icons.graphic_eq_rounded,
              color: surfaces.onPrimary,
              size: large ? 28 : 20,
            ),
          ),
          const SizedBox(height: IConfess.space4),
          Text(
            'I CONFESS',
            style: (large ? IConfess.display : IConfess.heading)
                .copyWith(color: surfaces.textPrimary, letterSpacing: 1.2),
          ),
        ],
      ),
    );
  }
}

/// Keeps the system UI from covering a focused field on small screens.
///
/// Flutter already resizes for the keyboard; what it does not do is scroll a
/// field into view when the field is inside a scroll view that has not been
/// laid out yet. Calling this after a focus change covers that case.
void revealField(ScrollController controller, double offset) {
  if (!controller.hasClients) return;
  final target = offset.clamp(0.0, controller.position.maxScrollExtent);
  controller.animateTo(
    target,
    duration: IConfess.motionDurationDeliberate,
    curve: Curves.easeOutCubic,
  );
}

/// Reads whether the user has asked the system to reduce motion.
///
/// Wrapped so callers do not each spell out the MediaQuery lookup, and so the
/// splash and the onboarding pager agree about when not to animate.
bool prefersReducedMotion(BuildContext context) =>
    MediaQuery.disableAnimationsOf(context);

/// Haptic feedback for a failed submission.
///
/// Guarded behind a try: haptics are unavailable on some platforms and in tests,
/// and a form that throws because it could not vibrate has failed at the one
/// job it had.
void failureFeedback() {
  try {
    HapticFeedback.mediumImpact();
  } on Object {
    // Deliberate: feedback is never essential.
  }
}
