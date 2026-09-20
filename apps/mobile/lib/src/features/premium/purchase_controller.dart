import 'dart:async';

import 'package:flutter/foundation.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:iconfess_api/iconfess_api.dart';

import '../../core/di/providers.dart';
import 'premium_providers.dart';
import 'purchase_gateway.dart';

/// Store product ids, per plan and platform.
///
/// The catalogue the server sells (`monthly`, `annual`) is not the catalogue the
/// stores sell: App Store Connect and the Play Console each carry their own
/// identifier, and the server maps its own product ids back to plans
/// (`APPLE_PRODUCT_MONTHLY`, `GOOGLE_PLAY_PRODUCT_MONTHLY`). A build therefore
/// has to be told what this store listing calls each plan, which is what these
/// dart-defines are for.
///
/// The defaults are the ids the deployment documentation uses. A store that does
/// not know an id answers "not found", which the paywall reports as a build
/// configuration problem rather than a payment failure.
const _appleProductIds = <String, String>{
  'monthly': String.fromEnvironment('ICONFESS_APPLE_MONTHLY', defaultValue: 'app.iconfess.monthly'),
  'annual': String.fromEnvironment('ICONFESS_APPLE_ANNUAL', defaultValue: 'app.iconfess.annual'),
};

const _googleProductIds = <String, String>{
  'monthly': String.fromEnvironment('ICONFESS_GOOGLE_MONTHLY', defaultValue: 'app.iconfess.monthly'),
  'annual': String.fromEnvironment('ICONFESS_GOOGLE_ANNUAL', defaultValue: 'app.iconfess.annual'),
};

/// The store product id for a catalogue plan, or null when the build does not
/// sell that plan.
///
/// Null is a real answer: the annual plan can be absent from a listing, and a
/// Subscribe button that buys the monthly plan because the annual id was missing
/// would charge the wrong price.
@visibleForTesting
String? storeProductIdFor(String planId, {required bool apple}) {
  final ids = apple ? _appleProductIds : _googleProductIds;
  final id = ids[planId];
  return (id == null || id.isEmpty) ? null : id;
}

/// What the paywall is doing.
@immutable
class PurchaseState {
  const PurchaseState({this.busy = false, this.message, this.error});

  /// A purchase or a verification is in flight.
  final bool busy;

  /// Something the user should read, on success.
  final String? message;

  /// Something went wrong, worded for the user.
  final String? error;

  bool get idle => !busy && message == null && error == null;
}

/// Runs the purchase flow: buy in the store, verify on the server, then finish.
///
/// The order matters and is the whole point of the class. The store is asked for
/// money, the *server* decides what the receipt is worth (nothing here can grant
/// Premium), and only then is the transaction marked finished. A client that
/// granted entitlement from the store's own "purchased" callback would be
/// trusting a value it printed itself.
class PremiumPurchaseController extends Notifier<PurchaseState> {
  @override
  PurchaseState build() {
    final gateway = ref.watch(purchaseGatewayProvider);

    final purchases = gateway.purchases.listen(_verify);
    final errors = gateway.errors.listen((message) {
      state = PurchaseState(error: message);
    });
    ref.onDispose(() {
      purchases.cancel();
      errors.cancel();
    });

    return const PurchaseState();
  }

  /// Starts a purchase for a catalogue plan.
  Future<void> purchase(String planId) async {
    final gateway = ref.read(purchaseGatewayProvider);
    final productId = storeProductIdFor(planId, apple: gateway.provider == 'apple');

    if (productId == null) {
      state = const PurchaseState(error: 'That plan is not available in this build.');
      return;
    }

    state = const PurchaseState(busy: true);
    try {
      await gateway.buy(productId);
    } on PurchaseException catch (e) {
      state = PurchaseState(error: e.message);
    } catch (e) {
      state = PurchaseState(error: 'The store could not start the purchase: $e');
    }
    // The result does not arrive here: the stores answer on their own stream,
    // which [_verify] handles. Clearing `busy` now would make the button
    // tappable again while the store is still deciding.
  }

  /// Asks the store to replay purchases this account already owns.
  ///
  /// Required by App Store review for a non-consumable, and the only recovery
  /// path for a user who reinstalled: without it a paid subscriber sees a
  /// paywall with no way to get their subscription back.
  Future<void> restore() async {
    final gateway = ref.read(purchaseGatewayProvider);
    state = const PurchaseState(busy: true);
    try {
      await gateway.restore();
    } catch (e) {
      state = PurchaseState(error: 'Could not restore purchases: $e');
    }
  }

  void clear() => state = const PurchaseState();

  /// Sends a completed purchase to the server and acts on its verdict.
  Future<void> _verify(StorePurchase purchase) async {
    final gateway = ref.read(purchaseGatewayProvider);
    state = const PurchaseState(busy: true);

    final result = await ref
        .read(subscriptionRepositoryProvider)
        .verifyReceipt(provider: purchase.provider, receipt: purchase.receipt);

    switch (result) {
      case WriteSuccess<Subscription>(value: final subscription):
        // Only now is the transaction finished. The stores re-deliver an
        // unfinished purchase, which is how a crash between "paid" and
        // "recorded" recovers - and also why completing early loses the sale.
        await gateway.complete(purchase.id);
        ref.invalidate(subscriptionProvider);
        ref.invalidate(entitlementsProvider);
        state = PurchaseState(
          message: subscription.isPremium
              ? (purchase.restored ? 'Subscription restored.' : 'You are Premium. Thank you.')
              : 'The store accepted the purchase, but this account is not Premium yet. '
                  'It will appear here as soon as the store confirms it.',
        );

      case WriteFailure<Subscription>(error: final error):
        if (_worthRetrying(error)) {
          // Left unfinished on purpose: the store replays it, and the next
          // attempt usually succeeds. Completing here would discard a purchase
          // the user paid for because of a dropped connection.
          state = PurchaseState(error: _retryMessage(error));
          return;
        }
        await gateway.complete(purchase.id);
        state = PurchaseState(error: error.message);
    }
  }

  static bool _worthRetrying(ApiException error) {
    if (error is NetworkException) return true;
    if (error is ApiError) return error.isServerFault || error.status == 503 || error.status == 429;
    return false;
  }

  static String _retryMessage(ApiException error) {
    if (error is NetworkException) {
      return 'Your purchase is safe. We could not reach the server to confirm it - '
          'it will be confirmed automatically when you are back online.';
    }
    return 'Your purchase is safe. The server could not confirm it just now; '
        'we will try again shortly.';
  }
}

/// The gateway the app runs on. Tests override this with a fake store.
final purchaseGatewayProvider = Provider<PurchaseGateway>((ref) => InAppPurchaseGateway());

final premiumPurchaseControllerProvider =
    NotifierProvider<PremiumPurchaseController, PurchaseState>(PremiumPurchaseController.new);
