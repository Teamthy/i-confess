import 'package:iconfess_api/iconfess_api.dart';
import 'package:test/test.dart';

void main() {
  test('verification reads the server entitlement, not the store plan', () {
    final result = Subscription.fromVerification({
      'verified': true,
      'plan': 'monthly',
      'state': 'active',
      'entitlements': {'Plan': 'premium'},
    });
    expect(result.isPremium, isTrue);
    expect(result.status, 'active');
  });

  test('a store plan without a positive server entitlement grants nothing', () {
    for (final response in <Map<String, dynamic>>[
      {'verified': true, 'plan': 'premium'},
      {'verified': false, 'entitlements': {'Plan': 'premium'}},
      {'verified': true, 'plan': 'annual', 'entitlements': {'Plan': 'free'}},
    ]) {
      expect(Subscription.fromVerification(response).isPremium, isFalse);
    }
  });
}
