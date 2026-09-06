# I CONFESS — mobile

Flutter app. Replaces the shell retired in `RETIREMENT.md` (removed with it).

## Layout

```
lib/
  main.dart                     bootstrap: prefs, keystore lookup, ProviderScope
  app.dart                      MaterialApp.router — theme and router only
  src/
    core/
      theme/      tokens.dart (generated, do not edit), theme.dart
      routing/    routes.dart, router.dart
      di/         providers.dart          composition root
      error/      error_mapper.dart       ApiException → copy + recovery
      analytics/  analytics.dart          vocabulary, blocklist, sinks
      persistence/persistence.dart        KV store + keystore adapter
      widgets/    screen.dart, states.dart
    features/
      auth/       auth_controller.dart
      shell/      app_shell.dart, placeholder_screen.dart
```

Feature screens beyond the shell are placeholders that name the phase which
builds them. That is deliberate: a placeholder that looks finished is how a repo
ends up with screens nobody remembers are empty.

## Conventions

- **Design tokens are generated.** Edit `design/tokens.json`, run
  `make design`, never edit `lib/src/core/theme/tokens.dart`. `make design-check`
  fails if the copy in this app drifts from the source.
- **Nothing constructs a client, cache or store inside a widget.** Resolve it
  from a provider so a screen can be tested by overriding one value.
- **Errors go through `ErrorMapper`.** "Something went wrong" is not acceptable
  copy; every failure needs an explanation and, where one exists, a recovery.
- **Secrets go to the keystore, never to `SharedPreferences`.** The token is
  handled by `SecureTokenStore` from `iconfess_api`.
- **Analytics events come from `AnalyticsEvents`.** An unknown name is refused
  rather than sent, because a typo fragments a metric across two names.

## Commands

```
flutter pub get
flutter analyze
flutter test
```

`make mobile-check` from the repository root runs both, and `make verify`
includes it. It reports `SKIPPED` if Flutter is not on `PATH` rather than passing
silently.
