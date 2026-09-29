import 'package:iconfess_api/iconfess_api.dart';

/// A licensed minister voice as the listener sees it. Carries no rights or
/// legal detail; the server decides what may be generated.
class MinisterVoice {
  const MinisterVoice({
    required this.id,
    required this.displayName,
    required this.language,
    this.locale = '',
    this.accent = '',
    this.description = '',
    this.sampleUrl = '',
    this.synthetic = true,
  });

  factory MinisterVoice.fromJson(Map<String, dynamic> j) => MinisterVoice(
        id: j['id'] as String? ?? '',
        displayName: (j['displayName'] as String?)?.trim().isNotEmpty == true
            ? j['displayName'] as String
            : (j['name'] as String? ?? ''),
        language: j['language'] as String? ?? '',
        locale: j['locale'] as String? ?? '',
        accent: j['accent'] as String? ?? '',
        description: j['description'] as String? ?? '',
        sampleUrl: j['sampleUrl'] as String? ?? '',
        // Default to true: a missing flag must never make audio look real.
        synthetic: j['synthetic'] as bool? ?? true,
      );

  final String id;
  final String displayName;
  final String language;
  final String locale;
  final String accent;
  final String description;
  final String sampleUrl;
  final bool synthetic;
}

class MinisterVoiceLibrary {
  const MinisterVoiceLibrary(this.voices, this.disclosure);
  final List<MinisterVoice> voices;
  final String disclosure;
}

/// The status of one generation job (`/audio/jobs/{id}`).
class VoiceGeneration {
  const VoiceGeneration({
    required this.id,
    required this.status,
    required this.disclosure,
    this.audioUrl,
    this.error = '',
    this.errorClass = '',
    this.durationMs = 0,
  });

  factory VoiceGeneration.fromJson(Map<String, dynamic> body) {
    final g = (body['generation'] as Map?)?.cast<String, dynamic>() ?? const {};
    return VoiceGeneration(
      id: g['generationId'] as String? ?? '',
      status: g['status'] as String? ?? 'QUEUED',
      audioUrl: body['audioUrl'] as String?,
      error: g['error'] as String? ?? '',
      errorClass: g['errorClass'] as String? ?? '',
      durationMs: (g['durationMs'] as num?)?.toInt() ?? 0,
      disclosure: body['disclosure'] as String? ?? 'AI-generated using an authorized synthetic voice.',
    );
  }

  final String id;
  final String status;
  final String? audioUrl;
  final String error;
  final String errorClass;
  final int durationMs;
  final String disclosure;

  static const terminal = {'COMPLETED', 'FAILED', 'CANCELLED'};
  bool get isTerminal => terminal.contains(status.toUpperCase());
  bool get isPlayable => status.toUpperCase() == 'COMPLETED' && (audioUrl?.isNotEmpty ?? false);
}

/// Listener access to the minister voice library. Inference never happens on
/// the device: it queues a server job and plays the signed result.
class MinisterVoiceRepository {
  MinisterVoiceRepository(this._api, {this.prefix = '/v1'});

  final ApiClient _api;
  final String prefix;

  Future<MinisterVoiceLibrary> list({String? language}) async {
    final body = await _api.get('$prefix/minister-voices',
        query: language == null ? null : {'language': language});
    final raw = (body['voices'] as List?) ?? const [];
    return MinisterVoiceLibrary(
      raw.whereType<Map>().map((m) => MinisterVoice.fromJson(m.cast<String, dynamic>())).toList(),
      body['disclosure'] as String? ?? 'Minister voices are AI-generated using an authorized synthetic voice.',
    );
  }

  Future<List<String>> styles(String voiceId) async {
    final body = await _api.get('$prefix/voices/${Uri.encodeComponent(voiceId)}/styles');
    return ((body['styles'] as List?) ?? const []).whereType<String>().toList();
  }

  Future<VoiceGeneration> generate({
    required String voiceId,
    required String text,
    required String language,
    String style = '',
    String purpose = 'confession',
  }) async {
    final body = await _api.post('$prefix/voices/generate', {
      'voiceId': voiceId,
      'text': text,
      'language': language,
      if (style.isNotEmpty) 'style': style,
      'purpose': purpose,
    });
    return VoiceGeneration.fromJson(body);
  }

  Future<VoiceGeneration> job(String id) async =>
      VoiceGeneration.fromJson(await _api.get('$prefix/audio/jobs/${Uri.encodeComponent(id)}'));

  Future<void> cancel(String id) => _api.post('$prefix/audio/jobs/${Uri.encodeComponent(id)}/cancel');

  /// Polls until the job ends or [timeout] passes. Backs off up to 5s so a
  /// long queue does not become a request storm.
  Future<VoiceGeneration> waitFor(
    VoiceGeneration g, {
    Duration timeout = const Duration(minutes: 3),
    Future<void> Function(Duration) sleep = _sleep,
  }) async {
    var current = g;
    var delay = const Duration(milliseconds: 800);
    final deadline = DateTime.now().add(timeout);
    while (!current.isTerminal && DateTime.now().isBefore(deadline)) {
      await sleep(delay);
      current = await job(current.id);
      final next = delay * 1.5;
      delay = next > const Duration(seconds: 5) ? const Duration(seconds: 5) : next;
    }
    return current;
  }

  static Future<void> _sleep(Duration d) => Future<void>.delayed(d);
}
