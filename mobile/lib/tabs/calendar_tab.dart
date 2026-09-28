import 'package:flutter/material.dart';

import '../api/api_client.dart';
import '../api/portfolio_api.dart';
import '../api/securities_api.dart';
import '../calendar/payouts.dart';
import '../calendar/payouts_chart.dart';
import '../portfolio/portfolio_app_bar.dart';
import '../portfolio/portfolio_store.dart';
import '../portfolio/stats_format.dart';
import '../securities/asset_screen.dart';
import '../securities/security_directory.dart';
import '../securities/security_widgets.dart';

/// Календарь дивидендов текущего портфеля: итог за период, в месяц и в день,
/// столбики по месяцам (получены / объявлены / прогноз) и список выплат.
///
/// Полученные — операции `dividend` портфеля; ожидаемые — дивиденды
/// по бумагам из открытых позиций (`/securities/{secid}/dividends`,
/// источник dohod.ru) × текущее количество, по дате закрытия реестра.
class CalendarTab extends StatefulWidget {
  const CalendarTab({super.key});

  @override
  State<CalendarTab> createState() => _CalendarTabState();
}

class _CalendarTabState extends State<CalendarTab> {
  final _store = PortfolioStore.instance;
  final _directory = SecurityDirectory.instance;
  final _portfolioApi = PortfolioApi();
  final _securitiesApi = SecuritiesApi();

  /// Дивиденды по тикеру — на время запуска (сервер и так кэширует сутки),
  /// сбрасываются при pull-to-refresh.
  final Map<String, List<Dividend>> _dividendCache = {};

  List<CashOperation>? _operations;
  int _failed = 0;
  String? _error;
  bool _loading = false;
  Future<void>? _pending;
  String? _loadedFor;
  int _loadedRevision = -1;
  int _seq = 0;

  CalendarPeriod? _period;
  bool _withTax = true;
  bool _showForecast = true;

  final Map<DateTime, GlobalKey> _monthKeys = {};

  @override
  void initState() {
    super.initState();
    _store.addListener(_onStore);
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (mounted) _onStore();
    });
  }

  @override
  void dispose() {
    _store.removeListener(_onStore);
    super.dispose();
  }

  void _onStore() {
    final id = _store.current?.id;
    if (id == null || !_store.hasStats(id)) return;
    if (id == _loadedFor && _store.revision == _loadedRevision) return;
    if (id != _loadedFor) {
      _operations = null;
      _period = null;
    }
    _loadedFor = id;
    _loadedRevision = _store.revision;
    _pending = _load(id);
  }

  Future<void> _load(String portfolioId) async {
    final seq = ++_seq;
    setState(() {
      _loading = true;
      _error = null;
    });
    final positions = _store.statsFor(portfolioId).positions;
    final secids = {
      for (final p in positions)
        if (!Security(secid: p.secid, board: p.board).isBond) p.secid,
    };
    var failed = 0;
    Future<void> loadDividends(String secid) async {
      if (_dividendCache.containsKey(secid)) return;
      try {
        _dividendCache[secid] = await _securitiesApi.dividends(secid);
      } catch (_) {
        failed++;
      }
    }

    try {
      final results = await Future.wait<Object?>([
        _portfolioApi.cashOperations(portfolioId),
        ...secids.map(loadDividends),
      ]);
      if (!mounted || seq != _seq) return;
      final operations = results.first as List<CashOperation>;
      setState(() {
        _operations = operations;
        _failed = failed;
        _loading = false;
      });
      final names = {
        ...secids,
        for (final op in operations)
          if (op.type == 'dividend' && op.secid != null) op.secid!,
      };
      if (await _directory.ensure(names) && mounted) setState(() {});
    } catch (e) {
      if (!mounted || seq != _seq) return;
      setState(() {
        _error = e is ApiException ? e.message : 'Не удалось загрузить выплаты';
        _loading = false;
      });
    }
  }

  Future<void> _refresh() async {
    if (!_store.loaded) return _store.load();
    _dividendCache.clear();
    await _store.loadStats();
    await _pending;
  }

  void _scrollToMonth(DateTime month) {
    final ctx = _monthKeys[month]?.currentContext;
    if (ctx == null) return;
    Scrollable.ensureVisible(
      ctx,
      duration: const Duration(milliseconds: 350),
      curve: Curves.easeOutCubic,
    );
  }

  void _openAsset(Payout p) {
    final secid = p.secid, board = p.board;
    if (secid == null || board == null || board.isEmpty) return;
    Navigator.of(context).push(
      MaterialPageRoute<void>(
        builder: (_) => AssetScreen(security: _directory.lookup(secid, board)),
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: portfolioAppBar(context),
      body: RefreshIndicator(
        onRefresh: _refresh,
        child: ListenableBuilder(
          listenable: _store,
          builder: (context, _) => ListView(
            physics: const AlwaysScrollableScrollPhysics(),
            padding: const EdgeInsets.fromLTRB(16, 16, 16, 32),
            children: _content(context),
          ),
        ),
      ),
    );
  }

  List<Widget> _content(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    final textTheme = Theme.of(context).textTheme;
    final current = _store.current;
    const spinner = Padding(
      padding: EdgeInsets.only(top: 80),
      child: Center(child: CircularProgressIndicator()),
    );
    if (current == null) {
      if (_store.error != null) return [_Message(text: _store.error!, color: scheme.error)];
      return const [spinner];
    }
    if (!_store.hasStats(current.id)) {
      if (_store.statsLoading) return const [spinner];
      return [_Message(text: 'Не удалось загрузить портфель', color: scheme.error)];
    }
    final stats = _store.statsFor(current.id);
    if (!stats.hasData) return const [EmptyPortfolio()];

    final operations = _operations;
    if (operations == null) {
      if (_error != null) return [_Message(text: _error!, color: scheme.error)];
      return const [spinner];
    }

    final now = DateTime.now();
    final today = dateOnly(now);
    final all = buildPayouts(
      operations: operations,
      positions: stats.positions,
      dividends: _dividendCache,
      today: today,
      withTax: _withTax,
    ).where((p) => _showForecast || p.status != PayoutStatus.forecast).toList();

    final periods = periodsFor(all, today);
    final period = periods.contains(_period) ? _period! : periods.first;
    final payouts = all.where((p) => period.contains(p.date, today)).toList();
    final total = payouts.fold(0.0, (s, p) => s + p.amount);
    final months = monthTotals(payouts);
    final statuses = {for (final p in payouts) p.status};

    return [
      Pills<CalendarPeriod>(
        values: periods,
        selected: period,
        label: (p) => p.label,
        onSelected: (p) => setState(() => _period = p),
      ),
      SizedBox(
        height: 3,
        child: _loading ? const LinearProgressIndicator() : null,
      ),
      const SizedBox(height: 17),
      _Summary(
        label: period.ahead ? 'За 12 месяцев' : 'Всего за год',
        total: total,
        withTax: _withTax,
        showForecast: _showForecast,
        onTax: (v) => setState(() => _withTax = v),
        onForecast: (v) => setState(() => _showForecast = v),
      ),
      if (_error != null) ...[
        const SizedBox(height: 12),
        Text(_error!, style: TextStyle(color: scheme.error)),
      ],
      if (_failed > 0) ...[
        const SizedBox(height: 12),
        Text(
          'Не удалось загрузить дивиденды по ${_failed == 1 ? 'одной бумаге' : '$_failed бумагам'}'
          ' — потяните вниз, чтобы повторить',
          style: textTheme.bodySmall?.copyWith(color: scheme.error),
        ),
      ],
      const SizedBox(height: 28),
      if (payouts.isEmpty)
        Padding(
          padding: const EdgeInsets.symmetric(vertical: 40),
          child: Column(
            children: [
              Icon(Icons.event_busy_rounded, size: 48, color: scheme.onSurfaceVariant),
              const SizedBox(height: 12),
              Text(
                period.ahead
                    ? 'В ближайший год выплат не ожидается'
                    : 'В ${period.year} году выплат нет',
                textAlign: TextAlign.center,
                style: textTheme.bodyLarge?.copyWith(color: scheme.onSurfaceVariant),
              ),
            ],
          ),
        )
      else ...[
        PayoutsChart(months: months, onTap: _scrollToMonth),
        const SizedBox(height: 12),
        PayoutsLegend(statuses: PayoutStatus.values.where(statuses.contains)),
        const SizedBox(height: 12),
        ..._list(context, payouts, months),
        if (statuses.any((s) => s != PayoutStatus.received)) ...[
          const SizedBox(height: 20),
          Text(
            'Ожидаемые выплаты — по данным dohod.ru на текущее количество бумаг'
            '${_withTax ? ', за вычетом НДФЛ 13%' : ', до налога'}. '
            'Дата — закрытие реестра, деньги приходят позже.',
            style: textTheme.bodySmall?.copyWith(color: scheme.onSurfaceVariant),
          ),
        ],
      ],
    ];
  }

  List<Widget> _list(BuildContext context, List<Payout> payouts, List<MonthTotal> months) {
    final children = <Widget>[];
    final totals = {for (final t in months) t.month: t};
    final newestFirst = [...payouts]..sort((a, b) {
        final byDate = b.date.compareTo(a.date);
        return byDate != 0 ? byDate : b.amount.compareTo(a.amount);
      });
    DateTime? month;
    (DateTime, PayoutStatus, bool)? day;
    for (final p in newestFirst) {
      final m = DateTime(p.date.year, p.date.month);
      if (m != month) {
        month = m;
        day = null;
        final key = _monthKeys.putIfAbsent(m, GlobalKey.new);
        children.add(_MonthHeader(key: key, total: totals[m]!));
      }
      final d = (p.date, p.status, p.awaiting);
      if (d != day) {
        day = d;
        children.add(_DayHeader(payout: p));
      }
      children.add(_PayoutTile(
        payout: p,
        security: p.secid == null ? null : _directory.lookup(p.secid!, p.board ?? ''),
        onTap: () => _openAsset(p),
      ));
    }
    return children;
  }
}

class _Summary extends StatelessWidget {
  const _Summary({
    required this.label,
    required this.total,
    required this.withTax,
    required this.showForecast,
    required this.onTax,
    required this.onForecast,
  });

  final String label;
  final double total;
  final bool withTax;
  final bool showForecast;
  final ValueChanged<bool> onTax;
  final ValueChanged<bool> onForecast;

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    final textTheme = Theme.of(context).textTheme;
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Row(
          children: [
            Expanded(
              child: Text(
                label,
                style: textTheme.titleMedium?.copyWith(color: scheme.onSurfaceVariant),
              ),
            ),
            PopupMenuButton<String>(
              tooltip: 'Настройки',
              icon: const Icon(Icons.more_horiz_rounded),
              onSelected: (v) {
                if (v == 'tax') {
                  onTax(!withTax);
                } else {
                  onForecast(!showForecast);
                }
              },
              itemBuilder: (_) => [
                CheckedPopupMenuItem(
                  value: 'tax',
                  checked: withTax,
                  child: const Text('За вычетом НДФЛ 13%'),
                ),
                CheckedPopupMenuItem(
                  value: 'forecast',
                  checked: showForecast,
                  child: const Text('Показывать прогнозы'),
                ),
              ],
            ),
          ],
        ),
        FittedBox(
          fit: BoxFit.scaleDown,
          alignment: Alignment.centerLeft,
          child: Text(
            formatMoney(total).replaceFirst('+', ''),
            style: textTheme.displaySmall?.copyWith(fontWeight: FontWeight.w700),
          ),
        ),
        const SizedBox(height: 16),
        _Average(label: 'В месяц', value: total / 12),
        _Average(label: 'В день', value: total / 365),
      ],
    );
  }
}

class _Average extends StatelessWidget {
  const _Average({required this.label, required this.value});

  final String label;
  final double value;

  @override
  Widget build(BuildContext context) {
    final textTheme = Theme.of(context).textTheme;
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 5),
      child: Row(
        children: [
          Container(
            width: 18,
            height: 18,
            decoration: BoxDecoration(
              shape: BoxShape.circle,
              border: Border.all(color: payoutColor(PayoutStatus.declared), width: 2),
            ),
          ),
          const SizedBox(width: 12),
          Expanded(child: Text(label, style: textTheme.bodyLarge)),
          Text(
            formatMoney(value).replaceFirst('+', ''),
            style: textTheme.bodyLarge?.copyWith(fontWeight: FontWeight.w700),
          ),
        ],
      ),
    );
  }
}

const _monthNames = [
  'январь', 'февраль', 'март', 'апрель', 'май', 'июнь',
  'июль', 'август', 'сентябрь', 'октябрь', 'ноябрь', 'декабрь',
];

const _monthsGenitive = [
  'янв.', 'февр.', 'мар.', 'апр.', 'мая', 'июн.',
  'июл.', 'авг.', 'сент.', 'окт.', 'нояб.', 'дек.',
];

const _weekdays = ['пн', 'вт', 'ср', 'чт', 'пт', 'сб', 'вс'];

class _MonthHeader extends StatelessWidget {
  const _MonthHeader({super.key, required this.total});

  final MonthTotal total;

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    final textTheme = Theme.of(context).textTheme;
    final received = total.byStatus.keys.every((s) => s == PayoutStatus.received);
    final amount = total.total;
    return Padding(
      padding: const EdgeInsets.only(top: 28, bottom: 4),
      child: Row(
        children: [
          Expanded(
            child: Text(
              '${_monthNames[total.month.month - 1]} ${total.month.year}',
              style: textTheme.headlineSmall?.copyWith(fontWeight: FontWeight.w700),
            ),
          ),
          Container(
            padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 8),
            decoration: BoxDecoration(
              color: scheme.surfaceContainerHighest,
              borderRadius: BorderRadius.circular(20),
            ),
            child: Text(
              formatMoney(amount),
              style: textTheme.titleSmall?.copyWith(
                fontWeight: FontWeight.w700,
                color: received ? changeColor(context, amount) : scheme.onSurface,
              ),
            ),
          ),
        ],
      ),
    );
  }
}

class _DayHeader extends StatelessWidget {
  const _DayHeader({required this.payout});

  final Payout payout;

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    final textTheme = Theme.of(context).textTheme;
    final d = payout.date;
    final color = payoutColor(payout.status);
    final chip = payout.awaiting ? 'Ждём выплату' : payoutStatusLabel(payout.status);
    return Padding(
      padding: const EdgeInsets.only(top: 16, bottom: 4),
      child: Row(
        children: [
          Icon(Icons.calendar_today_outlined, size: 16, color: scheme.onSurfaceVariant),
          const SizedBox(width: 8),
          Expanded(
            child: Text.rich(
              TextSpan(
                children: [
                  TextSpan(
                    text: '${d.day} ${_monthsGenitive[d.month - 1]}',
                    style: const TextStyle(fontWeight: FontWeight.w700),
                  ),
                  TextSpan(
                    text: ', ${_weekdays[d.weekday - 1]}',
                    style: TextStyle(color: scheme.onSurfaceVariant),
                  ),
                ],
              ),
              style: textTheme.titleSmall,
            ),
          ),
          Container(
            padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 5),
            decoration: BoxDecoration(
              color: color.withValues(alpha: 0.18),
              borderRadius: BorderRadius.circular(14),
            ),
            child: Text(
              chip,
              style: textTheme.labelLarge?.copyWith(
                color: Color.lerp(color, scheme.onSurface, 0.35),
                fontWeight: FontWeight.w600,
              ),
            ),
          ),
        ],
      ),
    );
  }
}

class _PayoutTile extends StatelessWidget {
  const _PayoutTile({required this.payout, required this.security, required this.onTap});

  final Payout payout;
  final Security? security;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    final textTheme = Theme.of(context).textTheme;
    final secid = payout.secid;
    final name = security?.title;
    final title = secid == null
        ? 'Дивиденды'
        : (name == null || name == secid ? secid : '$secid $name');

    final String subtitle;
    if (payout.status == PayoutStatus.received) {
      subtitle = 'Зачислено на счёт';
    } else {
      final perShare = formatPrice(payout.perShare ?? 0, bond: false);
      subtitle = '${_quantity(payout.quantity ?? 0)} шт. × $perShare · '
          '${payout.awaiting ? 'реестр закрыт' : 'закрытие реестра'}';
    }

    final amount = payout.amount;
    final amountColor = switch (payout.status) {
      PayoutStatus.received => changeColor(context, amount),
      PayoutStatus.declared => scheme.onSurface,
      PayoutStatus.forecast => scheme.onSurfaceVariant,
    };
    final prefix = payout.status == PayoutStatus.forecast ? '≈ ' : '';

    return InkWell(
      onTap: onTap,
      borderRadius: BorderRadius.circular(12),
      child: Padding(
        padding: const EdgeInsets.symmetric(vertical: 10),
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
                child: Icon(Icons.payments_rounded, color: scheme.onSecondaryContainer),
              ),
            const SizedBox(width: 14),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    title,
                    maxLines: 1,
                    overflow: TextOverflow.ellipsis,
                    style: textTheme.titleSmall?.copyWith(fontWeight: FontWeight.w600),
                  ),
                  const SizedBox(height: 2),
                  Text(
                    subtitle,
                    maxLines: 1,
                    overflow: TextOverflow.ellipsis,
                    style: textTheme.bodySmall?.copyWith(color: scheme.onSurfaceVariant),
                  ),
                ],
              ),
            ),
            const SizedBox(width: 12),
            Text(
              '$prefix${formatMoney(amount)}',
              style: textTheme.titleSmall?.copyWith(fontWeight: FontWeight.w700, color: amountColor),
            ),
          ],
        ),
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
        padding: const EdgeInsets.fromLTRB(16, 80, 16, 16),
        child: Text(
          '$text\nПотяните вниз, чтобы повторить',
          textAlign: TextAlign.center,
          style: Theme.of(context).textTheme.bodyLarge?.copyWith(color: color),
        ),
      );
}
