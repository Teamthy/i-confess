import 'package:flutter/widgets.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:iconfess_api/iconfess_api.dart';

import '../../../core/error/error_mapper.dart';
import 'auth_form.dart';

/// The submission state every auth screen needs, in one place.
///
/// Four things go wrong in forms often enough to be worth solving once:
///
///  - A second tap while the first is in flight. Two registrations is an
///    annoyance; two password resets is a support ticket.
///  - `setState` after the screen has been popped, which throws in a way that
///    looks like a crash in the flow rather than a late response.
///  - An exception escaping into the framework because a repository threw
///    something the screen did not expect. A form that goes blank on an error
///    loses what the listener typed.
///  - The same failure worded differently on five screens, because each one
///    mapped its own error.
///
/// Screens hold this as local state rather than a provider because a form's
/// busy flag and error text *are* local: they die with the screen, they are not
/// shared with anything, and a global "is a request in flight" flag is how one
/// screen's spinner ends up on another screen's button.
mixin AuthFormState<T extends ConsumerStatefulWidget> on ConsumerState<T> {
  bool _busy = false;
  bool _submitted = false;
  UserFacingError? _error;

  /// Whether a request is in flight. Drives the button and disables the form.
  bool get busy => _busy;

  /// Whether the listener has attempted a submission at least once.
  ///
  /// Field-level errors are withheld until then. Showing "Enter your email
  /// address." under an empty field the moment a screen appears is the fastest
  /// way to make a form feel like it is already complaining.
  bool get submitted => _submitted;

  /// The form-level error, if the last attempt produced one.
  UserFacingError? get error => _error;

  /// Which auth endpoints this screen talks to.
  ///
  /// Overridden by the two screens that consume a one-time token, because a 401
  /// from those means the token is dead and not that the password was wrong.
  AuthSurface get surface => AuthSurface.credentials;

  /// Runs [action] with the busy flag set and the error cleared.
  ///
  /// Returns null when the action threw; the mapped error is already in [error]
  /// and on screen, so the caller only has to stop. A null return does *not*
  /// mean the action failed on its own terms — a failed `WriteResult` is
  /// returned normally, because it carries the error the caller has to read.
  Future<R?> runGuarded<R>(Future<R> Function() action) async {
    if (_busy) return null;
    setState(() {
      _busy = true;
      _submitted = true;
      _error = null;
    });

    try {
      return await action();
    } on ApiException catch (e) {
      _report(ErrorMapper.describeAuth(e, surface: surface));
      return null;
    } on Object catch (e, st) {
      // A keystore that is locked, a provider that was not overridden, a cast
      // that assumed a field. None of these are the listener's fault and none
      // of them should blank the form, so they are described rather than thrown.
      debugPrint('auth form: unexpected failure: $e\n$st');
      _report(const UserFacingError(
        title: 'Something interrupted that',
        message: 'Nothing was sent. Try again — what you typed is still here.',
        primaryAction: ErrorAction.retry,
        retryable: true,
      ));
      return null;
    } finally {
      if (mounted) {
        setState(() => _busy = false);
      }
    }
  }

  /// Shows a failure the caller produced itself, such as a `WriteFailure`.
  void reportFailure(ApiException e) =>
      _report(ErrorMapper.describeAuth(e, surface: surface));

  void clearError() {
    if (_error == null) return;
    setState(() => _error = null);
  }

  /// Marks the form as submitted without a request, so field errors appear.
  ///
  /// Used when local validation fails: there is nothing to send, but the
  /// listener has to be told why the button did nothing.
  void markSubmitted() {
    if (_submitted) return;
    setState(() => _submitted = true);
  }

  void _report(UserFacingError e) {
    if (!mounted) return;
    setState(() => _error = e);
    failureFeedback();
  }
}
