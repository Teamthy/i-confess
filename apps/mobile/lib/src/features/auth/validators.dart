/// Form validation for the auth screens.
///
/// These are *mirrors* of checks the server already performs, not the authority
/// on them. The point of validating on the device is not to keep bad input away
/// from the API — the API rejects it regardless — it to tell the listener
/// before they press a button and wait for a round trip.
///
/// That makes agreement with the server the whole requirement. A rule that is
/// stricter than the server's refuses input that would have been accepted; one
/// that is weaker lets someone submit, wait, and read a rejection that could
/// have been a red line under a field. Every constant below is copied from
/// `server/internal/auth/password_policy.go`, and [passwordPolicySource] names
/// that file so a change there has somewhere obvious to be noticed.
library;

import 'dart:convert';

import 'package:characters/characters.dart';

/// Where these rules come from. Referenced by tests so the mirror cannot drift
/// silently.
const passwordPolicySource = 'server/internal/auth/password_policy.go';

/// NIST SP 800-63B, as the server implements it: a length floor, a byte ceiling
/// because bcrypt truncates at 72 and claims beyond that are false, and a check
/// against known-compromised values. No composition rules — requiring a symbol
/// makes people append "1!" to the word they were already using.
abstract final class PasswordPolicy {
  static const minLength = 8;
  static const maxBytes = 256;

  /// The server's embedded blocklist, reproduced so the device can say "too
  /// common" instead of waiting for a 400. Kept in sync by
  /// `auth_validators_test.dart`, which reads the Go source.
  static const common = <String>{
    'password', 'password1', 'password123', 'passw0rd',
    '12345678', '123456789', '1234567890', '12345678910',
    'qwerty123', 'qwertyuiop', 'abc12345', 'abcdefg1',
    'iloveyou', 'letmein1', 'welcome1', 'admin123',
    'football', 'baseball', 'trustno1', 'whatever',
    'changeme', 'pass1234', 'test1234', 'hello123',
  };
}

/// The longest display name the server will keep. Not a policy the server
/// states as a constant, so this is a client-side courtesy limit: long enough
/// that no real name is refused, short enough that a paste of a document is
/// caught here rather than by a 400 nobody can interpret.
const maxDisplayNameLength = 80;

/// An error message for one field, or null when the field is acceptable.
///
/// Deliberately a `String?` rather than a result object: these strings go
/// straight into `TextFormField.errorText`, and a richer type would exist only
/// to be unwrapped at every call site.
typedef FieldValidator = String? Function(String value);

abstract final class AuthValidators {
  /// Email addresses.
  ///
  /// Intentionally permissive. The server lowercases and trims, then looks the
  /// address up; it does not enforce a grammar, and neither should the app.
  /// What this rejects is what is certainly not an address — a missing `@`, an
  /// empty local part, spaces — because sending those costs a round trip that
  /// can only fail.
  static String? email(String value) {
    final v = value.trim();
    if (v.isEmpty) return 'Enter your email address.';

    // One `@` with something on both sides, and a domain with a dot in it.
    // Rejecting anything else is where the false negatives start: `a+b@x.io`
    // and `grace@sub.example.co.uk` are ordinary addresses.
    final at = v.indexOf('@');
    if (at < 1 || at == v.length - 1) return 'That doesn’t look like an email address.';
    if (v.indexOf('@', at + 1) != -1) return 'That doesn’t look like an email address.';

    final domain = v.substring(at + 1);
    if (domain.startsWith('.') || domain.endsWith('.')) {
      return 'That doesn’t look like an email address.';
    }
    if (!domain.contains('.')) return 'That doesn’t look like an email address.';
    if (v.contains(' ')) return 'Email addresses don’t contain spaces.';
    return null;
  }

  /// Passwords, matching the server exactly.
  ///
  /// Length is counted in characters and the ceiling in bytes, because that is
  /// what the server does — and the difference is not academic. Eight padlocks
  /// is eight characters and thirty-two bytes, and an app that checked bytes
  /// for the floor would refuse it while the server accepts it.
  static String? password(String value) {
    if (value.isEmpty) return 'Enter a password.';
    if (value.characters.length < PasswordPolicy.minLength) {
      return 'Use at least ${PasswordPolicy.minLength} characters.';
    }
    // Bytes, not characters: `len()` in Go counts bytes, and bcrypt truncates
    // at 72 of them. Checking UTF-16 code units here would accept something the
    // server rejects.
    if (utf8.encode(value).length > PasswordPolicy.maxBytes) {
      return 'That password is too long.';
    }
    if (value.trim().isEmpty) return 'A password can’t be only spaces.';
    if (PasswordPolicy.common.contains(value.toLowerCase())) {
      // Named specifically, as the server does: "too common" tells someone what
      // to do, where "invalid password" invites them to try "Password1".
      return 'That password shows up in known breaches. Choose something less common.';
    }
    return null;
  }

  /// A new password, which also has to differ from the old one.
  ///
  /// Only used where an old password is present. Reset flows have no old
  /// password to compare against, and inventing one would be a guess.
  static String? newPassword(String value, {String? previous}) {
    final base = password(value);
    if (base != null) return base;
    if (previous != null && previous.isNotEmpty && value == previous) {
      return 'That’s the password you’re replacing.';
    }
    return null;
  }

  /// The second password field.
  ///
  /// Compared on trimmed-equal, not exact-equal: a trailing space in one of two
  /// hidden fields is invisible, and refusing it with "doesn't match" sends
  /// someone retyping both.
  static String? Function(String) confirm(String first) => (String value) {
        if (value.isEmpty) return 'Repeat the password.';
        if (value.trim() != first.trim()) return 'The two passwords don’t match.';
        return null;
      };

  /// Display name. Optional, so empty is valid.
  static String? displayName(String value) {
    if (value.isEmpty) return null;
    if (value.trim().isEmpty) return 'A name can’t be only spaces.';
    if (value.characters.length > maxDisplayNameLength) {
      return 'That’s longer than $maxDisplayNameLength characters.';
    }
    return null;
  }

  /// A second-factor entry.
  ///
  /// Deliberately lax. The server accepts a six-digit code *or* a recovery code
  /// of the form `ABCD-EFGH-JKMN`, and a strict "six digits" rule here would
  /// lock out exactly the person who has lost their authenticator and needs the
  /// recovery path.
  static String? secondFactor(String value) {
    if (value.trim().isEmpty) return 'Enter the code from your authenticator.';
    return null;
  }

  /// A one-time token from an email link.
  ///
  /// Only checked for presence. The format is a base64url string the client has
  /// no business interpreting, and a token that looks right but is expired is
  /// indistinguishable from one that is malformed — the server is the only
  /// authority on either.
  static String? token(String value) {
    if (value.trim().isEmpty) return 'Enter the code from your email.';
    return null;
  }

  /// Pulls the token out of whatever was pasted.
  ///
  /// The email highlights a *link*, so pasting the link rather than the token
  /// inside it is the obvious move, and without this it fails in a misleading
  /// way: the URL goes to the server, the server answers `AUTH_TOKEN_INVALID`,
  /// and the screen explains that as "that code has expired". That diagnosis is
  /// wrong, and it sends someone hunting for a newer email when the one they
  /// have is fine.
  ///
  /// Anything that is not a URL is returned as it was — a bare token must pass
  /// through untouched. The URL is never opened, so an origin we do not
  /// recognise costs nothing: we send the token to our own API and the worst
  /// case is an invalid-token error.
  static String tokenFromPaste(String value) {
    final trimmed = value.trim();
    if (!trimmed.contains('://')) return trimmed;
    final token = Uri.tryParse(trimmed)?.queryParameters['token'];
    if (token == null || token.isEmpty) return trimmed;
    return token;
  }
}
