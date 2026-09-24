import 'package:flutter/material.dart';

import '../api/api_client.dart';
import '../api/auth_api.dart';
import '../api/session.dart';
import '../main_shell.dart';

import 'register_screen.dart';
import 'validators.dart';
import 'widgets.dart';

class LoginScreen extends StatefulWidget {
  const LoginScreen({super.key});

  @override
  State<LoginScreen> createState() => _LoginScreenState();
}

class _LoginScreenState extends State<LoginScreen> {
  final _formKey = GlobalKey<FormState>();
  final _email = TextEditingController();
  final _password = TextEditingController();
  final _api = AuthApi();
  bool _loading = false;

  @override
  void dispose() {
    _email.dispose();
    _password.dispose();
    super.dispose();
  }

  Future<void> _submit() async {
    if (_loading || !_formKey.currentState!.validate()) return;
    FocusScope.of(context).unfocus();
    setState(() => _loading = true);
    try {
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
                title: 'Добро пожаловать',
                subtitle: 'Введите email и пароль, чтобы продолжить',
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
                controller: _password,
                label: 'Пароль',
                icon: Icons.lock_outline,
                obscure: true,
                textInputAction: TextInputAction.done,
                autofillHints: const [AutofillHints.password],
                validator: validateRequired,
                onSubmitted: (_) => _submit(),
              ),
              const SizedBox(height: 16),
              AuthSwitchPrompt(
                question: 'Нет аккаунта?',
                action: 'Зарегистрироваться',
                onTap: () => Navigator.of(context)
                    .pushReplacement(fadeRoute(const RegisterScreen())),
              ),
              const SizedBox(height: 16),
              PrimaryButton(label: 'Войти', onPressed: _submit, loading: _loading),
            ],
          ),
        ),
      ),
    );
  }
}
