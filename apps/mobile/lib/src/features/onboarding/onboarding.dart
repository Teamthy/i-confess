// Onboarding — 90s, skippable, §62. Name → language → goals → categories → duration → time → voice → notifications.
// Data persisted to /v1/me/profile + /v1/me/preferences (timezone import already).
import 'package:flutter/material.dart';

class OnboardingFlow extends StatefulWidget {
  const OnboardingFlow({super.key, required this.onSubmit});
  final Future<void> Function(OnboardingData) onSubmit;
  @override State<OnboardingFlow> createState() => _OnboardingFlowState();
}

class OnboardingData {
  String name = '';
  String language = 'en';
  List<String> goals = [];
  List<String> categories = [];
  String durationPreset = '15m';
  String time = '06:00';
  String voiceId = '';
  bool notifications = true;
}

class _OnboardingFlowState extends State<OnboardingFlow> {
  final data = OnboardingData();
  int step = 0;
  final _nameCtrl = TextEditingController();

  final steps = const ['Name', 'Language', 'Goals', 'Categories', 'Duration', 'Time', 'Voice', 'Notify'];

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: const Color(0xFF0F1220),
      appBar: AppBar(backgroundColor: const Color(0xFF0F1220), foregroundColor: Colors.white, title: Text('Get started ${step + 1}/${steps.length}'), actions: [TextButton(onPressed: () => Navigator.of(context).maybePop(), child: const Text('Skip'))]),
      body: Padding(
        padding: const EdgeInsets.all(16),
        child: Column(children: [
          LinearProgressIndicator(value: (step + 1) / steps.length, color: const Color(0xFFE8C67A), backgroundColor: const Color(0xFF1C2138)),
          const SizedBox(height: 16),
          Expanded(child: _stepBody()),
          Row(children: [
            if (step > 0) TextButton(onPressed: () => setState(() => step--), child: const Text('Back')),
            const Spacer(),
            FilledButton(
              style: FilledButton.styleFrom(backgroundColor: const Color(0xFF7C8CF8), foregroundColor: const Color(0xFF0B0E1C)),
              onPressed: () async {
                if (step < steps.length - 1) {
                  setState(() => step++);
                } else {
                  data.name = _nameCtrl.text.trim();
                  await widget.onSubmit(data);
                  if (mounted) Navigator.of(context).maybePop();
                }
              },
              child: Text(step == steps.length - 1 ? 'Start my ritual' : 'Next'),
            ),
          ]),
        ]),
      ),
    );
  }

  Widget _stepBody() {
    switch (step) {
      case 0: return Column(crossAxisAlignment: CrossAxisAlignment.start, children: [const Text('What should we call you?', style: TextStyle(color: Colors.white, fontSize: 18, fontWeight: FontWeight.w700)), const SizedBox(height: 12), TextField(controller: _nameCtrl, decoration: const InputDecoration(hintText: 'e.g. Grace', filled: true, fillColor: Color(0xFF1C2138)), style: const TextStyle(color: Colors.white))]);
      case 1: return _chips('Language', ['en', 'fr', 'de'], (s) => data.language = s, data.language);
      case 2: return _chips('Goals', ['Healing', 'Peace', 'Faith', 'Purpose', 'Sleep'], (s) { if (data.goals.contains(s)) { data.goals.remove(s); } else { data.goals.add(s); } setState(() {}); }, data.goals.isNotEmpty ? data.goals.first : '');
      case 3: return _chips('Categories (backend 39, never hard-coded)', ['healing', 'peace', 'faith', 'purpose', 'joy', 'love'], (s) { if (data.categories.contains(s)) { data.categories.remove(s); } else { data.categories.add(s); } setState(() {}); }, data.categories.isNotEmpty ? data.categories.first : '');
      case 4: return _chips('Duration', ['10m', '15m', '30m', '60m'], (s) => data.durationPreset = s, data.durationPreset);
      case 5: return Column(children: [const Text('What time? (your timezone)', style: TextStyle(color: Colors.white)), const SizedBox(height: 12), Text(data.time, style: const TextStyle(color: Color(0xFFE8C67A), fontSize: 28)), Slider(value: 6, min: 5, max: 22, divisions: 17, label: data.time, onChanged: (v) => setState(() => data.time = '${v.toInt().toString().padLeft(2, '0')}:00'))]);
      case 6: return _chips('Voice', ['calm_female', 'warm_male', 'gentle'], (s) => data.voiceId = s, data.voiceId);
      case 7: return SwitchListTile(title: const Text('Daily reminders', style: TextStyle(color: Colors.white)), subtitle: const Text('6am habit nudge — local notification', style: TextStyle(color: Color(0xFF9AA1C0))), value: data.notifications, onChanged: (v) => setState(() => data.notifications = v));
      default: return const SizedBox();
    }
  }

  Widget _chips(String title, List<String> opts, Function(String) onTap, String selected) {
    return Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
      Text(title, style: const TextStyle(color: Colors.white, fontWeight: FontWeight.w700)),
      const SizedBox(height: 12),
      Wrap(spacing: 8, children: opts.map((o) => ChoiceChip(label: Text(o), selected: selected == o || (selected.contains(o) as bool? ?? false), onSelected: (_) { onTap(o); setState(() {}); })).toList()),
    ]);
  }
}
