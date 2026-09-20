import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:iconfess_api/iconfess_api.dart';

import '../../core/routing/routes.dart';
import '../../core/theme/theme.dart';
import '../../core/theme/tokens.dart';
import '../../core/widgets/screen.dart';
import '../../core/widgets/states.dart';
import 'builder_providers.dart';
import 'session_builder.dart';

/// The builder's second step: how long (§12, confess/duration).
///
/// The ladder is the engine's, not the UI's invention, and the strategy
/// selector defaults to BALANCED because that is what the engine does when a
/// caller does not name one. The live preview calls `POST /sessions/preview`
/// so the listener sees the plan the engine chose — including why a 45 minute
/// request might come back as 42 — before anything is created (§5.3).
class DurationScreen extends ConsumerWidget {
  const DurationScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final selected = ref.watch(builderDurationSecondsProvider);
    final strategy = ref.watch(builderStrategyProvider);
    final previewAsync = ref.watch(sessionPreviewProvider);
    final surfaces = AppSurfaces.of(context);

    return AppScaffold(
      title: 'How long?',
      bottom: FilledButton(
        key: const ValueKey('btn-duration-continue'),
        onPressed: () => context.go(AppRoutes.builderVoice),
        child: const Text('Continue'),
      ),
      body: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(
            'How much time will you give it?',
            style: IConfess.heading.copyWith(color: surfaces.textPrimary),
          ),
          const SizedBox(height: IConfess.space2),
          Text(
            'Complete confessions are never cut short, so the engine fills '
            'your length as closely as it honestly can.',
            style: IConfess.bodySm.copyWith(color: surfaces.textSecondary),
          ),
          const SizedBox(height: IConfess.space6),
          Wrap(
            spacing: IConfess.space2,
            runSpacing: IConfess.space2,
            children: [
              for (final preset in builderPresets)
                ChoiceChip(
                  key: ValueKey('chip-duration-${preset.name}'),
                  label: Text(preset.seconds <= 90 * 60
                      ? '${preset.minutesLabel} min'
                      : preset.label),
                  selected: selected == preset.seconds,
                  onSelected: (_) => ref
                      .read(builderDurationSecondsProvider.notifier)
                      .state = preset.seconds,
                ),
            ],
          ),
          const SizedBox(height: IConfess.space5),
          _CustomLengthField(
            key: const ValueKey('custom-length'),
            selectedSeconds: selected,
            onCommitted: (seconds) => ref
                .read(builderDurationSecondsProvider.notifier)
                .state = seconds,
          ),
          const SizedBox(height: IConfess.space8),
          Text('If the length is not exact', style: IConfess.subheading.copyWith(color: surfaces.textPrimary)),
          const SizedBox(height: IConfess.space3),
          for (final s in builderStrategies)
            RadioListTile<String>(
              key: ValueKey('radio-strategy-${s.name}'),
              contentPadding: EdgeInsets.zero,
              title: Text(s.title, style: IConfess.body.copyWith(color: surfaces.textPrimary)),
              subtitle: Text(s.subtitle,
                  style: IConfess.bodySm.copyWith(color: surfaces.textSecondary)),
              value: s.name,
              groupValue: strategy,
              onChanged: (value) {
                if (value != null) {
                  ref.read(builderStrategyProvider.notifier).state = value;
                }
              },
            ),
          const SizedBox(height: IConfess.space6),
          Text('What you would get', style: IConfess.subheading.copyWith(color: surfaces.textPrimary)),
          const SizedBox(height: IConfess.space3),
          previewAsync.when(
            loading: () => const Padding(
              padding: EdgeInsets.symmetric(vertical: IConfess.space5),
              child: ListSkeleton(rows: 1),
            ),
            error: (error, _) => Text(
              'The preview did not load. You can still continue — the session '
              'is checked again before it is created.',
              style: IConfess.bodySm.copyWith(color: surfaces.textSecondary),
            ),
            data: (result) => switch (result) {
              WriteSuccess<SessionPreview>(:final value) => _PreviewCard(preview: value),
              WriteFailure<SessionPreview>(:final error) => _PreviewFailure(error: error),
            },
          ),
        ],
      ),
    );
  }
}

/// The custom-length field. Whole minutes, 1–180: the same bounds the server
/// enforces, so the builder never submits a request it knows will be refused.
class _CustomLengthField extends StatefulWidget {
  const _CustomLengthField({
    super.key,
    required this.selectedSeconds,
    required this.onCommitted,
  });

  final int selectedSeconds;
  final ValueChanged<int> onCommitted;

  @override
  State<_CustomLengthField> createState() => _CustomLengthFieldState();
}

class _CustomLengthFieldState extends State<_CustomLengthField> {
  final _controller = TextEditingController();
  String? _error;

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  void _commit(String text) {
    final minutes = int.tryParse(text.trim());
    if (minutes == null || !isValidCustomMinutes(minutes)) {
      setState(() => _error = 'Enter a length from 1 to 180 minutes.');
      return;
    }
    setState(() => _error = null);
    widget.onCommitted(minutes * 60);
  }

  @override
  Widget build(BuildContext context) {
    final surfaces = AppSurfaces.of(context);
    final isCustom = !builderPresets.any((p) => p.seconds == widget.selectedSeconds);
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Row(
          children: [
            SizedBox(
              width: 96,
              child: TextField(
                key: const ValueKey('field-custom-minutes'),
                controller: _controller,
                keyboardType: TextInputType.number,
                decoration: InputDecoration(
                  hintText: 'Custom',
                  suffixText: 'min',
                  errorText: _error,
                  isDense: true,
                ),
                onSubmitted: _commit,
                onChanged: (v) {
                  if (_error != null) setState(() => _error = null);
                },
              ),
            ),
            const SizedBox(width: IConfess.space3),
            if (isCustom)
              Text(
                'Using your custom length',
                style: IConfess.bodySm.copyWith(color: surfaces.textSecondary),
              ),
          ],
        ),
      ],
    );
  }
}

class _PreviewCard extends StatelessWidget {
  const _PreviewCard({required this.preview});

  final SessionPreview preview;

  @override
  Widget build(BuildContext context) {
    final surfaces = AppSurfaces.of(context);
    return Container(
      key: const ValueKey('preview-card'),
      width: double.infinity,
      padding: const EdgeInsets.all(IConfess.space4),
      decoration: BoxDecoration(
        color: surfaces.surface,
        borderRadius: BorderRadius.circular(IConfess.radiusMd),
        border: Border.all(color: surfaces.border),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(preview.display, style: IConfess.subheading.copyWith(color: surfaces.textPrimary)),
          const SizedBox(height: IConfess.space2),
          if (preview.actualSeconds != preview.targetSeconds)
            Text(
              'Fills ${formatSeconds(preview.actualSeconds)} of the '
              '${formatSeconds(preview.targetSeconds)} you asked for — '
              'complete confessions are never cut short.',
              style: IConfess.bodySm.copyWith(color: surfaces.textSecondary),
            ),
          if (preview.voiceDowngraded)
            Padding(
              padding: const EdgeInsets.only(top: IConfess.space2),
              child: Text(
                'The voice you chose needs a subscription; a substitute '
                'voice will read this session.',
                style: IConfess.bodySm.copyWith(color: surfaces.textSecondary),
              ),
            ),
          if (preview.itemsPreview.isNotEmpty) ...[
            const SizedBox(height: IConfess.space3),
            for (final item in preview.itemsPreview)
              Padding(
                padding: const EdgeInsets.only(bottom: IConfess.space1),
                child: Row(
                  children: [
                    Expanded(
                      child: Text(
                        item.title.isEmpty ? item.confessionId : item.title,
                        overflow: TextOverflow.ellipsis,
                        style: IConfess.bodySm.copyWith(color: surfaces.textPrimary),
                      ),
                    ),
                    Text(
                      formatSeconds(item.durationSeconds),
                      style: IConfess.bodySm.copyWith(color: surfaces.textSecondary),
                    ),
                  ],
                ),
              ),
            if (preview.totalItems > preview.itemsPreview.length)
              Text(
                'and ${preview.totalItems - preview.itemsPreview.length} more',
                style: IConfess.bodySm.copyWith(color: surfaces.textSecondary),
              ),
          ],
        ],
      ),
    );
  }
}

class _PreviewFailure extends StatelessWidget {
  const _PreviewFailure({required this.error});

  final ApiException error;

  @override
  Widget build(BuildContext context) {
    final surfaces = AppSurfaces.of(context);
    final planCapped = error is ApiError && (error as ApiError).requiresSubscription;
    return Container(
      key: const ValueKey('preview-failure'),
      width: double.infinity,
      padding: const EdgeInsets.all(IConfess.space4),
      decoration: BoxDecoration(
        color: surfaces.surface,
        borderRadius: BorderRadius.circular(IConfess.radiusMd),
        border: Border.all(color: surfaces.border),
      ),
      child: Text(
        planCapped
            ? 'That length is more than your plan allows. Choose a shorter '
                'length, or continue and it will be checked again.'
            : 'The preview did not load. You can still continue — the session '
                'is checked again before it is created.',
        style: IConfess.bodySm.copyWith(color: surfaces.textSecondary),
      ),
    );
  }
}
