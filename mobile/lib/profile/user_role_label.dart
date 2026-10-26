import 'package:flutter/material.dart';

class UserRoleLabel extends StatelessWidget {
  const UserRoleLabel({super.key, required this.isAdmin});

  final bool? isAdmin;

  @override
  Widget build(BuildContext context) {
    final isAdmin = this.isAdmin;
    if (isAdmin == null) return const SizedBox.shrink();
    final theme = Theme.of(context);
    return Text(
      isAdmin ? 'Администратор' : 'Пользователь',
      style: theme.textTheme.bodySmall?.copyWith(
        color: isAdmin ? theme.colorScheme.error : theme.colorScheme.onSurface,
      ),
    );
  }
}
