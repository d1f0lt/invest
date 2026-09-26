import 'package:flutter/material.dart';

import '../portfolio/portfolio_app_bar.dart';
import 'placeholder.dart';

class CalendarTab extends StatelessWidget {
  const CalendarTab({super.key});

  @override
  Widget build(BuildContext context) => TabPlaceholder(
        appBar: portfolioAppBar(context),
        icon: Icons.calendar_month_rounded,
        text: 'Здесь будут дивиденды, купоны и другие события',
      );
}
