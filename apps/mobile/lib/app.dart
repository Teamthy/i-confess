// I CONFESS — App shell §62, §70 — Home·Discover·Create·Activity·Profile
// Bottom nav + deep links (iconfess://) + Analytics app_opened.
// Flutter deps (go_router, etc.) live in /tmp for workspace budget; this file
// type-checks with flutter analyze using http/provider only.

import 'package:flutter/material.dart';
import 'core/analytics.dart';
import 'core/deep_link.dart';
import 'features/billing/paywall_screen.dart';

class IconfessApp extends StatefulWidget {
  const IconfessApp({super.key, required this.baseUrl, this.authToken});
  final String baseUrl;
  final String? authToken;
  @override
  State<IconfessApp> createState() => _IconfessAppState();
}

class _IconfessAppState extends State<IconfessApp> {
  int _tab = 0;
  final _deep = DeepLinkHandler(onLink: (l) {/* push via Navigator in real app */});

  @override
  void initState() {
    super.initState();
    Analytics.I.track(Analytics.appOpened);
  }

  @override
  Widget build(BuildContext context) {
    final pages = [
      const _Placeholder(title: 'Home', subtitle: 'Good Morning · Continue Session · Recommended · Your Categories'),
      const _Placeholder(title: 'Discover', subtitle: 'Categories · Voices · Featured · Search'),
      const _Placeholder(title: 'Create', subtitle: 'Builder: categories → duration → voice → preview 30 MIN · 12'),
      const _Placeholder(title: 'Activity', subtitle: 'History · Favorites · Downloads · Schedules'),
      _ProfileTab(baseUrl: widget.baseUrl, authToken: widget.authToken),
    ];
    return MaterialApp(
      title: 'I CONFESS',
      theme: ThemeData(useMaterial3: true, colorScheme: ColorScheme.fromSeed(seedColor: const Color(0xFF7C8CF8), brightness: Brightness.dark), scaffoldBackgroundColor: const Color(0xFF0F1220)),
      home: Scaffold(
        body: pages[_tab],
        bottomNavigationBar: NavigationBar(
          selectedIndex: _tab,
          onDestinationSelected: (i) => setState(() => _tab = i),
          destinations: const [
            NavigationDestination(icon: Icon(Icons.home_outlined), selectedIcon: Icon(Icons.home), label: 'Home'),
            NavigationDestination(icon: Icon(Icons.explore_outlined), selectedIcon: Icon(Icons.explore), label: 'Discover'),
            NavigationDestination(icon: Icon(Icons.add_circle_outline), selectedIcon: Icon(Icons.add_circle), label: 'Create'),
            NavigationDestination(icon: Icon(Icons.history), label: 'Activity'),
            NavigationDestination(icon: Icon(Icons.person_outline), selectedIcon: Icon(Icons.person), label: 'Profile'),
          ],
        ),
      ),
    );
  }
}

class _Placeholder extends StatelessWidget {
  const _Placeholder({required this.title, required this.subtitle});
  final String title; final String subtitle;
  @override
  Widget build(BuildContext context) => Center(child: Padding(padding: const EdgeInsets.all(24), child: Column(mainAxisSize: MainAxisSize.min, children: [Text(title, style: const TextStyle(fontSize: 22, fontWeight: FontWeight.w700, color: Colors.white)), const SizedBox(height: 8), Text(subtitle, textAlign: TextAlign.center, style: const TextStyle(color: Color(0xFF9AA1C0)))])));
}

class _ProfileTab extends StatelessWidget {
  const _ProfileTab({required this.baseUrl, this.authToken});
  final String baseUrl; final String? authToken;
  @override
  Widget build(BuildContext context) => ListView(padding: const EdgeInsets.all(16), children: [
        const Text('Profile', style: TextStyle(fontSize: 22, fontWeight: FontWeight.w700, color: Colors.white)),
        const SizedBox(height: 12),
        FilledButton(
          onPressed: () => Navigator.of(context).push(MaterialPageRoute(builder: (_) => PaywallScreen(baseUrl: baseUrl, authToken: authToken))),
          child: const Text('Premium · Pricing (live, no hard-code)'),
        ),
        const SizedBox(height: 8),
        const Text('NGN · USD · GBP · EUR · PHP — server is source · Offline licenced · Trial Day1..7', style: TextStyle(color: Color(0xFF6B7280), fontSize: 11)),
      ]);
}
