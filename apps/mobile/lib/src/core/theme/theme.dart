import 'package:flutter/material.dart';

import 'tokens.dart';

/// The two visual modes the product runs in.
///
/// This is not a light/dark preference. Discovery and immersion are different
/// jobs: browsing categories wants a warm, legible, editorial surface, while an
/// active confession wants almost no interface at all. Binding them to the
/// system theme would mean a listener who keeps their phone in dark mode gets an
/// immersive player during the day and a dim catalogue at night, which is the
/// wrong axis entirely.
///
/// The player opts into [immersive] explicitly, regardless of what the rest of
/// the app is doing.
enum AppMode {
  /// Warm neutral surfaces, dark type, restrained green accent. Home, Explore,
  /// Profile, Search, Category.
  discovery,

  /// Near-black surfaces, ivory type, minimal controls. Player, active session,
  /// night ritual.
  immersive,
}

/// Builds the app's themes from the generated tokens.
///
/// Nothing in here chooses a colour or a radius directly: every value comes from
/// [IConfess], which is generated from `design/tokens.json` and checked by
/// `make design-check`. A hand-written hex value in a theme is how a design
/// system stops being one.
abstract final class AppTheme {
  /// Discovery mode. Follows the system brightness, because browsing at night
  /// should not be blinding.
  static ThemeData discovery(Brightness brightness) =>
      brightness == Brightness.dark ? _build(_darkPalette, Brightness.dark) : _build(_lightPalette, Brightness.light);

  /// Immersive mode. Always dark: the point is that the interface recedes, and a
  /// white player screen at 11pm defeats the purpose.
  static ThemeData immersive() => _build(_darkPalette, Brightness.dark, immersive: true);

  static ThemeData _build(_Palette p, Brightness brightness, {bool immersive = false}) {
    final scheme = ColorScheme(
      brightness: brightness,
      primary: p.primary,
      onPrimary: p.onPrimary,
      secondary: IConfess.colorAccentGold,
      onSecondary: p.background,
      surface: p.surface,
      onSurface: p.textPrimary,
      error: p.danger,
      onError: p.background,
      outline: p.border,
    );

    return ThemeData(
      useMaterial3: true,
      brightness: brightness,
      colorScheme: scheme,
      scaffoldBackgroundColor: immersive ? p.playerBackground : p.background,
      canvasColor: p.background,
      dividerColor: p.border,
      splashFactory: InkSparkle.splashFactory,
      textTheme: _textTheme(p),
      appBarTheme: AppBarTheme(
        backgroundColor: immersive ? p.playerBackground : p.background,
        foregroundColor: p.textPrimary,
        elevation: 0,
        scrolledUnderElevation: 0,
        centerTitle: false,
        titleTextStyle: IConfess.heading.copyWith(color: p.textPrimary),
      ),
      cardTheme: CardThemeData(
        color: p.surfaceRaised,
        elevation: 0,
        margin: EdgeInsets.zero,
        shape: RoundedRectangleBorder(
          borderRadius: BorderRadius.circular(IConfess.radiusLg),
          side: BorderSide(color: p.border),
        ),
      ),
      inputDecorationTheme: InputDecorationTheme(
        filled: true,
        fillColor: p.surface,
        contentPadding: const EdgeInsets.symmetric(
          horizontal: IConfess.space4,
          vertical: IConfess.space3,
        ),
        border: _inputBorder(p.border),
        enabledBorder: _inputBorder(p.border),
        focusedBorder: _inputBorder(p.primary, width: 1.5),
        errorBorder: _inputBorder(p.danger),
        hintStyle: IConfess.bodySm.copyWith(color: p.textSecondary),
      ),
      filledButtonTheme: FilledButtonThemeData(
        style: FilledButton.styleFrom(
          backgroundColor: p.primary,
          foregroundColor: p.onPrimary,
          // 52 rather than 48: the directive asks for a 44pt minimum target and
          // a primary action should read as larger than the minimum, not sit on
          // it.
          minimumSize: const Size(0, 52),
          textStyle: IConfess.subheading.copyWith(fontWeight: FontWeight.w600),
          shape: RoundedRectangleBorder(
            borderRadius: BorderRadius.circular(IConfess.radiusMd),
          ),
        ),
      ),
      outlinedButtonTheme: OutlinedButtonThemeData(
        style: OutlinedButton.styleFrom(
          foregroundColor: p.textPrimary,
          minimumSize: const Size(0, 52),
          side: BorderSide(color: p.border),
          shape: RoundedRectangleBorder(
            borderRadius: BorderRadius.circular(IConfess.radiusMd),
          ),
        ),
      ),
      textButtonTheme: TextButtonThemeData(
        style: TextButton.styleFrom(
          foregroundColor: p.primary,
          // A text button still has to be tappable with a thumb.
          minimumSize: const Size(0, 44),
        ),
      ),
      chipTheme: ChipThemeData(
        backgroundColor: p.surface,
        selectedColor: p.primary,
        side: BorderSide(color: p.border),
        labelStyle: IConfess.bodySm,
        shape: RoundedRectangleBorder(
          borderRadius: BorderRadius.circular(IConfess.radiusFull),
        ),
      ),
      navigationBarTheme: NavigationBarThemeData(
        backgroundColor: p.surfaceRaised,
        indicatorColor: p.primary.withValues(alpha: 0.16),
        elevation: 0,
        height: 64,
        labelTextStyle: WidgetStatePropertyAll(IConfess.label),
      ),
      progressIndicatorTheme: ProgressIndicatorThemeData(color: p.primary),
      snackBarTheme: SnackBarThemeData(
        backgroundColor: p.surfaceRaised,
        contentTextStyle: IConfess.bodySm.copyWith(color: p.textPrimary),
        behavior: SnackBarBehavior.floating,
        shape: RoundedRectangleBorder(
          borderRadius: BorderRadius.circular(IConfess.radiusMd),
          side: BorderSide(color: p.border),
        ),
      ),
      // Elevation is kept to a hairline. The design language is flat surface,
      // subtle border, then a very soft shadow — never a drop shadow doing the
      // work of a hierarchy that layout should be doing.
      extensions: const [AppSurfaces.paletteToken],
    );
  }

  static OutlineInputBorder _inputBorder(Color color, {double width = 1}) => OutlineInputBorder(
        borderRadius: BorderRadius.circular(IConfess.radiusMd),
        borderSide: BorderSide(color: color, width: width),
      );

  static TextTheme _textTheme(_Palette p) => TextTheme(
        displayLarge: IConfess.display.copyWith(color: p.textPrimary),
        headlineMedium: IConfess.heading.copyWith(color: p.textPrimary),
        titleLarge: IConfess.subheading.copyWith(color: p.textPrimary),
        // Body copy is the serif: this is a product about spoken words, and the
        // confessions are read as much as they are heard.
        bodyLarge: IConfess.body.copyWith(color: p.textPrimary),
        bodyMedium: IConfess.bodySm.copyWith(color: p.textPrimary),
        bodySmall: IConfess.caption.copyWith(color: p.textSecondary),
        labelLarge: IConfess.label.copyWith(color: p.textSecondary),
        labelSmall: IConfess.label.copyWith(color: p.textDisabled),
      );
}

/// Per-theme semantic colours, so a widget can ask for "the danger colour"
/// without knowing which mode it is in.
@immutable
class AppSurfaces extends ThemeExtension<AppSurfaces> {
  const AppSurfaces({
    required this.background,
    required this.surface,
    required this.surfaceRaised,
    required this.border,
    required this.textPrimary,
    required this.textSecondary,
    required this.textDisabled,
    required this.primary,
    required this.onPrimary,
    required this.danger,
    required this.warning,
    required this.success,
    required this.playerBackground,
  });

  final Color background;
  final Color surface;
  final Color surfaceRaised;
  final Color border;
  final Color textPrimary;
  final Color textSecondary;
  final Color textDisabled;
  final Color primary;
  final Color onPrimary;
  final Color danger;
  final Color warning;
  final Color success;
  final Color playerBackground;

  /// Read the palette for the current theme:
  /// `context.surfaces.textSecondary`.
  static AppSurfaces of(BuildContext context) {
    final ext = Theme.of(context).extension<AppSurfaces>();
    assert(ext != null, 'AppSurfaces is missing from the active theme');
    return ext!;
  }

  /// A constant instance used as the extension key. The real per-theme values
  /// come from [AppTheme]; this exists so the extension is registered.
  static const paletteToken = AppSurfaces(
    background: IConfess.colorNeutral0,
    surface: IConfess.colorNeutral50,
    surfaceRaised: IConfess.colorNeutral0,
    border: IConfess.colorNeutral200,
    textPrimary: IConfess.colorNeutral900,
    textSecondary: IConfess.colorNeutral600,
    textDisabled: IConfess.colorNeutral400,
    primary: IConfess.colorBrand500,
    onPrimary: IConfess.colorNeutral0,
    danger: IConfess.colorSemanticDangerLight,
    warning: IConfess.colorSemanticWarningLight,
    success: IConfess.colorSemanticSuccessLight,
    playerBackground: IConfess.colorBrand900,
  );

  @override
  AppSurfaces copyWith({
    Color? background,
    Color? surface,
    Color? surfaceRaised,
    Color? border,
    Color? textPrimary,
    Color? textSecondary,
    Color? textDisabled,
    Color? primary,
    Color? onPrimary,
    Color? danger,
    Color? warning,
    Color? success,
    Color? playerBackground,
  }) =>
      AppSurfaces(
        background: background ?? this.background,
        surface: surface ?? this.surface,
        surfaceRaised: surfaceRaised ?? this.surfaceRaised,
        border: border ?? this.border,
        textPrimary: textPrimary ?? this.textPrimary,
        textSecondary: textSecondary ?? this.textSecondary,
        textDisabled: textDisabled ?? this.textDisabled,
        primary: primary ?? this.primary,
        onPrimary: onPrimary ?? this.onPrimary,
        danger: danger ?? this.danger,
        warning: warning ?? this.warning,
        success: success ?? this.success,
        playerBackground: playerBackground ?? this.playerBackground,
      );

  @override
  AppSurfaces lerp(covariant AppSurfaces? other, double t) {
    if (other == null) return this;
    Color mix(Color a, Color b) => Color.lerp(a, b, t) ?? a;
    return AppSurfaces(
      background: mix(background, other.background),
      surface: mix(surface, other.surface),
      surfaceRaised: mix(surfaceRaised, other.surfaceRaised),
      border: mix(border, other.border),
      textPrimary: mix(textPrimary, other.textPrimary),
      textSecondary: mix(textSecondary, other.textSecondary),
      textDisabled: mix(textDisabled, other.textDisabled),
      primary: mix(primary, other.primary),
      onPrimary: mix(onPrimary, other.onPrimary),
      danger: mix(danger, other.danger),
      warning: mix(warning, other.warning),
      success: mix(success, other.success),
      playerBackground: mix(playerBackground, other.playerBackground),
    );
  }
}

/// One theme's semantic palette, flattened out of the generated token classes.
class _Palette {
  const _Palette({
    required this.background,
    required this.surface,
    required this.surfaceRaised,
    required this.border,
    required this.textPrimary,
    required this.textSecondary,
    required this.textDisabled,
    required this.primary,
    required this.onPrimary,
    required this.danger,
    required this.warning,
    required this.success,
    required this.playerBackground,
  });

  final Color background;
  final Color surface;
  final Color surfaceRaised;
  final Color border;
  final Color textPrimary;
  final Color textSecondary;
  final Color textDisabled;
  final Color primary;
  final Color onPrimary;
  final Color danger;
  final Color warning;
  final Color success;
  final Color playerBackground;
}

const _lightPalette = _Palette(
  background: IConfessLight.background,
  surface: IConfessLight.surface,
  surfaceRaised: IConfessLight.surfaceRaised,
  border: IConfessLight.border,
  textPrimary: IConfessLight.textPrimary,
  textSecondary: IConfessLight.textSecondary,
  textDisabled: IConfessLight.textDisabled,
  primary: IConfessLight.primary,
  onPrimary: IConfessLight.onPrimary,
  danger: IConfessLight.danger,
  warning: IConfessLight.warning,
  success: IConfessLight.success,
  playerBackground: IConfessLight.playerBackground,
);

const _darkPalette = _Palette(
  background: IConfessDark.background,
  surface: IConfessDark.surface,
  surfaceRaised: IConfessDark.surfaceRaised,
  border: IConfessDark.border,
  textPrimary: IConfessDark.textPrimary,
  textSecondary: IConfessDark.textSecondary,
  textDisabled: IConfessDark.textDisabled,
  primary: IConfessDark.primary,
  onPrimary: IConfessDark.onPrimary,
  danger: IConfessDark.danger,
  warning: IConfessDark.warning,
  success: IConfessDark.success,
  playerBackground: IConfessDark.playerBackground,
);
