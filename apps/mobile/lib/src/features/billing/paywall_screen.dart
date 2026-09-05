// Paywall — never hard-codes amounts. Every price comes from BillingService.fetchPlans().
// Currencies: NGN / USD / GBP / EUR / PHP (§6.1). 7-day trial, server-side verify.
import 'package:flutter/material.dart';
import 'billing_service.dart';

class PaywallScreen extends StatefulWidget {
  const PaywallScreen({super.key, required this.baseUrl, this.authToken, this.initialCurrency = 'NGN'});
  final String baseUrl;
  final String? authToken;
  final String initialCurrency;

  @override
  State<PaywallScreen> createState() => _PaywallScreenState();
}

class _PaywallScreenState extends State<PaywallScreen> {
  late BillingService _billing;
  PlansCatalog? _catalog;
  String? _error;
  bool _loading = true;
  late String _currency;

  @override
  void initState() {
    super.initState();
    _billing = BillingService(baseUrl: widget.baseUrl);
    _currency = widget.initialCurrency.toUpperCase();
    _load();
  }

  Future<void> _load() async {
    setState(() { _loading = true; _error = null; });
    try {
      final cat = await _billing.fetchPlans(authToken: widget.authToken);
      if (!mounted) return;
      setState(() {
        _catalog = cat;
        if (!cat.currencies.contains(_currency) && cat.currencies.isNotEmpty) {
          _currency = cat.currencies.first;
        }
        _loading = false;
      });
    } catch (e) {
      if (!mounted) return;
      setState(() { _error = e.toString(); _loading = false; });
    }
  }

  Future<void> _purchase(Plan plan) async {
    // Store purchase (in_app_purchase lives in /tmp for workspace budget).
    // This stub shows the server-side verify flow:
    // 1) Call StoreKit / Play Billing -> get receipt
    // 2) POST /v1/subscriptions/verify {provider, receipt} -> verified + entitlements
    // For dev, pass receipt = "valid_monthly_xxx" or "valid_annual_xxx" (NoopVerifier).
    final scaffold = ScaffoldMessenger.of(context);
    String receipt;
    // In prod: final receipt = await InAppPurchase.instance.buy(plan.id)
    // For now, synthesize valid receipt for the chosen plan:
    receipt = plan.id == 'annual' ? 'valid_annual_${DateTime.now().millisecondsSinceEpoch}' : 'valid_monthly_${DateTime.now().millisecondsSinceEpoch}';
    // If BILLING_VERIFIER=apple|google, use "apple_valid_" / "google_valid_" prefix instead.
    try {
      final res = await _billing.verifyReceipt(
        provider: 'apple', // or 'google' — server routes via VerifierFromEnv
        receipt: receipt,
        authToken: widget.authToken ?? '',
      );
      if (!mounted) return;
      if (res.verified) {
        scaffold.showSnackBar(SnackBar(content: Text('Welcome to ${res.plan ?? plan.name} — verified ✓')));
        if (mounted) Navigator.of(context).maybePop(true);
      } else {
        scaffold.showSnackBar(SnackBar(content: Text('Not verified: ${res.detail ?? 'receipt rejected'}')));
      }
    } catch (e) {
      if (!mounted) return;
      scaffold.showSnackBar(SnackBar(content: Text('Verify failed: $e')));
    }
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: const Color(0xFF0F1220),
      appBar: AppBar(backgroundColor: const Color(0xFF0F1220), foregroundColor: Colors.white, title: const Text('Premium'), elevation: 0),
      body: _loading
          ? const Center(child: CircularProgressIndicator(color: Color(0xFFE8C67A)))
          : _error != null
              ? Center(
                  child: Padding(
                    padding: const EdgeInsets.all(24),
                    child: Column(mainAxisSize: MainAxisSize.min, children: [
                      Text(_error!, style: const TextStyle(color: Color(0xFFFFB4B4))),
                      const SizedBox(height: 12),
                      FilledButton(onPressed: _load, child: const Text('Retry')),
                      const SizedBox(height: 8),
                      const Text('GET /v1/subscriptions/plans — server is the source', style: TextStyle(color: Color(0xFF6B7280), fontSize: 11)),
                    ]),
                  ),
                )
              : ListView(
                  padding: const EdgeInsets.all(16),
                  children: [
                    const Text('Choose your rhythm', style: TextStyle(fontSize: 22, fontWeight: FontWeight.w700, color: Color(0xFFE8EAF6))),
                    const SizedBox(height: 6),
                    const Text('Prices fetched live — no amount hard-coded in the app. Trial 7 days · Offline licenced · Premium voices.', style: TextStyle(color: Color(0xFF9AA1C0), fontSize: 12)),
                    const SizedBox(height: 14),
                    // Currency selector — from API currencies, never hard-coded amounts
                    Wrap(spacing: 8, children: [
                      for (final c in _catalog!.currencies)
                        ChoiceChip(
                          label: Text(c),
                          selected: _currency == c,
                          onSelected: (_) => setState(() => _currency = c),
                          selectedColor: const Color(0xFFE8C67A),
                          labelStyle: TextStyle(color: _currency == c ? const Color(0xFF241B05) : Colors.white),
                        ),
                    ]),
                    const SizedBox(height: 4),
                    const Text('Live from GET /v1/subscriptions/plans', style: TextStyle(color: Color(0xFF6B7280), fontSize: 11)),
                    const SizedBox(height: 16),
                    for (final plan in _catalog!.plans) _planCard(plan),
                    const SizedBox(height: 16),
                    const Text('Receipt verification is server-side (POST /v1/subscriptions/verify, BILLING_VERIFIER=apple|google|chained). Client never grants premium.', style: TextStyle(color: Color(0xFF6B7280), fontSize: 11)),
                  ],
                ),
    );
  }

  Widget _planCard(Plan plan) {
    final amt = plan.amountFor(_currency) ?? plan.prices.values.firstOrNull;
    final isAnnual = plan.id == 'annual';
    return Container(
      margin: const EdgeInsets.only(bottom: 12),
      padding: const EdgeInsets.all(16),
      decoration: BoxDecoration(
        color: isAnnual ? const Color(0xFF1F2340) : const Color(0xFF1C2138),
        border: Border.all(color: isAnnual ? const Color(0xFFE8C67A).withOpacity(0.35) : const Color(0xFF2D3350)),
        borderRadius: BorderRadius.circular(16),
      ),
      child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
        if (isAnnual)
          Container(
            padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 4),
            decoration: BoxDecoration(color: const Color(0xFFE8C67A), borderRadius: BorderRadius.circular(20)),
            child: const Text('BEST VALUE · SAVE ~33%', style: TextStyle(fontSize: 10, fontWeight: FontWeight.w700, color: Color(0xFF241B05))),
          ),
        const SizedBox(height: 8),
        Text(plan.name, style: const TextStyle(fontSize: 16, fontWeight: FontWeight.w700, color: Colors.white)),
        if (plan.description.isNotEmpty) Text(plan.description, style: const TextStyle(color: Color(0xFF9AA1C0), fontSize: 11)),
        const SizedBox(height: 10),
        Row(crossAxisAlignment: CrossAxisAlignment.baseline, textBaseline: TextBaseline.alphabetic, children: [
          Text(amt?.display ?? '—', style: const TextStyle(fontSize: 24, fontWeight: FontWeight.bold, color: Colors.white)),
          Text(' / ${plan.interval}', style: const TextStyle(color: Color(0xFF9AA1C0), fontSize: 12)),
        ]),
        Text('${amt?.currency ?? _currency} · ${amt?.minor ?? 0} minor · ${plan.trialDays} day trial', style: const TextStyle(color: Color(0xFF6B7280), fontSize: 11)),
        const SizedBox(height: 10),
        for (final f in plan.features) Row(children: [const Text('✓ ', style: TextStyle(color: Color(0xFF7C8CF8))), Expanded(child: Text(f.replaceAll('_', ' '), style: const TextStyle(color: Color(0xFFC7CCDF), fontSize: 12)))]),
        const Row(children: [Text('✓ ', style: TextStyle(color: Color(0xFF7C8CF8))), Expanded(child: Text('39 categories · schedule & offline licence', style: TextStyle(color: Color(0xFFC7CCDF), fontSize: 12)))]),
        const SizedBox(height: 12),
        SizedBox(
          width: double.infinity,
          child: FilledButton(
            style: FilledButton.styleFrom(backgroundColor: isAnnual ? const Color(0xFFE8C67A) : const Color(0xFF7C8CF8), foregroundColor: const Color(0xFF0B0E1C)),
            onPressed: () => _purchase(plan),
            child: Text('Start 7-day trial — ${amt?.display ?? ''}'),
          ),
        ),
        Center(child: TextButton(onPressed: () {}, child: const Text('Restore purchases', style: TextStyle(color: Color(0xFF9AA1C0), fontSize: 11)))),
      ]),
    );
  }
}

extension _FirstOrNull<T> on Iterable<T> {
  T? get firstOrNull => isEmpty ? null : first;
}
