import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:iconfess_api/iconfess_api.dart';
import 'package:just_audio/just_audio.dart';

import '../../core/di/providers.dart';
import '../../core/theme/theme.dart';
import '../../core/theme/tokens.dart';
import '../../core/widgets/screen.dart';
import 'minister_voice_repository.dart';

final ministerVoiceRepositoryProvider = Provider<MinisterVoiceRepository>(
  (ref) => MinisterVoiceRepository(ref.watch(apiClientProvider)),
);

final ministerVoiceLibraryProvider = FutureProvider.autoDispose<MinisterVoiceLibrary>(
  (ref) => ref.watch(ministerVoiceRepositoryProvider).list(),
);

/// Listener access to licensed minister voices. Every render is queued on
/// the server (rights are checked there each time), then played from a signed
/// URL. Every place a voice or a render appears carries the synthetic label.
class MinisterVoicesScreen extends ConsumerWidget {
  const MinisterVoicesScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final async = ref.watch(ministerVoiceLibraryProvider);
    final surfaces = AppSurfaces.of(context);
    return AppScaffold(
      title: 'Minister voices',
      body: async.when(
        loading: () => const Center(child: CircularProgressIndicator()),
        error: (e, _) => Center(child: Text("Couldn't load voices. ${_message(e)}")),
        data: (lib) => Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            SyntheticLabel(text: lib.disclosure),
            const SizedBox(height: IConfess.space3),
            if (lib.voices.isEmpty)
              Text('No minister voices are available right now.',
                  style: IConfess.body.copyWith(color: surfaces.textSecondary)),
            for (final v in lib.voices)
              ListTile(
                contentPadding: EdgeInsets.zero,
                leading: const Icon(Icons.record_voice_over_rounded),
                title: Text(v.displayName),
                subtitle: Text(
                  [v.locale.isNotEmpty ? v.locale : v.language, if (v.accent.isNotEmpty) v.accent, 'AI voice']
                      .join(' • '),
                  style: IConfess.caption.copyWith(color: surfaces.textSecondary),
                ),
                trailing: const Icon(Icons.chevron_right_rounded),
                onTap: () => Navigator.of(context).push(
                  MaterialPageRoute<void>(builder: (_) => MinisterVoiceComposer(voice: v)),
                ),
              ),
          ],
        ),
      ),
    );
  }
}

/// The disclosure shown next to minister voices and synthetic renders.
class SyntheticLabel extends StatelessWidget {
  const SyntheticLabel({required this.text, super.key});
  final String text;

  @override
  Widget build(BuildContext context) {
    final surfaces = AppSurfaces.of(context);
    return Container(
      width: double.infinity,
      padding: const EdgeInsets.all(IConfess.space3),
      decoration: BoxDecoration(
        color: IConfess.colorSemanticWarningLight.withValues(alpha: 0.12),
        borderRadius: BorderRadius.circular(IConfess.radiusMd),
      ),
      child: Row(children: [
        Icon(Icons.auto_awesome_rounded, size: 18, color: surfaces.textSecondary),
        const SizedBox(width: IConfess.space2),
        Expanded(child: Text(text, style: IConfess.bodySm.copyWith(color: surfaces.textPrimary))),
      ]),
    );
  }
}

class MinisterVoiceComposer extends ConsumerStatefulWidget {
  const MinisterVoiceComposer({required this.voice, super.key});
  final MinisterVoice voice;

  @override
  ConsumerState<MinisterVoiceComposer> createState() => _ComposerState();
}

class _ComposerState extends ConsumerState<MinisterVoiceComposer> {
  final _text = TextEditingController();
  final _player = AudioPlayer();
  List<String> _styles = const [];
  String _style = '';
  VoiceGeneration? _gen;
  String? _error;
  bool _busy = false;

  @override
  void initState() {
    super.initState();
    unawaited(_loadStyles());
  }

  Future<void> _loadStyles() async {
    try {
      final s = await ref.read(ministerVoiceRepositoryProvider).styles(widget.voice.id);
      if (mounted) setState(() => _styles = s);
    } catch (_) {
      // Styles are optional; the server falls back to the default reference.
    }
  }

  Future<void> _generate() async {
    final text = _text.text.trim();
    if (text.isEmpty) return;
    final repo = ref.read(ministerVoiceRepositoryProvider);
    setState(() {
      _busy = true;
      _error = null;
      _gen = null;
    });
    try {
      final language = widget.voice.locale.isNotEmpty ? widget.voice.locale : widget.voice.language;
      var g = await repo.generate(voiceId: widget.voice.id, text: text, language: language, style: _style);
      if (mounted) setState(() => _gen = g);
      g = await repo.waitFor(g);
      if (!mounted) return;
      setState(() => _gen = g);
      if (g.isPlayable) {
        await _player.setUrl(g.audioUrl!);
        unawaited(_player.play());
      } else if (!g.isTerminal) {
        setState(() => _error = 'Still rendering. Check back shortly.');
      } else {
        setState(() => _error = _jobMessage(g));
      }
    } catch (e) {
      if (mounted) setState(() => _error = _message(e));
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  void dispose() {
    final g = _gen;
    if (g != null && !g.isTerminal) {
      // Best effort: don't keep a GPU busy for a screen nobody is looking at.
      unawaited(ref.read(ministerVoiceRepositoryProvider).cancel(g.id).catchError((_) {}));
    }
    _player.dispose();
    _text.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final surfaces = AppSurfaces.of(context);
    final g = _gen;
    return AppScaffold(
      title: widget.voice.displayName,
      body: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          const SyntheticLabel(
            text: 'AI-generated using an authorized synthetic voice. '
                'This is not a recording of the minister speaking these words.',
          ),
          const SizedBox(height: IConfess.space4),
          TextField(
            controller: _text,
            minLines: 3,
            maxLines: 8,
            maxLength: 2000,
            decoration: const InputDecoration(labelText: 'Confession text'),
          ),
          if (_styles.isNotEmpty) ...[
            const SizedBox(height: IConfess.space2),
            Wrap(spacing: IConfess.space2, children: [
              for (final s in _styles)
                ChoiceChip(
                  label: Text(s),
                  selected: _style == s,
                  onSelected: (on) => setState(() => _style = on ? s : ''),
                ),
            ]),
          ],
          const SizedBox(height: IConfess.space4),
          FilledButton.icon(
            onPressed: _busy ? null : _generate,
            icon: _busy
                ? const SizedBox(width: 16, height: 16, child: CircularProgressIndicator(strokeWidth: 2))
                : const Icon(Icons.graphic_eq_rounded),
            label: Text(_busy ? 'Rendering…' : 'Generate'),
          ),
          const SizedBox(height: IConfess.space3),
          if (g != null)
            Text('Status: ${g.status.toLowerCase()}',
                style: IConfess.caption.copyWith(color: surfaces.textSecondary)),
          if (_error != null)
            Padding(
              padding: const EdgeInsets.only(top: IConfess.space2),
              child: Text(_error!, style: IConfess.bodySm.copyWith(color: surfaces.textPrimary)),
            ),
          if (g != null && g.isPlayable) ...[
            const SizedBox(height: IConfess.space3),
            SyntheticLabel(text: g.disclosure),
            StreamBuilder<bool>(
              stream: _player.playingStream,
              builder: (context, snap) => IconButton(
                iconSize: 48,
                icon: Icon(snap.data == true ? Icons.pause_circle_rounded : Icons.play_circle_rounded),
                onPressed: () => snap.data == true ? _player.pause() : _player.play(),
              ),
            ),
          ],
        ],
      ),
    );
  }
}

String _jobMessage(VoiceGeneration g) => switch (g.errorClass) {
      'rights' => "This voice isn't available for this use right now.",
      'content' => "This text can't be rendered in a minister voice.",
      _ when g.status.toUpperCase() == 'CANCELLED' => 'Cancelled.',
      _ => 'Rendering failed. Please try again.',
    };

String _message(Object e) => switch (e) {
      NetworkException() => 'Check your connection.',
      ApiError(status: 403) => "This voice isn't available for this use.",
      ApiError(status: 503) => 'Voice rendering is temporarily unavailable.',
      ApiError(:final message) => message,
      _ => '',
    };
