import 'package:flutter/material.dart';

import 'placeholder.dart';

class PortfolioTab extends StatelessWidget {
  const PortfolioTab({super.key});

  @override
  Widget build(BuildContext context) => const TabPlaceholder(
        title: 'Портфель',
        icon: Icons.business_center_rounded,
        text: 'Здесь будут ваши бумаги, сделки и доходность',
      );
}
