import 'package:flutter/material.dart';

import '../api/portfolio_api.dart';
import 'portfolio_create_screen.dart';
import 'portfolio_edit_screen.dart';
import 'portfolio_picker.dart';
import 'portfolio_store.dart';
import 'stats_format.dart';

/// Экран «Мои портфели»: поиск и список портфелей с краткой сводкой.
class PortfoliosScreen extends StatefulWidget {
  const PortfoliosScreen({super.key});

  @override
  State<PortfoliosScreen> createState() => _PortfoliosScreenState();
}

class _PortfoliosScreenState extends State<PortfoliosScreen> {
  final _store = PortfolioStore.instance;
  final _search = TextEditingController();
  String _query = '';

  @override
  void dispose() {
    _search.dispose();
    super.dispose();
  }

  Future<void> _create() => openPortfolioCreate(context);

  void _select(Portfolio p) {
    _store.select(p);
    Navigator.of(context).pop();
  }

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    return Scaffold(
      appBar: AppBar(
        title: const Text('Мои портфели'),
        actions: [
          IconButton(
            tooltip: 'Новый портфель',
            icon: const Icon(Icons.add_rounded),
            onPressed: _create,
          ),
          const SizedBox(width: 4),
        ],
      ),
      body: Column(
        children: [
          Padding(
            padding: const EdgeInsets.fromLTRB(16, 12, 16, 8),
            child: TextField(
              controller: _search,
              onChanged: (v) => setState(() => _query = v.trim().toLowerCase()),
              textInputAction: TextInputAction.search,
              decoration: InputDecoration(
                hintText: 'Поиск портфеля',
                prefixIcon: const Icon(Icons.search_rounded),
                suffixIcon: _query.isEmpty
                    ? null
                    : IconButton(
                        tooltip: 'Очистить',
                        icon: const Icon(Icons.close_rounded),
                        onPressed: () => setState(() {
                          _search.clear();
                          _query = '';
                        }),
                      ),
                filled: true,
                fillColor: scheme.surfaceContainerHighest,
                isDense: true,
                border: OutlineInputBorder(
                  borderRadius: BorderRadius.circular(14),
                  borderSide: BorderSide.none,
                ),
              ),
            ),
          ),
          Expanded(
            child: RefreshIndicator(
              onRefresh: _store.load,
              child: ListenableBuilder(listenable: _store, builder: (context, _) => _list()),
            ),
          ),
        ],
      ),
    );
  }

  Widget _list() {
    if (_store.portfolios.isEmpty) {
      final Widget child;
      if (_store.error != null && !_store.loading) {
        child = Column(
          children: [
            Text(_store.error!, textAlign: TextAlign.center),
            const SizedBox(height: 12),
            OutlinedButton(onPressed: _store.load, child: const Text('Повторить')),
          ],
        );
      } else {
        child = const Center(child: CircularProgressIndicator());
      }
      return ListView(
        physics: const AlwaysScrollableScrollPhysics(),
        padding: const EdgeInsets.all(32),
        children: [child],
      );
    }

    final items = _query.isEmpty
        ? _store.portfolios
        : _store.portfolios
            .where((p) => p.displayName.toLowerCase().contains(_query))
            .toList();
    if (items.isEmpty) {
      return ListView(
        physics: const AlwaysScrollableScrollPhysics(),
        padding: const EdgeInsets.all(32),
        children: const [Text('Ничего не найдено', textAlign: TextAlign.center)],
      );
    }
    return ListView.separated(
      physics: const AlwaysScrollableScrollPhysics(),
      padding: const EdgeInsets.fromLTRB(16, 4, 16, 24),
      itemCount: items.length,
      separatorBuilder: (_, _) => const SizedBox(height: 12),
      itemBuilder: (context, i) {
        final p = items[i];
        return PortfolioCard(
          portfolio: p,
          members: _store.membersOf(p),
          stats: _store.statsFor(p.id),
          selected: p.id == _store.current?.id,
          onTap: () => _select(p),
          onEdit: () => Navigator.of(context).push(
            MaterialPageRoute<void>(builder: (_) => PortfolioEditScreen(portfolio: p)),
          ),
        );
      },
    );
  }
}

/// Карточка портфеля: название и три метрики — прибыль, за день, доходность.
class PortfolioCard extends StatelessWidget {
  const PortfolioCard({
    super.key,
    required this.portfolio,
    this.members = const [],
    required this.stats,
    required this.selected,
    required this.onTap,
    required this.onEdit,
  });

  final Portfolio portfolio;

  /// Для составного портфеля — из чего он собран (подпись под названием).
  final List<Portfolio> members;
  final PortfolioStats stats;
  final bool selected;
  final VoidCallback onTap;
  final VoidCallback onEdit;

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    return Card(
      margin: EdgeInsets.zero,
      elevation: 0,
      color: scheme.surfaceContainerLow,
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(18),
        side: BorderSide(
          color: selected ? scheme.primary : scheme.outlineVariant,
          width: selected ? 1.5 : 1,
        ),
      ),
      clipBehavior: Clip.antiAlias,
      child: InkWell(
        onTap: onTap,
        child: Padding(
          padding: const EdgeInsets.all(16),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              Row(
                children: [
                  PortfolioAvatar(size: 40, composite: portfolio.isComposite),
                  const SizedBox(width: 12),
                  Expanded(
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        Text(
                          portfolio.displayName,
                          maxLines: 1,
                          overflow: TextOverflow.ellipsis,
                          style: Theme.of(context)
                              .textTheme
                              .titleMedium
                              ?.copyWith(fontWeight: FontWeight.w700),
                        ),
                        if (portfolio.isComposite)
                          Text(
                            compositeSubtitle(portfolio, members),
                            maxLines: 1,
                            overflow: TextOverflow.ellipsis,
                            style: Theme.of(context)
                                .textTheme
                                .bodySmall
                                ?.copyWith(color: scheme.onSurfaceVariant),
                          ),
                      ],
                    ),
                  ),
                  if (selected) Icon(Icons.check_circle_rounded, color: scheme.primary),
                  IconButton(
                    tooltip: 'Редактировать',
                    visualDensity: VisualDensity.compact,
                    icon: const Icon(Icons.edit_outlined),
                    onPressed: onEdit,
                  ),
                ],
              ),
              const SizedBox(height: 14),
              Row(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Expanded(
                    child: StatValue(
                      label: 'Прибыль',
                      money: stats.profit,
                      percent: stats.profitPercent,
                    ),
                  ),
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
      ),
    );
  }
}

/// «Составной · ИИС + Брокерский» (или «Составной · 2 портфеля», пока имена
/// участников не загружены).
String compositeSubtitle(Portfolio portfolio, List<Portfolio> members) {
  if (members.length == portfolio.memberIds.length) {
    return 'Составной · ${members.map((p) => p.displayName).join(' + ')}';
  }
  final n = portfolio.memberIds.length;
  final String word;
  if (n % 10 == 1 && n % 100 != 11) {
    word = 'портфель';
  } else if (n % 10 >= 2 && n % 10 <= 4 && (n % 100 < 12 || n % 100 > 14)) {
    word = 'портфеля';
  } else {
    word = 'портфелей';
  }
  return 'Составной · $n $word';
}
