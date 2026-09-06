import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../core/analytics/analytics.dart';
import '../../core/di/providers.dart';
import '../../core/persistence/persistence.dart';

/// Whether the listener has been through onboarding.
///
/// Persisted rather than inferred from "has an account", because the two are not
/// the same thing: someone who creates an account, force-quits on the last slide
/// and comes back tomorrow should not be walked through the tour again, and
/// someone who browses as a guest before signing up should still see it.
///
/// The value is read lazily. `build` cannot be async, and blocking the first
/// frame on a SharedPreferences read to decide whether to show three slides
/// nobody has asked for yet would be the wrong trade.
class OnboardingController extends Notifier<bool> {
  @override
  bool build() => false;

  bool _loaded = false;

  /// Reads the stored flag once. Later calls are no-ops, so a screen can call
  /// this from `initState` without racing itself.
  Future<void> load() async {
    if (_loaded) return;
    final raw = await ref.read(keyValueStoreProvider).readString(StoreKeys.onboardingSeen);
    _loaded = true;
    state = raw == 'true';
  }

  /// Records that onboarding was finished or skipped.
  ///
  /// Skipped counts. Three slides that can only be dismissed by completing them
  /// are not onboarding, they are a toll booth, and a listener who taps past
  /// them has still told us what they want.
  Future<void> complete() async {
    await ref.read(keyValueStoreProvider).writeString(StoreKeys.onboardingSeen, 'true');
    _loaded = true;
    state = true;
    ref.read(analyticsProvider).track(AnalyticsEvents.onboardingCompleted);
  }
}

final onboardingControllerProvider =
    NotifierProvider<OnboardingController, bool>(OnboardingController.new);
