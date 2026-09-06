import 'package:flutter/foundation.dart';

/// The product's event vocabulary.
///
/// Lifted deliberately from the retired shell, which had the right event names
/// and the wrong implementation (a singleton that `debugPrint`ed and carried a
/// TODO for production). The names are the asset; they are what analytics,
/// retention work and the server's own event log have to agree on, and inventing
/// a second vocabulary would make the two unreconcilable.
abstract final class AnalyticsEvents {
  static const appOpened = 'app_opened';
  static const onboardingCompleted = 'onboarding_completed';
  static const signInSucceeded = 'sign_in_succeeded';
  static const signUpSucceeded = 'sign_up_succeeded';
  static const signOut = 'sign_out';

  static const sessionCreated = 'session_created';
  static const sessionStarted = 'session_started';
  static const sessionPaused = 'session_paused';
  static const sessionResumed = 'session_resumed';
  static const sessionCompleted = 'session_completed';
  static const sessionAbandoned = 'session_abandoned';

  static const scheduleCreated = 'schedule_created';
  static const scheduleTriggered = 'schedule_triggered';

  static const downloadCreated = 'download_created';
  static const trialDayViewed = 'trial_day_viewed';
  static const subscriptionStarted = 'subscription_started';
  static const subscriptionVerified = 'subscription_verified';
  static const subscriptionCancelled = 'subscription_cancelled';

  static const searchPerformed = 'search_performed';
  static const categoryViewed = 'category_viewed';
  static const confessionViewed = 'confession_viewed';
  static const templateCreated = 'template_created';
  static const playbackProgressSynced = 'playback_progress_synced';

  static const all = <String>{
    appOpened, onboardingCompleted, signInSucceeded, signUpSucceeded, signOut,
    sessionCreated, sessionStarted, sessionPaused, sessionResumed,
    sessionCompleted, sessionAbandoned,
    scheduleCreated, scheduleTriggered,
    downloadCreated, trialDayViewed,
    subscriptionStarted, subscriptionVerified, subscriptionCancelled,
    searchPerformed, categoryViewed, confessionViewed, templateCreated,
    playbackProgressSynced,
  };
}

/// Property keys that must never carry a value.
///
/// Section 67–§68: no Scripture text, no email, no confession body, no
/// free-form user input. Analytics leaves the device, and a listener's prayer
/// content is not ours to send anywhere. The guard is in code rather than in a
/// convention because the convention is one careless `props: {...}` away from
/// being broken.
abstract final class AnalyticsBlocklist {
  static const keys = <String>{
    'email',
    'password',
    'token',
    'scripture',
    'scripture_text',
    'confession_text',
    'body',
    'text',
    'note',
    'notes',
    'query',
    'search_query',
    'phone',
    'name',
    'first_name',
    'last_name',
  };
}

/// Where analytics goes.
///
/// An interface rather than a service locator so a test can assert what was
/// tracked, and so the production transport can be swapped without touching a
/// single call site.
abstract interface class Analytics {
  /// Records an event. Must never throw: analytics failing cannot be allowed to
  /// break the flow that triggered it.
  void track(String event, {Map<String, Object?>? properties});

  /// Associates subsequent events with a user, for signed-in attribution.
  void identify(String userId);

  /// Clears the association on sign-out.
  void reset();
}

/// Strips blocked keys and reports what it removed, so a leak is visible in
/// debug rather than silently shipped.
///
/// Reporting goes through [onViolation] rather than an `assert`. An assertion
/// throws, and analytics must never be able to break the flow that triggered it:
/// a listener who completes a session should not get an exception because
/// somebody passed a `query` property.
Map<String, Object?> sanitizeProperties(
  Map<String, Object?>? properties, {
  void Function(String message)? onViolation,
}) {
  if (properties == null || properties.isEmpty) return const {};
  final out = <String, Object?>{};
  for (final entry in properties.entries) {
    if (AnalyticsBlocklist.keys.contains(entry.key.toLowerCase())) {
      onViolation?.call('analytics: dropped blocked property "${entry.key}"');
      continue;
    }
    out[entry.key] = entry.value;
  }
  return out;
}

/// Drops events that are not in the known vocabulary.
///
/// A typo'd event name is worse than a missing one: it fragments a metric across
/// two names and nobody notices until a dashboard is wrong. Refusing it in debug
/// surfaces the typo at the call site.
bool isKnownEvent(String event) => AnalyticsEvents.all.contains(event);

/// Development implementation: prints, and keeps a bounded in-memory record.
///
/// Not a stub — it is what makes analytics testable and visible during
/// development without a network dependency.
final class DebugAnalytics implements Analytics {
  DebugAnalytics({this.sink, this.onViolation});

  /// Where lines go. Defaults to [debugPrint]; tests inject a recorder.
  final void Function(String line)? sink;

  /// Where misuse is reported: an unknown event, or a property the blocklist
  /// removed. Defaults to [debugPrint] in debug builds.
  ///
  /// A callback rather than an assertion, because an assertion throws and
  /// analytics is not allowed to break the flow that triggered it.
  final void Function(String message)? onViolation;

  static const _maxRetained = 200;
  final List<TrackedEvent> events = [];
  String? _userId;

  void _emit(String line) => (sink ?? debugPrint)(line);

  void _violate(String message) {
    final report = onViolation;
    if (report != null) {
      report(message);
    } else if (kDebugMode) {
      debugPrint(message);
    }
  }

  @override
  void track(String event, {Map<String, Object?>? properties}) {
    if (!isKnownEvent(event)) {
      _violate('analytics: unknown event "$event" — add it to AnalyticsEvents');
      return;
    }
    final safe = sanitizeProperties(properties, onViolation: _violate);
    events.add(TrackedEvent(event, _userId, safe, DateTime.now()));
    if (events.length > _maxRetained) {
      events.removeRange(0, events.length - _maxRetained);
    }
    _emit('analytics $event user=${_userId ?? 'anon'} ${safe.isEmpty ? '' : safe}');
  }

  @override
  void identify(String userId) {
    _userId = userId;
    _emit('analytics identify ${userId.hashCode}');
  }

  @override
  void reset() {
    _userId = null;
    _emit('analytics reset');
  }
}

/// One recorded event, for tests and for the debug inspector.
@immutable
class TrackedEvent {
  const TrackedEvent(this.name, this.userId, this.properties, this.at);

  final String name;
  final String? userId;
  final Map<String, Object?> properties;
  final DateTime at;

  @override
  String toString() => '$name${properties.isEmpty ? '' : ' $properties'}';
}

/// Production implementation: no-op until a transport is chosen.
///
/// Deliberately explicit rather than defaulting to [DebugAnalytics]. Shipping the
/// debug implementation would mean shipping a print statement per event in
/// release builds, and silently discarding events with no signal is how an
/// analytics integration is believed to be working when it is not.
final class NoopAnalytics implements Analytics {
  const NoopAnalytics();

  @override
  void track(String event, {Map<String, Object?>? properties}) {}

  @override
  void identify(String userId) {}

  @override
  void reset() {}
}
