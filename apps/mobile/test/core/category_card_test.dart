import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:iconfess/src/core/theme/theme.dart';
import 'package:iconfess/src/core/widgets/category_card.dart';

void main() {
  Widget host(Widget child, Brightness brightness) => MaterialApp(
        theme: AppTheme.discovery(brightness),
        home: Scaffold(
          body: Center(child: SizedBox(width: 180, height: 240, child: child)),
        ),
      );

  testWidgets('renders every shape, in both themes, and is one tappable button',
      (tester) async {
    for (final brightness in Brightness.values) {
      // Indexes 0..4 cover all four shapes and the wrap-around.
      for (var i = 0; i < 5; i++) {
        var taps = 0;
        await tester.pumpWidget(host(
          CategoryCard(name: 'Healing', index: i, onTap: () => taps++),
          brightness,
        ));
        expect(tester.takeException(), isNull);
        expect(find.bySemanticsLabel('Healing'), findsOneWidget);

        await tester.tap(find.byType(CategoryCard));
        expect(taps, 1);
      }
    }
  });
}
