import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:iconfess_api/iconfess_api.dart';

import '../../core/di/providers.dart';
import '../../core/error/error_mapper.dart';
import '../../core/theme/theme.dart';
import '../../core/theme/tokens.dart';
import '../../core/widgets/screen.dart';

final communityFeedProvider = FutureProvider<List<Map<String, dynamic>>>((ref) async {
  final response = await ref.watch(apiClientProvider).getCommunityFeed();
  return (response['posts'] as List? ?? const [])
      .whereType<Map<String, dynamic>>()
      .map((post) => Map<String, dynamic>.from(post))
      .toList(growable: false);
});

final communityConfessionsProvider = FutureProvider<List<Map<String, dynamic>>>((ref) async {
  final response = await ref.watch(apiClientProvider).getCommunityConfessions();
  return (response['confessions'] as List? ?? const [])
      .whereType<Map<String, dynamic>>()
      .map((c) => Map<String, dynamic>.from(c))
      .toList(growable: false);
});

class CommunityScreen extends ConsumerWidget {
  const CommunityScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final surfaces = AppSurfaces.of(context);
    return DefaultTabController(
      length: 2,
      child: AppScaffold(
        title: 'Community',
        scrollable: false,
        body: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            TabBar(
              labelColor: surfaces.primary,
              unselectedLabelColor: surfaces.textSecondary,
              indicatorColor: surfaces.primary,
              labelStyle: IConfess.bodySm.copyWith(fontWeight: FontWeight.w600),
              unselectedLabelStyle: IConfess.bodySm,
              tabs: const [
                Tab(text: 'Stories'),
                Tab(text: 'Testimonies'),
              ],
            ),
            const SizedBox(height: IConfess.space4),
            const Expanded(
              child: TabBarView(children: [
                _StoriesTab(),
                _TestimoniesTab(),
              ]),
            ),
          ],
        ),
      ),
    );
  }
}

class _StoriesTab extends ConsumerWidget {
  const _StoriesTab();

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final feed = ref.watch(communityFeedProvider);
    return RefreshIndicator(
      onRefresh: () => ref.refresh(communityFeedProvider.future),
      child: feed.when(
        loading: () => const Center(child: CircularProgressIndicator()),
        error: (error, _) => ListView(children: [
          const SizedBox(height: 120),
          const Icon(Icons.cloud_off_outlined, size: 44),
          const Center(
              child: Padding(
                  padding: EdgeInsets.all(16), child: Text('Community could not be loaded.'))),
          Center(
              child: FilledButton(
                  onPressed: () => ref.invalidate(communityFeedProvider),
                  child: const Text('Try again'))),
        ]),
        data: (posts) => posts.isEmpty
            ? ListView(children: const [
                SizedBox(height: 140),
                Icon(Icons.forum_outlined, size: 44),
                Center(
                    child: Padding(
                        padding: EdgeInsets.all(16), child: Text('No approved stories yet.')))
              ])
            : ListView.separated(
                padding: const EdgeInsets.all(16),
                itemCount: posts.length,
                separatorBuilder: (_, _) => const SizedBox(height: 12),
                itemBuilder: (context, index) => _PostCard(post: posts[index]),
              ),
      ),
    );
  }
}

class _TestimoniesTab extends ConsumerWidget {
  const _TestimoniesTab();

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final feed = ref.watch(communityConfessionsProvider);
    return RefreshIndicator(
      onRefresh: () => ref.refresh(communityConfessionsProvider.future),
      child: feed.when(
        loading: () => const Center(child: CircularProgressIndicator()),
        error: (error, _) => ListView(children: [
          const SizedBox(height: 120),
          const Icon(Icons.menu_book_outlined, size: 44),
          const Center(
              child: Padding(
                  padding: EdgeInsets.all(16),
                  child: Text('Testimonies could not be loaded.'))),
          Center(
              child: FilledButton(
                  onPressed: () => ref.invalidate(communityConfessionsProvider),
                  child: const Text('Try again'))),
        ]),
        data: (confessions) => confessions.isEmpty
            ? ListView(children: const [
                SizedBox(height: 140),
                Icon(Icons.auto_stories_outlined, size: 44),
                Center(
                    child: Padding(
                        padding: EdgeInsets.all(16),
                        child: Text('No published testimonies yet. Be the first to share.')))
              ])
            : ListView.separated(
                padding: const EdgeInsets.all(16),
                itemCount: confessions.length,
                separatorBuilder: (_, _) => const SizedBox(height: 12),
                itemBuilder: (context, index) => _ConfessionCard(confession: confessions[index]),
              ),
      ),
    );
  }
}

class _PostCard extends ConsumerStatefulWidget {
  const _PostCard({required this.post});
  final Map<String, dynamic> post;
  @override
  ConsumerState<_PostCard> createState() => _PostCardState();
}

class _PostCardState extends ConsumerState<_PostCard> {
  bool busy = false;
  bool blocking = false;
  String? reacted;
  String? error;
  Future<void> react(String value) async {
    if (busy || reacted != null) return;
    setState(() {
      busy = true;
      error = null;
    });
    try {
      await ref
          .read(apiClientProvider)
          .postCommunityPostsByIdReact(widget.post['id'] as String, value);
      if (mounted) setState(() => reacted = value);
    } catch (_) {
      if (mounted) setState(() => error = 'Could not save reaction. Try again.');
    } finally {
      if (mounted) setState(() => busy = false);
    }
  }

  Future<void> blockAuthor() async {
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (context) => AlertDialog(
        title: const Text('Block this author?'),
        content: const Text('You will no longer see this author’s community posts. Their identity stays private.'),
        actions: [
          TextButton(onPressed: () => Navigator.pop(context, false), child: const Text('Cancel')),
          FilledButton(onPressed: () => Navigator.pop(context, true), child: const Text('Block author')),
        ],
      ),
    );
    if (confirmed != true || !mounted) return;
    setState(() {
      blocking = true;
      error = null;
    });
    final result = await ref.read(communityRepositoryProvider).blockPostAuthor(widget.post['id'] as String);
    if (!mounted) return;
    result.when(
      success: (_) {
        ref.invalidate(communityFeedProvider);
        ScaffoldMessenger.of(context).showSnackBar(const SnackBar(content: Text('Author blocked')));
      },
      failure: (failure) => setState(() => error = ErrorMapper.describe(failure).message),
    );
    if (mounted) setState(() => blocking = false);
  }

  @override
  Widget build(BuildContext context) => Card(
        child: Padding(
          padding: const EdgeInsets.all(16),
          child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
            Row(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Expanded(
                  child: Text(widget.post['body'] as String? ?? '',
                      style: Theme.of(context).textTheme.bodyLarge),
                ),
                PopupMenuButton<String>(
                  tooltip: 'Story options',
                  enabled: !busy && !blocking,
                  onSelected: (action) {
                    if (action == 'block-author') blockAuthor();
                  },
                  itemBuilder: (context) => const [
                    PopupMenuItem(
                      value: 'block-author',
                      child: Text('Block author'),
                    ),
                  ],
                ),
              ],
            ),
            const SizedBox(height: 12),
            Wrap(spacing: 8, children: [
              for (final item in const [('amen', 'Amen'), ('heart', 'Heart'), ('pray', 'Pray')])
                ActionChip(
                    label: Text(reacted == item.$1 ? '✓ ${item.$2}' : item.$2),
                    onPressed: busy || blocking || reacted != null ? null : () => react(item.$1))
            ]),
            if (error != null)
              Padding(
                  padding: const EdgeInsets.only(top: 8),
                  child: Text(error!,
                      style: TextStyle(color: Theme.of(context).colorScheme.error))),
          ]),
        ),
      );
}

class _ConfessionCard extends StatelessWidget {
  const _ConfessionCard({required this.confession});
  final Map<String, dynamic> confession;

  @override
  Widget build(BuildContext context) {
    final title = confession['title'] as String? ?? '';
    final text = confession['text'] as String? ?? '';
    final published =
        confession['published_at'] as String? ?? confession['created_at'] as String? ?? '';
    return Card(
      child: Padding(
        padding: const EdgeInsets.all(16),
        child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
          if (title.isNotEmpty)
            Text(title,
                style:
                    Theme.of(context).textTheme.titleMedium?.copyWith(fontWeight: FontWeight.w600)),
          if (title.isNotEmpty) const SizedBox(height: 8),
          Text(text, style: Theme.of(context).textTheme.bodyLarge),
          if (published.isNotEmpty) ...[
            const SizedBox(height: 12),
            Text(published.split('T').first, style: Theme.of(context).textTheme.labelSmall),
          ],
        ]),
      ),
    );
  }
}
