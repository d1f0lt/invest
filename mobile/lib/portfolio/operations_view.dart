import 'dart:async';

import 'package:flutter/material.dart';

import '../api/api_client.dart';
import '../api/portfolio_api.dart';
import '../securities/security_directory.dart';
import '../securities/security_widgets.dart';
import 'portfolio_app_bar.dart';
import 'portfolio_store.dart';
import 'stats_format.dart';

/// Одна строка ленты: сделка или денежная операция.
class _Operation {
  _Operation.trade(Trade t)
      : at = t.executedAt.toLocal(),
        type = t.isBuy ? 'buy' : 'sell',
        amount = t.cashFlow,
        currency = t.currency,
        secid = t.secid,
        board = t.board,
        trade = t,
        opening = t.isOpening,
        description = '';

  _Operation.cash(CashOperation c)
      : at = c.occurredAt.toLocal(),
        type = c.type,
        amount = c.amount,
        currency = c.currency,
        secid = c.secid,
        board = c.board,
        trade = null,
        opening = c.isOpening,
        description = c.description;

  final DateTime at;
  final String type;

  /// Движение денег со знаком.
  final double amount;
  final String currency;
  final String? secid;
  final String? board;
  final Trade? trade;
  final String description;

  /// Вводный остаток — было на счёте на начало отчёта.
  final bool opening;

  String get title => opening ? 'Вводный остаток' : switch (type) {
        'buy' => 'Покупка',
        'sell' => 'Продажа',
        'deposit' => 'Пополнение',
        'withdrawal' => 'Вывод средств',
        'dividend' => 'Дивиденды',
        'coupon' => 'Купон',
        'redemption' => 'Погашение',
        'tax' => 'Налог',
        'fee' => 'Комиссия',
        _ => 'Прочее',
      };

  IconData get icon => switch (type) {
        'deposit' => Icons.account_balance_wallet_rounded,
        'withdrawal' => Icons.north_east_rounded,
        'dividend' || 'coupon' => Icons.payments_rounded,
        'redemption' => Icons.event_available_rounded,
        'tax' => Icons.account_balance_rounded,
        'fee' => Icons.receipt_long_rounded,
        _ => Icons.more_horiz_rounded,
      };
}

/// «Операции»: сделки и денежные операции текущего портфеля, новые сверху,
/// с разбивкой по дням. Перечитываются при смене портфеля и после
/// обновления сводки (например, после загрузки отчёта).
class OperationsView extends StatefulWidget {
  const OperationsView({super.key});

  @override
  State<OperationsView> createState() => _OperationsViewState();
}

class _OperationsViewState extends State<OperationsView> with AutomaticKeepAliveClientMixin {
  final _api = PortfolioApi();
  final _store = PortfolioStore.instance;
  final _directory = SecurityDirectory.instance;

  List<_Operation>? _items;
  String? _error;
  bool _loading = false;
  Future<void>? _pending;

  /// Для каких портфеля и ревизии сводки загружены данные.
  String? _loadedFor;
  int _loadedRevision = -1;
  int _seq = 0;

  @override
  bool get wantKeepAlive => true;

  @override
  void initState() {
    super.initState();
    _store.addListener(_onStore);
    _onStore();
  }

  @override
  void dispose() {
    _store.removeListener(_onStore);
    super.dispose();
  }

  void _onStore() {
    final id = _store.current?.id;
    if (id == null) return;
    if (id != _loadedFor || _store.revision != _loadedRevision) {
      if (id != _loadedFor) _items = null; // другой портфель — не показываем чужие операции
      _loadedFor = id;
      _loadedRevision = _store.revision;
      _pending = _load(id);
    }
  }

  Future<void> _load(String portfolioId) async {
    final seq = ++_seq;
    setState(() {
      _loading = true;
      _error = null;
    });
    try {
      final (trades, cash) = await (
        _api.trades(portfolioId),
        _api.cashOperations(portfolioId),
      ).wait;
      final items = [
        ...trades.map(_Operation.trade),
        ...cash.where((c) => !c.isOpeningSecurities).map(_Operation.cash),
      ]..sort((a, b) => b.at.compareTo(a.at));
      if (!mounted || seq != _seq) return;
      setState(() {
        _items = items;
        _loading = false;
      });
      final secids = items.map((o) => o.secid).whereType<String>();
      if (await _directory.ensure(secids) && mounted) setState(() {});
    } catch (e) {
      if (!mounted || seq != _seq) return;
      final error = switch (e) {
        ParallelWaitError(:final errors) => _firstError(errors),
        _ => e,
      };
      setState(() {
        _error = error is ApiException ? error.message : 'Не удалось загрузить операции';
        _loading = false;
      });
    }
  }

  static Object? _firstError(Object? errors) {
    if (errors is (AsyncError?, AsyncError?)) return (errors.$1 ?? errors.$2)?.error;
    return errors;
  }

  Future<void> _refresh() async {
    if (!_store.loaded) return _store.load();
    await _store.loadStats(); // revision++ → _onStore запустит перезагрузку
    await _pending;
  }

  @override
  Widget build(BuildContext context) {
    super.build(context);
    return RefreshIndicator(
      onRefresh: _refresh,
      child: ListView(
        physics: const AlwaysScrollableScrollPhysics(),
        padding: const EdgeInsets.only(bottom: 24),
        children: _content(context),
      ),
    );
  }

  List<Widget> _content(BuildContext context) {
    final items = _items;
    final scheme = Theme.of(context).colorScheme;
    if (items == null) {
      if (_error != null) return [_Message(text: _error!, color: scheme.error)];
      return const [
        Padding(
          padding: EdgeInsets.only(top: 80),
          child: Center(child: CircularProgressIndicator()),
        ),
      ];
    }
    if (items.isEmpty) return const [EmptyPortfolio()];

    final children = <Widget>[
      SizedBox(height: 2, child: _loading ? const LinearProgressIndicator() : null),
      if (_error != null)
        Padding(
          padding: const EdgeInsets.fromLTRB(16, 8, 16, 0),
          child: Text(_error!, style: TextStyle(color: scheme.error)),
        ),
    ];
    DateTime? day;
    for (final op in items) {
      final d = DateTime(op.at.year, op.at.month, op.at.day);
      if (d != day) {
        day = d;
        children.add(_DayHeader(day: d));
      }
      children.add(_OperationTile(op: op, directory: _directory));
    }
    return children;
  }
}

const _months = [
  'января', 'февраля', 'марта', 'апреля', 'мая', 'июня',
  'июля', 'августа', 'сентября', 'октября', 'ноября', 'декабря',
];

class _DayHeader extends StatelessWidget {
  const _DayHeader({required this.day});

  final DateTime day;

  String _label() {
    final now = DateTime.now();
    final today = DateTime(now.year, now.month, now.day);
    final diff = today.difference(day).inDays;
    if (diff == 0) return 'Сегодня';
    if (diff == 1) return 'Вчера';
    final base = '${day.day} ${_months[day.month - 1]}';
    return day.year == now.year ? base : '$base ${day.year}';
  }

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.fromLTRB(16, 20, 16, 4),
      child: Text(
        _label(),
        style: Theme.of(context).textTheme.titleMedium?.copyWith(fontWeight: FontWeight.w700),
      ),
    );
  }
}

class _OperationTile extends StatelessWidget {
  const _OperationTile({required this.op, required this.directory});

  final _Operation op;
  final SecurityDirectory directory;

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    final textTheme = Theme.of(context).textTheme;
    final secid = op.secid;
    final security = secid == null ? null : directory.lookup(secid, op.board ?? '');

    final subtitle = switch (op.trade) {
      final t? => '${security?.title ?? t.secid} · ${_quantity(t.quantity)} шт. × '
          '${formatPrice(t.price, bond: false, currency: t.currency)}',
      null => security?.title ?? op.description,
    };

    final rounded = double.parse(op.amount.toStringAsFixed(2));
    final String amountText;
    final Color amountColor;
    if (op.opening && op.trade != null) {
      // Бумага на начало периода: деньги не двигались — показываем её стоимость.
      amountText = formatPrice(rounded.abs(), bond: false, currency: op.currency);
      amountColor = scheme.onSurfaceVariant;
    } else {
      amountText = '${rounded > 0 ? '+' : ''}'
          '${formatPrice(rounded, bond: false, currency: op.currency)}';
      amountColor = rounded > 0 ? changeColor(context, rounded) : scheme.onSurface;
    }

    return Padding(
      padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 10),
      child: Row(
        children: [
          if (secid != null)
            TickerBadge(secid: secid, size: 44, square: true)
          else
            Container(
              width: 44,
              height: 44,
              decoration: BoxDecoration(
                color: scheme.secondaryContainer,
                borderRadius: BorderRadius.circular(44 * 0.26),
              ),
              child: Icon(op.icon, color: scheme.onSecondaryContainer),
            ),
          const SizedBox(width: 14),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(op.title, style: textTheme.titleSmall?.copyWith(fontWeight: FontWeight.w600)),
                if (subtitle.isNotEmpty) ...[
                  const SizedBox(height: 2),
                  Text(
                    subtitle,
                    maxLines: 1,
                    overflow: TextOverflow.ellipsis,
                    style: textTheme.bodySmall?.copyWith(color: scheme.onSurfaceVariant),
                  ),
                ],
              ],
            ),
          ),
          const SizedBox(width: 12),
          Text(
            amountText,
            style: textTheme.titleSmall?.copyWith(fontWeight: FontWeight.w700, color: amountColor),
          ),
        ],
      ),
    );
  }

  static String _quantity(double q) =>
      q == q.roundToDouble() ? q.toStringAsFixed(0) : q.toString().replaceAll('.', ',');
}

class _Message extends StatelessWidget {
  const _Message({required this.text, required this.color});

  final String text;
  final Color color;

  @override
  Widget build(BuildContext context) => Padding(
        padding: const EdgeInsets.fromLTRB(32, 80, 32, 16),
        child: Text(
          '$text\nПотяните вниз, чтобы повторить',
          textAlign: TextAlign.center,
          style: Theme.of(context).textTheme.bodyLarge?.copyWith(color: color),
        ),
      );
}
