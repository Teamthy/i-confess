import 'dart:async';
import 'dart:io';

import 'package:flutter/foundation.dart';
import 'package:in_app_purchase/in_app_purchase.dart';

/// A product as the store describes it.
///
/// The price comes from the store, never from the app or the server: it is what
/// the store will charge, in the currency the store sees, and showing anything
/// else on a paywall is a consumer-protection problem rather than a rounding
/// one.
@immutable
class StoreProduct {
  const StoreProduct({required this.id, required this.title, required this.price});

  final String id;
  final String title;

  /// A localised, formatted price string, e.g. `₦4,500.00`.
  final String price;
}

/// A completed purchase the server has not verified yet.
@immutable
class StorePurchase {
  const StorePurchase({
    required this.id,
    required this.productId,
    required this.provider,
    required this.receipt,
    this.restored = false,
  });

  /// The store's own identifier for this transaction, used to complete it once
  /// the server has decided.
  final String id;
  final String productId;

  /// `apple` or `google` — the provider names the verify endpoint accepts.
  final String provider;

  /// The receipt, or the Play purchase token. The server decides what it means;
  /// this string is never interpreted on the device.
  final String receipt;

  /// Whether the store produced this by restoring rather than by a new charge.
  final bool restored;
}

/// The store, as the app needs it.
///
/// `in_app_purchase` is a platform channel, so it cannot run under
/// `flutter test`. Keeping it behind an interface is what lets the purchase
/// *flow* — buy, verify server-side, complete, retry on a transient failure —
/// be tested without a store account, which is the part that has logic worth
/// testing. The plugin is a thin adapter over the same flow.
abstract interface class PurchaseGateway {
  /// The provider name the server expects: `apple` or `google`. It also
  /// selects which store's product ids this build sells.
  String get provider;

  /// Products the store knows about, for the ids asked for. Unknown ids are
  /// omitted rather than invented.
  Future<List<StoreProduct>> loadProducts(Set<String> ids);

  /// Buys a product. The result arrives on [purchases] or [errors]: the stores
  /// answer asynchronously and may take minutes (a bank confirmation), so this
  /// future completing is not the purchase completing.
  Future<void> buy(String productId);

  /// Tells the store the transaction is finished.
  ///
  /// Called only after the server has recorded the entitlement, or after the
  /// server declined it for good. A purchase left unfinished is re-delivered by
  /// the store on every launch, which is the intended behaviour for a transient
  /// failure and an endless loop for a permanent one.
  Future<void> complete(String purchaseId);

  /// Asks the store for purchases this account already owns.
  Future<void> restore();

  /// Completed purchases awaiting server verification.
  Stream<StorePurchase> get purchases;

  /// Failures worth showing the user.
  Stream<String> get errors;
}

/// [PurchaseGateway] backed by `in_app_purchase`.
class InAppPurchaseGateway implements PurchaseGateway {
  InAppPurchaseGateway({InAppPurchase? store}) : _store = store ?? InAppPurchase.instance;

  final InAppPurchase _store;
  final _purchases = StreamController<StorePurchase>.broadcast();
  final _errors = StreamController<String>.broadcast();
  final _outstanding = <String, PurchaseDetails>{};

  StreamSubscription<List<PurchaseDetails>>? _subscription;

  @override
  Stream<StorePurchase> get purchases => _purchases.stream;

  @override
  Stream<String> get errors => _errors.stream;

  /// The provider name the server expects for this platform.
  @override
  String get provider {
    if (Platform.isIOS || Platform.isMacOS) return 'apple';
    // Play is the only store this app is published to on Android. A build that
    // adds another (Huawei AppGallery, for instance) needs its own gateway
    // rather than a guess here.
    return 'google';
  }

  void _listen() {
    _subscription ??= _store.purchaseStream.listen(
      _onPurchases,
      onError: (Object error) => _errors.add(error.toString()),
    );
  }

  @override
  Future<List<StoreProduct>> loadProducts(Set<String> ids) async {
    _listen();
    if (ids.isEmpty || !await _store.isAvailable()) return const [];
    final response = await _store.queryProductDetails(ids);
    return response.productDetails
        .map((d) => StoreProduct(id: d.id, title: d.title, price: d.price))
        .toList(growable: false);
  }

  @override
  Future<void> buy(String productId) async {
    _listen();
    if (!await _store.isAvailable()) {
      throw const PurchaseException('The store is not available on this device.');
    }
    final response = await _store.queryProductDetails({productId});
    if (response.productDetails.isEmpty) {
      // A product id that the store does not know is a build configured for a
      // different store listing. Saying so is more useful than a generic
      // failure, because the person seeing it can fix it.
      throw PurchaseException(
        response.notFoundIDs.isEmpty
            ? 'That plan is not available in this build.'
            : 'The store does not sell ${response.notFoundIDs.join(', ')}.',
      );
    }
    await _store.buyNonConsumable(
      purchaseParam: PurchaseParam(productDetails: response.productDetails.first),
    );
  }

  @override
  Future<void> complete(String purchaseId) async {
    final details = _outstanding.remove(purchaseId);
    if (details == null || !details.pendingCompletePurchase) return;
    await _store.completePurchase(details);
  }

  @override
  Future<void> restore() async {
    _listen();
    await _store.restorePurchases();
  }

  void _onPurchases(List<PurchaseDetails> purchases) {
    for (final purchase in purchases) {
      final id = purchase.purchaseID;
      if (purchase.status == PurchaseStatus.error) {
        _errors.add(purchase.error?.message ?? 'The purchase failed.');
        continue;
      }
      if (purchase.status == PurchaseStatus.canceled) {
        continue;
      }
      if (purchase.status == PurchaseStatus.pending) {
        // A bank confirmation can take minutes; the user is told to wait, and
        // nothing is verified until the store says the purchase completed.
        continue;
      }
      if (purchase.status != PurchaseStatus.purchased &&
          purchase.status != PurchaseStatus.restored) {
        continue;
      }
      if (id == null) {
        // Without an id there is nothing to complete later, so the transaction
        // would be re-delivered forever. The receipt is still handed to the
        // server, which is what decides entitlement.
        debugPrint('push: purchase without an id: ${purchase.productID}');
      } else {
        _outstanding[id] = purchase;
      }
      _purchases.add(StorePurchase(
        id: id ?? '',
        productId: purchase.productID,
        provider: provider,
        receipt: purchase.verificationData.serverVerificationData,
        restored: purchase.status == PurchaseStatus.restored,
      ));
    }
  }

  Future<void> dispose() async {
    await _subscription?.cancel();
    await _purchases.close();
    await _errors.close();
  }
}

/// A purchase that could not be started or finished.
class PurchaseException implements Exception {
  const PurchaseException(this.message);

  final String message;

  @override
  String toString() => message;
}
