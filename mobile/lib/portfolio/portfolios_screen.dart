import 'package:flutter/material.dart';

import '../api/api_client.dart';
import '../api/portfolio_api.dart';
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

  Future<void> _create() async {
    final name = await showDialog<String>(
      context: context,
      builder: (_) => const _NewPortfolioDialog(),
    );
    if (name == null || !mounted) return;
    try {
      await _store.create(name);
    } on ApiException catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(e.message)));
      }
    }
  }

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
    required this.stats,
    required this.selected,
    required this.onTap,
    required this.onEdit,
  });

  final Portfolio portfolio;
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
                  const PortfolioAvatar(size: 40),
                  const SizedBox(width: 12),
                  Expanded(
                    child: Text(
                      portfolio.displayName,
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                      style: Theme.of(context)
                          .textTheme
                          .titleMedium
                          ?.copyWith(fontWeight: FontWeight.w700),
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

class _NewPortfolioDialog extends StatefulWidget {
  const _NewPortfolioDialog();

  @override
  State<_NewPortfolioDialog> createState() => _NewPortfolioDialogState();
}

class _NewPortfolioDialogState extends State<_NewPortfolioDialog> {
  final _name = TextEditingController();

  @override
  void dispose() {
    _name.dispose();
    super.dispose();
  }

  void _submit() {
    if (_name.text.trim().isEmpty) return;
    Navigator.of(context).pop(_name.text.trim());
  }

  @override
  Widget build(BuildContext context) {
    return AlertDialog(
      title: const Text('Новый портфель'),
      content: TextField(
        controller: _name,
        autofocus: true,
        textCapitalization: TextCapitalization.sentences,
        decoration: const InputDecoration(hintText: 'Например, «ИИС»'),
        onSubmitted: (_) => _submit(),
      ),
      actions: [
        TextButton(onPressed: () => Navigator.of(context).pop(), child: const Text('Отмена')),
        FilledButton(onPressed: _submit, child: const Text('Создать')),
      ],
    );
  }
}
