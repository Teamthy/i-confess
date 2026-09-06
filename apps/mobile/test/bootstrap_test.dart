import 'package:flutter_test/flutter_test.dart';
import 'package:iconfess/main.dart' as app;

/// Importing `main.dart` is the point of this file.
///
/// No other test reaches the entry point, so without this import nothing in the
/// suite would compile it: a type error in the bootstrap — an override that no
/// longer matches, a provider renamed — would pass `flutter test` and only
/// surface when the app was actually built. Referencing [app.main] forces it
/// into the kernel.
void main() {
  test('the entry point compiles and has the expected shape', () {
    expect(app.main, isA<Future<void> Function()>());
  });
}
