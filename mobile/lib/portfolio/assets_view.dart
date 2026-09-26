import 'package:flutter/material.dart';

import '../api/portfolio_api.dart';
import '../api/securities_api.dart';
import '../securities/asset_screen.dart';
import '../securities/security_directory.dart';
import '../securities/security_widgets.dart';
import 'portfolio_app_bar.dart';
import 'portfolio_store.dart';
import 'stats_format.dart';

/// Группы бумаг в списке — по режиму торгов MOEX.
enum _Group {
  shares('Акции'),
  funds('Фонды'),
  bonds('Облигации'),
  other('Другое');

  const _Group(this.title);

  final String title;

  static _Group of(String board) => switch (board) {
        'TQBR' => shares,
        'TQTF' => funds,
        'TQOB' || 'TQCB' => bonds,
        _ => other,
      };
}

/// «Активы»: открытые позиции текущего портфеля (из `/pnl`) — количество,
/// текущая стоимость позиции и её изменение к цене покупки; ниже — рубли.
class AssetsView extends StatefulWidget {
  const AssetsView({super.key});

  @override
  State<AssetsView> createState() => _AssetsViewState();
}

class _AssetsViewState extends State<AssetsView> with AutomaticKeepAliveClientMixin {
  final _store = PortfolioStore.instance;
  final _directory = SecurityDirectory.instance;

  @override
  bool get wantKeepAlive => true;

  @override
  void initState() {
    super.initState();
    _store.addListener(_loadNames);
    _loadNames();
  }

  @override
  void dispose() {
    _store.removeListener(_loadNames);
    super.dispose();
  }

  /// Названия бумаг — отдельным запросом; пришли — перерисуемся.
  Future<void> _loadNames() async {
    final current = _store.current;
    if (current == null) return;
    final positions = _store.statsFor(current.id).positions;
    if (await _directory.ensure(positions.map((p) => p.secid)) && mounted) setState(() {});
  }

  void _open(Security security) {
    Navigator.of(context).push(
      MaterialPageRoute<void>(builder: (_) => AssetScreen(security: security)),
    );
  }

  @override
  Widget build(BuildContext context) {
    super.build(context);
    return RefreshIndicator(
      onRefresh: () => _store.loaded ? _store.loadStats() : _store.load(),
      child: ListenableBuilder(
        listenable: _store,
        builder: (context, _) => ListView(
          physics: const AlwaysScrollableScrollPhysics(),
          padding: const EdgeInsets.only(bottom: 24),
          children: _content(context),
        ),
      ),
    );
  }

  List<Widget> _content(BuildContext context) {
    final current = _store.current;
    if (current == null || (!_store.hasStats(current.id) && _store.statsLoading)) {
      if (_store.error != null && !_store.loading) return [_Message(text: _store.error!)];
      return const [_Spinner()];
    }
    if (!_store.hasStats(current.id)) {
      return const [_Message(text: 'Не удалось загрузить активы. Потяните вниз, чтобы повторить')];
    }
    final stats = _store.statsFor(current.id);
    if (!stats.hasData) return const [EmptyPortfolio()];

    final groups = <_Group, List<Position>>{};
    for (final p in stats.positions) {
      groups.putIfAbsent(_Group.of(p.board), () => []).add(p);
    }
    final showCash = stats.cashBalance.abs() >= 0.005;
    if (groups.isEmpty && !showCash) {
      return const [
        EmptyPortfolio(
          title: 'В портфеле пока нет бумаг',
          text: 'Загрузите отчёт брокера — здесь появятся ваши акции, '
              'облигации и фонды',
        ),
      ];
    }

    return [
      for (final group in _Group.values)
        if (groups[group] case final list?) ...[
          _SectionHeader(group.title),
          for (final p in list..sort((a, b) => b.value.compareTo(a.value)))
            _PositionTile(
              position: p,
              security: _directory.lookup(p.secid, p.board),
              onTap: _open,
            ),
        ],
      if (showCash) ...[
        const _SectionHeader('Валюта'),
        _CashTile(amount: stats.cashBalance),
      ],
    ];
  }
}

class _SectionHeader extends StatelessWidget {
  const _SectionHeader(this.title);

  final String title;

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.fromLTRB(16, 20, 16, 4),
      child: Text(
        title,
        style: Theme.of(context).textTheme.titleLarge?.copyWith(fontWeight: FontWeight.w700),
      ),
    );
  }
}

/// Строка позиции: значок, название, «тикер • N шт.»; справа стоимость
/// позиции и изменение к цене покупки (`−903,96 ₽ ▼ 5,22%`).
class _PositionTile extends StatelessWidget {
  const _PositionTile({required this.position, required this.security, required this.onTap});

  final Position position;
  final Security security;
  final ValueChanged<Security> onTap;

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    final textTheme = Theme.of(context).textTheme;
    final p = position;
    final muted = textTheme.bodyMedium?.copyWith(color: scheme.onSurfaceVariant);
    return InkWell(
      onTap: () => onTap(security),
      child: Padding(
        padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 12),
        child: Row(
          children: [
            TickerBadge(secid: p.secid, size: 48, square: true),
            const SizedBox(width: 14),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [
                  Row(
                    children: [
                      Expanded(
                        child: Text(
                          security.title,
                          maxLines: 1,
                          overflow: TextOverflow.ellipsis,
                          style: textTheme.titleMedium,
                        ),
                      ),
                      const SizedBox(width: 8),
                      Text(
                        formatPrice(p.value, bond: false, currency: security.currency),
                        style: textTheme.titleMedium?.copyWith(fontWeight: FontWeight.w700),
                      ),
                    ],
                  ),
                  const SizedBox(height: 4),
                  Row(
                    children: [
                      Expanded(
                        child: Text(
                          '${p.secid} • ${_quantity(p.quantity)} шт.',
                          maxLines: 1,
                          overflow: TextOverflow.ellipsis,
                          style: muted,
                        ),
                      ),
                      const SizedBox(width: 8),
                      if (p.unrealizedPnl case final pnl?)
                        _Change(money: pnl, percent: p.changePercent ?? 0, currency: security.currency)
                      else
                        Text('нет цены', style: muted),
                    ],
                  ),
                ],
              ),
            ),
          ],
        ),
      ),
    );
  }

  /// `80` → `80`, `1.5` → `1,5`, `12000` → `12 000`.
  static String _quantity(double q) {
    final text = q == q.roundToDouble() ? q.toStringAsFixed(0) : q.toString();
    final parts = text.split('.');
    final digits = parts[0];
    final grouped = StringBuffer();
    for (var i = 0; i < digits.length; i++) {
      if (i > 0 && (digits.length - i) % 3 == 0) grouped.write('\u00A0');
      grouped.write(digits[i]);
    }
    return parts.length > 1 ? '$grouped,${parts[1]}' : '$grouped';
  }
}

/// `+75,30 ₽ ▲ 2,98%` / `−903,96 ₽ ▼ 5,22%`, цветом по знаку.
class _Change extends StatelessWidget {
  const _Change({required this.money, required this.percent, this.currency});

  final double money;
  final double percent;
  final String? currency;

  @override
  Widget build(BuildContext context) {
    final color = changeColor(context, money);
    final style = Theme.of(context)
        .textTheme
        .bodyMedium
        ?.copyWith(color: color, fontWeight: FontWeight.w500);
    final rounded = double.parse(money.toStringAsFixed(2));
    final sign = rounded > 0 ? '+' : '';
    final icon = rounded > 0
        ? Icons.arrow_drop_up_rounded
        : rounded < 0
            ? Icons.arrow_drop_down_rounded
            : null;
    return Text.rich(
      TextSpan(
        style: style,
        children: [
          TextSpan(text: '$sign${formatPrice(rounded, bond: false, currency: currency)} '),
          if (icon != null)
            WidgetSpan(
              alignment: PlaceholderAlignment.middle,
              child: Icon(icon, size: 22, color: color),
            ),
          TextSpan(text: formatPercent(percent.abs()).replaceFirst('+', '')),
        ],
      ),
      maxLines: 1,
    );
  }
}

class _CashTile extends StatelessWidget {
  const _CashTile({required this.amount});

  final double amount;

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    final textTheme = Theme.of(context).textTheme;
    final text = formatPrice(amount, bond: false, currency: 'RUB');
    return Padding(
      padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 12),
      child: Row(
        children: [
          const _RuFlag(size: 48),
          const SizedBox(width: 14),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                Row(
                  children: [
                    Expanded(child: Text('Рубль', style: textTheme.titleMedium)),
                    Text(text, style: textTheme.titleMedium?.copyWith(fontWeight: FontWeight.w700)),
                  ],
                ),
                const SizedBox(height: 4),
                Text(
                  'RUB • $text',
                  style: textTheme.bodyMedium?.copyWith(color: scheme.onSurfaceVariant),
                ),
              ],
            ),
          ),
        ],
      ),
    );
  }
}

/// Флаг России в скруглённой плашке — значок рублёвого остатка.
class _RuFlag extends StatelessWidget {
  const _RuFlag({required this.size});

  final double size;

  @override
  Widget build(BuildContext context) {
    return ClipRRect(
      borderRadius: BorderRadius.circular(size * 0.26),
      child: SizedBox(
        width: size,
        height: size,
        child: const Column(
          children: [
            Expanded(child: ColoredBox(color: Colors.white, child: SizedBox.expand())),
            Expanded(child: ColoredBox(color: Color(0xFF1C57A7), child: SizedBox.expand())),
            Expanded(child: ColoredBox(color: Color(0xFFD7263D), child: SizedBox.expand())),
          ],
        ),
      ),
    );
  }
}

class _Spinner extends StatelessWidget {
  const _Spinner();

  @override
  Widget build(BuildContext context) => const Padding(
        padding: EdgeInsets.only(top: 80),
        child: Center(child: CircularProgressIndicator()),
      );
}

class _Message extends StatelessWidget {
  const _Message({required this.text});

  final String text;

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    return Padding(
      padding: const EdgeInsets.fromLTRB(32, 80, 32, 16),
      child: Text(
        text,
        textAlign: TextAlign.center,
        style: Theme.of(context).textTheme.bodyLarge?.copyWith(color: scheme.onSurfaceVariant),
      ),
    );
  }
}
