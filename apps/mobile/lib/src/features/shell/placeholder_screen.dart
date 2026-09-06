import 'package:flutter/material.dart';

import '../../core/theme/theme.dart';
import '../../core/theme/tokens.dart';
import '../../core/widgets/screen.dart';

/// A screen that exists so the router has somewhere to go.
///
/// Deliberately labelled with the phase that will build it rather than filled
/// with plausible-looking content. A placeholder that pretends to be finished is
/// how a repo ends up with screens nobody remembers are empty; one that names its
/// phase makes the remaining work visible.
class PlaceholderScreen extends StatelessWidget {
  const PlaceholderScreen({
    required this.title,
    required this.body,
    this.immersive = false,
    super.key,
  });

  final String title;
  final String body;
  final bool immersive;

  @override
  Widget build(BuildContext context) {
    return AppScaffold(
      title: title,
      immersive: immersive,
      body: Builder(
        builder: (context) {
          final surfaces = AppSurfaces.of(context);
          return Padding(
            padding: const EdgeInsets.only(top: IConfess.space9),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  title,
                  style: IConfess.display.copyWith(color: surfaces.textPrimary),
                ),
                const SizedBox(height: IConfess.space3),
                Text(
                  body,
                  style: IConfess.bodySm.copyWith(color: surfaces.textSecondary),
                ),
              ],
            ),
          );
        },
      ),
    );
  }
}
