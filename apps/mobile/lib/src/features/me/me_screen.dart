import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../core/routing/routes.dart';
import '../../core/theme/theme.dart';
import '../../core/theme/tokens.dart';
import '../../core/widgets/screen.dart';
import 'me_providers.dart';
import '../../core/di/providers.dart';

/// Me / Profile tab: bootstrap, profile, preferences, interests, sessions, premium, settings.
class MeScreen extends ConsumerWidget {
  const MeScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final bootstrapAsync = ref.watch(bootstrapProvider);
    final surfaces = AppSurfaces.of(context);

    return AppScaffold(
      title: 'Me',
      body: bootstrapAsync.when(
        loading: () => const Center(child: CircularProgressIndicator()),
        error: (e, _) => Center(child: Text('Failed: $e')),
        data: (loadable) {
          final bootstrap = loadable.valueOrNull;
          if (bootstrap == null) {
            return Center(child: Text('Could not load profile'));
          }
          return ListView(
            children: [
              // Header
              Row(
                children: [
                  CircleAvatar(
                    radius: 32,
                    backgroundImage: bootstrap.profile.avatarUrl.isNotEmpty
                        ? NetworkImage(bootstrap.profile.avatarUrl)
                        : null,
                    child: bootstrap.profile.avatarUrl.isEmpty
                        ? Text(bootstrap.profile.displayName.isNotEmpty
                            ? bootstrap.profile.displayName[0].toUpperCase()
                            : 'U')
                        : null,
                  ),
                  const SizedBox(width: IConfess.space4),
                  Expanded(
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        Text(
                          bootstrap.profile.displayName.isNotEmpty
                              ? bootstrap.profile.displayName
                              : bootstrap.account.email,
                          style: IConfess.subheading.copyWith(color: surfaces.textPrimary),
                        ),
                        Text(bootstrap.account.email,
                            style: IConfess.bodySm.copyWith(color: surfaces.textSecondary)),
                        if (bootstrap.completion.total > 0)
                          Padding(
                            padding: const EdgeInsets.only(top: IConfess.space1),
                            child: LinearProgressIndicator(
                              value: bootstrap.completion.fraction,
                              backgroundColor: surfaces.surfaceRaised,
                              color: IConfess.colorBrand500,
                            ),
                          ),
                      ],
                    ),
                  ),
                ],
              ),
              const SizedBox(height: IConfess.space5),
              // Entitlements summary
              Container(
                padding: const EdgeInsets.all(IConfess.space4),
                decoration: BoxDecoration(
                  color: surfaces.surfaceRaised,
                  borderRadius: BorderRadius.circular(IConfess.radiusMd),
                ),
                child: Row(
                  children: [
                    Icon(
                      bootstrap.entitlements.isPremium
                          ? Icons.workspace_premium_rounded
                          : Icons.person_rounded,
                      color: bootstrap.entitlements.isPremium
                          ? IConfess.colorAccentGold
                          : surfaces.textSecondary,
                    ),
                    const SizedBox(width: IConfess.space3),
                    Text('Plan: ${bootstrap.entitlements.plan}',
                        style: IConfess.body.copyWith(color: surfaces.textPrimary)),
                    const Spacer(),
                    Text('${bootstrap.entitlements.maxSessionSeconds ~/ 60} min max',
                        style: IConfess.caption.copyWith(color: surfaces.textSecondary)),
                  ],
                ),
              ),
              const SizedBox(height: IConfess.space5),
              _SectionHeader(label: 'Your library'),
              ListTile(
                leading: const Icon(Icons.collections_bookmark_rounded),
                title: const Text('Library'),
                subtitle: const Text('Collections, favorites, my confessions'),
                trailing: const Icon(Icons.chevron_right_rounded),
                onTap: () => context.go('/library'),
              ),
              ListTile(
                leading: const Icon(Icons.bookmark_rounded),
                title: const Text('Templates'),
                subtitle: const Text('Saved session shapes'),
                trailing: const Icon(Icons.chevron_right_rounded),
                onTap: () => context.go('/templates'),
              ),
              ListTile(
                leading: const Icon(Icons.download_rounded),
                title: const Text('Downloads'),
                subtitle: const Text('Offline content'),
                trailing: const Icon(Icons.chevron_right_rounded),
                onTap: () => context.go(AppRoutes.downloads),
              ),
              const Divider(),
              _SectionHeader(label: 'Account'),
              ListTile(
                leading: const Icon(Icons.edit_rounded),
                title: const Text('Edit profile'),
                trailing: const Icon(Icons.chevron_right_rounded),
                onTap: () => context.go('/me/edit'),
              ),
              ListTile(
                leading: const Icon(Icons.interests_rounded),
                title: const Text('Interests'),
                subtitle: Text(bootstrap.interests.isEmpty
                    ? 'Choose what matters'
                    : '${bootstrap.interests.length} interests'),
                trailing: const Icon(Icons.chevron_right_rounded),
                onTap: () => context.go('/settings/interests'),
              ),
              ListTile(
                leading: const Icon(Icons.workspace_premium_rounded),
                title: const Text('Premium'),
                subtitle: Text(bootstrap.entitlements.isPremium ? 'You are premium' : 'Unlock more'),
                trailing: const Icon(Icons.chevron_right_rounded),
                onTap: () => context.go('/me/premium'),
              ),
              ListTile(
                leading: const Icon(Icons.settings_rounded),
                title: const Text('Settings'),
                trailing: const Icon(Icons.chevron_right_rounded),
                onTap: () => context.go('/me/settings'),
              ),
              const Divider(),
              ListTile(
                leading: const Icon(Icons.logout_rounded),
                title: const Text('Sign out'),
                onTap: () async {
                  await ref.read(authRepositoryProvider).signOut();
                  if (context.mounted) context.go(AppRoutes.welcome);
                },
              ),
            ],
          );
        },
      ),
    );
  }
}

class _SectionHeader extends StatelessWidget {
  const _SectionHeader({required this.label});
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

class ProfileEditScreen extends ConsumerStatefulWidget {
  const ProfileEditScreen({super.key});

  @override
  ConsumerState<ProfileEditScreen> createState() => _ProfileEditScreenState();
}

class _ProfileEditScreenState extends ConsumerState<ProfileEditScreen> {
  final _name = TextEditingController();
  final _bio = TextEditingController();
  bool _saving = false;

  @override
  void dispose() {
    _name.dispose();
    _bio.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final profileAsync = ref.watch(profileProvider);
    final surfaces = AppSurfaces.of(context);

    return AppScaffold(
      title: 'Edit profile',
      body: profileAsync.when(
        loading: () => const Center(child: CircularProgressIndicator()),
        error: (e, _) => Center(child: Text('Failed: $e')),
        data: (loadable) {
          final profile = loadable.valueOrNull;
          if (profile != null && _name.text.isEmpty) {
            _name.text = profile.displayName;
            _bio.text = profile.bio;
          }
          return Column(
            children: [
              TextField(
                controller: _name,
                decoration: const InputDecoration(labelText: 'Display name'),
              ),
              const SizedBox(height: IConfess.space4),
              TextField(
                controller: _bio,
                decoration: const InputDecoration(labelText: 'Bio'),
                maxLines: 3,
              ),
              const SizedBox(height: IConfess.space5),
              FilledButton(
                onPressed: _saving
                    ? null
                    : () async {
                        setState(() => _saving = true);
                        final result = await ref.read(profileRepositoryProvider).updateProfile(
                              displayName: _name.text,
                              bio: _bio.text,
                            );
                        setState(() => _saving = false);
                        result.when(
                          success: (_) {
                            ref.invalidate(profileProvider);
                            ref.invalidate(bootstrapProvider);
                            if (mounted) context.pop();
                          },
                          failure: (e) => ScaffoldMessenger.of(context).showSnackBar(
                            SnackBar(content: Text('Failed: $e')),
                          ),
                        );
                      },
                child: _saving ? const CircularProgressIndicator() : const Text('Save'),
              ),
            ],
          );
        },
      ),
    );
  }
}
