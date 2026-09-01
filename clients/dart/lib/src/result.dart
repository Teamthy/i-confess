import 'api_error.dart';

/// The states every data-bearing screen must handle (§58, §100).
///
/// Modelled as a sealed class so the compiler forces each case to be handled.
/// The alternative — a struct with `loading`, `data` and `error` fields — lets
/// a screen render a spinner over stale data with an error underneath, which
/// is how "only the happy path was built" happens in practice.
sealed class Loadable<T> {
  const Loadable();

  /// Nothing requested yet.
  const factory Loadable.initial() = LoadInitial<T>;

  /// A first load is running; there is nothing to show.
  const factory Loadable.loading() = LoadLoading<T>;

  /// Data is available. [stale] marks a cached value being shown while a
  /// refresh runs, so the UI can indicate it without blanking the screen.
  const factory Loadable.loaded(T value, {bool stale, bool fromCache}) = LoadLoaded<T>;

  /// A load failed and there is nothing cached to fall back on.
  const factory Loadable.failed(ApiException error, {T? cached}) = LoadFailed<T>;

  /// Convenience for the common "show something if we have it" case.
  T? get valueOrNull => switch (this) {
        LoadLoaded<T>(:final value) => value,
        LoadFailed<T>(:final cached) => cached,
        _ => null,
      };

  bool get isLoading => this is LoadLoading<T>;
  bool get hasValue => valueOrNull != null;
}

final class LoadInitial<T> extends Loadable<T> {
  const LoadInitial();
}

final class LoadLoading<T> extends Loadable<T> {
  const LoadLoading();
}

final class LoadLoaded<T> extends Loadable<T> {
  const LoadLoaded(this.value, {this.stale = false, this.fromCache = false});

  final T value;

  /// Shown from cache while a refresh is in flight.
  final bool stale;

  /// Served from cache because the network was unavailable. The UI should say
  /// so: presenting week-old data as current is worse than admitting it is
  /// cached (§102).
  final bool fromCache;
}

final class LoadFailed<T> extends Loadable<T> {
  const LoadFailed(this.error, {this.cached});

  final ApiException error;

  /// A previously cached value, if any. Offered so a screen can show stale
  /// content with an error banner rather than an empty page.
  final T? cached;

  /// Whether retrying is likely to help. A 400 will fail identically; a
  /// network error or a 5xx may not.
  bool get isRetryable => switch (error) {
        NetworkException() => true,
        ApiError(:final status) => status >= 500 || status == 429,
      };
}

/// The outcome of a write.
///
/// Separate from [Loadable] because a mutation has different states: it does
/// not have a cached fallback, and "succeeded" often carries no value.
sealed class WriteResult<T> {
  const WriteResult();

  const factory WriteResult.success(T value) = WriteSuccess<T>;
  const factory WriteResult.failure(ApiException error) = WriteFailure<T>;

  bool get succeeded => this is WriteSuccess<T>;
}

final class WriteSuccess<T> extends WriteResult<T> {
  const WriteSuccess(this.value);
  final T value;
}

final class WriteFailure<T> extends WriteResult<T> {
  const WriteFailure(this.error);
  final ApiException error;

  /// Whether the failure was caused by being offline, which the UI should word
  /// differently from a rejection: "we'll retry when you're back" rather than
  /// "that didn't work".
  bool get isOffline => error is NetworkException;
}
