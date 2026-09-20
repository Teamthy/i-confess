import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:iconfess_api/iconfess_api.dart';

import '../../core/analytics/analytics.dart';
import '../../core/di/providers.dart';
import '../../core/routing/routes.dart';
import '../../core/theme/theme.dart';
import '../../core/theme/tokens.dart';
import '../../core/widgets/screen.dart';
import 'builder_providers.dart';
import 'session_builder.dart';
import 'confess_providers.dart';

/// The builder's last step: review, and the only place a session is created
/// (§12, confess/create).
///
/// The summary restates every choice in the listener's own words, the preview
/// shows what the engine would build, and Create is the commit. After it
/// succeeds the shape is offered as a template — here, rather than buried in
/// the player (§5.4) — because this is the moment the listener knows they
/// will want this exact session again.
///
/// There is deliberately no play button here. Playback is the player's
/// surface (PHASE 24); a button that cannot play would be a lie with an icon.
class ReviewScreen extends ConsumerStatefulWidget {
  const ReviewScreen({super.key});

  @override
  ConsumerState<ReviewScreen> createState() => _ReviewScreenState();
}

class _ReviewScreenState extends ConsumerState<ReviewScreen> {
  bool _creating = false;
  bool _savingTemplate = false;
  WriteResult<ListeningSession>? _createResult;
  WriteResult<SessionTemplate>? _templateResult;

  @override
  void initState() {
    super.initState();
    // Arriving here starts a new attempt; a previous success must not read
    // as this one.
    ref.read(createdSessionProvider.notifier).state = null;
  }

  Future<void> _create() async {
    final categories = ref.read(selectedCategoriesProvider);
    if (categories.isEmpty || _creating) return;
    setState(() {
      _creating = true;
      _createResult = null;
    });
    final result = await ref.read(contentRepositoryProvider).createSession(
          categoryIds: categories.toList(growable: false),
          durationSeconds: ref.read(builderDurationSecondsProvider),
          voiceId: ref.read(builderVoiceIdProvider),
        );
    if (!mounted) return;
    setState(() {
      _creating = false;
      _createResult = result;
    });
    if (result is WriteSuccess<ListeningSession>) {
      ref.read(createdSessionProvider.notifier).state = result.value;
      // The product's central-loop event. Properties stay structural:
      // lengths and counts, never category names or session content.
      ref.read(analyticsProvider).track(
            AnalyticsEvents.sessionCreated,
            properties: {
              'item_count': result.value.items.length,
              'duration_seconds': result.value.durationSeconds,
              'has_voice': result.value.voiceId.isNotEmpty,
            },
          );
    }
  }

  Future<void> _saveTemplate() async {
    final categories = ref.read(selectedCategoriesProvider);
    if (categories.isEmpty || _savingTemplate) return;
    final name = _templateNameController.text.trim();
    if (name.isEmpty) {
      setState(() => _templateError = 'Give this shape a name.');
      return;
    }
    setState(() {
      _savingTemplate = true;
      _templateError = null;
      _templateResult = null;
    });
    final result = await ref.read(contentRepositoryProvider).createTemplate(
          name: name,
          categoryIds: categories.toList(growable: false),
          voiceId: ref.read(builderVoiceIdProvider),
        );
    if (!mounted) return;
    setState(() {
      _savingTemplate = false;
      _templateResult = result;
    });
    if (result is WriteSuccess<SessionTemplate>) {
      ref.read(savedTemplateProvider.notifier).state = result.value;
      ref.read(analyticsProvider).track(AnalyticsEvents.templateCreated);
    }
  }

  final TextEditingController _templateNameController = TextEditingController();
  String? _templateError;

  @override
  void dispose() {
    _templateNameController.dispose();
    super.dispose();
  }

  void _done() {
    resetBuilder(ref);
    // Home, not pop: the builder's steps behind this screen are consumed,
    // and back should not walk the listener through them again.
    context.go(AppRoutes.home);
  }

  @override
  Widget build(BuildContext context) {
    final surfaces = AppSurfaces.of(context);
    final created = ref.watch(createdSessionProvider);
    final categoryIds = ref.watch(selectedCategoriesProvider);
    final categoryNames = ref.watch(confessCategoriesProvider).asData?.value.valueOrNull ??
        const <Category>[];
    final duration = ref.watch(builderDurationSecondsProvider);
    final strategyName = ref.watch(builderStrategyProvider);
    final strategy = builderStrategies.firstWhere(
      (s) => s.name == strategyName,
      orElse: () => builderStrategies.first,
    );
    final voiceName = ref.watch(selectedVoiceProvider);

    return AppScaffold(
      title: 'Review',
      body: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(
            'Here is what will be spoken.',
            style: IConfess.heading.copyWith(color: surfaces.textPrimary),
          ),
          const SizedBox(height: IConfess.space6),
          _SummaryRow(
            label: 'Over',
            value: categoryNames
                .where((c) => categoryIds.contains(c.id))
                .map((c) => c.name)
                .join(', '),
          ),
          _SummaryRow(label: 'Length', value: formatSeconds(duration)),
          _SummaryRow(label: 'Fitting', value: strategy.title),
          _SummaryRow(label: 'Voice', value: voiceName ?? 'No preference'),
          const SizedBox(height: IConfess.space7),

          if (created != null) ...[
            _CreatedSurface(session: created),
            const SizedBox(height: IConfess.space5),
            _TemplateOffer(
              controller: _templateNameController,
              error: _templateError,
              saving: _savingTemplate,
              result: _templateResult,
              onSave: _saveTemplate,
            ),
            const SizedBox(height: IConfess.space6),
            FilledButton(
              key: const ValueKey('btn-done'),
              onPressed: _done,
              child: const Text('Done'),
            ),
          ] else ...[
            if (_createResult is WriteFailure<ListeningSession>)
              _CreateFailure(
                error: (_createResult as WriteFailure<ListeningSession>).error,
                onAdjustLength: () => context.go(AppRoutes.builderDuration),
                onRetry: _create,
              ),
            FilledButton(
              key: const ValueKey('btn-create'),
              onPressed: categoryIds.isEmpty || _creating ? null : _create,
              child: _creating
                  ? const SizedBox(
                      height: 18,
                      width: 18,
                      child: CircularProgressIndicator(strokeWidth: 2),
                    )
                  : const Text('Create this session'),
            ),
          ],
        ],
      ),
    );
  }
}

class _SummaryRow extends StatelessWidget {
  const _SummaryRow({required this.label, required this.value});

  final String label;
  final String value;

  @override
  Widget build(BuildContext context) {
    final surfaces = AppSurfaces.of(context);
    return Padding(
      padding: const EdgeInsets.only(bottom: IConfess.space3),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          SizedBox(
            width: 88,
            child: Text(label,
                style: IConfess.bodySm.copyWith(color: surfaces.textSecondary)),
          ),
          Expanded(
            child: Text(
              value.isEmpty ? '—' : value,
              style: IConfess.body.copyWith(color: surfaces.textPrimary),
            ),
          ),
        ],
      ),
    );
  }
}

/// The created session, honestly: its composition, any locked items and the
/// reason there is no play button yet.
class _CreatedSurface extends StatelessWidget {
  const _CreatedSurface({required this.session});

  final ListeningSession session;

  @override
  Widget build(BuildContext context) {
    final surfaces = AppSurfaces.of(context);
    return Container(
      key: const ValueKey('created-session'),
      width: double.infinity,
      padding: const EdgeInsets.all(IConfess.space4),
      decoration: BoxDecoration(
        color: surfaces.surface,
        borderRadius: BorderRadius.circular(IConfess.radiusMd),
        border: Border.all(color: surfaces.success),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              Icon(Icons.check_circle_rounded, color: surfaces.success, size: 20),
              const SizedBox(width: IConfess.space2),
              Expanded(
                child: Text(
                  'Session created — ${formatSeconds(session.durationSeconds)}, '
                  '${session.items.length} '
                  '${session.items.length == 1 ? 'confession' : 'confessions'}',
                  style: IConfess.subheading.copyWith(color: surfaces.textPrimary),
                ),
              ),
            ],
          ),
          const SizedBox(height: IConfess.space3),
          for (final item in session.items)
            Padding(
              padding: const EdgeInsets.only(bottom: IConfess.space1),
              child: Row(
                children: [
                  Expanded(
                    child: Text(
                      item.title.isEmpty ? item.confessionId : item.title,
                      overflow: TextOverflow.ellipsis,
                      style: IConfess.bodySm.copyWith(
                        color: item.locked
                            ? surfaces.textSecondary
                            : surfaces.textPrimary,
                      ),
                    ),
                  ),
                  if (item.locked)
                    Icon(Icons.lock_outline_rounded,
                        size: 14, color: surfaces.textSecondary)
                  else
                    Text(
                      formatSeconds(item.durationSeconds),
                      style:
                          IConfess.bodySm.copyWith(color: surfaces.textSecondary),
                    ),
                ],
              ),
            ),
          if (session.hasLockedItems)
            Padding(
              padding: const EdgeInsets.only(top: IConfess.space2),
              child: Text(
                'Locked items are included so you can see the full plan; they '
                'play with a subscription.',
                style: IConfess.bodySm.copyWith(color: surfaces.textSecondary),
              ),
            ),
          const SizedBox(height: IConfess.space3),
          Text(
            'Playback opens in the player, arriving with the next phase.',
            style: IConfess.bodySm.copyWith(color: surfaces.textSecondary),
          ),
        ],
      ),
    );
  }
}

class _TemplateOffer extends StatelessWidget {
  const _TemplateOffer({
    required this.controller,
    required this.error,
    required this.saving,
    required this.result,
    required this.onSave,
  });

  final TextEditingController controller;
  final String? error;
  final bool saving;
  final WriteResult<SessionTemplate>? result;
  final VoidCallback onSave;

  @override
  Widget build(BuildContext context) {
    final surfaces = AppSurfaces.of(context);
    final saved = result is WriteSuccess<SessionTemplate>
        ? (result as WriteSuccess<SessionTemplate>).value
        : null;
    return Container(
      key: const ValueKey('template-offer'),
      width: double.infinity,
      padding: const EdgeInsets.all(IConfess.space4),
      decoration: BoxDecoration(
        color: surfaces.surface,
        borderRadius: BorderRadius.circular(IConfess.radiusMd),
        border: Border.all(color: surfaces.border),
      ),
      child: saved != null
          ? Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text('Saved as "${saved.name}"',
                    style:
                        IConfess.body.copyWith(color: surfaces.textPrimary)),
                const SizedBox(height: IConfess.space2),
                Text(
                  'Start it any time from My ritual. Share it with '
                  '${saved.shareUrl.isEmpty ? 'its link' : 'this link'}: '
                  '${saved.deeplink.isEmpty ? saved.shareUrl : saved.deeplink}',
                  style: IConfess.bodySm.copyWith(color: surfaces.textSecondary),
                ),
              ],
            )
          : Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text('Keep this shape?',
                    style: IConfess.subheading.copyWith(color: surfaces.textPrimary)),
                const SizedBox(height: IConfess.space2),
                Text(
                  'Save it as a ritual template and the same categories and '
                  'voice can be rebuilt any day, against whatever is new in '
                  'the library.',
                  style: IConfess.bodySm.copyWith(color: surfaces.textSecondary),
                ),
                const SizedBox(height: IConfess.space3),
                Row(
                  children: [
                    Expanded(
                      child: TextField(
                        key: const ValueKey('field-template-name'),
                        controller: controller,
                        decoration: InputDecoration(
                          hintText: 'Name it — "Morning on fear"',
                          errorText: error,
                          isDense: true,
                        ),
                        onSubmitted: (_) => onSave(),
                      ),
                    ),
                    const SizedBox(width: IConfess.space3),
                    OutlinedButton(
                      key: const ValueKey('btn-save-template'),
                      onPressed: saving ? null : onSave,
                      child: saving
                          ? const SizedBox(
                              height: 16,
                              width: 16,
                              child: CircularProgressIndicator(strokeWidth: 2),
                            )
                          : const Text('Save'),
                    ),
                  ],
                ),
                if (result is WriteFailure<SessionTemplate>)
                  Padding(
                    padding: const EdgeInsets.only(top: IConfess.space2),
                    child: Text(
                      'It did not save. You can try again — the session '
                      'itself is safe.',
                      style: IConfess.bodySm.copyWith(color: surfaces.warning),
                    ),
                  ),
              ],
            ),
    );
  }
}

class _CreateFailure extends StatelessWidget {
  const _CreateFailure({required this.error, required this.onAdjustLength, required this.onRetry});

  final ApiException error;
  final VoidCallback onAdjustLength;
  final VoidCallback onRetry;

  @override
  Widget build(BuildContext context) {
    final surfaces = AppSurfaces.of(context);
    final api = error is ApiError ? error as ApiError : null;
    final planCapped = api?.requiresSubscription ?? false;
    return Container(
      key: const ValueKey('create-failure'),
      width: double.infinity,
      padding: const EdgeInsets.all(IConfess.space4),
      margin: const EdgeInsets.only(bottom: IConfess.space4),
      decoration: BoxDecoration(
        color: surfaces.surface,
        borderRadius: BorderRadius.circular(IConfess.radiusMd),
        border: Border.all(color: surfaces.warning),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(
            planCapped
                ? 'That length is more than your plan allows.'
                : (api == null || api.isServerFault || api.isRateLimited)
                    ? 'The session did not save. Check your connection and try again.'
                    : 'The session did not save.',
            style: IConfess.body.copyWith(color: surfaces.textPrimary),
          ),
          const SizedBox(height: IConfess.space3),
          if (planCapped)
            OutlinedButton(
              key: const ValueKey('btn-adjust-length'),
              onPressed: onAdjustLength,
              child: const Text('Choose a shorter length'),
            )
          else
            OutlinedButton(
              key: const ValueKey('btn-retry-create'),
              onPressed: onRetry,
              child: const Text('Try again'),
            ),
        ],
      ),
    );
  }
}
