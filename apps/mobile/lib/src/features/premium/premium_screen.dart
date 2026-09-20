import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:iconfess_api/iconfess_api.dart';

import '../../core/theme/theme.dart';
import '../../core/theme/tokens.dart';
import '../../core/widgets/screen.dart';
import 'premium_providers.dart';
import 'purchase_controller.dart';

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

    // Purchase outcomes arrive on their own stream, minutes after the tap when
    // a bank confirmation is involved, so they are surfaced as they happen
    // rather than returned from the button press.
    ref.listen<PurchaseState>(premiumPurchaseControllerProvider, (_, next) {
      final message = next.error ?? next.message;
      if (message == null) return;
      ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(message)));
      ref.read(premiumPurchaseControllerProvider.notifier).clear();
    });

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
                    // Required by App Store review for a non-consumable, and
                    // the only way back for someone who reinstalled: without
                    // it a paying subscriber sees a paywall and no route to
                    // the subscription they already own.
                    Align(
                      alignment: Alignment.centerRight,
                      child: TextButton(
                        onPressed: ref.watch(premiumPurchaseControllerProvider).busy ? null
                            : () => ref.read(premiumPurchaseControllerProvider.notifier).restore(),
                        child: const Text('Restore purchases'),
                      ),
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

class _PlanCard extends ConsumerWidget {
  const _PlanCard({required this.plan});
  final Plan plan;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final surfaces = AppSurfaces.of(context);
    final purchase = ref.watch(premiumPurchaseControllerProvider);
    final gateway = ref.watch(purchaseGatewayProvider);
    final id = storeProductIdFor(plan.id, apple: gateway.provider == 'apple');
    final products = ref.watch(storeProductsProvider);
    final product = products.asData?.value[id];
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
          Text(product?.price ?? (products.isLoading
              ? 'Loading store price…' : 'Unavailable in this store'),
              style: IConfess.subheading.copyWith(color: surfaces.textPrimary)),
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
            // Disabled while the store is deciding: two taps on a slow
            // connection is how a user ends up buying twice.
            onPressed: purchase.busy || product == null
                ? null
                : () => ref.read(premiumPurchaseControllerProvider.notifier).purchase(plan.id),
            child: purchase.busy
                ? const SizedBox(
                    height: 18,
                    width: 18,
                    child: CircularProgressIndicator(strokeWidth: 2),
                  )
                : const Text('Subscribe'),
          ),
          const SizedBox(height: IConfess.space2),
          Text(
            'Payment is taken by the App Store or Google Play. Premium is activated '
            'by our server once the store confirms the purchase.',
            style: IConfess.caption.copyWith(color: surfaces.textSecondary),
          ),
        ],
      ),
    );
  }
}
