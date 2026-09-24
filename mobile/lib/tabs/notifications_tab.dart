import 'package:flutter/material.dart';

import 'placeholder.dart';

class NotificationsTab extends StatelessWidget {
  const NotificationsTab({super.key});

  @override
  Widget build(BuildContext context) => const TabPlaceholder(
        title: 'Уведомления',
        icon: Icons.notifications_rounded,
        text: 'Здесь будут уведомления о дивидендах, купонах и изменениях цен',
      );
}
