import Flutter
import UIKit
import UserNotifications

@main
@objc class AppDelegate: FlutterAppDelegate, FlutterImplicitEngineDelegate, FlutterStreamHandler {
  private var pushEvents: FlutterEventSink?
  private var pendingPushTaps: [[String: String]] = []

  override func application(
    _ application: UIApplication,
    didFinishLaunchingWithOptions launchOptions: [UIApplication.LaunchOptionsKey: Any]?
  ) -> Bool {
    UNUserNotificationCenter.current().delegate = self
    return super.application(application, didFinishLaunchingWithOptions: launchOptions)
  }

  func didInitializeImplicitFlutterEngine(_ engineBridge: FlutterImplicitEngineBridge) {
    GeneratedPluginRegistrant.register(with: engineBridge.pluginRegistry)
    // Firebase handles FCM messages, but the server sends directly to APNs on
    // iOS. Those notifications have no gcm.message_id and must not depend on
    // Firebase's tap callback. Buffer a cold-start tap until Dart subscribes.
    if let registrar = engineBridge.pluginRegistry.registrar(forPlugin: "IConfessPushTaps") {
      FlutterEventChannel(name: "app.iconfess/push_taps", binaryMessenger: registrar.messenger())
        .setStreamHandler(self)
    }
  }

  private func directReminder(_ notification: UNNotification) -> [String: String]? {
    let info = notification.request.content.userInfo
    guard info["gcm.message_id"] == nil,
          let data = info["data"] as? [String: String],
          data["deeplink"] != nil else { return nil }
    return data
  }

  override func userNotificationCenter(
    _ center: UNUserNotificationCenter,
    willPresent notification: UNNotification,
    withCompletionHandler completionHandler: @escaping (UNNotificationPresentationOptions) -> Void
  ) {
    if directReminder(notification) != nil {
      completionHandler([.banner, .list, .sound])
      return
    }
    super.userNotificationCenter(center, willPresent: notification, withCompletionHandler: completionHandler)
  }

  override func userNotificationCenter(
    _ center: UNUserNotificationCenter,
    didReceive response: UNNotificationResponse,
    withCompletionHandler completionHandler: @escaping () -> Void
  ) {
    if response.actionIdentifier == UNNotificationDefaultActionIdentifier,
       let data = directReminder(response.notification) {
      if let sink = pushEvents {
        sink(data)
      } else {
        pendingPushTaps.append(data)
        pendingPushTaps = Array(pendingPushTaps.suffix(10))
      }
      completionHandler()
      return
    }
    super.userNotificationCenter(center, didReceive: response, withCompletionHandler: completionHandler)
  }

  func onListen(withArguments arguments: Any?, eventSink events: @escaping FlutterEventSink) -> FlutterError? {
    pushEvents = events
    for tap in pendingPushTaps { events(tap) }
    pendingPushTaps.removeAll()
    return nil
  }

  func onCancel(withArguments arguments: Any?) -> FlutterError? {
    pushEvents = nil
    return nil
  }
}
