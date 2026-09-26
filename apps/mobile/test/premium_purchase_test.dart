import 'dart:async';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:iconfess/src/core/di/providers.dart';
import 'package:iconfess/src/features/premium/purchase_controller.dart';
import 'package:iconfess/src/features/premium/purchase_gateway.dart';
import 'package:iconfess_api/iconfess_api.dart';

import 'support/fake_api_client.dart';

/// The purchase flow: buy in the store, verify on the server, then finish.
///
/// The order is the point. The device asks the store for money and the *server*
/// decides what the receipt is worth; nothing here can grant Premium. These
/// tests pin that down, along with the two ways it can go wrong quietly: a
/// transaction completed before the server has recorded it (the sale is lost)
/// and a transaction never completed after a permanent rejection (the store
/// replays it forever).

class FakeStoreFront implements PurchaseGateway {
  final _purchases = StreamController<StorePurchase>.broadcast();
  final _errors = StreamController<String>.broadcast();

  final List<String> bought = [];
  final List<String> completed = [];
  int restores = 0;
  PurchaseException? buyFailure;

  @override
  void start() {}

  @override
  String get provider => 'apple';

  @override
  Stream<StorePurchase> get purchases => _purchases.stream;

  @override
  Stream<String> get errors => _errors.stream;

  @override
  Future<List<StoreProduct>> loadProducts(Set<String> ids) async =>
      [for (final id in ids) StoreProduct(id: id, title: 'Premium', price: '₦4,500')];

  @override
  Future<void> buy(String productId) async {
    if (buyFailure != null) throw buyFailure!;
    bought.add(productId);
  }

  @override
  Future<void> complete(String purchaseId) async => completed.add(purchaseId);

  @override
  Future<void> restore() async => restores++;

  void deliver(StorePurchase purchase) => _purchases.add(purchase);

  Future<void> dispose() async {
    await _purchases.close();
    await _errors.close();
  }
}

void main() {
  late FakeApiClient api;
  late FakeStoreFront store;
  late ProviderContainer container;

  setUp(() {
    api = FakeApiClient(tokens: InMemoryTokenStore());
    store = FakeStoreFront();
    container = ProviderContainer(overrides: [
      apiClientProvider.overrideWithValue(api),
      purchaseGatewayProvider.overrideWithValue(store),
    ]);
    addTearDown(container.dispose);
    addTearDown(store.dispose);
    // Keep the controller alive for the duration of the test.
    addTearDown(container.listen(premiumPurchaseControllerProvider, (_, _) {}).close);
  });

  PremiumPurchaseController controller() =>
      container.read(premiumPurchaseControllerProvider.notifier);
  PurchaseState state() => container.read(premiumPurchaseControllerProvider);

  test('buying uses the store product id, not the catalogue plan id', () async {
    await controller().purchase('monthly');
    expect(store.bought, ['app.iconfess.monthly']);

    controller().clear();
    await controller().purchase('annual');
    expect(store.bought, ['app.iconfess.monthly', 'app.iconfess.annual']);
  });

  test('a plan this build does not sell is refused before the store is asked', () async {
    await controller().purchase('lifetime');
    expect(store.bought, isEmpty);
    expect(state().error, isNotNull);
  });

  test('a verified purchase is completed and reports Premium', () async {
    api.respond('/subscriptions/verify', {
      'verified': true,
      'plan': 'monthly',
      'state': 'active',
      'entitlements': {'Plan': 'premium'},
    });

    controller().purchase('monthly');
    await Future<void>.delayed(Duration.zero);

    store.deliver(const StorePurchase(
      id: 'tx-1',
      productId: 'app.iconfess.monthly',
      provider: 'apple',
      receipt: 'signed-receipt',
    ));
    await pumpEventQueue();

    expect(api.callCount('/subscriptions/verify'), 1);
    final body = api.bodyOf('/subscriptions/verify')!;
    expect(body['provider'], 'apple');
    expect(body['receipt'], 'signed-receipt');
    expect(store.completed, ['tx-1']);
    expect(state().message, contains('Premium'));
    expect(state().busy, isFalse);
  });

  // A dropped connection is not a rejected purchase. Completing the
  // transaction here would tell the store to forget a sale the server has not
  // recorded, and the user would have paid for nothing.
  test('a server fault leaves the transaction open for another attempt', () async {
    api.respondWith(
      '/subscriptions/verify',
      const ApiError(status: 503, code: 'PROVIDER_UNAVAILABLE', message: 'verifier unconfigured'),
    );

    store.deliver(const StorePurchase(
      id: 'tx-2',
      productId: 'app.iconfess.monthly',
      provider: 'apple',
      receipt: 'signed-receipt',
    ));
    await pumpEventQueue();

    expect(store.completed, isEmpty, reason: 'a retryable failure must not finish the purchase');
    expect(state().error, isNotNull);
    expect(state().busy, isFalse);
  });

  test('a rejected receipt is finished and reported', () async {
    api.respondWith(
      '/subscriptions/verify',
      const ApiError(status: 400, code: 'RECEIPT_INVALID', message: 'receipt is not signed by Apple'),
    );

    store.deliver(const StorePurchase(
      id: 'tx-3',
      productId: 'app.iconfess.monthly',
      provider: 'apple',
      receipt: 'forged',
    ));
    await pumpEventQueue();

    expect(store.completed, ['tx-3'],
        reason: 'a permanent rejection would otherwise be replayed by the store forever');
    expect(state().error, contains('signed by Apple'));
  });

  test('an offline verification is retried rather than finished', () async {
    api.respondWith(
      '/subscriptions/verify',
      const NetworkException('no route to host'),
    );

    store.deliver(const StorePurchase(
      id: 'tx-4',
      productId: 'app.iconfess.monthly',
      provider: 'apple',
      receipt: 'signed-receipt',
    ));
    await pumpEventQueue();

    expect(store.completed, isEmpty);
    expect(state().error, contains('safe'));
  });

  test('expired authentication does not finish a paid transaction', () async {
    api.respondWith('/subscriptions/verify',
        const ApiError(status: 401, code: 'UNAUTHORIZED', message: 'sign in again'));
    store.deliver(const StorePurchase(id: 'tx-auth', productId: 'app.iconfess.monthly',
        provider: 'apple', receipt: 'signed-receipt'));
    await pumpEventQueue();
    expect(store.completed, isEmpty);
    expect(state().busy, isFalse);
  });

  test('a second tap while buying does not make a second purchase', () async {
    await controller().purchase('monthly');
    await controller().purchase('monthly');
    expect(store.bought, ['app.iconfess.monthly']);
  });

  test('restore with no transactions stops showing a spinner', () async {
    await controller().restore();
    expect(state().busy, isFalse);
  });

  test('restore asks the store to replay what the account owns', () async {
    await controller().restore();
    expect(store.restores, 1);
  });

  test('a store that cannot start the purchase says so', () async {
    store.buyFailure = const PurchaseException('The store is not available on this device.');
    await controller().purchase('monthly');
    expect(state().error, 'The store is not available on this device.');
    expect(state().busy, isFalse);
  });

  test('store product ids are per platform', () {
    expect(storeProductIdFor('monthly', apple: true), isNotNull);
    expect(storeProductIdFor('monthly', apple: false), isNotNull);
    expect(storeProductIdFor('unknown', apple: true), isNull);
  });
}
