import 'package:flutter/material.dart';

import '../api/api_client.dart';
import '../api/auth_api.dart';
import '../api/session.dart';
import '../auth/login_screen.dart';
import '../auth/validators.dart';
import '../auth/widgets.dart';
import '../portfolio/portfolio_store.dart';
import '../profile/change_password_screen.dart';
import '../profile/delete_account_dialog.dart';
import '../profile/editable_field.dart';

class ProfileTab extends StatefulWidget {
  const ProfileTab({super.key});

  @override
  State<ProfileTab> createState() => _ProfileTabState();
}

class _ProfileTabState extends State<ProfileTab> {
  final _api = AuthApi();
  UserProfile? _user;
  ApiException? _loadError;
  bool _loading = true;
  bool _loggingOut = false;

  @override
  void initState() {
    super.initState();
    _load();
  }

  Future<void> _load() async {
    setState(() {
      _loading = true;
      _loadError = null;
    });
    try {
      final user = await _api.me();
      if (mounted) setState(() => _user = user);
    } on ApiException catch (e) {
      if (mounted) setState(() => _loadError = e);
    } catch (_) {
      if (mounted) {
        setState(() => _loadError = const ApiException('Не удалось загрузить профиль'));
      }
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  void _toLogin() {
    Navigator.of(context).pushAndRemoveUntil(fadeRoute(const LoginScreen()), (_) => false);
  }

  Future<void> _saveUsername(String value) async {
    final user = await _api.updateMe(username: value);
    if (mounted) setState(() => _user = user);
  }

  Future<void> _saveEmail(String value) async {
    final user = await _api.updateMe(email: value);
    if (mounted) setState(() => _user = user);
  }

  void _changePassword() {
    Navigator.of(context).push(
      MaterialPageRoute<void>(builder: (_) => const ChangePasswordScreen()),
    );
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

  Future<void> _deleteAccount() async {
    final deleted = await showDeleteAccountDialog(context, _api);
    if (!deleted) return;
    await Session.instance.clear();
    PortfolioStore.instance.reset();
    if (mounted) _toLogin();
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: const Text('Профиль')),
      body: ListView(
        padding: const EdgeInsets.all(24),
        children: [
          if (_loading && _user == null)
            const Padding(
              padding: EdgeInsets.symmetric(vertical: 40),
              child: Center(child: CircularProgressIndicator()),
            )
          else if (_user == null)
            _buildError(context)
          else
            ..._buildProfile(context, _user!),
          const SizedBox(height: 32),
          ..._buildActions(context),
        ],
      ),
    );
  }

  Widget _buildError(BuildContext context) {
    final error = _loadError;
    return Column(
      children: [
        Text(error?.message ?? 'Не удалось загрузить профиль', textAlign: TextAlign.center),
        const SizedBox(height: 12),
        if (error?.statusCode == 401)
          FilledButton(onPressed: _toLogin, child: const Text('Войти заново'))
        else
          OutlinedButton(onPressed: _load, child: const Text('Повторить')),
      ],
    );
  }

  List<Widget> _buildProfile(BuildContext context, UserProfile user) {
    final scheme = Theme.of(context).colorScheme;
    final textTheme = Theme.of(context).textTheme;
    final name = user.username ?? user.email;
    return [
      Center(
        child: CircleAvatar(
          radius: 44,
          backgroundColor: scheme.primaryContainer,
          child: Text(
            name.characters.first.toUpperCase(),
            style: textTheme.headlineMedium?.copyWith(color: scheme.onPrimaryContainer),
          ),
        ),
      ),
      const SizedBox(height: 24),
      Card(
        margin: EdgeInsets.zero,
        elevation: 0,
        color: scheme.surfaceContainerLow,
        clipBehavior: Clip.antiAlias,
        shape: RoundedRectangleBorder(
          borderRadius: BorderRadius.circular(16),
          side: BorderSide(color: scheme.outlineVariant),
        ),
        child: Column(
          children: [
            EditableField(
              label: 'Логин',
              value: user.username,
              icon: Icons.person_outline_rounded,
              validator: validateLogin,
              autofillHints: const [AutofillHints.username],
              onSave: _saveUsername,
            ),
            Divider(height: 1, indent: 56, color: scheme.outlineVariant),
            EditableField(
              label: 'Email',
              value: user.email,
              icon: Icons.alternate_email_rounded,
              validator: validateEmail,
              keyboardType: TextInputType.emailAddress,
              autofillHints: const [AutofillHints.email],
              onSave: _saveEmail,
            ),
          ],
        ),
      ),
    ];
  }

  List<Widget> _buildActions(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    final shape = RoundedRectangleBorder(borderRadius: BorderRadius.circular(16));
    const size = Size.fromHeight(52);
    return [
      if (_user != null) ...[
        OutlinedButton.icon(
          onPressed: _changePassword,
          icon: const Icon(Icons.lock_reset_rounded),
          label: const Text('Сменить пароль'),
          style: OutlinedButton.styleFrom(minimumSize: size, shape: shape),
        ),
        const SizedBox(height: 12),
      ],
      OutlinedButton.icon(
        onPressed: _loggingOut ? null : _logout,
        icon: const Icon(Icons.logout_rounded),
        label: const Text('Выйти'),
        style: OutlinedButton.styleFrom(
          foregroundColor: scheme.error,
          minimumSize: size,
          shape: shape,
        ),
      ),
      if (_user != null) ...[
        const SizedBox(height: 12),
        FilledButton.icon(
          onPressed: _loggingOut ? null : _deleteAccount,
          icon: const Icon(Icons.delete_forever_outlined),
          label: const Text('Удалить аккаунт'),
          style: FilledButton.styleFrom(
            backgroundColor: scheme.errorContainer,
            foregroundColor: scheme.onErrorContainer,
            minimumSize: size,
            shape: shape,
          ),
        ),
      ],
    ];
  }
}
