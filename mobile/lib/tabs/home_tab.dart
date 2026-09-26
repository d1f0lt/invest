import 'package:flutter/material.dart';

import '../api/portfolio_api.dart';
import '../portfolio/portfolio_app_bar.dart';
import '../portfolio/portfolio_store.dart';
import '../portfolio/stats_format.dart';

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

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    return Scaffold(
      appBar: portfolioAppBar(context, upload: true),
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
              content = const EmptyPortfolio();
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
