import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../core/routing/routes.dart';
import '../../core/theme/theme.dart';
import '../../core/theme/tokens.dart';
import '../../core/widgets/screen.dart';
import 'templates_providers.dart';

/// Templates list: saved shapes.
class TemplatesScreen extends ConsumerWidget {
  const TemplatesScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final async = ref.watch(templatesProvider);
    final surfaces = AppSurfaces.of(context);

    return AppScaffold(
      title: 'Templates',
      body: async.when(
        loading: () => const Center(child: CircularProgressIndicator()),
        error: (e, _) => Center(child: Text('Failed: $e')),
        data: (loadable) {
          final templates = loadable.valueOrNull ?? [];
          if (templates.isEmpty) {
            return Center(
              child: Column(
                mainAxisAlignment: MainAxisAlignment.center,
                children: [
                  Icon(Icons.bookmark_border_rounded, size: 48, color: surfaces.textSecondary),
                  const SizedBox(height: IConfess.space3),
                  Text('No templates yet',
                      style: IConfess.body.copyWith(color: surfaces.textPrimary)),
                  const SizedBox(height: IConfess.space2),
                  Text('Save your builder choices as a template',
                      style: IConfess.bodySm.copyWith(color: surfaces.textSecondary)),
                  const SizedBox(height: IConfess.space4),
                  FilledButton(
                    onPressed: () => context.go(AppRoutes.confess),
                    child: const Text('Build a session'),
                  ),
                ],
              ),
            );
          }
          return ListView.separated(
            itemCount: templates.length,
            separatorBuilder: (_, _) => const SizedBox(height: IConfess.space2),
            itemBuilder: (context, i) {
              final t = templates[i];
              return Material(
                color: surfaces.surfaceRaised,
                borderRadius: BorderRadius.circular(IConfess.radiusMd),
                child: InkWell(
                  borderRadius: BorderRadius.circular(IConfess.radiusMd),
                  onTap: () => context.go('/templates/${t.id}'),
                  child: Padding(
                    padding: const EdgeInsets.all(IConfess.space4),
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        Row(
                          children: [
                            Expanded(
                              child: Text(t.name.isNotEmpty ? t.name : 'Untitled template',
                                  style: IConfess.subheading.copyWith(color: surfaces.textPrimary)),
                            ),
                            if (t.isPublic) const Icon(Icons.public_rounded, size: 16),
                          ],
                        ),
                        if (t.description.isNotEmpty) ...[
                          const SizedBox(height: IConfess.space1),
                          Text(t.description,
                              maxLines: 2,
                              overflow: TextOverflow.ellipsis,
                              style: IConfess.bodySm.copyWith(color: surfaces.textSecondary)),
                        ],
                        const SizedBox(height: IConfess.space2),
                        Text('${t.categoryIds.length} categories • ${t.voiceId.isNotEmpty ? 'with voice' : 'no voice'}',
                            style: IConfess.caption.copyWith(color: surfaces.textSecondary)),
                      ],
                    ),
                  ),
                ),
              );
            },
          );
        },
      ),
    );
  }
}

class TemplateDetailScreen extends ConsumerWidget {
  const TemplateDetailScreen({required this.templateId, super.key});
  final String templateId;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final async = ref.watch(templateDetailProvider(templateId));
    final surfaces = AppSurfaces.of(context);

    return AppScaffold(
      title: 'Template',
      body: async.when(
        loading: () => const Center(child: CircularProgressIndicator()),
        error: (e, _) => Center(child: Text('Failed: $e')),
        data: (loadable) {
          final t = loadable.valueOrNull;
          if (t == null) return const Center(child: Text('Not found'));
          return Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(t.name, style: IConfess.heading.copyWith(color: surfaces.textPrimary)),
              const SizedBox(height: IConfess.space2),
              Text(t.description, style: IConfess.body.copyWith(color: surfaces.textSecondary)),
              const SizedBox(height: IConfess.space4),
              Wrap(
                spacing: IConfess.space2,
                children: [
                  for (final catId in t.categoryIds) Chip(label: Text(catId)),
                ],
              ),
              const SizedBox(height: IConfess.space5),
              FilledButton.icon(
                onPressed: () async {
                  final result = await ref.read(templateRepositoryProvider).startTemplate(t.id);
                  result.when(
                    success: (session) => context.go(AppRoutes.player),
                    failure: (e) => ScaffoldMessenger.of(context).showSnackBar(
                      SnackBar(content: Text('Failed to start: $e')),
                    ),
                  );
                },
                icon: const Icon(Icons.play_arrow_rounded),
                label: const Text('Start this template'),
              ),
              const SizedBox(height: IConfess.space3),
              if (t.shareUrl.isNotEmpty)
                OutlinedButton.icon(
                  onPressed: () {},
                  icon: const Icon(Icons.share_rounded),
                  label: Text('Share: ${t.shareUrl}'),
                ),
            ],
          );
        },
      ),
    );
  }
}

class TemplateShareScreen extends ConsumerWidget {
  const TemplateShareScreen({required this.token, super.key});
  final String token;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final async = ref.watch(sharedTemplateProvider(token));
    final surfaces = AppSurfaces.of(context);

    return AppScaffold(
      title: 'Shared template',
      body: async.when(
        loading: () => const Center(child: CircularProgressIndicator()),
        error: (e, _) => Center(
          child: Column(
            mainAxisAlignment: MainAxisAlignment.center,
            children: [
              Text('Could not load shared template',
                  style: IConfess.subheading.copyWith(color: surfaces.textPrimary)),
              const SizedBox(height: IConfess.space2),
              Text(e.toString(), style: IConfess.bodySm.copyWith(color: surfaces.textSecondary)),
            ],
          ),
        ),
        data: (loadable) {
          final t = loadable.valueOrNull;
          if (t == null) return const Center(child: Text('Not found or private'));
          return Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text('A ritual shared with you',
                  style: IConfess.caption.copyWith(color: surfaces.textSecondary)),
              const SizedBox(height: IConfess.space2),
              Text(t.name, style: IConfess.heading.copyWith(color: surfaces.textPrimary)),
              const SizedBox(height: IConfess.space3),
              Text(t.description, style: IConfess.body.copyWith(color: surfaces.textPrimary)),
              const SizedBox(height: IConfess.space5),
              FilledButton(
                onPressed: () => context.go(AppRoutes.welcome),
                child: const Text('Try I CONFESS'),
              ),
            ],
          );
        },
      ),
    );
  }
}
