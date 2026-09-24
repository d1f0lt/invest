import 'package:flutter/material.dart';

import 'placeholder.dart';

class CalendarTab extends StatelessWidget {
  const CalendarTab({super.key});

  @override
  Widget build(BuildContext context) => const TabPlaceholder(
        title: 'Календарь',
        icon: Icons.calendar_month_rounded,
        text: 'Здесь будут дивиденды, купоны и другие события',
      );
}
