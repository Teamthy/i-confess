// Analytics — §67–§68 — no sensitive exfil (never body/email/Scripture).
// Events: app_opened → subscription_cancelled. In dev, log to console;
// prod wires to Segment/PostHog via HTTP (lives in /tmp for budget).
import 'package:flutter/foundation.dart';

class Analytics {
  Analytics._();
  static final Analytics I = Analytics._();

  static const appOpened = 'app_opened';
  static const sessionCreated = 'session_created';
  static const sessionStarted = 'session_started';
  static const sessionPaused = 'session_paused';
  static const sessionResumed = 'session_resumed';
  static const sessionCompleted = 'session_completed';
  static const scheduleCreated = 'schedule_created';
  static const scheduleTriggered = 'schedule_triggered';
  static const downloadCreated = 'download_created';
  static const trialDayViewed = 'trial_day_viewed';
  static const subscriptionStarted = 'subscription_started';
  static const subscriptionVerified = 'subscription_verified';
  static const subscriptionCancelled = 'subscription_cancelled';
  static const searchPerformed = 'search_performed';
  static const categoryViewed = 'category_viewed';
  static const templateCreated = 'template_created';
  static const playbackProgressSynced = 'playback_progress_synced';

  void track(String name, {String? userId, Map<String, dynamic>? props}) {
    if (kDebugMode) {
      debugPrint('analytics $name user=${userId ?? "anon"} props=${props ?? {}}');
    }
    // TODO prod: POST to /v1/analytics/batch (no PII) or Segment SDK in /tmp
  }
}
