import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:iconfess_api/iconfess_api.dart';

import '../../core/di/providers.dart';

/// Library state (§35–§37, PHASE 27).
///
/// Every read goes through `LibraryRepository` rather than calling the API
/// client directly. That is what gives the library its offline behaviour: the
/// repository's `cachedRead` serves the last good payload when the network is
/// gone and marks it stale, which a raw `api.get` in a provider cannot do —
/// and which is what these providers did before this phase.
///
/// The three tabs load independently. A favourites call that fails must not
/// blank the collections tab: they are separate endpoints and a partial
/// library is far more useful than an error page.

/// The listener's own collections, with item counts.
final libraryCollectionsProvider =
    FutureProvider.autoDispose<Loadable<List<UserCollection>>>((ref) {
  return ref.watch(libraryRepositoryProvider).collections();
});

/// Favourites, newest first, with titles resolved by the server.
final libraryFavoritesProvider =
    FutureProvider.autoDispose<Loadable<List<Favorite>>>((ref) {
  return ref.watch(libraryRepositoryProvider).favorites();
});

/// Confessions the listener wrote, with their moderation status.
final libraryConfessionsProvider =
    FutureProvider.autoDispose<Loadable<List<UserConfession>>>((ref) {
  return ref.watch(libraryRepositoryProvider).myConfessions();
});

/// One collection with its items.
///
/// Keyed by id and auto-disposed so backing out of a collection and into
/// another does not serve the first one's items.
final collectionDetailProvider = FutureProvider.autoDispose
    .family<Loadable<UserCollection>, String>((ref, id) {
  return ref.watch(libraryRepositoryProvider).collection(id);
});

/// Writes for the library, exposed as one object so widgets do not each reach
/// for the repository and reinvent the refresh.
///
/// Every mutation invalidates the affected providers on success. Without that
/// the user creates a collection, the server accepts it, and the list they are
/// looking at does not change — the single most common way a CRUD surface
/// feels broken while being technically correct.
final libraryActionsProvider = Provider.autoDispose<LibraryActions>((ref) {
  return LibraryActions(ref);
});

class LibraryActions {
  LibraryActions(this._ref);

  final Ref _ref;

  LibraryRepository get _repo => _ref.read(libraryRepositoryProvider);

  Future<WriteResult<UserCollection>> createCollection(
    String name, {
    String description = '',
  }) async {
    final result = await _repo.createCollection(name, description: description);
    if (result.succeeded) _ref.invalidate(libraryCollectionsProvider);
    return result;
  }

  Future<WriteResult<UserCollection>> renameCollection(String id, String name) async {
    final result = await _repo.updateCollection(id, name: name);
    if (result.succeeded) {
      _ref.invalidate(libraryCollectionsProvider);
      _ref.invalidate(collectionDetailProvider(id));
    }
    return result;
  }

  Future<WriteResult<void>> deleteCollection(String id) async {
    final result = await _repo.deleteCollection(id);
    if (result.succeeded) _ref.invalidate(libraryCollectionsProvider);
    return result;
  }

  Future<WriteResult<void>> removeFromCollection(
      String collectionId, String confessionId) async {
    final result = await _repo.removeFromCollection(collectionId, confessionId);
    if (result.succeeded) {
      _ref.invalidate(collectionDetailProvider(collectionId));
      // The count shown on the list tab came from this collection, so it is
      // wrong now too.
      _ref.invalidate(libraryCollectionsProvider);
    }
    return result;
  }

  /// Removes a favourite of any entity type.
  ///
  /// The library lists favourited categories and voices as well as
  /// confessions, so unfavouriting has to pass the type through rather than
  /// assume 'confession' — which would leave a favourited category
  /// un-removable from the only screen that lists it.
  Future<WriteResult<void>> unfavorite(Favorite favorite) async {
    final result = await _repo.removeFavorite(
      entityType: favorite.entityType,
      entityId: favorite.entityId,
    );
    if (result.succeeded) _ref.invalidate(libraryFavoritesProvider);
    return result;
  }

  Future<WriteResult<UserConfession>> submitConfession(String id) async {
    final result = await _repo.submitConfession(id);
    if (result.succeeded) _ref.invalidate(libraryConfessionsProvider);
    return result;
  }
}
