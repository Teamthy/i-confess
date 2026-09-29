import 'package:flutter_test/flutter_test.dart';
import 'package:iconfess/src/features/minister_voices/minister_voice_repository.dart';

void main() {
  test('a voice without a synthetic flag is still treated as synthetic', () {
    final v = MinisterVoice.fromJson({'id': 'v1', 'name': 'Pastor A', 'language': 'en'});
    expect(v.synthetic, isTrue);
    expect(v.displayName, 'Pastor A');
  });

  test('generation parses status, url and terminal state', () {
    final g = VoiceGeneration.fromJson({
      'generation': {'generationId': 'g1', 'status': 'COMPLETED', 'durationMs': 1200},
      'audioUrl': 'https://cdn.example/a.mp3',
      'disclosure': 'AI-generated using an authorized synthetic voice.',
    });
    expect(g.isTerminal, isTrue);
    expect(g.isPlayable, isTrue);
    expect(g.durationMs, 1200);
  });

  test('a rights-failed or queued job is not playable', () {
    final queued = VoiceGeneration.fromJson({'generation': {'generationId': 'g', 'status': 'QUEUED'}});
    expect(queued.isTerminal, isFalse);
    expect(queued.isPlayable, isFalse);
    final blocked = VoiceGeneration.fromJson({
      'generation': {'generationId': 'g', 'status': 'FAILED', 'errorClass': 'rights'},
    });
    expect(blocked.isTerminal, isTrue);
    expect(blocked.isPlayable, isFalse);
  });
}
