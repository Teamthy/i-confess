import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:iconfess/src/core/error/error_mapper.dart';
import 'package:iconfess/src/core/theme/theme.dart';
import 'package:iconfess/src/core/theme/tokens.dart';
import 'package:iconfess/src/core/widgets/states.dart';
import 'package:iconfess_api/iconfess_api.dart';

/// Hue in degrees, matching design/test_design.py so the app and the design
/// system cannot disagree about what "green" means.
double hueOf(Color c) {
  // Color.r/g/b are already normalised to 0..1 in current Flutter; the 0-255
  // .red/.green/.blue accessors are the deprecated ones.
  final r = c.r, g = c.g, b = c.b;
  final max = [r, g, b].reduce((a, b) => a > b ? a : b);
  final min = [r, g, b].reduce((a, b) => a < b ? a : b);
  final d = max - min;
  if (d == 0) return 0;
  final h = switch (max) {
    _ when max == r => 60 * (((g - b) / d) % 6),
    _ when max == g => 60 * ((b - r) / d + 2),
    _ => 60 * ((r - g) / d + 4),
  };
  return h < 0 ? h + 360 : h;
}

void main() {
  group('theme', () {
    test('brand primary is green in both modes', () {
      // The generated tokens are guarded by design/test_design.py in CI. This is
      // the same assertion on the app side, so a token change that breaks the
      // brand fails here too rather than only in the design build.
      final hue = hueOf(IConfess.colorBrand500);
      expect(hue, greaterThanOrEqualTo(100));
      expect(hue, lessThanOrEqualTo(170));
      expect(IConfess.colorBrand500, const Color(0xFF2A9D76));
    });

    test('discovery theme takes its colours from the tokens', () {
      final light = AppTheme.discovery(Brightness.light);
      final dark = AppTheme.discovery(Brightness.dark);

      expect(light.colorScheme.primary, IConfessLight.primary);
      expect(light.scaffoldBackgroundColor, IConfessLight.background);
      expect(dark.colorScheme.primary, IConfessDark.primary);
      expect(dark.scaffoldBackgroundColor, IConfessDark.background);
      expect(light.dividerColor, IConfessLight.border);
    });

    test('immersive mode is dark and uses the player surface', () {
      final theme = AppTheme.immersive();
      expect(theme.brightness, Brightness.dark);
      expect(theme.scaffoldBackgroundColor, IConfessDark.playerBackground);
      // Immersive is not the same as "discovery in dark mode": the surfaces
      // differ, which is what makes the transition into the player visible.
      expect(theme.scaffoldBackgroundColor,
          isNot(AppTheme.discovery(Brightness.dark).scaffoldBackgroundColor));
    });

    test('touch targets meet the 44pt minimum', () {
      final theme = AppTheme.discovery(Brightness.light);
      final filled = theme.filledButtonTheme.style?.minimumSize
          ?.resolve(const <WidgetState>{});
      final outlined = theme.outlinedButtonTheme.style?.minimumSize
          ?.resolve(const <WidgetState>{});
      final text = theme.textButtonTheme.style?.minimumSize
          ?.resolve(const <WidgetState>{});

      expect(filled?.height, greaterThanOrEqualTo(44));
      expect(outlined?.height, greaterThanOrEqualTo(44));
      expect(text?.height, greaterThanOrEqualTo(44));
    });

    test('radii come from the token set, not hand-picked values', () {
      // Section 32: 8/12/16/20 and 999 for pills. A stray 17 or 28 is exactly
      // what makes an app look assembled from templates.
      expect(IConfess.radiusSm, 8);
      expect(IConfess.radiusMd, 12);
      expect(IConfess.radiusLg, 16);
      expect(IConfess.radiusXl, 20);
      expect(IConfess.radiusFull, 999);

      final theme = AppTheme.discovery(Brightness.light);
      final card = (theme.cardTheme.shape as RoundedRectangleBorder).borderRadius
          as BorderRadius;
      expect(card.topLeft.x, IConfess.radiusLg);
    });

    test('spacing follows the 4-point scale', () {
      // Section 31 forbids arbitrary values like 13 or 19.
      const scale = [
        IConfess.space0, IConfess.space1, IConfess.space2, IConfess.space3,
        IConfess.space4, IConfess.space5, IConfess.space6, IConfess.space7,
        IConfess.space8, IConfess.space9, IConfess.space10,
      ];
      for (final v in scale) {
        expect(v % 4, 0, reason: '$v is not on the 4-point scale');
      }
    });

    test('both palettes are registered as a theme extension', () {
      for (final theme in [
        AppTheme.discovery(Brightness.light),
        AppTheme.discovery(Brightness.dark),
        AppTheme.immersive(),
      ]) {
        expect(theme.extension<AppSurfaces>(), isNotNull);
      }
    });

    test('body copy uses the serif, interface text the sans', () {
      // Section 30: at most two families, and the split has a reason — the
      // confessions are read as well as heard.
      final theme = AppTheme.discovery(Brightness.light);
      expect(theme.textTheme.bodyLarge?.fontFamily, 'Source Serif 4');
      expect(theme.textTheme.bodyMedium?.fontFamily, 'Inter');
      expect(theme.textTheme.labelLarge?.fontFamily, 'Inter');
    });
  });

  group('error mapping', () {
    test('a network failure explains offline and offers downloads', () {
      final e = ErrorMapper.describe(const NetworkException('socket closed'));
      expect(e.title, contains('offline'));
      expect(e.retryable, isTrue);
      expect(e.primaryAction, ErrorAction.viewDownloads);
    });

    test('an expired session routes to sign-in and says history is safe', () {
      final e = ErrorMapper.describe(const ApiError(
        status: 401,
        code: ErrorCodes.sessionExpired,
        message: 'expired',
      ));
      expect(e.primaryAction, ErrorAction.signIn);
      expect(e.message.toLowerCase(), contains('saved'));
    });

    test('a rate limit names the wait rather than saying "later"', () {
      final e = ErrorMapper.describe(const ApiError(
        status: 429,
        code: ErrorCodes.rateLimited,
        message: 'slow down',
        retryAfter: Duration(seconds: 30),
      ));
      expect(e.message, contains('30s'));
      expect(e.retryable, isTrue);
    });

    test('a 400 does not offer a retry that will fail identically', () {
      final e = ErrorMapper.describe(const ApiError(
        status: 400,
        code: '',
        message: 'bad request',
      ));
      expect(e.retryable, isFalse);
      expect(e.primaryAction, isNot(ErrorAction.retry));
    });

    test('a 500 is retryable', () {
      final e = ErrorMapper.describe(const ApiError(
        status: 503,
        code: '',
        message: 'unavailable',
      ));
      expect(e.retryable, isTrue);
      expect(e.primaryAction, ErrorAction.retry);
    });

    test('an entitlement failure offers the upgrade and a way back', () {
      final e = ErrorMapper.describe(const ApiError(
        status: 403,
        code: ErrorCodes.entitlementRequired,
        message: 'premium',
      ));
      expect(e.primaryAction, ErrorAction.upgrade);
      expect(e.secondaryAction, ErrorAction.goBack);
    });

    test('unpublished audio does not read as a bug', () {
      final e = ErrorMapper.describe(const ApiError(
        status: 409,
        code: ErrorCodes.audioNotPublished,
        message: 'not published',
      ));
      expect(e.title.toLowerCase(), contains('available'));
      // The copy must not read as a fault in the app or in the listener: the
      // recording is simply still being prepared.
      expect(e.message.toLowerCase(), isNot(contains('error')));
      expect(e.message.toLowerCase(), isNot(contains('failed')));
      expect(e.message.toLowerCase(), contains('try'));
    });

    test('no mapped error is the bare "something went wrong"', () {
      // Section 38. Every message must say what to do; a title alone is not
      // enough, and an error with no action at all is only acceptable when there
      // is genuinely nothing to offer.
      final cases = <ApiException>[
        const NetworkException('offline'),
        const ApiError(status: 400, code: '', message: ''),
        const ApiError(status: 401, code: ErrorCodes.sessionExpired, message: ''),
        const ApiError(status: 403, code: ErrorCodes.entitlementRequired, message: ''),
        const ApiError(status: 404, code: ErrorCodes.resourceNotFound, message: ''),
        const ApiError(status: 429, code: ErrorCodes.rateLimited, message: ''),
        const ApiError(status: 500, code: '', message: ''),
        const ApiError(status: 503, code: '', message: ''),
        const ApiError(status: 418, code: 'UNKNOWN_THING', message: ''),
      ];
      for (final c in cases) {
        final e = ErrorMapper.describe(c);
        expect(e.title, isNotEmpty);
        expect(e.message.length, greaterThan(20),
            reason: '${e.title} has no real explanation');
        expect(e.message.toLowerCase(), isNot(contains('something went wrong')));
      }
    });

    test('every action has a button label', () {
      for (final action in ErrorAction.values) {
        expect(ErrorState.labelFor(action), isNotEmpty);
      }
    });
  });
}
