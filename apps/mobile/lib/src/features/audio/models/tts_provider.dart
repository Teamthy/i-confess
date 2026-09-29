/// TTS (Text-to-Speech) provider model.
///
/// Represents an available TTS provider for audio generation.
import 'package:freezed_annotation/freezed_annotation.dart';

part 'tts_provider.freezed.dart';
part 'tts_provider.g.dart';

/// TTS provider model.
@freezed
class TtsProvider with _$TtsProvider {
  const factory TtsProvider({
    required String name,
    required String displayName,
    required String status,
    @Default([]) List<String> capabilities,
  }) = _TtsProvider;

  factory TtsProvider.fromJson(Map<String, dynamic> json) =>
      _$TtsProviderFromJson(json);
}

/// Response for listing TTS providers.
@freezed
class TtsProvidersResponse with _$TtsProvidersResponse {
  const factory TtsProvidersResponse({
    @Default([]) List<TtsProvider> providers,
    required String defaultProvider,
  }) = _TtsProvidersResponse;

  factory TtsProvidersResponse.fromJson(Map<String, dynamic> json) =>
      _$TtsProvidersResponseFromJson(json);
}
