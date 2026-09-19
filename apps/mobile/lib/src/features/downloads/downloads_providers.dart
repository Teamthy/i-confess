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
