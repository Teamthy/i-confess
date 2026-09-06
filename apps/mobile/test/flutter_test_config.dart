import 'dart:async';
import 'dart:io';

import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';

/// Loads real Roboto before any test runs.
///
/// Golden tests are only worth their maintenance cost if the pixels mean
/// something, and without a font every glyph renders as a box: the comparison
/// still passes and fails, but a reviewer looking at the image learns nothing
/// and a spacing regression is indistinguishable from a font regression.
///
/// The files are committed rather than read from the SDK's `material_fonts`
/// cache on purpose. That cache is not part of any Flutter contract, and a
/// golden that depends on it is a golden that breaks on a machine that has a
/// slightly different SDK.
Future<void> testExecutable(FutureOr<void> Function() testMain) async {
  await _loadRoboto();
  return testMain();
}

Future<void> _loadRoboto() async {
  TestWidgetsFlutterBinding.ensureInitialized();

  // The design tokens ask for 'Inter' and 'Source Serif 4', neither of which
  // ships with the app, so on any device the platform substitutes. In the test
  // environment an unknown family renders as placeholder boxes, which would
  // make every golden illegible. Registering the committed Roboto under the
  // names the styles request gives real, reproducible glyphs without bundling
  // licensed fonts.
  const families = ['Roboto', 'Inter', 'Source Serif 4'];
  for (final family in families) {
    final loader = FontLoader(family);
    for (final weight in const ['Regular', 'Medium', 'Bold']) {
      final bytes = File('test/fonts/Roboto-$weight.ttf').readAsBytesSync();
      loader.addFont(Future.value(ByteData.view(bytes.buffer)));
    }
    await loader.load();
  }
}
