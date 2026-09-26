import 'package:flutter/material.dart';

import '../api/api_client.dart';
import '../api/auth_api.dart';
import 'password_field.dart';

/// Подтверждение удаления аккаунта паролем. Возвращает `true`, если аккаунт
/// удалён на сервере.
Future<bool> showDeleteAccountDialog(BuildContext context, AuthApi api) async {
  final deleted = await showDialog<bool>(
    context: context,
    builder: (_) => _DeleteAccountDialog(api: api),
  );
  return deleted ?? false;
}

class _DeleteAccountDialog extends StatefulWidget {
  const _DeleteAccountDialog({required this.api});

  final AuthApi api;

  @override
  State<_DeleteAccountDialog> createState() => _DeleteAccountDialogState();
}

class _DeleteAccountDialogState extends State<_DeleteAccountDialog> {
  final _password = TextEditingController();
  bool _deleting = false;
  String? _error;

  @override
  void dispose() {
    _password.dispose();
    super.dispose();
  }

  Future<void> _delete() async {
    if (_deleting) return;
    if (_password.text.isEmpty) {
      setState(() => _error = 'Введите пароль');
      return;
    }
    setState(() {
      _deleting = true;
      _error = null;
    });
    try {
      await widget.api.deleteMe(_password.text);
      if (mounted) Navigator.of(context).pop(true);
    } on ApiException catch (e) {
      if (mounted) setState(() => _error = e.message);
    } finally {
      if (mounted) setState(() => _deleting = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    return AlertDialog(
      icon: Icon(Icons.warning_amber_rounded, color: scheme.error),
      title: const Text('Удалить аккаунт?'),
      content: Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          const Text('Аккаунт будет удалён без возможности восстановления. '
              'Для подтверждения введите пароль.'),
          const SizedBox(height: 16),
          PasswordField(
            controller: _password,
            label: 'Пароль',
            autofocus: true,
            autofillHints: const [AutofillHints.password],
            textInputAction: TextInputAction.done,
            errorText: _error,
            onChanged: (_) {
              if (_error != null) setState(() => _error = null);
            },
            onSubmitted: (_) => _delete(),
          ),
        ],
      ),
      actions: [
        TextButton(
          onPressed: _deleting ? null : () => Navigator.of(context).pop(false),
          child: const Text('Отмена'),
        ),
        FilledButton(
          onPressed: _deleting ? null : _delete,
          style: FilledButton.styleFrom(
            backgroundColor: scheme.error,
            foregroundColor: scheme.onError,
          ),
          child: _deleting
              ? SizedBox.square(
                  dimension: 18,
                  child: CircularProgressIndicator(strokeWidth: 2, color: scheme.onError),
                )
              : const Text('Удалить'),
        ),
      ],
    );
  }
}
