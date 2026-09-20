import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:iconfess_api/iconfess_api.dart';

import '../../core/di/providers.dart';

final communityFeedProvider = FutureProvider<List<Map<String, dynamic>>>((ref) async {
  final response = await ref.watch(apiClientProvider).getCommunityFeed();
  return (response['posts'] as List? ?? const [])
      .whereType<Map>()
      .map((post) => Map<String, dynamic>.from(post))
      .toList(growable: false);
});

class CommunityScreen extends ConsumerWidget {
  const CommunityScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final feed = ref.watch(communityFeedProvider);
    return Scaffold(
      appBar: AppBar(title: const Text('Community')),
      body: RefreshIndicator(
        onRefresh: () => ref.refresh(communityFeedProvider.future),
        child: feed.when(
          loading: () => const Center(child: CircularProgressIndicator()),
          error: (error, _) => ListView(children: [
            const SizedBox(height: 120),
            const Icon(Icons.cloud_off_outlined, size: 44),
            const Center(child: Padding(padding: EdgeInsets.all(16), child: Text('Community could not be loaded.'))),
            Center(child: FilledButton(onPressed: () => ref.invalidate(communityFeedProvider), child: const Text('Try again'))),
          ]),
          data: (posts) => posts.isEmpty
              ? ListView(children: const [SizedBox(height: 140), Icon(Icons.forum_outlined, size: 44), Center(child: Padding(padding: EdgeInsets.all(16), child: Text('No approved stories yet.')))])
              : ListView.separated(
                  padding: const EdgeInsets.all(16),
                  itemCount: posts.length,
                  separatorBuilder: (_, _) => const SizedBox(height: 12),
                  itemBuilder: (context, index) => _PostCard(post: posts[index]),
                ),
        ),
      ),
    );
  }
}

class _PostCard extends ConsumerStatefulWidget {
  const _PostCard({required this.post});
  final Map<String, dynamic> post;
  @override ConsumerState<_PostCard> createState() => _PostCardState();
}

class _PostCardState extends ConsumerState<_PostCard> {
  bool busy = false;
  String? reacted;
  String? error;
  Future<void> react(String value) async {
    if (busy || reacted != null) return;
    setState(() { busy = true; error = null; });
    try {
      await ref.read(apiClientProvider).postCommunityPostsByIdReact(widget.post['id'] as String, value);
      if (mounted) setState(() => reacted = value);
    } catch (_) {
      if (mounted) setState(() => error = 'Could not save reaction. Try again.');
    } finally { if (mounted) setState(() => busy = false); }
  }
  @override Widget build(BuildContext context) => Card(child: Padding(
    padding: const EdgeInsets.all(16),
    child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
      Text(widget.post['body'] as String? ?? '', style: Theme.of(context).textTheme.bodyLarge),
      const SizedBox(height: 12),
      Wrap(spacing: 8, children: [for (final item in const [('amen', 'Amen'), ('heart', 'Heart'), ('pray', 'Pray')]) ActionChip(label: Text(reacted == item.$1 ? '✓ ${item.$2}' : item.$2), onPressed: busy || reacted != null ? null : () => react(item.$1))]),
      if (error != null) Padding(padding: const EdgeInsets.only(top: 8), child: Text(error!, style: TextStyle(color: Theme.of(context).colorScheme.error))),
    ]),
  ));
}
