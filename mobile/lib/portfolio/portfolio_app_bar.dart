import 'package:flutter/material.dart';

import '../favorites/favorites_screen.dart';
import '../reports/broker_select_screen.dart';
import '../search/asset_search_screen.dart';
import 'portfolio_picker.dart';

/// Общая шапка вкладок (кроме «Профиля»): слева выбор портфеля,
/// справа поиск активов и избранное; [upload] — ещё «+» (загрузить отчёт).
AppBar portfolioAppBar(
  BuildContext context, {
  bool upload = false,
  PreferredSizeWidget? bottom,
}) {
  return AppBar(
    centerTitle: false,
    titleSpacing: 10,
    title: const PortfolioSelector(),
    bottom: bottom,
    actions: [
      IconButton(
        tooltip: 'Поиск активов',
        icon: const Icon(Icons.search_rounded),
        onPressed: () => Navigator.of(context).push(
          MaterialPageRoute<void>(builder: (_) => const AssetSearchScreen()),
        ),
      ),
      IconButton(
        tooltip: 'Избранное',
        icon: const Icon(Icons.star_outline_rounded),
        onPressed: () => Navigator.of(context).push(
          MaterialPageRoute<void>(builder: (_) => const FavoritesScreen()),
        ),
      ),
      if (upload)
        IconButton(
          tooltip: 'Загрузить отчёт',
          icon: const Icon(Icons.add_rounded),
          onPressed: () => openReportUpload(context),
        ),
      const SizedBox(width: 4),
    ],
  );
}

/// Пустой портфель: вместо нулей — призыв загрузить первый отчёт.
class EmptyPortfolio extends StatelessWidget {
  const EmptyPortfolio({
    super.key,
    this.title = 'Загрузите первый отчёт',
    this.text = 'Добавьте отчёт брокера — и здесь появятся ваши бумаги, операции, '
        'прибыль и доходность',
  });

  final String title;
  final String text;

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    final textTheme = Theme.of(context).textTheme;
    return Padding(
      padding: const EdgeInsets.fromLTRB(16, 56, 16, 16),
      child: Column(
        children: [
          Container(
            width: 88,
            height: 88,
            decoration: BoxDecoration(shape: BoxShape.circle, color: scheme.primaryContainer),
            child: Icon(Icons.upload_file_rounded, size: 44, color: scheme.onPrimaryContainer),
          ),
          const SizedBox(height: 20),
          Text(
            title,
            textAlign: TextAlign.center,
            style: textTheme.titleLarge?.copyWith(fontWeight: FontWeight.w700),
          ),
          const SizedBox(height: 8),
          Text(
            text,
            textAlign: TextAlign.center,
            style: textTheme.bodyMedium?.copyWith(color: scheme.onSurfaceVariant),
          ),
          const SizedBox(height: 24),
          FilledButton.icon(
            onPressed: () => openReportUpload(context),
            icon: const Icon(Icons.add_rounded),
            label: const Text('Загрузить отчёт'),
          ),
        ],
      ),
    );
  }
}
