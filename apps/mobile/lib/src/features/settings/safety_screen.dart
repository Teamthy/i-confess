import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:iconfess_api/iconfess_api.dart';

import '../../core/error/error_mapper.dart';
import '../../core/theme/tokens.dart';
import '../../core/widgets/screen.dart';
import '../moderation/moderation_providers.dart';

/// Listener-owned block boundaries and appeals. A block changes only what the
/// listener is served; it is not a moderation penalty against the other person.
class SafetyScreen extends ConsumerWidget {
  const SafetyScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) => DefaultTabController(
        length: 2,
        child: AppScaffold(
          title: 'Safety',
          scrollable: false,
          body: Column(
            children: [
              const TabBar(tabs: [Tab(text: 'Blocked accounts'), Tab(text: 'Appeals')]),
              Expanded(
                child: TabBarView(
                  children: [
                    _BlocksTab(ref: ref),
                    _AppealsTab(ref: ref),
                  ],
                ),
              ),
            ],
          ),
        ),
      );
}

class _BlocksTab extends StatelessWidget {
  const _BlocksTab({required this.ref});
  final WidgetRef ref;

  @override
  Widget build(BuildContext context) => RefreshIndicator(
        onRefresh: () async { await ref.refresh(moderationBlocksProvider.future); },
        child: ref.watch(moderationBlocksProvider).when(
              loading: () => const Center(child: CircularProgressIndicator()),
              error: (error, _) => _LoadError(message: '$error', onRetry: () => ref.invalidate(moderationBlocksProvider)),
              data: (result) => switch (result) {
                LoadLoaded<List<UserBlock>>(:final value) when value.isNotEmpty => ListView(
                    padding: const EdgeInsets.all(IConfess.space4),
                    children: [
                      Align(
                        alignment: Alignment.centerRight,
                        child: FilledButton.tonalIcon(
                          onPressed: () => _addBlock(context, ref),
                          icon: const Icon(Icons.block_rounded),
                          label: const Text('Block account'),
                        ),
                      ),
                      const SizedBox(height: IConfess.space3),
                      for (final block in value)
                        Card(
                          child: ListTile(
                            leading: const Icon(Icons.person_off_outlined),
                            title: const Text('Blocked account'),
                            subtitle: SelectableText(block.blockedId),
                            trailing: TextButton(
                              onPressed: () => _unblock(context, ref, block.blockedId),
                              child: const Text('Unblock'),
                            ),
                          ),
                        ),
                    ],
                  ),
                LoadLoaded<List<UserBlock>>() => ListView(
                    padding: const EdgeInsets.all(IConfess.space5),
                    children: [
                      const SizedBox(height: IConfess.space5),
                      const Icon(Icons.shield_outlined, size: 48),
                      const SizedBox(height: IConfess.space3),
                      const Text('No blocked accounts', textAlign: TextAlign.center),
                      const SizedBox(height: IConfess.space4),
                      Center(child: FilledButton.tonalIcon(
                        onPressed: () => _addBlock(context, ref),
                        icon: const Icon(Icons.block_rounded),
                        label: const Text('Block account'),
                      )),
                    ],
                  ),
                LoadFailed<List<UserBlock>>(:final error) => _LoadError(message: error.toString(), onRetry: () => ref.invalidate(moderationBlocksProvider)),
                _ => const Center(child: CircularProgressIndicator()),
              },
            ),
      );

  Future<void> _addBlock(BuildContext context, WidgetRef ref) async {
    final id = await _textDialog(
      context,
      title: 'Block an account',
      label: 'Account ID',
      helper: 'Blocking is private to your account. Use a community story menu to block its anonymous author without seeing their ID.',
      submitLabel: 'Block',
    );
    if (id == null || id.isEmpty || !context.mounted) return;
    final result = await ref.read(moderationRepositoryProvider).block(id);
    if (!context.mounted) return;
    result.when(
      success: (_) {
        ref.invalidate(moderationBlocksProvider);
        _message(context, 'Account blocked');
      },
      failure: (error) => _message(context, ErrorMapper.describe(error).message),
    );
  }

  Future<void> _unblock(BuildContext context, WidgetRef ref, String userId) async {
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (context) => AlertDialog(
        title: const Text('Unblock this account?'),
        content: const Text('Their content may appear to you again.'),
        actions: [
          TextButton(onPressed: () => Navigator.pop(context, false), child: const Text('Cancel')),
          FilledButton(onPressed: () => Navigator.pop(context, true), child: const Text('Unblock')),
        ],
      ),
    );
    if (confirmed != true || !context.mounted) return;
    final result = await ref.read(moderationRepositoryProvider).unblock(userId);
    if (!context.mounted) return;
    result.when(
      success: (_) {
        ref.invalidate(moderationBlocksProvider);
        _message(context, 'Account unblocked');
      },
      failure: (error) => _message(context, ErrorMapper.describe(error).message),
    );
  }
}

class _AppealsTab extends StatelessWidget {
  const _AppealsTab({required this.ref});
  final WidgetRef ref;

  @override
  Widget build(BuildContext context) => RefreshIndicator(
        onRefresh: () async { await ref.refresh(moderationAppealsProvider.future); }
        child: ref.watch(moderationAppealsProvider).when(
              loading: () => const Center(child: CircularProgressIndicator()),
              error: (error, _) => _LoadError(message: '$error', onRetry: () => ref.invalidate(moderationAppealsProvider)),
              data: (result) => switch (result) {
                LoadLoaded<List<ModerationAppeal>>(:final value) when value.isNotEmpty => ListView(
                    padding: const EdgeInsets.all(IConfess.space4),
                    children: [
                      Align(
                        alignment: Alignment.centerRight,
                        child: FilledButton.tonalIcon(
                          onPressed: () => _fileAppeal(context, ref),
                          icon: const Icon(Icons.gavel_rounded),
                          label: const Text('Appeal a decision'),
                        ),
                      ),
                      const SizedBox(height: IConfess.space3),
                      for (final appeal in value) _AppealCard(appeal: appeal),
                    ],
                  ),
                LoadLoaded<List<ModerationAppeal>>() => ListView(
                    padding: const EdgeInsets.all(IConfess.space5),
                    children: [
                      const SizedBox(height: IConfess.space5),
                      const Icon(Icons.gavel_rounded, size: 48),
                      const SizedBox(height: IConfess.space3),
                      const Text('No appeals yet', textAlign: TextAlign.center),
                      const SizedBox(height: IConfess.space4),
                      Center(child: FilledButton.tonalIcon(
                        onPressed: () => _fileAppeal(context, ref),
                        icon: const Icon(Icons.gavel_rounded),
                        label: const Text('Appeal a decision'),
                      )),
                    ],
                  ),
                LoadFailed<List<ModerationAppeal>>(:final error) => _LoadError(message: error.toString(), onRetry: () => ref.invalidate(moderationAppealsProvider)),
                _ => const Center(child: CircularProgressIndicator()),
              },
            ),
      );

  Future<void> _fileAppeal(BuildContext context, WidgetRef ref) async {
    final input = await _appealDialog(context);
    if (input == null || !context.mounted) return;
    final result = await ref.read(moderationRepositoryProvider).appeal(
          decisionType: input.$1,
          decisionId: input.$2,
          statement: input.$3,
        );
    if (!context.mounted) return;
    result.when(
      success: (_) {
        ref.invalidate(moderationAppealsProvider);
        _message(context, 'Appeal submitted');
      },
      failure: (error) => _message(context, ErrorMapper.describe(error).message),
    );
  }
}

class _AppealCard extends StatelessWidget {
  const _AppealCard({required this.appeal});
  final ModerationAppeal appeal;

  @override
  Widget build(BuildContext context) => Card(
        child: Padding(
          padding: const EdgeInsets.all(IConfess.space4),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Row(children: [
                Expanded(child: Text(appeal.decisionType == 'report' ? 'Report decision' : 'Confession decision', style: Theme.of(context).textTheme.titleMedium)),
                Chip(label: Text(_appealStatus(appeal.status))),
              ]),
              Text('Decision ID: ${appeal.decisionId}', style: Theme.of(context).textTheme.bodySmall),
              const SizedBox(height: IConfess.space2),
              Text(appeal.statement),
              if (appeal.decisionNote.isNotEmpty) ...[
                const SizedBox(height: IConfess.space2),
                Text('Moderator response: ${appeal.decisionNote}'),
              ],
            ],
          ),
        ),
      );
}

Future<String?> _textDialog(
  BuildContext context, {
  required String title,
  required String label,
  required String helper,
  required String submitLabel,
}) async {
  final controller = TextEditingController();
  try {
    return await showDialog<String>(
      context: context,
      builder: (context) => AlertDialog(
        title: Text(title),
        content: TextField(
          controller: controller,
          autofocus: true,
          decoration: InputDecoration(labelText: label, helperText: helper),
        ),
        actions: [
          TextButton(onPressed: () => Navigator.pop(context), child: const Text('Cancel')),
          FilledButton(
            onPressed: () {
              final value = controller.text.trim();
              if (value.isNotEmpty) Navigator.pop(context, value);
            },
            child: Text(submitLabel),
          ),
        ],
      ),
    );
  } finally {
    controller.dispose();
  }
}

Future<(String, String, String)?> _appealDialog(BuildContext context) async {
  final id = TextEditingController();
  final statement = TextEditingController();
  var type = 'confession';
  try {
    return await showDialog<(String, String, String)>(
      context: context,
      builder: (context) => StatefulBuilder(
        builder: (context, setState) => AlertDialog(
          title: const Text('Appeal a decision'),
          content: SingleChildScrollView(
            child: Column(mainAxisSize: MainAxisSize.min, children: [
              DropdownButtonFormField<String>(
                value: type,
                decoration: const InputDecoration(labelText: 'Decision type'),
                items: const [
                  DropdownMenuItem(value: 'confession', child: Text('Confession decision')),
                  DropdownMenuItem(value: 'report', child: Text('Report decision')),
                ],
                onChanged: (value) => setState(() => type = value ?? type),
              ),
              TextField(controller: id, decoration: const InputDecoration(labelText: 'Decision ID')),
              TextField(
                controller: statement,
                minLines: 2,
                maxLines: 5,
                decoration: const InputDecoration(labelText: 'Why should this decision be reviewed?'),
              ),
            ]),
          ),
          actions: [
            TextButton(onPressed: () => Navigator.pop(context), child: const Text('Cancel')),
            FilledButton(
              onPressed: () {
                final decisionId = id.text.trim();
                final note = statement.text.trim();
                if (decisionId.isNotEmpty && note.isNotEmpty) {
                  Navigator.pop(context, (type, decisionId, note));
                }
              },
              child: const Text('Submit appeal'),
            ),
          ],
        ),
      ),
    );
  } finally {
    id.dispose();
    statement.dispose();
  }
}

class _LoadError extends StatelessWidget {
  const _LoadError({required this.message, required this.onRetry});
  final String message;
  final VoidCallback onRetry;

  @override
  Widget build(BuildContext context) => ListView(
        padding: const EdgeInsets.all(IConfess.space5),
        children: [
          Text(message, textAlign: TextAlign.center),
          const SizedBox(height: IConfess.space3),
          Center(child: OutlinedButton(onPressed: onRetry, child: const Text('Try again'))),
        ],
      );
}

String _appealStatus(String status) => switch (status) {
      'submitted' => 'Submitted',
      'under_review' => 'Under review',
      'upheld' => 'Decision upheld',
      'overturned' => 'Decision overturned',
      _ => status,
    };

void _message(BuildContext context, String text) =>
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(text)));
