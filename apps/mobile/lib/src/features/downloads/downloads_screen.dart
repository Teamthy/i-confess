import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../core/theme/theme.dart';
import '../../core/theme/tokens.dart';
import '../../core/widgets/screen.dart';
import 'downloads_providers.dart';

/// Downloads: offline licences, expiry, renew.
class DownloadsScreen extends ConsumerWidget {
  const DownloadsScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final async = ref.watch(downloadsProvider);
    final expiring = ref.watch(expiringDownloadsProvider);
    final surfaces = AppSurfaces.of(context);

    return AppScaffold(
      title: 'Downloads',
      body: async.when(
        loading: () => const Center(child: CircularProgressIndicator()),
        error: (e, _) => Center(child: Text('Failed: $e')),
        data: (loadable) {
          final lib = loadable.valueOrNull;
          if (lib == null || lib.downloads.isEmpty) {
            return Center(
              child: Column(
                mainAxisAlignment: MainAxisAlignment.center,
                children: [
                  Icon(Icons.download_done_rounded, size: 48, color: surfaces.textSecondary),
                  const SizedBox(height: IConfess.space3),
                  Text('No offline content',
                      style: IConfess.body.copyWith(color: surfaces.textPrimary)),
                  Text('Download confessions for offline listening',
                      style: IConfess.bodySm.copyWith(color: surfaces.textSecondary)),
                ],
              ),
            );
          }
          return Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              if (expiring.isNotEmpty)
                Container(
                  width: double.infinity,
                  padding: const EdgeInsets.all(IConfess.space4),
                  decoration: BoxDecoration(
                    color: IConfess.colorSemanticWarningLight.withValues(alpha: 0.15),
                    borderRadius: BorderRadius.circular(IConfess.radiusMd),
                  ),
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text('${expiring.length} expiring soon',
                          style: IConfess.subheading.copyWith(color: surfaces.textPrimary)),
                      Text('Renew before your next flight',
                          style: IConfess.bodySm.copyWith(color: surfaces.textSecondary)),
                    ],
                  ),
                ),
              const SizedBox(height: IConfess.space3),
              Text('${lib.used} / ${lib.limit} used • ${lib.offlineHoursAllowed}h offline allowed',
                  style: IConfess.caption.copyWith(color: surfaces.textSecondary)),
              const SizedBox(height: IConfess.space3),
              Expanded(
                child: ListView.separated(
                  itemCount: lib.downloads.length,
                  separatorBuilder: (_, _) => const Divider(height: 1),
                  itemBuilder: (context, i) {
                    final d = lib.downloads[i];
                    return ListTile(
                      leading: Icon(d.expired ? Icons.error_outline_rounded : Icons.audiotrack_rounded),
                      title: Text(d.title.isNotEmpty ? d.title : d.confessionId),
                      subtitle: Text(
                        d.expired
                            ? 'Expired'
                            : d.expiresAt != null
                                ? 'Expires ${d.expiresAt!.toLocal().toString().split(' ').first}'
                                : d.status,
                        style: IConfess.caption.copyWith(color: surfaces.textSecondary),
                      ),
                      trailing: PopupMenuButton(
                        itemBuilder: (context) => [
                          const PopupMenuItem(value: 'renew', child: Text('Renew')),
                          const PopupMenuItem(value: 'remove', child: Text('Remove')),
                        ],
                        onSelected: (value) async {
                          if (value == 'renew') {
                            await ref.read(libraryRepositoryProvider).renewDownload(d.id);
                            ref.invalidate(downloadsProvider);
                          } else if (value == 'remove') {
                            await ref.read(libraryRepositoryProvider).removeDownload(d.id);
                            ref.invalidate(downloadsProvider);
                          }
                        },
                      ),
                    );
                  },
                ),
              ),
            ],
          );
        },
      ),
    );
  }
}
