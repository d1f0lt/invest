import 'package:flutter/material.dart';

import '../api/api_client.dart';
import '../api/auth_api.dart';
import '../api/session.dart';
import '../main_shell.dart';

import 'login_screen.dart';
import 'validators.dart';
import 'widgets.dart';

class RegisterScreen extends StatefulWidget {
  const RegisterScreen({super.key});

  @override
  State<RegisterScreen> createState() => _RegisterScreenState();
}

class _RegisterScreenState extends State<RegisterScreen> {
  final _formKey = GlobalKey<FormState>();
  final _email = TextEditingController();
  final _login = TextEditingController();
  final _password = TextEditingController();
  final _repeat = TextEditingController();
  final _api = AuthApi();
  bool _loading = false;

  static final _loginRe = RegExp(r'^[a-zA-Z0-9_.]+$');

  @override
  void dispose() {
    _email.dispose();
    _login.dispose();
    _password.dispose();
    _repeat.dispose();
    super.dispose();
  }

  String? _validateLogin(String? v) {
    final value = v?.trim() ?? '';
    if (value.isEmpty) return 'Заполните поле';
    if (value.length < 3) return 'Минимум 3 символа';
    if (!_loginRe.hasMatch(value)) return 'Только латиница, цифры, «_» и «.»';
    return null;
  }

  String? _validatePassword(String? v) {
    final value = v ?? '';
    if (value.isEmpty) return 'Заполните поле';
    if (value.length < 8) return 'Минимум 8 символов';
    return null;
  }

  String? _validateRepeat(String? v) {
    if ((v ?? '').isEmpty) return 'Заполните поле';
    if (v != _password.text) return 'Пароли не совпадают';
    return null;
  }

  Future<void> _submit() async {
    if (_loading || !_formKey.currentState!.validate()) return;
    FocusScope.of(context).unfocus();
    setState(() => _loading = true);
    try {
      await _api.register(
        email: _email.text,
        username: _login.text,
        password: _password.text,
      );
      // Сразу входим, чтобы не заставлять вводить данные второй раз.
      await Session.instance.save(
          await _api.login(email: _email.text, password: _password.text));
      if (!mounted) return;
      Navigator.of(context).pushAndRemoveUntil(fadeRoute(const MainShell()), (_) => false);
    } on ApiException catch (e) {
      if (mounted) showAuthError(context, e.message);
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    return AuthScaffold(
      child: AutofillGroup(
        child: Form(
          key: _formKey,
          child: Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              const AuthHeader(
                title: 'Создать аккаунт',
                subtitle: 'Заполните данные для регистрации',
              ),
              const SizedBox(height: 28),
              AuthTextField(
                controller: _email,
                label: 'Email',
                icon: Icons.mail_outline,
                keyboardType: TextInputType.emailAddress,
                autofillHints: const [AutofillHints.email],
                validator: validateEmail,
              ),
              const SizedBox(height: 16),
              AuthTextField(
                controller: _login,
                label: 'Логин',
                icon: Icons.person_outline,
                autofillHints: const [AutofillHints.newUsername],
                validator: _validateLogin,
              ),
              const SizedBox(height: 16),
              AuthTextField(
                controller: _password,
                label: 'Пароль',
                icon: Icons.lock_outline,
                obscure: true,
                autofillHints: const [AutofillHints.newPassword],
                validator: _validatePassword,
              ),
              const SizedBox(height: 16),
              AuthTextField(
                controller: _repeat,
                label: 'Повторите пароль',
                icon: Icons.lock_reset_outlined,
                obscure: true,
                textInputAction: TextInputAction.done,
                autofillHints: const [AutofillHints.newPassword],
                validator: _validateRepeat,
                onSubmitted: (_) => _submit(),
              ),
              const SizedBox(height: 16),
              AuthSwitchPrompt(
                question: 'Есть аккаунт?',
                action: 'Войти',
                onTap: () => Navigator.of(context)
                    .pushReplacement(fadeRoute(const LoginScreen())),
              ),
              const SizedBox(height: 16),
              PrimaryButton(
                label: 'Зарегистрироваться',
                onPressed: _submit,
                loading: _loading,
              ),
            ],
          ),
        ),
      ),
    );
  }
}
