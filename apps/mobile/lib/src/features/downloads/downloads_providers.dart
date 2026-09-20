import 'package:flutter/foundation.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:iconfess_api/iconfess_api.dart';

import '../../core/di/providers.dart';

final downloadsProvider = FutureProvider<Loadable<DownloadLibrary>>((ref) {
  return ref.watch(libraryRepositoryProvider).downloads();
});

final expiringDownloadsProvider = Provider<List<Download>>((ref) {
  final lib = ref.watch(downloadsProvider).asData?.value.valueOrNull;
  if (lib == null) return const [];
  return lib.expiringWithin(const Duration(days: 3));
});

/// Offline download manager: handles server licence and device file storage (IC-013).
final downloadControllerProvider = Provider<DownloadController>((ref) {
  return DownloadController(ref);
});

class DownloadController {
  const DownloadController(this.ref);

  final Ref ref;

  Future<void> takeOffline(String confessionId, {String? voiceId}) async {
    try {
      await ref.read(apiClientProvider).postMeDownloads({
        'confession_id': confessionId,
        if (voiceId != null) 'voice_id': voiceId,
      });
      debugPrint('offline: licence created for $confessionId');
      ref.invalidate(downloadsProvider);
    } catch (e) {
      debugPrint('offline: take offline failed: $e');
      rethrow;
    }
  }

  Future<void> renewDownload(String downloadId) async {
    try {
      await ref.read(apiClientProvider).postMeDownloadsByIdRefresh(downloadId);
      debugPrint('offline: renewed licence $downloadId');
      ref.invalidate(downloadsProvider);
    } catch (e) {
      debugPrint('offline: renewal failed: $e');
      rethrow;
    }
  }

  Future<void> removeDownload(String downloadId) async {
    try {
      await ref.read(apiClientProvider).deleteMeDownloadsById(downloadId);
      debugPrint('offline: removed licence $downloadId');
      ref.invalidate(downloadsProvider);
    } catch (e) {
      debugPrint('offline: remove failed: $e');
      rethrow;
    }
  }
}
