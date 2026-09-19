import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:iconfess_api/iconfess_api.dart';

import '../../core/di/providers.dart';
import 'session_builder.dart';

/// The builder's steps after category selection: how long, which voice,
/// and the review that creates the session (§12 — the core loop
/// `confess → duration → voice → create → player`).
///
/// The selections are held client-side until the review screen commits, for
/// the same reason PHASE 22 held the category set locally: writing a draft
/// session server-side before the listener has confirmed would create
/// orphans every time someone changes their mind.

/// How long the next session should be, in seconds. The first rung of the
/// ladder is the default, so Continue is always meaningful from the moment
/// the listener has picked a category.
final builderDurationSecondsProvider = StateProvider<int>((ref) => builderPresets.first.seconds);

/// How the engine packs the requested length, canonical per the engine's
/// `NormalizeStrategy`. Defaults to BALANCED, matching `DefaultStrategy`.
final builderStrategyProvider = StateProvider<String>((ref) => defaultStrategy);

/// The requested voice, or null for "no preference" — the engine then picks
/// from what the plan and rights allow. Null is a real choice, not an
/// omission: the summary says "No preference" rather than leaving a gap.
final builderVoiceIdProvider = StateProvider<String?>((ref) => null);

/// The voice catalogue. Cached by the repository like categories, so a voice
/// an admin adds appears on the next cold read rather than mid-session.
final builderVoicesProvider =
    FutureProvider<Loadable<List<Voice>>>((ref) {
  return ref.watch(contentRepositoryProvider).voices();
});

/// The voices the picker may offer: `status == 'active'` only.
///
/// `GET /voices` returns every row in the catalogue, retired ones included.
/// A voice that cannot be licensed must not appear as selectable (§5): the
/// rights gate would refuse it at generation time, and a session built on it
/// could not render. The filter lives in one selector so the picker, the
/// review summary and any future voice surface cannot disagree.
final selectableVoicesProvider = Provider<List<Voice>>((ref) {
  final loadable = ref.watch(builderVoicesProvider).asData?.value;
  final voices = loadable?.valueOrNull ?? const <Voice>[];
  return voices.where((v) => v.isSelectable).toList(growable: false);
});

/// The voice the listener chose, resolved to a name. Null means "no
/// preference"; the summary words that as a choice rather than a blank.
final selectedVoiceProvider = Provider<String?>((ref) {
  final id = ref.watch(builderVoiceIdProvider);
  if (id == null || id.isEmpty) return null;
  for (final v in ref.watch(selectableVoicesProvider)) {
    if (v.id == id) return v.name;
  }
  return id;
});

/// What the engine would build for the current selection (§5.3).
///
/// A dry-run, not a draft: the server persists nothing, and the repository
/// refuses to cache the answer. It returns the client's [WriteResult] rather
/// than [Loadable] because the interesting failures are rejections — a plan
/// cap (402) or an impossible length (422) — and the review screen words
/// those differently from an outage.
///
/// The provider is not auto-disposed across steps: walking back to change the
/// duration and returning re-previews, which is the point.
final sessionPreviewProvider =
    FutureProvider<WriteResult<SessionPreview>>((ref) {
  final categories = ref.watch(selectedCategoriesProvider);
  final duration = ref.watch(builderDurationSecondsProvider);
  final strategy = ref.watch(builderStrategyProvider);
  final voiceId = ref.watch(builderVoiceIdProvider);
  if (categories.isEmpty) {
    // No categories, no plan. The preview is simply withheld rather than
    // answered with a server round-trip that could only say "at least one
    // category is required".
    return Future.value(
      const WriteFailure<SessionPreview>(
        ApiError(status: 0, code: 'no_categories', message: 'no categories selected'),
      ),
    );
  }
  return ref.watch(contentRepositoryProvider).previewSession(
        categoryIds: categories.toList(growable: false),
        durationSeconds: duration,
        voiceId: voiceId,
        strategy: strategy,
      );
});

/// The session the review screen created, if one exists. Kept in provider
/// state rather than screen state so that saving the shape as a template —
/// which reads the same categories — and the success surface stay consistent
/// if the widget tree rebuilds.
final createdSessionProvider = StateProvider<ListeningSession?>((ref) => null);

/// The template the review screen saved, if it did. One template per created
/// session; a second save is a second template, which the UI prevents.
final savedTemplateProvider = StateProvider<SessionTemplate?>((ref) => null);

/// Clears the whole builder: categories, length, strategy, voice and any
/// created result. Called after a session is acknowledged, so the next visit
/// to the confess tab starts from "what do you want to speak over your life
/// today?" rather than last week's choices.
void resetBuilder(WidgetRef ref) {
  ref.read(selectedCategoriesProvider.notifier).state = <String>{};
  ref.read(builderDurationSecondsProvider.notifier).state =
      builderPresets.first.seconds;
  ref.read(builderStrategyProvider.notifier).state = defaultStrategy;
  ref.read(builderVoiceIdProvider.notifier).state = null;
  ref.read(createdSessionProvider.notifier).state = null;
  ref.read(savedTemplateProvider.notifier).state = null;
  // The preview answers for the old selection; keeping it would let the
  // duration step show a plan for categories that are no longer chosen.
  ref.invalidate(sessionPreviewProvider);
}
