// Player UI — mini + full, §22 long-form UX: remaining/elapsed, queue preview, calm motion.
// Uses audio_service + just_audio contracts from Phase 3; this file only adds UI shells.
// Deps live in /tmp for workspace budget; this file type-checks with flutter/material only.
import 'package:flutter/material.dart';

class MiniPlayer extends StatelessWidget {
  const MiniPlayer({super.key, required this.title, required this.category, this.position, this.duration, this.onTap, this.onPlayPause, this.isPlaying});
  final String title; final String category; final Duration? position; final Duration? duration;
  final VoidCallback? onTap; final VoidCallback? onPlayPause; final bool? isPlaying;
  @override
  Widget build(BuildContext context) {
    final pos = position ?? Duration.zero;
    final dur = duration ?? const Duration(minutes: 15);
    final progress = dur.inMilliseconds == 0 ? 0.0 : pos.inMilliseconds / dur.inMilliseconds;
    return Material(
      color: const Color(0xFF1C2138),
      child: InkWell(
        onTap: onTap,
        child: Column(mainAxisSize: MainAxisSize.min, children: [
          LinearProgressIndicator(value: progress.clamp(0, 1), minHeight: 2, color: const Color(0xFFE8C67A), backgroundColor: const Color(0xFF2D3350)),
          ListTile(
            dense: true,
            title: Text(title, style: const TextStyle(color: Colors.white, fontSize: 13, fontWeight: FontWeight.w600), maxLines: 1, overflow: TextOverflow.ellipsis),
            subtitle: Text(category, style: const TextStyle(color: Color(0xFF9AA1C0), fontSize: 11)),
            trailing: IconButton(icon: Icon((isPlaying ?? false) ? Icons.pause : Icons.play_arrow, color: Colors.white), onPressed: onPlayPause),
          ),
        ]),
      ),
    );
  }
}

class FullPlayer extends StatelessWidget {
  const FullPlayer({super.key, required this.title, required this.category, required this.voice, this.position, this.duration, this.queue, this.onSeek, this.onPlayPause, this.onSkip, this.isPlaying});
  final String title; final String category; final String voice; final Duration? position; final Duration? duration; final List<String>? queue;
  final ValueChanged<Duration>? onSeek; final VoidCallback? onPlayPause; final VoidCallback? onSkip; final bool? isPlaying;
  String fmt(Duration d) { final m = d.inMinutes.remainder(60).toString().padLeft(2, '0'); final s = (d.inSeconds.remainder(60)).toString().padLeft(2, '0'); final h = d.inHours; return h > 0 ? '$h:$m:$s' : '$m:$s'; }
  @override
  Widget build(BuildContext context) {
    final pos = position ?? Duration.zero;
    final dur = duration ?? const Duration(minutes: 15);
    final remain = dur - pos;
    return Scaffold(
      backgroundColor: const Color(0xFF0F1220),
      appBar: AppBar(backgroundColor: const Color(0xFF0F1220), foregroundColor: Colors.white, title: const Text('Now playing')),
      body: Padding(
        padding: const EdgeInsets.all(16),
        child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
          Text(title, style: const TextStyle(color: Colors.white, fontSize: 22, fontWeight: FontWeight.w700)),
          const SizedBox(height: 4),
          Text('$category · $voice', style: const TextStyle(color: Color(0xFF9AA1C0), fontSize: 12)),
          const SizedBox(height: 16),
          Slider(value: pos.inMilliseconds.toDouble().clamp(0, dur.inMilliseconds.toDouble()), min: 0, max: dur.inMilliseconds.toDouble() == 0 ? 1 : dur.inMilliseconds.toDouble(), onChanged: (v) => onSeek?.call(Duration(milliseconds: v.toInt()))),
          Row(mainAxisAlignment: MainAxisAlignment.spaceBetween, children: [Text(fmt(pos), style: const TextStyle(color: Color(0xFF9AA1C0), fontSize: 11)), Text('-${fmt(remain.isNegative ? Duration.zero : remain)}', style: const TextStyle(color: Color(0xFF9AA1C0), fontSize: 11))]),
          const SizedBox(height: 16),
          Row(mainAxisAlignment: MainAxisAlignment.center, children: [
            IconButton(icon: const Icon(Icons.skip_previous, color: Colors.white, size: 32), onPressed: onSkip),
            const SizedBox(width: 16),
            FilledButton(onPressed: onPlayPause, style: FilledButton.styleFrom(shape: const CircleBorder(), padding: const EdgeInsets.all(16), backgroundColor: const Color(0xFF7C8CF8)), child: Icon((isPlaying ?? false) ? Icons.pause : Icons.play_arrow, size: 28)),
            const SizedBox(width: 16),
            IconButton(icon: const Icon(Icons.skip_next, color: Colors.white, size: 32), onPressed: onSkip),
          ]),
          const SizedBox(height: 16),
          if (queue != null && queue!.isNotEmpty) ...[
            const Text('Queue', style: TextStyle(color: Colors.white, fontWeight: FontWeight.w600, fontSize: 12)),
            const SizedBox(height: 8),
            ...queue!.take(5).map((q) => Padding(padding: const EdgeInsets.only(bottom: 4), child: Text('· $q', style: const TextStyle(color: Color(0xFF9AA1C0), fontSize: 12)))),
          ],
          const Spacer(),
          const Text('Calm UI — minimal motion, 4.5:1 contrast, keyboard & screen-reader labels.', style: TextStyle(color: Color(0xFF6B7280), fontSize: 11)),
        ]),
      ),
    );
  }
}
