import 'dart:io';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:iconfess/app.dart';
import 'package:iconfess/src/core/analytics/analytics.dart';
import 'package:iconfess/src/core/di/providers.dart';
import 'package:iconfess/src/core/routing/router.dart';
import 'package:iconfess/src/core/routing/routes.dart';
import 'package:iconfess/src/features/auth/auth_controller.dart';
import 'package:iconfess/src/features/confess/builder_providers.dart';
import 'package:iconfess/src/features/confess/session_builder.dart';
import 'package:iconfess_api/iconfess_api.dart';
import 'package:shared_preferences/shared_preferences.dart';

import 'support/fake_api_client.dart';

/// The session builder, end to end, against the scripted socket.
///
/// The walk asserts the core loop §12 promises — categories, length, voice,
/// create — and that what the client sends is what the server's handlers
/// defined in `server/internal/api/handlers.go`. The drift guard at the
/// bottom keeps the client's restated engine constants honest.
void main() {
  late ProviderContainer container;
  late FakeApiClient api;
  late InMemoryTokenStore tokens;
  late DebugAnalytics analytics;

  setUp(() {
    SharedPreferences.setMockInitialValues({});
    tokens = InMemoryTokenStore();
    api = FakeApiClient(tokens: tokens);
    analytics = DebugAnalytics(sink: (_) {});
    api.respond('/categories', {
      'data': [
        {'id': 'cat-fear', 'name': 'Fear', 'description': '', 'premium': false},
        {'id': 'cat-heal', 'name': 'Healing', 'description': '', 'premium': false},
      ],
    });
    api.respond('/voices', {
      'data': [
        {
          'id': 'grace',
          'name': 'Grace',
          'description': 'Warm, calm professional narration voice.',
          'gender': 'female',
          'premium': false,
          'status': 'active',
        },
        {
          'id': 'solace',
          'name': 'Solace',
          'description': '',
          'premium': true,
          'status': 'active',
        },
        {
          'id': 'old',
          'name': 'Retired Voice',
          'description': '',
          'premium': false,
          'status': 'retired',
        },
      ],
    });
    api.respond('/sessions/preview', {
      'target_seconds': 600,
      'actual_seconds': 540,
      'total_items': 4,
      'items_preview': [
        {'confession_id': 'c1', 'title': 'Fear not', 'duration_seconds': 300},
        {'confession_id': 'c2', 'title': 'Peace, be still', 'duration_seconds': 180},
        {'confession_id': 'c3', 'title': 'The Lord is my light', 'duration_seconds': 60},
      ],
      'voice_id': 'grace',
      'voice_downgraded': false,
      'strategy': 'BALANCED',
      'display': '10 MIN • 4 • grace',
    });
    api.respond('/sessions', {
      'id': 's-1',
      'type': 'custom',
      'status': 'STARTING',
      'duration_seconds': 600,
      'voice_id': 'grace',
      'voice_downgraded': false,
      'items': [
        {'confession_id': 'c1', 'title': 'Fear not', 'audio_url': '', 'duration_seconds': 300, 'position': 0},
        {'confession_id': 'c2', 'title': 'Peace, be still', 'audio_url': '', 'duration_seconds': 180, 'position': 1},
      ],
    });
    api.respond('/templates', {
      'template': {
        'id': 't-1',
        'user_id': 'u-1',
        'name': 'Morning on fear',
        'category_ids': ['cat-fear'],
        'voice_id': 'grace',
        'is_public': false,
        'share_token': 'tok-1',
      },
      'share_url': 'https://iconfess.app/t/tok-1',
      'deeplink': 'iconfess://t/tok-1',
    });
  });

  Future<void> pumpBuilder(
    WidgetTester tester, {
    String location = AppRoutes.confess,
  }) async {
    final prefs = await SharedPreferences.getInstance();
    container = ProviderContainer(
      overrides: [
        sharedPreferencesProvider.overrideWithValue(prefs),
        tokenStoreProvider.overrideWithValue(tokens),
        apiClientProvider.overrideWithValue(api),
        analyticsProvider.overrideWithValue(analytics),
        authControllerProvider.overrideWith(() => _SignedIn()),
      ],
    );
    addTearDown(container.dispose);

    await tester.pumpWidget(
      UncontrolledProviderScope(container: container, child: const IConfessApp()),
    );
    await tester.pumpAndSettle();
    container.read(routerProvider).go(location);
    await tester.pumpAndSettle();
  }

  void selectCategory(String id) {
    final current = container.read(selectedCategoriesProvider);
    container.read(selectedCategoriesProvider.notifier).state = {...current, id};
  }

  testWidgets('the walk reaches a created session: category, length, voice, create',
      (tester) async {
    await pumpBuilder(tester);

    // Step one: choose an area.
    await tester.tap(find.byKey(const ValueKey('chip-cat-fear')));
    await tester.pumpAndSettle();
    await tester.tap(find.byKey(const ValueKey('btn-continue')));
    await tester.pumpAndSettle();

    // Step two: the ladder, the engine's preview, the strategy default.
    expect(find.text('How much time will you give it?'), findsOneWidget);
    expect(find.byKey(const ValueKey('chip-duration-10m')), findsOneWidget);
    expect(find.byKey(const ValueKey('chip-duration-180m')), findsOneWidget);
    expect(find.text('10 MIN • 4 • grace'), findsOneWidget,
        reason: 'the preview restates the server display verbatim');
    expect(find.text('Fills 9 min of the 10 min you asked for'), findsOneWidget,
        reason: 'the gap between asked and filled is shown, not hidden');
    final balanced = tester
        .widget<RadioListTile<String>>(
          find.byKey(const ValueKey('radio-strategy-BALANCED')),
        )
        .groupValue;
    expect(balanced, 'BALANCED',
        reason: 'the default is the engine default, BALANCED');

    // A custom length the server would refuse is refused here first.
    await tester.enterText(
        find.byKey(const ValueKey('field-custom-minutes')), '500');
    await tester.testTextInput.receiveAction(TextInputAction.done);
    await tester.pumpAndSettle();
    expect(find.text('Enter a length from 1 to 180 minutes.'), findsOneWidget);

    await tester.tap(find.byKey(const ValueKey('btn-duration-continue')));
    await tester.pumpAndSettle();

    // Step three: voices, only the selectable ones.
    expect(find.text('Who will speak it with you?'), findsOneWidget);
    expect(find.text('No preference'), findsOneWidget);
    expect(find.text('Grace'), findsOneWidget);
    expect(find.text('Solace'), findsOneWidget);
    expect(find.text('Retired Voice'), findsNothing,
        reason: 'a voice that cannot be licensed is not offered');
    expect(find.text('Premium'), findsOneWidget);
    await tester.tap(find.byKey(const ValueKey('voice-solace')));
    await tester.pumpAndSettle();
    await tester.tap(find.byKey(const ValueKey('btn-voice-continue')));
    await tester.pumpAndSettle();

    // Step four: review, and the only create.
    expect(find.text('Here is what will be spoken.'), findsOneWidget);
    expect(find.text('Solace'), findsOneWidget, reason: 'the summary names the voice');
    await tester.tap(find.byKey(const ValueKey('btn-create')));
    await tester.pumpAndSettle();

    final body = api.bodyOf('/sessions');
    expect(body, isNotNull);
    expect(body!['category_ids'], ['cat-fear']);
    expect(body['duration_seconds'], 600);
    expect(body['voice_id'], 'solace');
    expect(find.byKey(const ValueKey('created-session')), findsOneWidget);
    expect(find.text('Playback opens in the player, arriving with the next phase.'),
        findsOneWidget,
        reason: 'no play button until the player exists; the gap is explained');

    expect(
      analytics.events.map((e) => e.name),
      contains(AnalyticsEvents.sessionCreated),
    );
  });

  testWidgets('a plan-capped create says so and offers a shorter length',
      (tester) async {
    api.respondWith(
      '/sessions',
      const ApiError(
        status: 402,
        code: 'entitlement_required',
        message: 'your plan allows sessions up to 30 minutes',
      ),
    );

    await pumpBuilder(tester);
    selectCategory('cat-fear');
    container.read(routerProvider).go(AppRoutes.builderCreate);
    await tester.pumpAndSettle();

    await tester.tap(find.byKey(const ValueKey('btn-create')));
    await tester.pumpAndSettle();

    expect(find.text('That length is more than your plan allows.'), findsOneWidget);
    expect(find.byKey(const ValueKey('btn-adjust-length')), findsOneWidget);
  });

  testWidgets('walking back to change the duration re-previews', (tester) async {
    await pumpBuilder(tester);
    selectCategory('cat-fear');

    container.read(builderDurationSecondsProvider.notifier).state = 1800;
    container.read(routerProvider).go(AppRoutes.builderDuration);
    await tester.pumpAndSettle();

    final body = api.bodyOf('/sessions/preview');
    expect(body, isNotNull);
    expect(body!['duration_seconds'], 1800,
        reason: 'the preview always reflects the current selection');
    expect((body['category_ids'] as List), ['cat-fear']);
  });

  testWidgets('"Build a session with this" pre-selects the category and says so',
      (tester) async {
    api.respond('/confessions/conf-1', {
      'id': 'conf-1',
      'category_id': 'cat-heal',
      'title': 'I am healed',
      'short_text': 'By His stripes',
      'status': 'published',
      'variants': [],
      'scriptures': [],
    });

    await pumpBuilder(
      tester,
      location: '${AppRoutes.confess}?confession=conf-1',
    );
    await tester.pumpAndSettle();

    expect(find.textContaining('Building from "I am healed"'), findsOneWidget);
    expect(container.read(selectedCategoriesProvider), {'cat-heal'},
        reason: 'the handoff must carry the confession into the builder');
  });

  group('drift guard — the client restates the engine', () {
    test('ladder, bounds and strategies match server/internal/engine/planner.go',
        () {
      final source = readRepoFile('server/internal/engine/planner.go');

      // Every client preset must exist in the engine with the same seconds.
      // The engine writes arithmetic (`Seconds: 10 * 60`), so the capture
      // admits digits joined by ` * `, never a free-form expression.
      for (final preset in builderPresets) {
        final pattern = RegExp(
          'Name: "${preset.name}", Label: "${preset.label}", Seconds: (\\d+(?: \\* \\d+)*)',
        );
        final match = pattern.firstMatch(source);
        expect(match, isNotNull,
            reason: 'engine preset ${preset.name} ("${preset.label}") is '
                'missing or renamed in planner.go');
        expect(resolveSeconds(match!.group(1)!), preset.seconds,
            reason: 'engine preset ${preset.name} no longer resolves to '
                '${preset.seconds}s — update session_builder.dart');
      }

      // And the engine must not have grown rungs the client hides. Only the
      // numeric rungs count; `custom` is an input mode, not a rung.
      final rungs = RegExp('Name: "(\\d+m)", Label:').allMatches(source);
      expect(rungs.map((m) => m.group(1)), hasLength(builderPresets.length),
          reason: 'the engine has a rung the builder does not offer');

      // The length bounds the handlers enforce.
      final handlers = readRepoFile('server/internal/api/handlers.go');
      expect(handlers.contains('3*3600'), isTrue,
          reason: 'maxSessionSeconds mirrors the handlers\' 3-hour ceiling');
      expect(handlers.contains('req.DurationSeconds < 60'), isTrue,
          reason: 'minSessionSeconds mirrors the handlers\' 1-minute floor');
      expect(maxSessionSeconds, 3 * 3600);
      expect(minSessionSeconds, 60);

      // The strategy vocabulary and its default.
      for (final s in builderStrategies) {
        expect(
          source.contains('Strategy${_canonical(s.name)} = "${s.name}"'),
          isTrue,
          reason: 'strategy ${s.name} is missing from planner.go',
        );
      }
      expect(source.contains('DefaultStrategy = StrategyBalanced'), isTrue);
    });
  });
}

class _SignedIn extends AuthController {
  @override
  AuthState build() => const AuthState.signedIn(userId: 'u-1');
}

/// Reads a repository file from the app root (`apps/mobile`), where
/// `flutter test` runs. Builder constants are mirrors; a mirror is only
/// honest if it is checked against its source.
String readRepoFile(String relativePath) {
  final file = File('../../$relativePath');
  expect(file.existsSync(), isTrue,
      reason: '$relativePath must exist for the drift guard to mean anything');
  return file.readAsStringSync();
}

/// `BALANCED` → `Balanced`, the Go constant suffix (`StrategyBalanced`).
String _canonical(String s) =>
    s.isEmpty ? s : '${s[0]}${s.substring(1).toLowerCase()}';

/// Resolves the engine's `Seconds:` expressions, which are arithmetic
/// (`10 * 60`) rather than literals.
int resolveSeconds(String expression) {
  final cleaned = expression.replaceAll(RegExp(r'[^0-9*]'), '');
  final parts = cleaned.split('*').where((p) => p.isNotEmpty).map(int.parse);
  return parts.reduce((a, b) => a * b);
}
