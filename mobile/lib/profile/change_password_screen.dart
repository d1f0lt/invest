import 'package:flutter/material.dart';

import '../api/api_client.dart';
import '../api/auth_api.dart';
import '../api/session.dart';
import '../auth/validators.dart';
import 'password_field.dart';

/// «Смена пароля». Сервер отзывает остальные сессии и выдаёт этому
/// устройству новую пару токенов — сохраняем её в [Session].
class ChangePasswordScreen extends StatefulWidget {
  const ChangePasswordScreen({super.key});

  @override
  State<ChangePasswordScreen> createState() => _ChangePasswordScreenState();
}

class _ChangePasswordScreenState extends State<ChangePasswordScreen> {
  final _api = AuthApi();
  final _formKey = GlobalKey<FormState>();
  final _current = TextEditingController();
  final _password = TextEditingController();
  final _repeat = TextEditingController();
  bool _saving = false;
  String? _currentError;

  @override
  void dispose() {
    _current.dispose();
    _password.dispose();
    _repeat.dispose();
    super.dispose();
  }

  String? _validateNew(String? v) {
    final error = validatePassword(v);
    if (error != null) return error;
    if (v == _current.text) return 'Новый пароль совпадает с текущим';
    return null;
  }

  String? _validateRepeat(String? v) {
    if ((v ?? '').isEmpty) return 'Заполните поле';
    if (v != _password.text) return 'Пароли не совпадают';
    return null;
  }

  Future<void> _save() async {
    setState(() => _currentError = null);
    if (_saving || !_formKey.currentState!.validate()) return;
    FocusScope.of(context).unfocus();
    setState(() => _saving = true);
    try {
      final tokens = await _api.changePassword(
        currentPassword: _current.text,
        newPassword: _password.text,
      );
      await Session.instance.save(tokens);
      if (!mounted) return;
      ScaffoldMessenger.of(context)
        ..hideCurrentSnackBar()
        ..showSnackBar(const SnackBar(
          behavior: SnackBarBehavior.floating,
          content: Text('Пароль изменён. На других устройствах нужно войти заново'),
        ));
      Navigator.of(context).pop();
    } on ApiException catch (e) {
      if (!mounted) return;
      if (e.statusCode == 403) {
        setState(() => _currentError = e.message);
      } else {
        ScaffoldMessenger.of(context)
          ..hideCurrentSnackBar()
          ..showSnackBar(SnackBar(behavior: SnackBarBehavior.floating, content: Text(e.message)));
      }
    } finally {
      if (mounted) setState(() => _saving = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: const Text('Смена пароля')),
      body: SafeArea(
        child: Form(
          key: _formKey,
          child: ListView(
            padding: const EdgeInsets.fromLTRB(16, 8, 16, 16),
            children: [
              PasswordField(
                controller: _current,
                label: 'Текущий пароль',
                autofocus: true,
                autofillHints: const [AutofillHints.password],
                errorText: _currentError,
                validator: validateRequired,
                onChanged: (_) {
                  if (_currentError != null) setState(() => _currentError = null);
                },
              ),
              const SizedBox(height: 16),
              PasswordField(
                controller: _password,
                label: 'Новый пароль',
                autofillHints: const [AutofillHints.newPassword],
                validator: _validateNew,
              ),
              const SizedBox(height: 16),
              PasswordField(
                controller: _repeat,
                label: 'Повторите новый пароль',
                autofillHints: const [AutofillHints.newPassword],
                textInputAction: TextInputAction.done,
                validator: _validateRepeat,
                onSubmitted: (_) => _save(),
              ),
              const SizedBox(height: 24),
              SizedBox(
                height: 52,
                child: FilledButton(
                  onPressed: _saving ? null : _save,
                  style: FilledButton.styleFrom(
                    shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(14)),
                  ),
                  child: _saving
                      ? const SizedBox.square(
                          dimension: 22,
                          child: CircularProgressIndicator(strokeWidth: 2.5),
                        )
                      : const Text('Сменить пароль'),
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }
}
