import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:iconfess_api/iconfess_api.dart';

import '../../core/di/providers.dart';

final moderationBlocksProvider = FutureProvider<Loadable<List<UserBlock>>>(
  (ref) => ref.watch(moderationRepositoryProvider).blocks(),
);

final moderationAppealsProvider = FutureProvider<Loadable<List<ModerationAppeal>>>(
  (ref) => ref.watch(moderationRepositoryProvider).appeals(),
);
