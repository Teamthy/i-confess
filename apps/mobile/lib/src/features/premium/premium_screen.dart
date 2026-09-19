import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../core/theme/theme.dart';
import '../../core/theme/tokens.dart';
import '../../core/widgets/screen.dart';
import 'premium_providers.dart';

/// Premium paywall: plans with regional pricing (NGN/USD/GBP/EUR/PHP), trial journey, entitlements.
///
/// Prices come from server, never hard-coded. Verification is server-side.
class PremiumScreen extends ConsumerWidget {
  const PremiumScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final plansAsync = ref.watch(plansProvider);
    final subAsync = ref.watch(subscriptionProvider);
    final entAsync = ref.watch(entitlementsProvider);
    final trialAsync = ref.watch(trialProvider);
    final surfaces = AppSurfaces.of(context);

    return AppScaffold(
      title: 'Premium',
      body: SingleChildScrollView(
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            // Current plan
            subAsync.when(
              loading: () => const LinearProgressIndicator(),
              error: (_, _) => const SizedBox.shrink(),
              data: (loadable) {
                final sub = loadable.valueOrNull;
                if (sub == null) return const SizedBox.shrink();
                return Container(
                  width: double.infinity,
                  padding: const EdgeInsets.all(IConfess.space4),
                  decoration: BoxDecoration(
                    color: sub.isPremium ? IConfess.colorAccentGold.withValues(alpha: 0.15) : surfaces.surfaceRaised,
                    borderRadius: BorderRadius.circular(IConfess.radiusLg),
                  ),
                  child: Row(
                    children: [
                      Icon(sub.isPremium ? Icons.workspace_premium_rounded : Icons.person_rounded,
                          color: sub.isPremium ? IConfess.colorAccentGold : surfaces.textSecondary),
                      const SizedBox(width: IConfess.space3),
                      Column(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: [
                          Text('Current plan: ${sub.plan}',
                              style: IConfess.subheading.copyWith(color: surfaces.textPrimary)),
                          Text('${sub.maxSessionSeconds ~/ 60} min max • ${sub.status}',
                              style: IConfess.caption.copyWith(color: surfaces.textSecondary)),
                        ],
                      ),
                    ],
                  ),
                );
              },
            ),
            const SizedBox(height: IConfess.space5),
            Text('Unlock the full practice',
                style: IConfess.heading.copyWith(color: surfaces.textPrimary)),
            const SizedBox(height: IConfess.space2),
            Text('Longer sessions, premium voices, offline listening, and your own confessions.',
                style: IConfess.body.copyWith(color: surfaces.textSecondary)),
            const SizedBox(height: IConfess.space5),
            // Entitlements
            entAsync.when(
              loading: () => const SizedBox.shrink(),
              error: (_, _) => const SizedBox.shrink(),
              data: (loadable) {
                final ent = loadable.valueOrNull;
                if (ent == null) return const SizedBox.shrink();
                return Wrap(
                  spacing: IConfess.space2,
                  runSpacing: IConfess.space2,
                  children: [
                    _EntChip(label: 'Premium voices', enabled: ent.premiumVoices),
                    _EntChip(label: 'Premium content', enabled: ent.premiumContent),
                    _EntChip(label: 'Offline', enabled: ent.offlineDownloads),
                    _EntChip(label: 'Personal', enabled: ent.personalConfessions),
                  ],
                );
              },
            ),
            const SizedBox(height: IConfess.space5),
            // Plans
            plansAsync.when(
              loading: () => const Center(child: CircularProgressIndicator()),
              error: (e, _) => Text('Could not load plans: $e'),
              data: (loadable) {
                final plans = loadable.valueOrNull ?? [];
                if (plans.isEmpty) return Text('Plans are on the way.');
                return Column(
                  children: [
                    for (final plan in plans)
                      Padding(
                        padding: const EdgeInsets.only(bottom: IConfess.space3),
                        child: _PlanCard(plan: plan),
                      ),
                  ],
                );
              },
            ),
            const SizedBox(height: IConfess.space5),
            // Trial journey
            trialAsync.when(
              loading: () => const SizedBox.shrink(),
              error: (_, _) => const SizedBox.shrink(),
              data: (loadable) {
                final days = loadable.valueOrNull ?? [];
                if (days.isEmpty) return const SizedBox.shrink();
                return Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text('Your first week',
                        style: IConfess.subheading.copyWith(color: surfaces.textPrimary)),
                    const SizedBox(height: IConfess.space3),
                    for (final day in days.take(7))
                      ListTile(
                        leading: CircleAvatar(child: Text('${day.day}')),
                        title: Text(day.title),
                        subtitle: Text(day.description),
                      ),
                  ],
                );
              },
            ),
          ],
        ),
      ),
    );
  }
}

class _EntChip extends StatelessWidget {
  const _EntChip({required this.label, required this.enabled});
  final String label;
  final bool enabled;

  @override
  Widget build(BuildContext context) {
    return Chip(
      label: Text(label),
      avatar: Icon(enabled ? Icons.check_rounded : Icons.close_rounded, size: 16),
      backgroundColor: enabled ? IConfess.colorBrand50 : null,
    );
  }
}

class _PlanCard extends StatelessWidget {
  const _PlanCard({required this.plan});
  final dynamic plan;

  @override
  Widget build(BuildContext context) {
    final surfaces = AppSurfaces.of(context);
    return Container(
      width: double.infinity,
      padding: const EdgeInsets.all(IConfess.space5),
      decoration: BoxDecoration(
        color: surfaces.surfaceRaised,
        borderRadius: BorderRadius.circular(IConfess.radiusLg),
        border: Border.all(color: IConfess.colorBrand200),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              Expanded(
                child: Text(plan.name.isNotEmpty ? plan.name : plan.id,
                    style: IConfess.subheading.copyWith(color: surfaces.textPrimary)),
              ),
              Container(
                padding: const EdgeInsets.symmetric(horizontal: IConfess.space2, vertical: IConfess.space1),
                decoration: BoxDecoration(
                  color: IConfess.colorAccentGold,
                  borderRadius: BorderRadius.circular(IConfess.radiusFull),
                ),
                child: Text(plan.interval.toUpperCase(),
                    style: IConfess.label.copyWith(color: Colors.black)),
              ),
            ],
          ),
          const SizedBox(height: IConfess.space2),
          Text(plan.description, style: IConfess.bodySm.copyWith(color: surfaces.textSecondary)),
          const SizedBox(height: IConfess.space3),
          // Prices: show NGN, USD, GBP, EUR, PHP if available
          Wrap(
            spacing: IConfess.space2,
            children: [
              for (final curr in ['NGN', 'USD', 'GBP', 'EUR', 'PHP'])
                if (plan.priceFor(curr).isNotEmpty)
                  Chip(label: Text(plan.priceFor(curr))),
            ],
          ),
          const SizedBox(height: IConfess.space3),
          if (plan.features.isNotEmpty)
            Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                for (final f in plan.features)
                  Padding(
                    padding: const EdgeInsets.only(bottom: IConfess.space1),
                    child: Row(
                      children: [
                        const Icon(Icons.check_rounded, size: 16, color: IConfess.colorBrand600),
                        const SizedBox(width: IConfess.space2),
                        Expanded(child: Text(f, style: IConfess.bodySm.copyWith(color: surfaces.textPrimary))),
                      ],
                    ),
                  ),
              ],
            ),
          const SizedBox(height: IConfess.space4),
          FilledButton(
            onPressed: () {},
            child: Text(plan.trialDays > 0 ? 'Start ${plan.trialDays}-day trial' : 'Subscribe'),
          ),
        ],
      ),
    );
  }
}
