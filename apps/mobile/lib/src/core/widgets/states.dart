import 'package:flutter/material.dart';

import '../error/error_mapper.dart';
import '../theme/theme.dart';
import '../theme/tokens.dart';

/// An empty state that says what to do next.
///
/// Section 19 of the directive: every major feature needs a real empty state.
/// "Nothing saved yet" with no way forward is a dead end; the same words with an
/// action is an invitation.
class EmptyState extends StatelessWidget {
  const EmptyState({
    required this.title,
    required this.message,
    this.icon,
    this.actionLabel,
    this.onAction,
    super.key,
  });

  final String title;
  final String message;
  final IconData? icon;
  final String? actionLabel;
  final VoidCallback? onAction;

  @override
  Widget build(BuildContext context) {
    final surfaces = AppSurfaces.of(context);
    return Center(
      child: Padding(
        padding: const EdgeInsets.symmetric(
          horizontal: IConfess.space6,
          vertical: IConfess.space9,
        ),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            if (icon != null)
              Container(
                width: 56,
                height: 56,
                decoration: BoxDecoration(
                  color: surfaces.surface,
                  shape: BoxShape.circle,
                  border: Border.all(color: surfaces.border),
                ),
                child: Icon(icon, color: surfaces.textSecondary, size: 24),
              ),
            const SizedBox(height: IConfess.space5),
            Text(
              title,
              textAlign: TextAlign.center,
              style: IConfess.subheading.copyWith(color: surfaces.textPrimary),
            ),
            const SizedBox(height: IConfess.space2),
            Text(
              message,
              textAlign: TextAlign.center,
              style: IConfess.bodySm.copyWith(color: surfaces.textSecondary, height: 1.6),
            ),
            if (actionLabel != null && onAction != null) ...[
              const SizedBox(height: IConfess.space6),
              FilledButton(onPressed: onAction, child: Text(actionLabel!)),
            ],
          ],
        ),
      ),
    );
  }
}

/// A failure state built from [ErrorMapper], so the copy is the same everywhere
/// and always offers a recovery.
class ErrorState extends StatelessWidget {
  const ErrorState({required this.error, this.onAction, super.key});

  final UserFacingError error;

  /// Called with the action the listener chose. The state does not know how to
  /// retry, sign in or open downloads — that belongs to the screen.
  final void Function(ErrorAction action)? onAction;

  @override
  Widget build(BuildContext context) {
    final surfaces = AppSurfaces.of(context);
    return Center(
      child: Padding(
        padding: const EdgeInsets.symmetric(
          horizontal: IConfess.space6,
          vertical: IConfess.space9,
        ),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Icon(Icons.error_outline_rounded, color: surfaces.textSecondary, size: 28),
            const SizedBox(height: IConfess.space4),
            Text(
              error.title,
              textAlign: TextAlign.center,
              style: IConfess.subheading.copyWith(color: surfaces.textPrimary),
            ),
            const SizedBox(height: IConfess.space2),
            Text(
              error.message,
              textAlign: TextAlign.center,
              style: IConfess.bodySm.copyWith(color: surfaces.textSecondary, height: 1.6),
            ),
            const SizedBox(height: IConfess.space6),
            if (error.primaryAction != null && onAction != null)
              FilledButton(
                onPressed: () => onAction!(error.primaryAction!),
                child: Text(labelFor(error.primaryAction!)),
              ),
            if (error.secondaryAction != null && onAction != null) ...[
              const SizedBox(height: IConfess.space2),
              TextButton(
                onPressed: () => onAction!(error.secondaryAction!),
                child: Text(labelFor(error.secondaryAction!)),
              ),
            ],
          ],
        ),
      ),
    );
  }

  static String labelFor(ErrorAction action) => switch (action) {
        ErrorAction.retry => 'Try again',
        ErrorAction.signIn => 'Sign in',
        ErrorAction.viewDownloads => 'View downloads',
        ErrorAction.refreshSession => 'Continue',
        ErrorAction.upgrade => 'See Premium',
        ErrorAction.contactSupport => 'Contact support',
        ErrorAction.goBack => 'Go back',
      };
}

/// A skeleton that holds the exact shape of what it is standing in for.
///
/// Section 39: a spinner does not tell the listener what is coming, and swapping
/// a spinner for content shifts the layout. A skeleton that matches the real
/// geometry means the screen feels like it was already there.
class Skeleton extends StatelessWidget {
  const Skeleton({
    this.height = 16,
    this.width,
    this.radius,
    super.key,
  });

  final double height;
  final double? width;
  final double? radius;

  @override
  Widget build(BuildContext context) {
    final surfaces = AppSurfaces.of(context);
    final reduced = MediaQuery.disableAnimationsOf(context);
    return Container(
      height: height,
      width: width,
      decoration: BoxDecoration(
        color: surfaces.surface,
        borderRadius: BorderRadius.circular(radius ?? IConfess.radiusSm),
        border: Border.all(color: surfaces.border.withValues(alpha: 0.6)),
      ),
      // A shimmer is motion for its own sake when the user has asked for none.
      child: reduced ? null : const _Shimmer(),
    );
  }
}

class _Shimmer extends StatefulWidget {
  const _Shimmer();

  @override
  State<_Shimmer> createState() => _ShimmerState();
}

class _ShimmerState extends State<_Shimmer> with SingleTickerProviderStateMixin {
  late final AnimationController _controller = AnimationController(
    vsync: this,
    duration: IConfess.motionDurationDeliberate * 2,
  )..repeat();

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final surfaces = AppSurfaces.of(context);
    return AnimatedBuilder(
      animation: _controller,
      builder: (context, _) => DecoratedBox(
        decoration: BoxDecoration(
          gradient: LinearGradient(
            begin: Alignment(-1 + _controller.value * 2, 0),
            end: Alignment(_controller.value * 2, 0),
            colors: [
              surfaces.surfaceRaised.withValues(alpha: 0),
              surfaces.surfaceRaised.withValues(alpha: 0.7),
              surfaces.surfaceRaised.withValues(alpha: 0),
            ],
          ),
          borderRadius: BorderRadius.circular(IConfess.radiusSm),
        ),
      ),
    );
  }
}

/// The loading placeholder for a list-shaped screen.
class ListSkeleton extends StatelessWidget {
  const ListSkeleton({this.rows = 4, super.key});

  final int rows;

  @override
  Widget build(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        const Skeleton(height: 28, width: 180),
        const SizedBox(height: IConfess.space6),
        for (var i = 0; i < rows; i++) ...[
          Container(
            height: 92,
            decoration: BoxDecoration(
              color: AppSurfaces.of(context).surface,
              borderRadius: BorderRadius.circular(IConfess.radiusLg),
              border: Border.all(color: AppSurfaces.of(context).border),
            ),
            padding: const EdgeInsets.all(IConfess.space4),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              mainAxisAlignment: MainAxisAlignment.center,
              children: [
                Skeleton(height: 14, width: 120 + (i.isEven ? 40 : 0)),
                const SizedBox(height: IConfess.space2),
                const Skeleton(height: 12, width: 80),
              ],
            ),
          ),
          const SizedBox(height: IConfess.space3),
        ],
      ],
    );
  }
}
