import 'package:flutter/material.dart';

import '../theme/theme.dart';
import '../theme/tokens.dart';

/// The screen container every feature builds inside.
///
/// Exists so that safe-area handling, horizontal margins and scroll behaviour are
/// decided once. A screen that gets its own `SafeArea` and its own padding is how
/// an app ends up with 16px on one screen and 20px on the next, which reads as
/// assembled from templates.
class AppScaffold extends StatelessWidget {
  const AppScaffold({
    required this.body,
    this.title,
    this.actions,
    this.leading,
    this.immersive = false,
    this.bottom,
    this.scrollable = true,
    super.key,
  });

  final Widget body;
  final String? title;
  final List<Widget>? actions;
  final Widget? leading;

  /// Renders in the immersive palette: near-black surfaces, ivory type, minimal
  /// chrome. Used by the player and the night ritual.
  final bool immersive;

  /// Pinned below the content and above the home indicator — a primary action or
  /// a mini player.
  final Widget? bottom;

  final bool scrollable;

  @override
  Widget build(BuildContext context) {
    final theme = immersive ? AppTheme.immersive() : Theme.of(context);

    // The Builder is what lets the body read the palette that is actually in
    // force here. Reading it from the outer context would return the app-wide
    // theme, so an immersive screen would paint dark surfaces with light text
    // colours.
    return Theme(
      data: theme,
      child: Builder(
        builder: (context) {
          final content = scrollable
              ? SingleChildScrollView(
                  padding: const EdgeInsets.fromLTRB(
                    IConfess.space5,
                    IConfess.space4,
                    IConfess.space5,
                    IConfess.space7,
                  ),
                  child: body,
                )
              : Padding(
                  padding: const EdgeInsets.symmetric(horizontal: IConfess.space5),
                  child: body,
                );

          return Scaffold(
            backgroundColor: theme.scaffoldBackgroundColor,
            appBar: title == null
                ? null
                : AppBar(
                    leading: leading,
                    title: Text(title!),
                    actions: actions,
                  ),
            body: SafeArea(
              // The app bar already accounts for the top inset; applying it again
              // here would push the content down twice.
              top: title == null,
              bottom: bottom == null,
              child: Column(
                children: [
                  Expanded(child: content),
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
          );
        },
      ),
    );
  }
}

/// A section heading: the small, letterspaced label above a group of content.
///
/// Deliberately not a heading style. "YOUR RHYTHM" above three rows is a label,
/// and styling it as a heading is what makes a screen feel like a stack of
/// unrelated cards.
class SectionHeader extends StatelessWidget {
  const SectionHeader(this.label, {this.trailing, super.key});

  final String label;
  final Widget? trailing;

  @override
  Widget build(BuildContext context) {
    final surfaces = AppSurfaces.of(context);
    return Padding(
      padding: const EdgeInsets.only(bottom: IConfess.space3, top: IConfess.space6),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.center,
        children: [
          Expanded(
            child: Text(
              label.toUpperCase(),
              style: IConfess.label.copyWith(color: surfaces.textSecondary),
            ),
          ),
          ?trailing,
        ],
      ),
    );
  }
}
