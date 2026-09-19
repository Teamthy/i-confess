/// The builder's own constants, mirrored from the server's engine.
///
/// The duration ladder and the length bounds live in
/// `server/internal/engine/planner.go` and the session handlers; the strategy
/// vocabulary lives beside them. The client re-states them because a builder
/// that offers a length the server refuses is a dead end, and a builder that
/// hides a length the server accepts is a smaller product than the one that
/// shipped.
///
/// Restating invites drift, so `test/builder_test.dart` reads the Go source and
/// fails if the two ever disagree — the same guard PHASE 19 built for the
/// password policy. Change the engine here first, then the mirror.
library;

/// One rung on the builder's duration ladder.
final class BuilderPreset {
  const BuilderPreset({required this.name, required this.label, required this.seconds});

  /// The wire name (`10m`), matching `engine.PresetFor`. Sent as
  /// `duration_preset` by deep links; the interactive builder always sends
  /// resolved seconds, which the server accepts from any caller.
  final String name;

  /// Human wording, verbatim from the engine so the app and the API docs
  /// cannot disagree about what a length is called.
  final String label;

  final int seconds;

  /// `45` for a forty-five minute rung — how the chips label themselves.
  String get minutesLabel => '${seconds ~/ 60}';
}

/// The ladder, ascending, exactly as `engine.presets` declares it — custom
/// excluded, because custom is an input, not a rung.
const builderPresets = <BuilderPreset>[
  BuilderPreset(name: '10m', label: '10 minutes', seconds: 10 * 60),
  BuilderPreset(name: '15m', label: '15 minutes', seconds: 15 * 60),
  BuilderPreset(name: '30m', label: '30 minutes', seconds: 30 * 60),
  BuilderPreset(name: '45m', label: '45 minutes', seconds: 45 * 60),
  BuilderPreset(name: '60m', label: '60 minutes', seconds: 60 * 60),
  BuilderPreset(name: '90m', label: '90 minutes', seconds: 90 * 60),
  BuilderPreset(name: '120m', label: '2 hours', seconds: 120 * 60),
  BuilderPreset(name: '180m', label: '3 hours', seconds: 180 * 60),
];

/// The shortest session the server accepts (handlers: "between 1 minute and
/// 3 hours"). Custom lengths below this are refused, so the builder refuses
/// them first, with the same rule.
const minSessionSeconds = 60;

/// The longest session the server accepts — and the free plan's ceiling is
/// lower; the server answers 402 and the builder surfaces that honestly
/// rather than pretending to know every plan's cap locally.
const maxSessionSeconds = 3 * 3600;

/// One packing strategy, worded for a listener rather than an engineer.
final class BuilderStrategy {
  const BuilderStrategy({required this.name, required this.title, required this.subtitle});

  /// The wire name, canonical per `engine.NormalizeStrategy`.
  final String name;
  final String title;
  final String subtitle;
}

/// The strategies the engine accepts, default first, matching
/// `engine.Strategies()`. The wording paraphrases the engine's own doc
/// comments; the invariant each sentence describes is the engine's, not the
/// UI's.
const builderStrategies = <BuilderStrategy>[
  BuilderStrategy(
    name: 'BALANCED',
    title: 'Balanced',
    subtitle:
        'The closest fit, never more than one confession over. The default.',
  ),
  BuilderStrategy(
    name: 'EXACT',
    title: 'Exact',
    subtitle:
        'Land precisely on the length or refuse. For rituals where the length is the point.',
  ),
  BuilderStrategy(
    name: 'CLOSEST',
    title: 'Closest',
    subtitle: 'Minimise the difference, in either direction.',
  ),
  BuilderStrategy(
    name: 'UNDER',
    title: 'Under',
    subtitle:
        'Never run long. For a commute or a hard stop.',
  ),
  BuilderStrategy(
    name: 'OVER',
    title: 'Over',
    subtitle:
        'Cover the full length, even if it runs a little past.',
  ),
];

/// The default strategy, matching `engine.DefaultStrategy`.
const defaultStrategy = 'BALANCED';

/// `1800` → `30 min`; `5400` → `1 h 30 min`. Used by the duration chips'
/// custom field and the review summary.
String formatSeconds(int seconds) {
  final h = seconds ~/ 3600;
  final m = (seconds % 3600) ~/ 60;
  if (h > 0 && m > 0) return '$h h $m min';
  if (h > 0) return '$h h';
  return '$m min';
}

/// Whether a custom minute value would be accepted by the server. The custom
/// field collects whole minutes; anything from 1 to 180 inclusive converts to
/// a legal `duration_seconds`.
bool isValidCustomMinutes(int minutes) =>
    minutes >= 1 && minutes * 60 <= maxSessionSeconds;
