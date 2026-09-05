// Deep links — §4.4
//
// Push sends `iconfess://schedules/{id}/start` (and `https://iconfess.app/schedules/{id}/start`
// as web fallback). This module parses both and routes to the player.
//
// Real `app_links` / `uni_links` lives in /tmp for workspace budget;
// this file exposes a pure parser + a stub handler that works in tests
// and web preview. To enable on device:
//
//  1. Add to pubspec.yaml (in /tmp):
//       app_links: ^6.1.0
//       # or uni_links
//  2. In main.dart: `initDeepLinks(navigatorKey, onScheduleStart)`
//  3. Android: intent-filter for iconfess:// + https, iOS: Associated Domains

import 'package:flutter/foundation.dart';

enum DeepLinkKind { scheduleStart, sessionPlay, templateShare, unknown }

class DeepLink {
  final DeepLinkKind kind;
  final String id;
  final String raw;
  const DeepLink({required this.kind, required this.id, required this.raw});

  static DeepLink? tryParse(String? raw) {
    if (raw == null || raw.isEmpty) return null;
    final uri = Uri.tryParse(raw);
    if (uri == null) return null;

    // iconfess://schedules/{id}/start
    // iconfess://session/{id}
    // https://iconfess.app/schedules/{id}/start
    // https://iconfess.app/session/{id}
    // https://iconfess.app/s/{id} (short)
    final host = uri.host.toLowerCase();
    final scheme = uri.scheme.toLowerCase();
    final segments = uri.pathSegments.where((s) => s.isNotEmpty).toList();

    bool isIconfessHost = scheme == 'iconfess' || host.contains('iconfess.app') || host.contains('iconfess');

    if (!isIconfessHost) return null;

    if (segments.length >= 2 && segments[0] == 'schedules' && segments[1].isNotEmpty) {
      // schedules/{id}/start or schedules/{id}
      return DeepLink(kind: DeepLinkKind.scheduleStart, id: segments[1], raw: raw);
    }
    if (segments.isNotEmpty && segments[0] == 's' && segments.length >= 2) {
      return DeepLink(kind: DeepLinkKind.sessionPlay, id: segments[1], raw: raw);
    }
    if (segments.isNotEmpty && segments[0] == 'session' && segments.length >= 2) {
      return DeepLink(kind: DeepLinkKind.sessionPlay, id: segments[1], raw: raw);
    }
    if (segments.isNotEmpty && segments[0] == 't' && segments.length >= 2) {
      return DeepLink(kind: DeepLinkKind.templateShare, id: segments[1], raw: raw);
    }
    // fallback: last segment as id if it looks like an id
    if (segments.isNotEmpty) {
      final last = segments.last;
      if (last.length >= 6) return DeepLink(kind: DeepLinkKind.unknown, id: last, raw: raw);
    }
    return DeepLink(kind: DeepLinkKind.unknown, id: '', raw: raw);
  }
}

// Stub handler — in production, wire to app_links stream.
class DeepLinkHandler {
  final void Function(DeepLink) onLink;

  DeepLinkHandler({required this.onLink});

  // Call from tests or from push notification tap handler
  void handleRaw(String raw) {
    final link = DeepLink.tryParse(raw);
    if (link == null) {
      debugPrint('deep_link: unparseable $raw');
      return;
    }
    debugPrint('deep_link: $raw → ${link.kind} ${link.id}');
    onLink(link);
  }

  // To enable on device (uncomment when app_links is bundled):
  //
  // StreamSubscription<Uri>? _sub;
  // Future<void> init() async {
  //   final appLinks = AppLinks();
  //   final initial = await appLinks.getInitialLink();
  //   if (initial != null) handleRaw(initial.toString());
  //   _sub = appLinks.uriLinkStream.listen((uri) => handleRaw(uri.toString()),
  //       onError: (e) => debugPrint('deep_link stream error: $e'));
  // }
  // void dispose() => _sub?.cancel();
}
