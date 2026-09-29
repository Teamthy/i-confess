/// Text-to-speech provider metadata returned by the audio API.
import 'package:flutter/foundation.dart';

@immutable
class TtsProvider {
  const TtsProvider({
    required this.name,
    required this.displayName,
    required this.status,
    this.capabilities = const [],
  });

  final String name;
  final String displayName;
  final String status;
  final List<String> capabilities;

  bool get isAvailable => status == 'available';

  factory TtsProvider.fromJson(Map<String, dynamic> json) => TtsProvider(
        name: _string(json, 'name'),
        displayName: _string(json, 'display_name', _string(json, 'displayName')),
        status: _string(json, 'status', 'unavailable'),
        capabilities: (json['capabilities'] as List<dynamic>?)
                ?.whereType<String>()
                .toList(growable: false) ??
            const [],
      );
}

@immutable
class TtsProvidersResponse {
  const TtsProvidersResponse({
    this.providers = const [],
    this.defaultProvider = '',
  });

  final List<TtsProvider> providers;
  final String defaultProvider;

  factory TtsProvidersResponse.fromJson(Map<String, dynamic> json) =>
      TtsProvidersResponse(
        providers: (json['providers'] as List<dynamic>?)
                ?.whereType<Map>()
                .map((item) => TtsProvider.fromJson(
                      Map<String, dynamic>.from(item),
                    ))
                .toList(growable: false) ??
            const [],
        defaultProvider: _string(
          json,
          'default',
          _string(json, 'default_provider', _string(json, 'defaultProvider')),
        ),
      );
}

String _string(Map<String, dynamic> json, String key, [String fallback = '']) {
  final value = json[key];
  return value is String ? value : fallback;
}
