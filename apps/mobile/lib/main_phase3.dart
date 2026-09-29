/// Compatibility entrypoint retained for old build configurations.
///
/// The maintained application bootstrap (including audio initialization) is
/// `main.dart`; audio services are not constructed in a second entrypoint.
import 'main.dart' as app;

Future<void> main() => app.main();
