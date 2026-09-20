import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../core/routing/routes.dart';
import '../../core/theme/theme.dart';
import '../../core/theme/tokens.dart';
import '../../core/widgets/screen.dart';
import 'settings_providers.dart';
import '../../core/di/providers.dart';
import '../../core/push/push_registration.dart';

/// Settings: preferences, notifications, devices, export, deletion, interests.
class SettingsScreen extends ConsumerWidget {
  const SettingsScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final surfaces = AppSurfaces.of(context);
    return AppScaffold(
      title: 'Settings',
      body: ListView(
        children: [
          _Section(label: 'Preferences'),
          ListTile(
            leading: const Icon(Icons.tune_rounded),
            title: const Text('Playback & notifications'),
            trailing: const Icon(Icons.chevron_right_rounded),
            onTap: () => context.go('/settings/preferences'),
          ),
          ListTile(
            leading: const Icon(Icons.interests_rounded),
            title: const Text('Interests'),
            trailing: const Icon(Icons.chevron_right_rounded),
            onTap: () => context.go('/settings/interests'),
          ),
          ListTile(
            leading: const Icon(Icons.notifications_rounded),
            title: const Text('Notifications'),
            trailing: const Icon(Icons.chevron_right_rounded),
            onTap: () => context.go('/settings/notifications'),
          ),
          const Divider(),
          _Section(label: 'Devices & security'),
          ListTile(
            leading: const Icon(Icons.devices_rounded),
            title: const Text('Devices'),
            trailing: const Icon(Icons.chevron_right_rounded),
            onTap: () => context.go('/settings/devices'),
          ),
          ListTile(
            leading: const Icon(Icons.security_rounded),
            title: const Text('Security'),
            trailing: const Icon(Icons.chevron_right_rounded),
            onTap: () => context.go('/settings/security'),
          ),
          const Divider(),
          _Section(label: 'Data'),
          ListTile(
            leading: const Icon(Icons.download_rounded),
            title: const Text('Export data'),
            onTap: () async {
              try {
                final json = await ref.read(apiClientProvider).getMeExport();
                if (context.mounted) {
                  ScaffoldMessenger.of(context).showSnackBar(
                    SnackBar(content: Text('Export ready: ${json['url'] ?? 'check email'}')),
                  );
                }
              } catch (e) {
                if (context.mounted) {
                  ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text('Failed: $e')));
                }
              }
            },
          ),
          ListTile(
            leading: Icon(Icons.delete_forever_rounded, color: IConfess.colorSemanticDangerLight),
            title: Text('Delete account', style: TextStyle(color: IConfess.colorSemanticDangerLight)),
            onTap: () => context.go('/settings/deletion'),
          ),
        ],
      ),
    );
  }
}

class _Section extends StatelessWidget {
  const _Section({required this.label});
  final String label;

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: IConfess.space2),
      child: Text(label,
          style: IConfess.label.copyWith(
            color: AppSurfaces.of(context).textSecondary,
            letterSpacing: 0.8,
          )),
    );
  }
}

class PreferencesScreen extends ConsumerWidget {
  const PreferencesScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final async = ref.watch(preferencesProvider);
    return AppScaffold(
      title: 'Preferences',
      body: async.when(
        loading: () => const Center(child: CircularProgressIndicator()),
        error: (e, _) => Center(child: Text('Failed: $e')),
        data: (loadable) {
          final prefs = loadable.valueOrNull;
          if (prefs == null) return const Center(child: Text('No preferences'));
          return ListView(
            children: [
              SwitchListTile(
                title: const Text('Autoplay'),
                value: prefs.autoplay,
                onChanged: (v) async {
                  await ref.read(profileRepositoryProvider).updatePreferences({'autoplay': v});
                  ref.invalidate(preferencesProvider);
                },
              ),
              SwitchListTile(
                title: const Text('Notifications'),
                value: prefs.notificationsEnabled,
                onChanged: (v) async {
                  await ref.read(profileRepositoryProvider).updatePreferences({'notifications_enabled': v});
                  ref.invalidate(preferencesProvider);
                },
              ),
              SwitchListTile(
                title: const Text('Recommendations'),
                value: prefs.recommendationsEnabled,
                onChanged: (v) async {
                  await ref.read(profileRepositoryProvider).updatePreferences({'recommendations_enabled': v});
                  ref.invalidate(preferencesProvider);
                },
              ),
              ListTile(
                title: const Text('Default duration'),
                subtitle: Text('${prefs.defaultDuration ~/ 60} min'),
              ),
              ListTile(
                title: const Text('Theme'),
                subtitle: Text(prefs.theme),
              ),
            ],
          );
        },
      ),
    );
  }
}

class InterestsScreen extends ConsumerStatefulWidget {
  const InterestsScreen({super.key});

  @override
  ConsumerState<InterestsScreen> createState() => _InterestsScreenState();
}

class _InterestsScreenState extends ConsumerState<InterestsScreen> {
  final Set<String> _selected = {};

  @override
  Widget build(BuildContext context) {
    final categoriesAsync = ref.watch(contentRepositoryProvider).categories();
    return AppScaffold(
      title: 'Interests',
      body: FutureBuilder(
        future: categoriesAsync,
        builder: (context, snapshot) {
          final data = snapshot.data;
          final categories = data?.valueOrNull ?? [];
          if (categories.isEmpty) return const Center(child: CircularProgressIndicator());
          return Column(
            children: [
              Expanded(
                child: Wrap(
                  spacing: IConfess.space2,
                  runSpacing: IConfess.space2,
                  children: [
                    for (final cat in categories)
                      FilterChip(
                        label: Text(cat.name),
                        selected: _selected.contains(cat.id),
                        onSelected: (sel) {
                          setState(() {
                            if (sel) {
                              _selected.add(cat.id);
                            } else {
                              _selected.remove(cat.id);
                            }
                          });
                        },
                      ),
                  ],
                ),
              ),
              FilledButton(
                onPressed: () async {
                  final result = await ref.read(profileRepositoryProvider).setInterests(_selected.toList());
                  result.when(
                    success: (_) {
                      if (mounted) context.pop();
                    },
                    failure: (e) => ScaffoldMessenger.of(context).showSnackBar(
                      SnackBar(content: Text('Failed: $e')),
                    ),
                  );
                },
                child: const Text('Save interests'),
              ),
            ],
          );
        },
      ),
    );
  }
}

class NotificationsScreen extends ConsumerWidget {
  const NotificationsScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final async = ref.watch(notificationPrefsProvider);
    return AppScaffold(
      title: 'Notifications',
      body: async.when(
        loading: () => const Center(child: CircularProgressIndicator()),
        error: (e, _) => Center(child: Text('Failed: $e')),
        data: (loadable) {
          final data = loadable.valueOrNull ?? {};
          return ListView(
            children: [
              SwitchListTile(
                title: const Text('Scheduled reminders'),
                value: data['scheduled'] == true,
                onChanged: (v) async {
                  await ref.read(apiClientProvider).patchMeNotifications({'scheduled': v});
                  ref.invalidate(notificationPrefsProvider);
                },
              ),
              SwitchListTile(
                title: const Text('Content updates'),
                value: data['content'] == true,
                onChanged: (v) async {
                  await ref.read(apiClientProvider).patchMeNotifications({'content': v});
                  ref.invalidate(notificationPrefsProvider);
                },
              ),
            ],
          );
        },
      ),
    );
  }
}

class DevicesScreen extends ConsumerWidget {
  const DevicesScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final async = ref.watch(devicesProvider);
    return AppScaffold(
      title: 'Devices',
      body: async.when(
        loading: () => const Center(child: CircularProgressIndicator()),
        error: (e, _) => Center(child: Text('Failed: $e')),
        data: (loadable) {
          final devices = loadable.valueOrNull ?? [];
          if (devices.isEmpty) return const Center(child: Text('No devices'));
          return ListView.builder(
            itemCount: devices.length,
            itemBuilder: (context, i) {
              final d = devices[i] as Map;
              return ListTile(
                title: Text(d['platform']?.toString() ?? 'Device'),
                subtitle: Text(d['id']?.toString() ?? ''),
                trailing: IconButton(
                  icon: const Icon(Icons.delete_outline_rounded),
                  onPressed: () async {
                    await ref.read(apiClientProvider).deleteMeDevicesById(d['id'].toString());
                    ref.invalidate(devicesProvider);
                  },
                ),
              );
            },
          );
        },
      ),
    );
  }
}

class DeletionScreen extends ConsumerStatefulWidget {
  const DeletionScreen({super.key});

  @override
  ConsumerState<DeletionScreen> createState() => _DeletionScreenState();
}

class _DeletionScreenState extends ConsumerState<DeletionScreen> {
  final _confirm = TextEditingController();
  bool _deleting = false;

  @override
  void dispose() {
    _confirm.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final surfaces = AppSurfaces.of(context);
    return AppScaffold(
      title: 'Delete account',
      body: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text('This will permanently delete your account and all data.',
              style: IConfess.body.copyWith(color: IConfess.colorSemanticDangerLight)),
          const SizedBox(height: IConfess.space4),
          Text('Type DELETE to confirm',
              style: IConfess.bodySm.copyWith(color: surfaces.textSecondary)),
          const SizedBox(height: IConfess.space2),
          TextField(
            controller: _confirm,
            decoration: const InputDecoration(
              hintText: 'DELETE',
              border: OutlineInputBorder(),
            ),
          ),
          const SizedBox(height: IConfess.space5),
          FilledButton(
            style: FilledButton.styleFrom(backgroundColor: IConfess.colorSemanticDangerLight),
            onPressed: _confirm.text == 'DELETE' && !_deleting
                ? () async {
                    setState(() => _deleting = true);
                    try {
                      await ref.read(apiClientProvider).postMeDeletion({'confirm': 'DELETE'});
                      if (mounted) context.go(AppRoutes.welcome);
                    } catch (e) {
                      if (mounted) {
                        ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text('Failed: $e')));
                      }
                    } finally {
                      setState(() => _deleting = false);
                    }
                  }
                : null,
            child: _deleting ? const CircularProgressIndicator() : const Text('Delete account'),
          ),
        ],
      ),
    );
  }
}
