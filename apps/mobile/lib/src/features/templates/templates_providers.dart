import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:iconfess_api/iconfess_api.dart';

import '../../core/di/providers.dart';

final templatesProvider = FutureProvider<Loadable<List<SessionTemplate>>>((ref) {
  return ref.watch(templateRepositoryProvider).templates();
});

final templateDetailProvider =
    FutureProvider.family<Loadable<SessionTemplate>, String>((ref, id) {
  return ref.watch(templateRepositoryProvider).template(id);
});

final sharedTemplateProvider =
    FutureProvider.family<Loadable<SessionTemplate>, String>((ref, token) {
  return ref.watch(templateRepositoryProvider).sharedTemplate(token);
});
