import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:iconfess/src/features/moderation/moderation_providers.dart';
import 'package:iconfess/src/features/settings/safety_screen.dart';
import 'package:iconfess_api/iconfess_api.dart';

void main() {
  testWidgets('shows blocked-account and appeal management states', (tester) async {
    await tester.pumpWidget(
      ProviderScope(
        overrides: [
          moderationBlocksProvider.overrideWith(
            (ref) async => const Loadable<List<UserBlock>>.loaded([]),
          ),
          moderationAppealsProvider.overrideWith(
            (ref) async => const Loadable<List<ModerationAppeal>>.loaded([]),
          ),
        ],
        child: const MaterialApp(home: SafetyScreen()),
      ),
    );
    await tester.pumpAndSettle();

    expect(find.text('No blocked accounts'), findsOneWidget);
    expect(find.text('Block account'), findsOneWidget);

    await tester.tap(find.text('Appeals'));
    await tester.pumpAndSettle();
    expect(find.text('No appeals yet'), findsOneWidget);
    expect(find.text('Appeal a decision'), findsOneWidget);
  });

  testWidgets('requires an account ID before opening a block request', (tester) async {
    await tester.pumpWidget(
      ProviderScope(
        overrides: [
          moderationBlocksProvider.overrideWith(
            (ref) async => const Loadable<List<UserBlock>>.loaded([]),
          ),
          moderationAppealsProvider.overrideWith(
            (ref) async => const Loadable<List<ModerationAppeal>>.loaded([]),
          ),
        ],
        child: const MaterialApp(home: SafetyScreen()),
      ),
    );
    await tester.pumpAndSettle();
    await tester.tap(find.text('Block account'));
    await tester.pumpAndSettle();

    expect(find.text('Account ID'), findsOneWidget);
    expect(find.text('Blocking is private to your account. Use a community story menu to block its anonymous author without seeing their ID.'), findsOneWidget);
  });
}
