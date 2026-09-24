import 'package:flutter/material.dart';

import '../api/portfolio_api.dart';
import '../portfolio/portfolio_picker.dart';
import '../portfolio/portfolio_store.dart';
import '../portfolio/stats_format.dart';
import '../reports/broker_select_screen.dart';
import '../search/asset_search_screen.dart';

class HomeTab extends StatefulWidget {
  const HomeTab({super.key});

  @override
  State<HomeTab> createState() => _HomeTabState();
}

class _HomeTabState extends State<HomeTab> {
  final _store = PortfolioStore.instance;

  @override
  void initState() {
    super.initState();
    if (!_store.loaded) _store.load();
  }

  void _soon(String what) {
    ScaffoldMessenger.of(context)
      ..hideCurrentSnackBar()
      ..showSnackBar(SnackBar(content: Text('$what — скоро')));
  }

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    return Scaffold(
      appBar: AppBar(
        centerTitle: false,
        titleSpacing: 10,
        title: const PortfolioSelector(),
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
            onPressed: () => _soon('Избранное'), // TODO: список избранных бумаг
          ),
          IconButton(
            tooltip: 'Загрузить отчёт',
            icon: const Icon(Icons.add_rounded),
            onPressed: () => openReportUpload(context),
          ),
          const SizedBox(width: 4),
        ],
      ),
      body: RefreshIndicator(
        onRefresh: _store.load,
        child: ListenableBuilder(
          listenable: _store,
          builder: (context, _) {
            final current = _store.current;
            final Widget content;
            if (current == null || (!_store.hasStats(current.id) && _store.statsLoading)) {
              content = _store.error != null && current == null
                  ? const SizedBox.shrink()
                  : const Padding(
                      padding: EdgeInsets.only(top: 80),
                      child: Center(child: CircularProgressIndicator()),
                    );
            } else if (_store.hasStats(current.id) && !_store.statsFor(current.id).hasData) {
              content = _EmptyPortfolio(onUpload: () => openReportUpload(context));
            } else {
              // Сводка не загрузилась — показываем нули, ошибку не выдумываем.
              content = _SummaryCard(stats: _store.statsFor(current.id));
            }
            return ListView(
              physics: const AlwaysScrollableScrollPhysics(),
              padding: const EdgeInsets.all(16),
              children: [
                content,
                if (_store.error != null) ...[
                  const SizedBox(height: 16),
                  Text(
                    _store.error!,
                    textAlign: TextAlign.center,
                    style: TextStyle(color: scheme.error),
                  ),
                ],
              ],
            );
          },
        ),
      ),
    );
  }
}

/// Сводка по текущему портфелю: прибыль крупно, ниже — за день и доходность.
class _SummaryCard extends StatelessWidget {
  const _SummaryCard({required this.stats});

  final PortfolioStats stats;

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    return Card(
      margin: EdgeInsets.zero,
      elevation: 0,
      color: scheme.surfaceContainerLow,
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(20),
        side: BorderSide(color: scheme.outlineVariant),
      ),
      child: Padding(
        padding: const EdgeInsets.all(20),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            StatValue(
              label: 'Прибыль',
              money: stats.profit,
              percent: stats.profitPercent,
              large: true,
            ),
            const SizedBox(height: 16),
            Row(
              children: [
                Expanded(
                  child: StatValue(
                    label: 'За день',
                    money: stats.dayProfit,
                    percent: stats.dayPercent,
                  ),
                ),
                Expanded(
                  child: StatValue(
                    label: 'Доходность',
                    percent: stats.yieldPercent,
                    colored: false,
                  ),
                ),
              ],
            ),
          ],
        ),
      ),
    );
  }
}

/// Пустой портфель: вместо нулевой сводки — призыв загрузить первый отчёт.
class _EmptyPortfolio extends StatelessWidget {
  const _EmptyPortfolio({required this.onUpload});

  final VoidCallback onUpload;

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
            'Загрузите первый отчёт',
            textAlign: TextAlign.center,
            style: textTheme.titleLarge?.copyWith(fontWeight: FontWeight.w700),
          ),
          const SizedBox(height: 8),
          Text(
            'Добавьте отчёт брокера — и здесь появятся прибыль, доходность '
            'и изменения за день',
            textAlign: TextAlign.center,
            style: textTheme.bodyMedium?.copyWith(color: scheme.onSurfaceVariant),
          ),
          const SizedBox(height: 24),
          FilledButton.icon(
            onPressed: onUpload,
            icon: const Icon(Icons.add_rounded),
            label: const Text('Загрузить отчёт'),
          ),
        ],
      ),
    );
  }
}
