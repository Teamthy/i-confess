import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:iconfess_api/iconfess_api.dart';

import '../../core/theme/theme.dart';
import '../../core/theme/tokens.dart';
import '../../core/widgets/screen.dart';
import '../../core/widgets/states.dart';
import 'builder_providers.dart';

/// The builder's third step: which voice (§12, confess/voice).
///
/// "No preference" is a first-class choice — the engine then picks from what
/// the plan and rights allow — and the picker offers only voices that can
/// actually be licensed. A voice the rights gate would refuse must never be
/// selectable (§5): selecting it would build a session that fails at
/// generation, and the failure would be the listener's to discover.
class VoiceScreen extends ConsumerWidget {
  const VoiceScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final voicesAsync = ref.watch(builderVoicesProvider);
    final selectedId = ref.watch(builderVoiceIdProvider);
    final surfaces = AppSurfaces.of(context);

    return AppScaffold(
      title: 'Which voice?',
      bottom: FilledButton(
        key: const ValueKey('btn-voice-continue'),
        onPressed: () => context.go(AppRoutes.builderCreate),
        child: const Text('Continue'),
      ),
      body: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(
            'Who will speak it with you?',
            style: IConfess.heading.copyWith(color: surfaces.textPrimary),
          ),
          const SizedBox(height: IConfess.space2),
          Text(
            'Every voice is a real person, licensed to read the Scriptures '
            'over your life.',
            style: IConfess.bodySm.copyWith(color: surfaces.textSecondary),
          ),
          const SizedBox(height: IConfess.space6),
          voicesAsync.when(
            loading: () => const ListSkeleton(rows: 4),
            error: (error, _) => ErrorState(
              error: const UserFacingError(
                title: 'Voices did not load',
                message: 'Check your connection and try again.',
                primaryAction: ErrorAction.retry,
                retryable: true,
              ),
              onAction: (_) => ref.invalidate(builderVoicesProvider),
            ),
            data: (loadable) {
              if (loadable is LoadFailed<List<Voice>>) {
                return ErrorState(
                  error: const UserFacingError(
                    title: 'Voices did not load',
                    message: 'Check your connection and try again.',
                    primaryAction: ErrorAction.retry,
                    retryable: true,
                  ),
                  onAction: (_) => ref.invalidate(builderVoicesProvider),
                );
              }
              final voices = ref.watch(selectableVoicesProvider);
              if (voices.isEmpty) {
                return const EmptyState(
                  title: 'No voices yet',
                  message: 'The voice catalogue is being prepared.',
                );
              }
              return Column(
                children: [
                  _VoiceRow(
                    key: const ValueKey('voice-none'),
                    name: 'No preference',
                    description: 'We will choose a voice your plan allows.',
                    premium: false,
                    selected: selectedId == null,
                    onTap: () =>
                        ref.read(builderVoiceIdProvider.notifier).state = null,
                  ),
                  for (final v in voices)
                    _VoiceRow(
                      key: ValueKey('voice-${v.id}'),
                      name: v.name,
                      description: v.description,
                      premium: v.premium,
                      selected: selectedId == v.id,
                      onTap: () =>
                          ref.read(builderVoiceIdProvider.notifier).state = v.id,
                    ),
                ],
              );
            },
          ),
        ],
      ),
    );
  }
}

class _VoiceRow extends StatelessWidget {
  const _VoiceRow({
    super.key,
    required this.name,
    required this.description,
    required this.premium,
    required this.selected,
    required this.onTap,
  });

  final String name;
  final String description;
  final bool premium;
  final bool selected;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final surfaces = AppSurfaces.of(context);
    return Card(
      margin: const EdgeInsets.only(bottom: IConfess.space2),
      color: surfaces.surface,
      elevation: 0,
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(IConfess.radiusMd),
        side: BorderSide(
          color: selected ? surfaces.primary : surfaces.border,
          width: selected ? 2 : 1,
        ),
      ),
      child: ListTile(
        onTap: onTap,
        title: Row(
          children: [
            Expanded(
              child: Text(name,
                  style: IConfess.body.copyWith(color: surfaces.textPrimary)),
            ),
            if (premium)
              Container(
                padding: const EdgeInsets.symmetric(
                  horizontal: IConfess.space2,
                  vertical: 2,
                ),
                decoration: BoxDecoration(
                  color: surfaces.surface,
                  borderRadius: BorderRadius.circular(IConfess.radiusFull),
                  border: Border.all(color: IConfess.colorAccentGold),
                ),
                child: Text(
                  'Premium',
                  style: IConfess.bodySm
                      .copyWith(color: IConfess.colorAccentGold, fontSize: 11),
                ),
              ),
          ],
        ),
        subtitle: description.isEmpty
            ? null
            : Text(description,
                style: IConfess.bodySm.copyWith(color: surfaces.textSecondary)),
        trailing: selected
            ? Icon(Icons.check_circle_rounded, color: surfaces.primary)
            : null,
      ),
    );
  }
}
