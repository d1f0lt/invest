import 'package:flutter/material.dart';

import '../portfolio/portfolio_app_bar.dart';
import 'placeholder.dart';

class NotificationsTab extends StatelessWidget {
  const NotificationsTab({super.key});

  @override
  Widget build(BuildContext context) => TabPlaceholder(
        appBar: portfolioAppBar(context),
        icon: Icons.notifications_rounded,
        text: 'Здесь будут уведомления о дивидендах, купонах и изменениях цен',
      );
}
