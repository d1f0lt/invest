import 'dart:async';

import 'package:flutter/material.dart';

import '../api/api_client.dart';
import '../api/portfolio_api.dart';
import '../api/securities_api.dart';
import '../securities/security_directory.dart';
import '../securities/security_widgets.dart';
import 'operations_view.dart';
import 'portfolio_picker.dart';
import 'portfolio_store.dart';
import 'stats_format.dart';

/// Вкладка «В портфеле» карточки актива: позиция по бумаге в текущем
/// портфеле (стоимость, количество, сумма приобретения, доля), прибыль по
/// бумаге за всё время и её операции.
///
/// Позиция и прибыль — из уже загруженной сводки `/pnl` ([PortfolioStore]),
/// операции — `trades` + `cash-operations` портфеля, отфильтрованные по
/// тикеру. Перечитываются при смене портфеля и после обновления сводки.
class AssetPositionView extends StatefulWidget {
  const AssetPositionView({super.key, required this.security});

  final Security security;

  @override
  State<AssetPositionView> createState() => _AssetPositionViewState();
}

class _AssetPositionViewState extends State<AssetPositionView> {
  final _api = PortfolioApi();
  final _store = PortfolioStore.instance;

  List<PortfolioOperation>? _ops;

  /// Сколько всего потрачено на покупки бумаги (с комиссиями и НКД) —
  /// база для процента прибыли.
  double _invested = 0;
  String? _opsError;
  bool _opsLoading = false;

  String? _loadedFor;
  int _loadedRevision = -1;
  int _seq = 0;

  String get _secid => widget.security.secid;

  @override
  void initState() {
    super.initState();
    _store.addListener(_onStore);
    if (!_store.loaded && !_store.loading) {
      WidgetsBinding.instance.addPostFrameCallback((_) => _store.load());
    }
    _onStore();
  }

  @override
  void dispose() {
    _store.removeListener(_onStore);
    super.dispose();
  }

  void _onStore() {
    final id = _store.current?.id;
    if (id == null || !_store.hasStats(id)) return;
    if (id != _loadedFor || _store.revision != _loadedRevision) {
      if (id != _loadedFor) _ops = null;
      _loadedFor = id;
      _loadedRevision = _store.revision;
      _loadOps(id);
    }
  }

  Future<void> _loadOps(String portfolioId) async {
    final seq = ++_seq;
    setState(() {
      _opsLoading = true;
      _opsError = null;
    });
    try {
      final (trades, cash) = await (
        _api.trades(portfolioId),
        _api.cashOperations(portfolioId),
      ).wait;
      final own = trades.where((t) => t.secid == _secid).toList();
      final items = [
        ...own.map(PortfolioOperation.trade),
        ...cash
            .where((c) => c.secid == _secid && !c.isOpeningSecurities)
            .map(PortfolioOperation.cash),
      ]..sort((a, b) => b.at.compareTo(a.at));
      final invested = own
          .where((t) => t.isBuy)
          .fold(0.0, (sum, t) => sum + t.quantity * t.price + t.fee + t.accruedInterest);
      if (!mounted || seq != _seq) return;
      setState(() {
        _ops = items;
        _invested = invested;
        _opsLoading = false;
      });
    } catch (e) {
      if (!mounted || seq != _seq) return;
      final error = switch (e) {
        ParallelWaitError(:final errors) => _firstError(errors),
        _ => e,
      };
      setState(() {
        _opsError = error is ApiException ? error.message : 'Не удалось загрузить операции';
        _opsLoading = false;
      });
    }
  }

  static Object? _firstError(Object? errors) {
    if (errors is (AsyncError?, AsyncError?)) return (errors.$1 ?? errors.$2)?.error;
    return errors;
  }

  void _retryOps() {
    final id = _loadedFor;
    if (id != null) _loadOps(id);
  }

  @override
  Widget build(BuildContext context) {
    return ListenableBuilder(
      listenable: _store,
      builder: (context, _) => Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          const Align(alignment: Alignment.centerLeft, child: PortfolioSelector()),
          const SizedBox(height: 12),
          ..._content(context),
        ],
      ),
    );
  }

  List<Widget> _content(BuildContext context) {
    final portfolio = _store.current;
    if (portfolio == null || !_store.hasStats(portfolio.id)) {
      final failed = (_store.loaded && !_store.statsLoading) || (_store.error != null && !_store.loading);
      if (!failed) return const [_Loading()];
      return [
        _Notice(
          icon: Icons.cloud_off_rounded,
          title: 'Не удалось загрузить портфель',
          text: _store.error ?? 'Проверьте соединение и попробуйте ещё раз',
          action: OutlinedButton(
            onPressed: _store.loaded ? _store.loadStats : _store.load,
            child: const Text('Повторить'),
          ),
        ),
      ];
    }

    final stats = _store.statsFor(portfolio.id);
    final holding = _Holding.of(stats.instruments.where((p) => p.secid == _secid));
    if (holding == null) {
      return [
        _Notice(
          icon: Icons.business_center_outlined,
          title: 'Нет в портфеле',
          text: 'В портфеле «${portfolio.displayName}» нет операций с ${widget.security.secid}',
        ),
      ];
    }

    final total = stats.totalValue;
    final share = holding.open && total > 0 ? holding.value / total * 100 : null;

    return [
      if (holding.open)
        _PositionCard(holding: holding, share: share)
      else
        const _ClosedCard(),
      const SizedBox(height: 28),
      _ProfitSection(
        holding: holding,
        invested: _ops == null ? null : _invested,
        bond: widget.security.isBond || holding.coupons != 0,
      ),
      const SizedBox(height: 28),
      ..._operations(context),
    ];
  }

  List<Widget> _operations(BuildContext context) {
    final textTheme = Theme.of(context).textTheme;
    final scheme = Theme.of(context).colorScheme;
    final ops = _ops;

    final header = Row(
      children: [
        Text('Операции', style: textTheme.titleLarge?.copyWith(fontWeight: FontWeight.w700)),
        if (ops != null && ops.isNotEmpty) ...[
          const SizedBox(width: 8),
          Text('${ops.length}', style: textTheme.titleMedium?.copyWith(color: scheme.onSurfaceVariant)),
        ],
        const Spacer(),
        if (_opsLoading && ops != null)
          const SizedBox(width: 16, height: 16, child: CircularProgressIndicator(strokeWidth: 2)),
      ],
    );

    if (ops == null) {
      return [
        header,
        if (_opsError != null)
          Padding(
            padding: const EdgeInsets.symmetric(vertical: 16),
            child: Column(
              children: [
                Text(_opsError!, style: textTheme.bodyMedium?.copyWith(color: scheme.onSurfaceVariant)),
                const SizedBox(height: 8),
                OutlinedButton(onPressed: _retryOps, child: const Text('Повторить')),
              ],
            ),
          )
        else
          const _Loading(top: 24),
      ];
    }

    final children = <Widget>[header];
    if (_opsError != null) {
      children.add(Padding(
        padding: const EdgeInsets.only(top: 8),
        child: Text(_opsError!, style: textTheme.bodySmall?.copyWith(color: scheme.error)),
      ));
    }
    if (ops.isEmpty) {
      children.add(Padding(
        padding: const EdgeInsets.only(top: 12),
        child: Text('Операций нет', style: textTheme.bodyMedium?.copyWith(color: scheme.onSurfaceVariant)),
      ));
      return children;
    }
    DateTime? day;
    for (final op in ops) {
      final d = DateTime(op.at.year, op.at.month, op.at.day);
      if (d != day) {
        day = d;
        children.add(OperationDayHeader(day: d, padding: const EdgeInsets.fromLTRB(0, 16, 0, 4)));
      }
      children.add(OperationTile(
        op: op,
        directory: SecurityDirectory.instance,
        showSecurity: false,
        padding: const EdgeInsets.symmetric(vertical: 10),
      ));
    }
    return children;
  }
}

/// Позиция по бумаге в портфеле — сумма по всем режимам торгов с этим
/// тикером (одна и та же бумага могла прийти из отчётов с разными board).
class _Holding {
  const _Holding({
    required this.quantity,
    required this.cost,
    required this.marketValue,
    required this.unrealizedPnl,
    required this.dayChange,
    required this.realizedPnl,
    required this.dividends,
    required this.coupons,
    required this.accruedInterest,
    required this.totalPnl,
  });

  static _Holding? of(Iterable<Position> parts) {
    final list = parts.toList();
    if (list.isEmpty) return null;
    double sum(double Function(Position p) f) => list.fold(0.0, (s, p) => s + f(p));
    double? sumOpt(double? Function(Position p) f) {
      final open = list.where((p) => p.quantity > 0);
      if (open.isEmpty || open.any((p) => f(p) == null)) return null;
      return open.fold<double>(0.0, (s, p) => s + f(p)!);
    }

    return _Holding(
      quantity: sum((p) => p.quantity),
      cost: sum((p) => p.cost),
      marketValue: sumOpt((p) => p.marketValue),
      unrealizedPnl: sumOpt((p) => p.unrealizedPnl),
      dayChange: sumOpt((p) => p.dayChange),
      realizedPnl: sum((p) => p.realizedPnl),
      dividends: sum((p) => p.dividends),
      coupons: sum((p) => p.coupons),
      accruedInterest: sum((p) => p.accruedInterest),
      totalPnl: sum((p) => p.totalPnl),
    );
  }

  final double quantity;

  /// Сумма приобретения текущего количества (средняя цена × количество,
  /// комиссии покупок включены).
  final double cost;
  final double? marketValue;
  final double? unrealizedPnl;
  final double? dayChange;
  final double realizedPnl;
  final double dividends;
  final double coupons;
  final double accruedInterest;
  final double totalPnl;

  bool get open => quantity > 0;

  double get value => marketValue ?? cost;

  double get avgCost => quantity == 0 ? 0 : cost / quantity;

  double? get unrealizedPercent {
    final pnl = unrealizedPnl;
    return pnl == null || cost == 0 ? null : pnl / cost * 100;
  }

  double? get dayPercent {
    final change = dayChange, mv = marketValue;
    if (change == null || mv == null || mv - change == 0) return null;
    return change / (mv - change) * 100;
  }
}

class _PositionCard extends StatelessWidget {
  const _PositionCard({required this.holding, required this.share});

  final _Holding holding;
  final double? share;

  @override
  Widget build(BuildContext context) {
    final textTheme = Theme.of(context).textTheme;
    final scheme = Theme.of(context).colorScheme;
    final h = holding;
    final pnl = h.unrealizedPnl, pct = h.unrealizedPercent;
    final day = h.dayChange, dayPct = h.dayPercent;

    return _Card(
      children: [
        Padding(
          padding: const EdgeInsets.fromLTRB(0, 16, 0, 14),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text('Стоимость', style: textTheme.labelLarge?.copyWith(color: scheme.onSurfaceVariant)),
              const SizedBox(height: 2),
              Text(
                _rub(h.value),
                maxLines: 1,
                overflow: TextOverflow.ellipsis,
                style: textTheme.headlineSmall?.copyWith(fontWeight: FontWeight.w700),
              ),
              const SizedBox(height: 2),
              if (pnl != null && pct != null)
                _Signed(money: pnl, percent: pct, style: textTheme.bodyMedium)
              else
                Text(
                  'нет текущей цены — по цене покупки',
                  style: textTheme.bodySmall?.copyWith(color: scheme.onSurfaceVariant),
                ),
            ],
          ),
        ),
        _Line(label: 'Количество', value: '${_quantity(h.quantity)} шт.'),
        _Line(label: 'Сумма приобретения', value: _rub(h.cost)),
        _Line(label: 'Средняя цена', value: _rub(h.avgCost)),
        if (share != null) _Line(label: 'Доля в портфеле', value: _percent(share!)),
        if (day != null && dayPct != null)
          _Line(
            label: 'За день',
            child: _Signed(money: day, percent: dayPct, style: textTheme.bodyMedium, bold: true),
          ),
      ],
    );
  }
}

class _ClosedCard extends StatelessWidget {
  const _ClosedCard();

  @override
  Widget build(BuildContext context) {
    final textTheme = Theme.of(context).textTheme;
    final scheme = Theme.of(context).colorScheme;
    return _Card(
      children: [
        Padding(
          padding: const EdgeInsets.symmetric(vertical: 16),
          child: Row(
            children: [
              Icon(Icons.inventory_2_outlined, color: scheme.onSurfaceVariant),
              const SizedBox(width: 12),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text('Позиция закрыта', style: textTheme.titleSmall?.copyWith(fontWeight: FontWeight.w600)),
                    const SizedBox(height: 2),
                    Text(
                      'Сейчас этой бумаги в портфеле нет. Ниже — прибыль и операции за всё время',
                      style: textTheme.bodySmall?.copyWith(color: scheme.onSurfaceVariant),
                    ),
                  ],
                ),
              ),
            ],
          ),
        ),
      ],
    );
  }
}

/// «Прибыль»: итог за всё время крупно (в % — от всех покупок бумаги),
/// ниже — из чего он складывается.
class _ProfitSection extends StatelessWidget {
  const _ProfitSection({required this.holding, required this.invested, required this.bond});

  final _Holding holding;

  /// null — операции ещё не загружены, процент не показываем.
  final double? invested;
  final bool bond;

  @override
  Widget build(BuildContext context) {
    final textTheme = Theme.of(context).textTheme;
    final scheme = Theme.of(context).colorScheme;
    final h = holding;
    final base = invested;
    final percent = base != null && base > 0 ? h.totalPnl / base * 100 : null;
    final totalColor = changeColor(context, h.totalPnl);

    Widget money(double v) => Text(
          formatMoney(v),
          style: textTheme.bodyMedium?.copyWith(fontWeight: FontWeight.w600, color: changeColor(context, v)),
        );

    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Text('Прибыль', style: textTheme.titleLarge?.copyWith(fontWeight: FontWeight.w700)),
        const SizedBox(height: 12),
        _Card(
          children: [
            Padding(
              padding: const EdgeInsets.fromLTRB(0, 16, 0, 14),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text('За всё время', style: textTheme.labelLarge?.copyWith(color: scheme.onSurfaceVariant)),
                  const SizedBox(height: 2),
                  Text(
                    formatMoney(h.totalPnl),
                    style: textTheme.headlineSmall?.copyWith(fontWeight: FontWeight.w700, color: totalColor),
                  ),
                  if (percent != null) ...[
                    const SizedBox(height: 2),
                    Text(
                      '${formatPercent(percent)} от вложенных ${_rub(base!)}',
                      style: textTheme.bodySmall?.copyWith(color: scheme.onSurfaceVariant),
                    ),
                  ],
                ],
              ),
            ),
            if (h.open && h.unrealizedPnl != null)
              _Line(label: 'Изменение цены', child: money(h.unrealizedPnl!)),
            if (h.realizedPnl != 0 || !h.open)
              _Line(label: 'Зафиксированная', child: money(h.realizedPnl)),
            if (!bond || h.dividends != 0) _Line(label: 'Дивиденды', child: money(h.dividends)),
            if (bond) _Line(label: 'Купоны', child: money(h.coupons)),
            if (h.accruedInterest != 0) _Line(label: 'НКД', child: money(h.accruedInterest)),
          ],
        ),
        const SizedBox(height: 8),
        Text(
          'Комиссии учтены в цене покупки и продажи, дивиденды — после налога',
          style: textTheme.bodySmall?.copyWith(color: scheme.onSurfaceVariant),
        ),
      ],
    );
  }
}

/// `+1 234,00 ₽ (+5,2%)` — цветом по знаку.
class _Signed extends StatelessWidget {
  const _Signed({required this.money, required this.percent, this.style, this.bold = false});

  final double money;
  final double percent;
  final TextStyle? style;
  final bool bold;

  @override
  Widget build(BuildContext context) {
    return Text(
      '${formatMoney(money)} (${formatPercent(percent)})',
      textAlign: TextAlign.right,
      style: style?.copyWith(
        color: changeColor(context, money),
        fontWeight: bold ? FontWeight.w600 : FontWeight.w500,
      ),
    );
  }
}

/// Карточка в стиле «О компании»: скруглённая рамка, строки через разделитель.
class _Card extends StatelessWidget {
  const _Card({required this.children});

  final List<Widget> children;

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    final divider = Divider(height: 1, color: scheme.outlineVariant.withValues(alpha: 0.6));
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 2),
      decoration: BoxDecoration(
        color: scheme.surfaceContainerLow,
        borderRadius: BorderRadius.circular(20),
        border: Border.all(color: scheme.outlineVariant),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          for (var i = 0; i < children.length; i++) ...[
            if (i > 0) divider,
            children[i],
          ],
        ],
      ),
    );
  }
}

/// Строка карточки: подпись слева, значение справа.
class _Line extends StatelessWidget {
  const _Line({required this.label, this.value, this.child});

  final String label;
  final String? value;
  final Widget? child;

  @override
  Widget build(BuildContext context) {
    final textTheme = Theme.of(context).textTheme;
    final scheme = Theme.of(context).colorScheme;
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 13),
      child: Row(
        children: [
          Text(label, style: textTheme.bodyMedium?.copyWith(color: scheme.onSurfaceVariant)),
          const SizedBox(width: 16),
          Expanded(
            child: Align(
              alignment: Alignment.centerRight,
              child: child ??
                  Text(
                    value ?? '',
                    textAlign: TextAlign.right,
                    style: textTheme.bodyMedium?.copyWith(fontWeight: FontWeight.w600),
                  ),
            ),
          ),
        ],
      ),
    );
  }
}

class _Notice extends StatelessWidget {
  const _Notice({required this.icon, required this.title, required this.text, this.action});

  final IconData icon;
  final String title;
  final String text;
  final Widget? action;

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    final textTheme = Theme.of(context).textTheme;
    return Padding(
      padding: const EdgeInsets.fromLTRB(24, 32, 24, 0),
      child: Column(
        children: [
          Icon(icon, size: 48, color: scheme.onSurfaceVariant),
          const SizedBox(height: 12),
          Text(title, style: textTheme.titleMedium?.copyWith(fontWeight: FontWeight.w600)),
          const SizedBox(height: 6),
          Text(
            text,
            textAlign: TextAlign.center,
            style: textTheme.bodyMedium?.copyWith(color: scheme.onSurfaceVariant),
          ),
          if (action != null) ...[const SizedBox(height: 12), action!],
        ],
      ),
    );
  }
}

class _Loading extends StatelessWidget {
  const _Loading({this.top = 48});

  final double top;

  @override
  Widget build(BuildContext context) => Padding(
        padding: EdgeInsets.only(top: top),
        child: const Center(child: CircularProgressIndicator()),
      );
}

/// Суммы позиции всегда в рублях: цены облигаций бэкенд уже пересчитал
/// из % номинала.
String _rub(double v) => formatPrice(v, bond: false, currency: 'RUB');

/// Доля без знака: `12,5%`.
String _percent(double v) => formatPercent(v).replaceFirst('+', '');

String _quantity(double q) {
  if (q == q.roundToDouble()) {
    final digits = q.toStringAsFixed(0);
    final grouped = StringBuffer();
    for (var i = 0; i < digits.length; i++) {
      if (i > 0 && (digits.length - i) % 3 == 0) grouped.write(' ');
      grouped.write(digits[i]);
    }
    return grouped.toString();
  }
  return q.toString().replaceAll('.', ',');
}
