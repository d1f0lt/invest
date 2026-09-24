import 'package:flutter/material.dart';

import '../api/api_client.dart';
import '../api/auth_api.dart';
import '../api/session.dart';
import '../auth/login_screen.dart';
import '../auth/widgets.dart';
import '../portfolio/portfolio_store.dart';

class ProfileTab extends StatefulWidget {
  const ProfileTab({super.key});

  @override
  State<ProfileTab> createState() => _ProfileTabState();
}

class _ProfileTabState extends State<ProfileTab> {
  final _api = AuthApi();
  late Future<UserProfile> _profile = _load();
  bool _loggingOut = false;

  Future<UserProfile> _load() => _api.me();

  void _toLogin() {
    Navigator.of(context).pushAndRemoveUntil(fadeRoute(const LoginScreen()), (_) => false);
  }

  Future<void> _logout() async {
    setState(() => _loggingOut = true);
    final refreshToken = await Session.instance.clear();
    PortfolioStore.instance.reset();
    if (refreshToken != null) {
      try {
        await _api.logout(refreshToken);
      } on ApiException {
        // Локально уже вышли; если сервер недоступен, токен просто истечёт сам.
      }
    }
    if (mounted) _toLogin();
  }

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    final textTheme = Theme.of(context).textTheme;

    return Scaffold(
      appBar: AppBar(title: const Text('Профиль')),
      body: ListView(
        padding: const EdgeInsets.all(24),
        children: [
          FutureBuilder<UserProfile>(
            future: _profile,
            builder: (context, snapshot) {
              if (snapshot.connectionState != ConnectionState.done) {
                return const Padding(
                  padding: EdgeInsets.symmetric(vertical: 40),
                  child: Center(child: CircularProgressIndicator()),
                );
              }
              if (snapshot.hasError) {
                final error = snapshot.error;
                final message = error is ApiException ? error.message : 'Не удалось загрузить профиль';
                return Column(
                  children: [
                    Text(message, textAlign: TextAlign.center),
                    const SizedBox(height: 12),
                    if (error is ApiException && error.statusCode == 401)
                      FilledButton(onPressed: _toLogin, child: const Text('Войти заново'))
                    else
                      OutlinedButton(
                        onPressed: () => setState(() => _profile = _load()),
                        child: const Text('Повторить'),
                      ),
                  ],
                );
              }
              final user = snapshot.data!;
              final name = user.username ?? user.email;
              return Column(
                children: [
                  CircleAvatar(
                    radius: 44,
                    backgroundColor: scheme.primaryContainer,
                    child: Text(
                      name.characters.first.toUpperCase(),
                      style: textTheme.headlineMedium?.copyWith(color: scheme.onPrimaryContainer),
                    ),
                  ),
                  const SizedBox(height: 16),
                  Text(name, style: textTheme.titleLarge),
                  if (user.username != null) ...[
                    const SizedBox(height: 4),
                    Text(user.email, style: textTheme.bodyMedium?.copyWith(color: scheme.onSurfaceVariant)),
                  ],
                ],
              );
            },
          ),
          const SizedBox(height: 32),
          OutlinedButton.icon(
            onPressed: _loggingOut ? null : _logout,
            icon: const Icon(Icons.logout_rounded),
            label: const Text('Выйти'),
            style: OutlinedButton.styleFrom(
              foregroundColor: scheme.error,
              minimumSize: const Size.fromHeight(52),
              shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(16)),
            ),
          ),
        ],
      ),
    );
  }
}
