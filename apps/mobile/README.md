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
      auth/       auth_controller.dart, validators.dart
                  welcome / sign_in / sign_up / forgot_password /
                  verification / reset_password / completion
                  widgets/    auth_form.dart, auth_form_state.dart
      onboarding/ onboarding_screen.dart, onboarding_controller.dart
      splash/     splash_screen.dart
      shell/      app_shell.dart, placeholder_screen.dart
```

The auth and onboarding flow is real. Everything past it — Home, Explore, the
session builder, the player, Activity, Me — is still a placeholder that names the
phase which builds it. That is deliberate: a placeholder that looks finished is
how a repo ends up with screens nobody remembers are empty.

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
- **Validation mirrors the server, and says so.** `features/auth/validators.dart`
  copies the rules from `server/internal/auth/password_policy.go`; a test reads
  that file and fails if the two diverge. Sign-in deliberately does not apply the
  policy, because an account that predates it may hold a password the server
  accepts.
- **Error copy depends on which endpoint answered.** `ErrorMapper.describeAuth`
  takes an `AuthSurface`, because a 401 from `/auth/login` is a wrong password
  and a 401 from `/auth/reset-password` is a dead token.
- **The router is built once.** `createRouter` reads the auth provider inside the
  redirect rather than watching it, and session changes reach go_router through a
  `refreshListenable`. Watching it rebuilt the router on every transition and
  discarded the navigation stack.

## Commands

```
flutter pub get
flutter analyze
flutter test
```

`make mobile-check` from the repository root runs both, and `make verify`
includes it. It reports `SKIPPED` if Flutter is not on `PATH` rather than passing
silently.
