import 'dart:convert';
import 'package:http/http.dart' as http;

/// BillingService is the single source of price truth in mobile.
/// Prices are NEVER hard-coded; they come from GET /v1/subscriptions/plans.
/// Currencies: NGN/USD/GBP/EUR/PHP.
class BillingService {
  BillingService({required this.baseUrl, http.Client? client})
      : _client = client ?? http.Client();

  final String baseUrl;
  final http.Client _client;

  /// Fetch catalog. Pass currency to filter (e.g. NGN) or null for all.
  Future<PlansCatalog> fetchPlans({String? currency, String? authToken}) async {
    var url = '$baseUrl/v1/subscriptions/plans';
    if (currency != null) url += '?currency=$currency';
    final resp = await _client.get(
      Uri.parse(url),
      headers: {
        if (authToken != null) 'Authorization': 'Bearer $authToken',
        'Accept': 'application/json',
      },
    );
    if (resp.statusCode != 200) {
      throw Exception('plans HTTP ${resp.statusCode}: ${resp.body}');
    }
    return PlansCatalog.fromJson(jsonDecode(resp.body) as Map<String, dynamic>);
  }

  /// Server-side verification: POST /v1/subscriptions/verify
  /// receipt must be a store receipt; server (billing.Verifier) is the authority.
  Future<VerifyResult> verifyReceipt({
    required String provider, // apple | google | stripe
    required String receipt,
    required String authToken,
  }) async {
    final resp = await _client.post(
      Uri.parse('$baseUrl/v1/subscriptions/verify'),
      headers: {
        'Authorization': 'Bearer $authToken',
        'Content-Type': 'application/json',
      },
      body: jsonEncode({'provider': provider, 'receipt': receipt}),
    );
    final body = jsonDecode(resp.body) as Map<String, dynamic>;
    if (resp.statusCode != 200) {
      throw Exception(body['error'] ?? 'verify failed ${resp.statusCode}');
    }
    return VerifyResult.fromJson(body);
  }
}

class PlansCatalog {
  PlansCatalog({required this.plans, required this.currencies});
  final List<Plan> plans;
  final List<String> currencies;

  factory PlansCatalog.fromJson(Map<String, dynamic> j) => PlansCatalog(
        plans: (j['plans'] as List).map((e) => Plan.fromJson(e as Map<String, dynamic>)).toList(),
        currencies: (j['currencies'] as List).cast<String>(),
      );
}

class Plan {
  Plan({required this.id, required this.name, required this.interval, required this.trialDays, required this.prices, required this.features});
  final String id; // monthly | annual
  final String name;
  final String interval; // month | year
  final int trialDays;
  final Map<String, Amount> prices; // currency -> amount
  final List<String> features;

  Amount? amountFor(String currency) => prices[currency.toUpperCase()];

  factory Plan.fromJson(Map<String, dynamic> j) => Plan(
        id: j['id'] as String,
        name: j['name'] as String,
        interval: j['interval'] as String,
        trialDays: (j['trial_days'] as num).toInt(),
        prices: (j['prices'] as Map<String, dynamic>).map((k, v) => MapEntry(k, Amount.fromJson(v as Map<String, dynamic>))),
        features: (j['features'] as List).cast<String>(),
      );
}

class Amount {
  Amount({required this.currency, required this.minor, required this.display});
  final String currency;
  final int minor;
  final String display; // e.g. ₦1,500
  factory Amount.fromJson(Map<String, dynamic> j) => Amount(
        currency: j['currency'] as String,
        minor: (j['minor'] as num).toInt(),
        display: j['display'] as String,
      );
}

class VerifyResult {
  VerifyResult({required this.verified, this.plan, this.provider, this.detail});
  final bool verified;
  final String? plan;
  final String? provider;
  final String? detail;
  factory VerifyResult.fromJson(Map<String, dynamic> j) => VerifyResult(
        verified: j['verified'] as bool,
        plan: j['plan'] as String?,
        provider: j['provider'] as String?,
        detail: (j['detail'] ?? j['error']) as String?,
      );
}
