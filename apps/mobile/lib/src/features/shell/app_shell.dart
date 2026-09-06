import 'package:flutter/material.dart';
import 'package:go_router/go_router.dart';

import '../../core/routing/routes.dart';
import '../../core/theme/theme.dart';
import '../../core/theme/tokens.dart';

/// The five-destination shell.
///
/// The centre action is the confession builder and is rendered differently from
/// its neighbours, because it is the primary action of the product. The emphasis
/// is a filled circle at the same height as the other targets rather than a
/// floating button: a FAB would cover content, sit outside the bar's rhythm, and
/// read as a platform default rather than a decision.
class AppShell extends StatelessWidget {
  const AppShell({required this.navigationShell, super.key});

  final StatefulNavigationShell navigationShell;

  @override
  Widget build(BuildContext context) {
    final surfaces = AppSurfaces.of(context);
    return Scaffold(
      body: navigationShell,
      bottomNavigationBar: DecoratedBox(
        // Keyed so tests can scope "the tab targets" to the bar now that real
        // screens (home) contribute their own InkWells to the tree.
        key: const ValueKey('app-shell-bar'),
        decoration: BoxDecoration(
          color: surfaces.surfaceRaised,
          border: Border(top: BorderSide(color: surfaces.border)),
        ),
        child: SafeArea(
          top: false,
          child: SizedBox(
            height: 64,
            child: Row(
              children: [
                _Tab(
                  index: 0,
                  icon: Icons.home_outlined,
                  activeIcon: Icons.home_rounded,
                  label: 'Home',
                  shell: navigationShell,
                ),
                _Tab(
                  index: 1,
                  icon: Icons.explore_outlined,
                  activeIcon: Icons.explore_rounded,
                  label: 'Explore',
                  shell: navigationShell,
                ),
                const _ConfessAction(index: 2),
                _Tab(
                  index: 3,
                  icon: Icons.insights_outlined,
                  activeIcon: Icons.insights_rounded,
                  label: 'Activity',
                  shell: navigationShell,
                ),
                _Tab(
                  index: 4,
                  icon: Icons.person_outline,
                  activeIcon: Icons.person_rounded,
                  label: 'Me',
                  shell: navigationShell,
                ),
              ],
            ),
          ),
        ),
      ),
    );
  }
}

class _Tab extends StatelessWidget {
  const _Tab({
    required this.index,
    required this.icon,
    required this.activeIcon,
    required this.label,
    required this.shell,
  });

  final int index;
  final IconData icon;
  final IconData activeIcon;
  final String label;
  final StatefulNavigationShell shell;

  @override
  Widget build(BuildContext context) {
    final surfaces = AppSurfaces.of(context);
    final selected = shell.currentIndex == index;
    final color = selected ? surfaces.primary : surfaces.textSecondary;

    return Expanded(
      child: Semantics(
        button: true,
        selected: selected,
        label: label,
        child: InkWell(
          onTap: () => shell.goBranch(index, initialLocation: index == shell.currentIndex),
          // 44pt is the practical minimum for a thumb; the bar is 64 tall so
          // there is room to be generous.
          child: SizedBox(
            height: 64,
            child: Column(
              mainAxisAlignment: MainAxisAlignment.center,
              children: [
                Icon(selected ? activeIcon : icon, size: 22, color: color),
                const SizedBox(height: 2),
                Text(
                  label,
                  style: IConfess.label.copyWith(
                    color: color,
                    fontWeight: selected ? FontWeight.w600 : FontWeight.w500,
                  ),
                ),
              ],
            ),
          ),
        ),
      ),
    );
  }
}

/// The centre action.
///
/// Filled, circular, and the same 64pt row height as its neighbours so the bar
/// stays one rhythm. It navigates rather than switching a branch: the session
/// builder is a flow, not a destination the listener lives in.
class _ConfessAction extends StatelessWidget {
  const _ConfessAction({required this.index});

  final int index;

  @override
  Widget build(BuildContext context) {
    final surfaces = AppSurfaces.of(context);
    final selected = index == 2;

    return Expanded(
      child: Semantics(
        button: true,
        label: 'Create a confession session',
        child: InkWell(
          onTap: () => context.goNamed(AppRouteNames.confess),
          child: SizedBox(
            height: 64,
            child: Column(
              mainAxisAlignment: MainAxisAlignment.center,
              children: [
                Container(
                  width: 44,
                  height: 44,
                  decoration: BoxDecoration(
                    color: selected ? surfaces.primary : surfaces.primary,
                    shape: BoxShape.circle,
                  ),
                  child: Icon(Icons.graphic_eq_rounded, color: surfaces.onPrimary, size: 22),
                ),
                const SizedBox(height: 2),
                Text(
                  'Confess',
                  style: IConfess.label.copyWith(
                    color: selected ? surfaces.primary : surfaces.textSecondary,
                    fontWeight: selected ? FontWeight.w600 : FontWeight.w500,
                  ),
                ),
              ],
            ),
          ),
        ),
      ),
    );
  }
}
