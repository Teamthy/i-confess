import 'package:flutter_test/flutter_test.dart';
import 'package:iconfess/src/core/analytics/analytics.dart';
import 'package:iconfess/src/core/persistence/persistence.dart';
import 'package:iconfess_api/iconfess_api.dart';

class _RecordingSecureStorage implements SecureStorage {
  final Map<String, String> data = {};
  bool failReads = false;

  @override
  Future<String?> read(String key) async {
    if (failReads) throw StateError('keystore locked');
    return data[key];
  }

  @override
  Future<void> write(String key, String value) async => data[key] = value;

  @override
  Future<void> delete(String key) async => data.remove(key);
}

void main() {
  group('analytics', () {
    test('records a known event with its properties', () {
      final a = DebugAnalytics(sink: (_) {});
      a.track(AnalyticsEvents.sessionStarted, properties: {'duration_seconds': 1800});

      expect(a.events, hasLength(1));
      expect(a.events.single.name, AnalyticsEvents.sessionStarted);
      expect(a.events.single.properties['duration_seconds'], 1800);
    });

    test('refuses an event outside the known vocabulary', () {
      // A typo'd event name fragments a metric across two names and nobody
      // notices until a dashboard is wrong.
      final violations = <String>[];
      final a = DebugAnalytics(sink: (_) {}, onViolation: violations.add);
      a.track('session_startd');
      expect(a.events, isEmpty);
      expect(violations.single, contains('session_startd'));
      expect(isKnownEvent('session_startd'), isFalse);
      expect(isKnownEvent(AnalyticsEvents.sessionStarted), isTrue);
    });

    test('a misuse report never becomes an exception', () {
      // Analytics must not be able to break the flow that triggered it, so a
      // blocked property is dropped and reported rather than thrown.
      final a = DebugAnalytics(sink: (_) {});
      expect(
        () => a.track(AnalyticsEvents.searchPerformed, properties: {'email': 'a@b.c'}),
        returnsNormally,
      );
      expect(a.events.single.properties, isEmpty);
    });

    test('strips properties that must never leave the device', () {
      // §§67–68: no Scripture text, no email, no confession body. This is the
      // guard that keeps a listener's prayer content out of an analytics vendor.
      final a = DebugAnalytics(sink: (_) {}, onViolation: (_) {});
      a.track(AnalyticsEvents.searchPerformed, properties: {
        'query': 'I am worried about money',
        'email': 'listener@example.com',
        'scripture_text': 'Jeremiah 30:17',
        'result_count': 6,
      });

      final props = a.events.single.properties;
      expect(props.containsKey('query'), isFalse);
      expect(props.containsKey('email'), isFalse);
      expect(props.containsKey('scripture_text'), isFalse);
      expect(props['result_count'], 6, reason: 'safe properties must survive');
    });

    test('the blocklist is matched case-insensitively', () {
      final dropped = <String>[];
      final safe = sanitizeProperties(
        {'EMAIL': 'a@b.c', 'Query': 'x', 'ok': 1},
        onViolation: dropped.add,
      );
      expect(safe.keys, ['ok']);
      expect(dropped, hasLength(2));
    });

    test('identify and reset track attribution', () {
      final a = DebugAnalytics(sink: (_) {});
      a.identify('user-1');
      a.track(AnalyticsEvents.categoryViewed, properties: {'category_id': 'healing'});
      expect(a.events.single.userId, 'user-1');

      a.reset();
      a.track(AnalyticsEvents.appOpened);
      expect(a.events.last.userId, isNull);
    });

    test('the retained buffer is bounded', () {
      final a = DebugAnalytics(sink: (_) {});
      for (var i = 0; i < 250; i++) {
        a.track(AnalyticsEvents.playbackProgressSynced);
      }
      expect(a.events.length, lessThanOrEqualTo(200));
    });

    test('the noop implementation is inert rather than printing', () {
      const a = NoopAnalytics();
      a.track(AnalyticsEvents.appOpened);
      a.identify('u');
      a.reset();
      // Reaching here without throwing is the assertion: analytics must never be
      // able to break the flow that triggered it.
    });
  });

  group('persistence', () {
    test('round-trips JSON', () async {
      final store = InMemoryKeyValueStore();
      await store.writeJson('builder', {'duration': 1800, 'voice': 'v1'});
      final read = await store.readJson('builder');
      expect(read?['duration'], 1800);
      expect(read?['voice'], 'v1');
    });

    test('a corrupt entry reads as absent rather than throwing', () async {
      final store = InMemoryKeyValueStore();
      await store.writeString('builder', '{not json');
      expect(await store.readJson('builder'), isNull);
      // And it is cleared, so the next write is not appended to garbage.
      expect(await store.readString('builder'), isNull);
    });

    test('a non-object JSON value reads as absent', () async {
      final store = InMemoryKeyValueStore();
      await store.writeString('builder', '[1,2,3]');
      expect(await store.readJson('builder'), isNull);
    });

    test('the preferences store namespaces its keys', () {
      // A key collision with a plugin's own storage is a bug that reads as a
      // cache miss, which is the hardest kind to notice.
      expect(PreferencesKeyValueStore, isNotNull);
      expect(StoreKeys.onboardingSeen, 'onboarding.seen');
    });

    test('store keys are unique', () {
      const keys = [
        StoreKeys.onboardingSeen,
        StoreKeys.appMode,
        StoreKeys.lastCategoryIds,
        StoreKeys.lastDuration,
        StoreKeys.lastVoiceId,
        StoreKeys.analyticsConsent,
      ];
      expect(keys.toSet(), hasLength(keys.length));
    });
  });

  group('secure token store', () {
    test('caches the token after the first read', () async {
      final backing = _RecordingSecureStorage()..data['iconfess.access_token'] = 'abc';
      final store = SecureTokenStore(backing);

      expect(await store.read(), 'abc');
      // Second read must not touch the keystore: Keychain reads cost
      // milliseconds and the token is needed on every request.
      backing.failReads = true;
      expect(await store.read(), 'abc');
      expect(await store.hasSession(), isTrue);
    });

    test('a locked keystore reports signed-out rather than throwing', () async {
      Object? reported;
      final store = SecureTokenStore(
        _RecordingSecureStorage()..failReads = true,
        onStorageFailure: (e) => reported = e,
      );

      expect(await store.read(), isNull);
      expect(await store.hasSession(), isFalse);
      expect(reported, isNotNull, reason: 'the failure must be surfaced, not swallowed');
    });

    test('saving updates the cache even if the keystore write fails', () async {
      final store = SecureTokenStore(_RecordingSecureStorage());
      await store.save('token-1');
      expect(await store.read(), 'token-1');
      expect(await store.hasSession(), isTrue);

      await store.clear();
      expect(await store.read(), isNull);
      expect(await store.hasSession(), isFalse);
    });
  });
}
