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
///
/// Deliberately NOT autoDispose. Widgets reach this through `ref.read` in a
/// tap handler, and a `read` registers no listener — so an autoDispose
/// provider is disposed immediately, and the `ref.invalidate` after the await
/// throws `UnmountedRefException` instead of refreshing anything. The write
/// had already reached the server by then, so the symptom was the worst kind:
/// the change was saved and the screen never showed it. This object holds no
/// state beyond the ref, so keeping it alive costs nothing.
final libraryActionsProvider = Provider<LibraryActions>((ref) {
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

  /// Adds a confession to a collection, from the confession's own page (G-43).
  ///
  /// The add gesture has to exist somewhere other than the collection: an
  /// item is chosen while reading the confession, and a listener who has to
  /// leave the page to file it will not file it.
  Future<WriteResult<UserCollection>> addToCollection(
      String collectionId, String confessionId) async {
    final result = await _repo.addToCollection(collectionId, confessionId);
    if (result.succeeded) {
      _ref.invalidate(collectionDetailProvider(collectionId));
      _ref.invalidate(libraryCollectionsProvider);
    }
    return result;
  }

  /// Persists a new item order for one collection (G-43).
  ///
  /// The client sends the complete order, not a move: the server rewrites
  /// positions from the array, so a partial patch would silently renumber
  /// everything the drag did not mention.
  Future<WriteResult<UserCollection>> reorderCollection(
      String collectionId, List<String> confessionIds) async {
    final result = await _repo.reorderCollection(collectionId, confessionIds);
    if (result.succeeded) {
      // The detail view re-reads in the server's order rather than trusting
      // the optimistic local one: if a row was removed while the drag was in
      // flight, the server is right and the screen should say so.
      _ref.invalidate(collectionDetailProvider(collectionId));
      _ref.invalidate(libraryCollectionsProvider);
    }
    return result;
  }

  /// Sets or clears a collection's cover image (G-44).
  ///
  /// An empty string is a value, not an omission: it clears the cover and the
  /// row falls back to the monogram.
  Future<WriteResult<UserCollection>> updateCover(String collectionId, String coverUrl) async {
    final result = await _repo.updateCollection(collectionId, coverUrl: coverUrl);
    if (result.succeeded) {
      _ref.invalidate(collectionDetailProvider(collectionId));
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
