import 'dart:io';

import 'package:flutter_test/flutter_test.dart';
import 'package:iconfess/src/core/error/error_mapper.dart';
import 'package:iconfess/src/features/auth/validators.dart';
import 'package:iconfess_api/iconfess_api.dart';

void main() {
  group('email validation', () {
    test('accepts the addresses people actually have', () {
      for (final address in [
        'grace@example.com',
        'a@b.co',
        'first.last@sub.domain.example.co.uk',
        'grace+iconfess@example.com',
        "o'brien@example.ie",
      ]) {
        expect(AuthValidators.email(address), isNull, reason: address);
      }
    });

    test('trims, because the server trims too', () {
      expect(AuthValidators.email('  grace@example.com  '), isNull);
    });

    test('rejects what cannot be an address', () {
      for (final address in [
        '',
        '   ',
        'grace',
        '@example.com',
        'grace@',
        'grace@example',
        'grace@@example.com',
        'grace@.example.com',
        'grace @example.com',
      ]) {
        expect(AuthValidators.email(address), isNotNull, reason: address);
      }
    });
  });

  group('password validation mirrors the server', () {
    test('the floor is eight characters, counted as characters not bytes', () {
      // Eight padlocks: eight graphemes, thirty-two UTF-8 bytes. A validator
      // that counted bytes for the floor would refuse this and the server would
      // accept it, which is the failure mode that matters — the client saying no
      // to something the API allows.
      expect(AuthValidators.password('🔒' * 8), isNull);
      expect(AuthValidators.password('🔒' * 7), isNotNull);
      expect(AuthValidators.password('a' * 8), isNull);
      expect(AuthValidators.password('a' * 7), isNotNull);
    });

    test('the ceiling is 256 bytes, matching Go\'s len()', () {
      expect(AuthValidators.password('a' * 256), isNull);
      expect(AuthValidators.password('a' * 257), isNotNull);
      // 86 padlocks is 344 bytes: over the ceiling in bytes while being well
      // under it in characters.
      expect(AuthValidators.password('🔒' * 86), isNotNull);
    });

    test('whitespace-only is refused', () {
      expect(AuthValidators.password('        '), isNotNull);
    });

    test('breached passwords are named, not called invalid', () {
      final message = AuthValidators.password('password');
      expect(message, isNotNull);
      expect(message, contains('breach'),
          reason: '"invalid password" invites someone to try "Password1"; '
              '"shows up in known breaches" tells them what to do');
      expect(AuthValidators.password('PASSWORD'), isNotNull,
          reason: 'the server lowercases before the lookup, so must the client');
    });

    test('no composition rules are imposed', () {
      // NIST SP 800-63B, as the server implements it. A rule requiring a symbol
      // would be a client-side invention the server does not enforce, and would
      // refuse passwords the API accepts.
      expect(AuthValidators.password('correct horse battery'), isNull);
      expect(AuthValidators.password('alllowercaseletters'), isNull);
    });

    test('the mirror is read from the Go source, not copied by hand', () {
      // This is the guard that keeps the two in step. It reads the server's own
      // policy file and compares, so a change to MinPasswordLength fails a
      // Flutter test rather than quietly diverging until someone is locked out.
      final go = _readServerPasswordPolicy();
      expect(go.minLength, PasswordPolicy.minLength,
          reason: 'client and server disagree on the minimum length');
      expect(go.maxBytes, PasswordPolicy.maxBytes,
          reason: 'client and server disagree on the maximum length');
      expect(go.common, PasswordPolicy.common,
          reason: 'the breach blocklist has drifted');
    });
  });

  group('confirm and new passwords', () {
    test('a trailing space does not count as a mismatch', () {
      // Both fields are hidden. Refusing "correct horse battery " against
      // "correct horse battery" sends someone retyping both to fix something
      // they cannot see.
      expect(AuthValidators.confirm('correct horse battery')('correct horse battery '),
          isNull);
      expect(AuthValidators.confirm('correct horse battery')('something else'),
          isNotNull);
    });

    test('a new password may not be the one it replaces', () {
      expect(
        AuthValidators.newPassword('correct horse battery', previous: 'correct horse battery'),
        isNotNull,
      );
      expect(
        AuthValidators.newPassword('correct horse battery', previous: 'the old one'),
        isNull,
      );
    });
  });

  group('second factor and tokens', () {
    test('a recovery code is accepted, because that path is the point', () {
      expect(AuthValidators.secondFactor('123456'), isNull);
      expect(AuthValidators.secondFactor('ABCD-EFGH-JKMN'), isNull,
          reason: 'a six-digit-only rule would lock out the person who lost '
              'their authenticator');
      expect(AuthValidators.secondFactor('  '), isNotNull);
    });

    test('a token is checked for presence only', () {
      // The format is the server's business. A token that looks right and is
      // expired is indistinguishable from one that is malformed, and a client
      // that guessed would reject the wrong one.
      expect(AuthValidators.token('anything-at-all'), isNull);
      expect(AuthValidators.token(''), isNotNull);
    });
  });

  group('auth error mapping', () {
    test('a failed sign-in is not reported as an ended session', () {
      // The bug this pins. The auth handlers emit no error code, so a 401 from
      // /auth/login arrived at the generic mapper and came out as "Your session
      // has ended. Sign in again to continue." — a claim about the listener's
      // account that the server never made, shown to someone who mistyped.
      final auth = ErrorMapper.describeAuth(
        const ApiError(status: 401, code: '', message: 'invalid credentials'),
      );
      final generic = ErrorMapper.describe(
        const ApiError(status: 401, code: '', message: 'invalid credentials'),
      );

      expect(auth.title, 'That didn’t match');
      expect(auth.message, contains('Check the email and password'));
      expect(generic.title, 'You need to sign in',
          reason: 'the generic mapping is still right for an authenticated call');
    });

    test('an unauthenticated 401 does not offer a retry of the same input', () {
      final auth = ErrorMapper.describeAuth(
        const ApiError(status: 401, code: '', message: 'invalid credentials'),
      );
      expect(auth.retryable, isFalse,
          reason: 'pressing the same button with the same password fails again');
    });

    test('a suspended account points at support, and stays vague', () {
      final auth = ErrorMapper.describeAuth(
        const ApiError(status: 403, code: '', message: 'this account is not available'),
      );
      expect(auth.primaryAction, ErrorAction.contactSupport);
      expect(auth.message, isNot(contains('suspended')),
          reason: 'moderation state is not the caller\'s business (S80)');
    });

    test('a password-policy rejection carries the server\'s own words', () {
      final auth = ErrorMapper.describeAuth(
        const ApiError(
          status: 400,
          code: '',
          message: 'password must be at least 8 characters',
        ),
      );
      expect(auth.message, 'Password must be at least 8 characters');
      expect(auth.retryable, isFalse);
    });

    test('being offline is not a credential problem', () {
      final auth = ErrorMapper.describeAuth(
        const NetworkException('You appear to be offline.'),
      );
      expect(auth.title, 'You’re offline');
      expect(auth.message, contains('Nothing was sent'));
      expect(auth.retryable, isTrue);
    });

    test('a rate limit names the wait and says the account is not locked', () {
      final auth = ErrorMapper.describeAuth(
        const ApiError(
          status: 429,
          code: ErrorCodes.rateLimited,
          message: 'too many attempts',
          retryAfter: Duration(seconds: 45),
        ),
      );
      expect(auth.message, contains('45s'));
      expect(auth.message, contains('not locked'),
          reason: 'the throttle never locks an account (S20), and a listener '
              'who believes theirs is locked will not try again');
      expect(auth.retryable, isTrue);
    });

    test('a 503 during sign-in is described as the server\'s problem', () {
      // The login handler returns 503 when it cannot read the MFA enrolment,
      // because it refuses rather than skipping the second factor.
      final auth = ErrorMapper.describeAuth(
        const ApiError(
            status: 503, code: '', message: 'sign-in is temporarily unavailable'),
      );
      expect(auth.title, contains('unavailable'));
      expect(auth.retryable, isTrue);
    });
  });

  group('auth error codes', () {
    // The auth handlers were fixed to emit stable codes after PHASE 19 shipped.
    // These pin the new path *and* the fallback, because both have to work: a
    // new client against an old server, and an old client against a new one.

    test('a coded 401 is read from the code', () {
      final auth = ErrorMapper.describeAuth(const ApiError(
        status: 401,
        code: ErrorCodes.invalidCredentials,
        message: 'invalid credentials',
      ));
      expect(auth.title, 'That didn’t match');
      expect(auth.primaryAction, ErrorAction.retry);
    });

    test('a 401 with no code still reads correctly against an older server', () {
      final auth = ErrorMapper.describeAuth(
        const ApiError(status: 401, code: '', message: 'invalid credentials'),
      );
      expect(auth.title, 'That didn’t match',
          reason: 'the status fallback is what an un-upgraded server relies on');
    });

    test('a refused account routes to support without naming the reason', () {
      final auth = ErrorMapper.describeAuth(const ApiError(
        status: 403,
        code: ErrorCodes.accountUnavailable,
        message: 'this account is not available',
      ));
      expect(auth.primaryAction, ErrorAction.contactSupport);
      expect(auth.message.toLowerCase(), isNot(contains('suspend')),
          reason: 'the code exists so the message does not have to disclose '
              'moderation state (S80)');
    });

    test('AUTH_TOKEN_INVALID means different things per surface', () {
      const error = ApiError(
        status: 401,
        code: ErrorCodes.tokenInvalid,
        message: 'invalid or expired verification token',
      );

      final onToken = ErrorMapper.describeAuth(error, surface: AuthSurface.token);
      final onCredentials = ErrorMapper.describeAuth(error);

      expect(onToken.title, 'That code has expired');
      expect(onCredentials.title, 'That didn’t match',
          reason: 'one code, two meanings — the caller has to say which '
              'endpoint it was talking to');
    });

    test('a validation failure carries the server\'s rule', () {
      final auth = ErrorMapper.describeAuth(const ApiError(
        status: 400,
        code: ErrorCodes.validationFailed,
        message: 'password must be at least 8 characters',
      ));
      expect(auth.message, 'Password must be at least 8 characters');
    });

    test('an unavailable auth service is retryable', () {
      final auth = ErrorMapper.describeAuth(const ApiError(
        status: 503,
        code: ErrorCodes.authUnavailable,
        message: 'sign-in is temporarily unavailable, try again',
      ));
      expect(auth.retryable, isTrue);
      expect(auth.title, contains('unavailable'));
    });
  });

  group('tokenFromPaste', () {
    // The email highlights a link. Pasting the link instead of the token inside
    // it is the obvious move, and it used to fail with a diagnosis that was
    // wrong: the server answered AUTH_TOKEN_INVALID and the screen said "that
    // code has expired", sending someone hunting for a newer email.

    test('a bare token passes through untouched', () {
      expect(AuthValidators.tokenFromPaste('  tok-1  '), 'tok-1');
    });

    test('the link the email sends yields the token inside it', () {
      expect(
        AuthValidators.tokenFromPaste(
            'https://iconfess.app/verify-email?token=abc123def456'),
        'abc123def456',
      );
    });

    test('a reset link yields its token too', () {
      expect(
        AuthValidators.tokenFromPaste(
            'https://iconfess.app/reset-password?token=reset-tok'),
        'reset-tok',
      );
    });

    test('a link with no token is left alone rather than emptied', () {
      expect(
        AuthValidators.tokenFromPaste('https://iconfess.app/verify-email'),
        'https://iconfess.app/verify-email',
        reason: 'emptying it would turn a confusing paste into an empty field, '
            'which reads as "you typed nothing"',
      );
    });

    test('a link from another origin still yields its token', () {
      expect(
        AuthValidators.tokenFromPaste('https://somewhere.else/v?token=x'),
        'x',
        reason: 'the URL is never opened; the token goes to our own API, so an '
            'origin we do not recognise costs one invalid-token error',
      );
    });

    test('a malformed link still yields its token', () {
      // Written as "an unparseable URL is returned as it was", and it failed:
      // Uri.tryParse accepts almost anything, including 'not a url at all',
      // and reads the query out of it. The expectation was wrong, not the
      // code -- a mangled link that still names its token is better answered
      // than one that is echoed back and rejected.
      expect(AuthValidators.tokenFromPaste('http://%zz?token=x'), 'x');
    });

    test('a bare token containing a query separator is still passed through', () {
      // The `://` guard is what separates a token from a link, because Dart
      // will happily parse either. Tokens are base64url, so this cannot occur
      // in practice; it is here so the guard is not mistaken for a URL check.
      expect(AuthValidators.tokenFromPaste('tok-1'), 'tok-1');
    });
  });
}

/// Reads the server's password policy out of its Go source.
class _GoPolicy {
  const _GoPolicy(this.minLength, this.maxBytes, this.common);
  final int minLength;
  final int maxBytes;
  final Set<String> common;
}

_GoPolicy _readServerPasswordPolicy() {
  final candidates = [
    '../../server/internal/auth/password_policy.go',
    'server/internal/auth/password_policy.go',
  ];
  File? found;
  for (final path in candidates) {
    final file = File(path);
    if (file.existsSync()) {
      found = file;
      break;
    }
  }
  // Failing rather than skipping: a test that cannot find the source it is
  // comparing against has not verified anything, and a silent skip is how a
  // guard disappears without anyone noticing.
  expect(found, isNotNull,
      reason: 'could not find password_policy.go from ${Directory.current.path}');

  final source = found!.readAsStringSync();

  int constant(String name) {
    final match = RegExp('$name = (\\d+)').firstMatch(source);
    expect(match, isNotNull, reason: '$name missing from the server policy');
    return int.parse(match!.group(1)!);
  }

  final block = RegExp(r'commonPasswords = map\[string\]bool\{(.*?)\}', dotAll: true)
      .firstMatch(source);
  expect(block, isNotNull, reason: 'breach blocklist missing from the server policy');
  final common = RegExp(r'"([^"]+)":\s*true')
      .allMatches(block!.group(1)!)
      .map((m) => m.group(1)!)
      .toSet();
  expect(common, isNotEmpty);

  return _GoPolicy(constant('MinPasswordLength'), constant('MaxPasswordLength'), common);
}
