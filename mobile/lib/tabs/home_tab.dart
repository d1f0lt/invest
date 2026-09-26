import 'package:flutter/material.dart';

import '../api/api_client.dart';
import '../api/portfolio_api.dart';
import '../api/securities_api.dart';
import '../portfolio/portfolio_app_bar.dart';
import '../portfolio/portfolio_store.dart';
import '../portfolio/stats_format.dart';
import '../securities/asset_screen.dart';
import '../securities/price_chart.dart';
import '../securities/security_directory.dart';
import '../securities/security_widgets.dart';

/// Главная: текущая стоимость портфеля, прибыль, изменение за день,
/// пассивный доход, график стоимости; ниже — топ изменений за день среди
/// бумаг портфеля и его состав.
class HomeTab extends StatefulWidget {
  const HomeTab({super.key});

  @override
  State<HomeTab> createState() => _HomeTabState();
}

class _HomeTabState extends State<HomeTab> {
  final _store = PortfolioStore.instance;
  final _directory = SecurityDirectory.instance;
  final _portfolioApi = PortfolioApi();

  static const _maxMovers = 5;

  ValueRange _range = ValueRange.all;

  /// Графики по ключу «портфель/период/ревизия сводки» — после загрузки
  /// отчёта ревизия меняется, и график перечитывается.
  final Map<String, List<ValuePoint>> _history = {};
  String? _historyLoadingKey;
  String? _historyError;
  int? _scrub;

  bool _gainers = true;

  @override
  void initState() {
    super.initState();
    if (!_store.loaded) _store.load();
    _store.addListener(_onStoreChanged);
    // setState — только после первого кадра, не во время сборки дерева.
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (!mounted) return;
      _onStoreChanged();
    });
  }

  @override
  void dispose() {
    _store.removeListener(_onStoreChanged);
    super.dispose();
  }

  String? _historyKey() {
    final current = _store.current;
    if (current == null || !_store.hasStats(current.id)) return null;
    return '${current.id}/${_range.apiName}/${_store.revision}';
  }

  void _onStoreChanged() {
    final current = _store.current;
    if (current == null) return;
    _loadHistory();
    final positions = _store.statsFor(current.id).positions;
    _directory.ensure(positions.map((p) => p.secid)).then((added) {
      if (added && mounted) setState(() {});
    });
  }

  Future<void> _loadHistory() async {
    final key = _historyKey();
    final current = _store.current;
    if (key == null || current == null) return;
    if (_history.containsKey(key) || _historyLoadingKey == key) return;
    if (!_store.statsFor(current.id).hasData) return;
    final range = _range;
    setState(() {
      _historyLoadingKey = key;
      _historyError = null;
    });
    try {
      final points = await _portfolioApi.history(current.id, range);
      if (!mounted) return;
      setState(() {
        _history[key] = points;
        _scrub = null;
      });
    } on ApiException catch (e) {
      if (mounted) setState(() => _historyError = e.message);
    } catch (_) {
      if (mounted) setState(() => _historyError = 'Не удалось загрузить график');
    } finally {
      if (mounted && _historyLoadingKey == key) setState(() => _historyLoadingKey = null);
    }
  }

  void _selectRange(ValueRange range) {
    if (range == _range) return;
    setState(() {
      _range = range;
      _scrub = null;
      _historyError = null;
    });
    _loadHistory();
  }

  Future<void> _refresh() => _store.load();

  void _openAsset(Security security) {
    Navigator.of(context).push(
      MaterialPageRoute<void>(builder: (_) => AssetScreen(security: security)),
    );
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: portfolioAppBar(context, upload: true),
      body: RefreshIndicator(
        onRefresh: _refresh,
        child: ListenableBuilder(
          listenable: _store,
          builder: (context, _) => ListView(
            physics: const AlwaysScrollableScrollPhysics(),
            padding: const EdgeInsets.fromLTRB(16, 20, 16, 32),
            children: _content(context),
          ),
        ),
      ),
    );
  }

  List<Widget> _content(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    final current = _store.current;
    if (current == null || (!_store.hasStats(current.id) && _store.statsLoading)) {
      if (_store.error != null && current == null) {
        return [
          Padding(
            padding: const EdgeInsets.symmetric(vertical: 24),
            child: Text(
              _store.error!,
              textAlign: TextAlign.center,
              style: TextStyle(color: scheme.error),
            ),
          ),
        ];
      }
      return const [
        Padding(
          padding: EdgeInsets.only(top: 80),
          child: Center(child: CircularProgressIndicator()),
        ),
      ];
    }

    final stats = _store.statsFor(current.id);
    if (_store.hasStats(current.id) && !stats.hasData) {
      return const [EmptyPortfolio()];
    }

    // Нет сводки (не загрузилась) — нет и графика.
    final key = _historyKey();
    final points = key == null ? const <ValuePoint>[] : _history[key];
    final scrubbed = points != null && _scrub != null && _scrub! < points.length
        ? points[_scrub!]
        : null;

    return [
      // Сводка не загрузилась — показываем нули, ошибку не выдумываем.
      _Summary(stats: stats, scrubbed: scrubbed),
      const SizedBox(height: 20),
      _ValueChart(
        points: points,
        range: _range,
        loading: _historyLoadingKey != null,
        error: _historyError,
        onRetry: _loadHistory,
        onScrub: (i) => setState(() => _scrub = i),
      ),
      const SizedBox(height: 12),
      Pills<ValueRange>(
        values: ValueRange.values,
        selected: _range,
        label: (r) => switch (r) {
          ValueRange.week => '7д',
          ValueRange.month => '1м',
          ValueRange.year => '1г',
          ValueRange.all => 'Всё',
        },
        onSelected: _selectRange,
        compact: true,
      ),
      if (_store.error != null) ...[
        const SizedBox(height: 16),
        Text(_store.error!, textAlign: TextAlign.center, style: TextStyle(color: scheme.error)),
      ],
      if (stats.positions.isNotEmpty) ...[
        const SizedBox(height: 36),
        _MoversSection(
          positions: stats.positions,
          gainers: _gainers,
          max: _maxMovers,
          lookup: _directory.lookup,
          onToggle: (v) => setState(() => _gainers = v),
          onOpen: _openAsset,
        ),
        const SizedBox(height: 36),
        _StructureSection(stats: stats),
      ],
    ];
  }
}

// --- Сводка ---

class _Summary extends StatelessWidget {
  const _Summary({required this.stats, this.scrubbed});

  final PortfolioStats stats;

  /// Точка графика под пальцем: тогда вверху её дата и стоимость.
  final ValuePoint? scrubbed;

  @override
  Widget build(BuildContext context) {
    final textTheme = Theme.of(context).textTheme;
    final scheme = Theme.of(context).colorScheme;
    final point = scrubbed;
    final value = point?.value ?? stats.totalValue;
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Text(
          point == null ? 'Текущая стоимость' : _longDate(point.date),
          style: textTheme.titleMedium?.copyWith(color: scheme.onSurfaceVariant),
        ),
        const SizedBox(height: 4),
        FittedBox(
          fit: BoxFit.scaleDown,
          alignment: Alignment.centerLeft,
          child: Text(
            _unsigned(formatMoney(value), value),
            style: textTheme.displaySmall?.copyWith(fontWeight: FontWeight.w700),
          ),
        ),
        const SizedBox(height: 16),
        _MetricRow(
          label: 'Прибыль',
          child: _Change(money: stats.profit, percent: stats.profitPercent),
        ),
        _MetricRow(
          label: 'За день',
          child: _Change(money: stats.dayProfit, percent: stats.dayPercent),
        ),
        _MetricRow(
          label: 'Пассивный доход',
          child: Text(
            '${_unsigned(formatMoney(stats.passiveIncome), stats.passiveIncome)}   '
            '${_unsigned(formatPercent(stats.passivePercent), stats.passivePercent)}',
            style: textTheme.bodyLarge?.copyWith(fontWeight: FontWeight.w500),
          ),
        ),
      ],
    );
  }
}

class _MetricRow extends StatelessWidget {
  const _MetricRow({required this.label, required this.child});

  final String label;
  final Widget child;

  @override
  Widget build(BuildContext context) {
    final textTheme = Theme.of(context).textTheme;
    final scheme = Theme.of(context).colorScheme;
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 5),
      child: Row(
        children: [
          Text(label, style: textTheme.bodyLarge?.copyWith(color: scheme.onSurfaceVariant)),
          const SizedBox(width: 12),
          Expanded(
            child: Align(alignment: Alignment.centerRight, child: child),
          ),
        ],
      ),
    );
  }
}

/// `+30,44 ₽ ▲ 0,11%` / `−763,42 ₽ ▼ 2,56%`, цветом по знаку.
class _Change extends StatelessWidget {
  const _Change({required this.money, required this.percent, this.style});

  final double money;
  final double percent;
  final TextStyle? style;

  @override
  Widget build(BuildContext context) {
    final color = changeColor(context, money);
    final base = (style ?? Theme.of(context).textTheme.bodyLarge)!
        .copyWith(color: color, fontWeight: FontWeight.w600);
    final icon = _arrow(money);
    return Text.rich(
      TextSpan(
        style: base,
        children: [
          TextSpan(text: '${formatMoney(money)} '),
          if (icon != null)
            WidgetSpan(
              alignment: PlaceholderAlignment.middle,
              child: Icon(icon, size: (base.fontSize ?? 16) * 1.5, color: color),
            ),
          TextSpan(text: _unsigned(formatPercent(percent), percent)),
        ],
      ),
      maxLines: 1,
      overflow: TextOverflow.ellipsis,
    );
  }
}

/// Процент с треугольником: `▲ 1,24%`.
class _PercentChange extends StatelessWidget {
  const _PercentChange({required this.percent, this.style});

  final double percent;
  final TextStyle? style;

  @override
  Widget build(BuildContext context) {
    final color = changeColor(context, percent);
    final base = (style ?? Theme.of(context).textTheme.bodyMedium)!
        .copyWith(color: color, fontWeight: FontWeight.w600);
    final icon = _arrow(percent);
    return Text.rich(
      TextSpan(
        style: base,
        children: [
          if (icon != null)
            WidgetSpan(
              alignment: PlaceholderAlignment.middle,
              child: Icon(icon, size: (base.fontSize ?? 14) * 1.5, color: color),
            ),
          TextSpan(text: _unsigned(formatPercent(percent), percent)),
        ],
      ),
      maxLines: 1,
    );
  }
}

IconData? _arrow(double value) {
  final rounded = double.parse(value.toStringAsFixed(2));
  if (rounded > 0) return Icons.arrow_drop_up_rounded;
  if (rounded < 0) return Icons.arrow_drop_down_rounded;
  return null;
}

/// Убирает знак, который добавляют [formatMoney]/[formatPercent], там, где
/// он не нужен (стоимость, пассивный доход, процент рядом со стрелкой).
String _unsigned(String text, double value) =>
    value < 0 && text.startsWith('−') || value > 0 && text.startsWith('+')
        ? text.substring(1)
        : text;

// --- График стоимости ---

class _ValueChart extends StatelessWidget {
  const _ValueChart({
    required this.points,
    required this.range,
    required this.loading,
    required this.error,
    required this.onRetry,
    required this.onScrub,
  });

  final List<ValuePoint>? points;
  final ValueRange range;
  final bool loading;
  final String? error;
  final VoidCallback onRetry;
  final ValueChanged<int?> onScrub;

  static const _height = 220.0;

  @override
  Widget build(BuildContext context) {
    final textTheme = Theme.of(context).textTheme;
    final scheme = Theme.of(context).colorScheme;
    final list = points;

    Widget message(Widget child) => SizedBox(height: _height, child: Center(child: child));

    if (list == null) {
      if (error != null && !loading) {
        return message(Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Text(error!, textAlign: TextAlign.center, style: TextStyle(color: scheme.error)),
            const SizedBox(height: 8),
            OutlinedButton(onPressed: onRetry, child: const Text('Повторить')),
          ],
        ));
      }
      return message(const CircularProgressIndicator());
    }
    if (list.isEmpty) {
      return message(Text(
        'Нет данных за этот период',
        style: textTheme.bodyMedium?.copyWith(color: scheme.onSurfaceVariant),
      ));
    }

    final span = list.last.date.difference(list.first.date).inDays;
    final byYear = range == ValueRange.all && span > 400;
    final byMonth = !byYear && (range == ValueRange.year || range == ValueRange.all) && span > 45;
    return Stack(
      children: [
        PriceChart(
          points: [for (final p in list) ChartPoint(p.date, p.value)],
          height: _height,
          xTickKey: (t) => byYear ? _msk(t).year : (byMonth ? _msk(t).month : _msk(t).day),
          xLabel: (t) => byYear
              ? '${_msk(t).year}'
              : (byMonth ? _monthsShort[_msk(t).month - 1] : '${_msk(t).day}'),
          skipFirstTick: byYear || byMonth,
          yLabel: _axisMoney,
          onScrub: onScrub,
        ),
        if (loading)
          const Positioned(left: 0, right: 0, top: 0, child: LinearProgressIndicator(minHeight: 2)),
      ],
    );
  }
}

/// Подпись оси Y: `850`, `29,5 тыс`, `1,2 млн`.
String _axisMoney(double v) {
  final abs = v.abs();
  String short(double x, String unit) {
    var text = x.toStringAsFixed(x.abs() >= 100 ? 0 : 1);
    text = text.replaceFirst(RegExp(r'\.0$'), '');
    return '${text.replaceAll('.', ',')} $unit';
  }

  if (abs >= 1e6) return short(v / 1e6, 'млн');
  if (abs >= 1e3) return short(v / 1e3, 'тыс');
  return v.toStringAsFixed(0);
}

// --- Топ изменений за день ---

class _MoversSection extends StatelessWidget {
  const _MoversSection({
    required this.positions,
    required this.gainers,
    required this.max,
    required this.lookup,
    required this.onToggle,
    required this.onOpen,
  });

  final List<Position> positions;
  final bool gainers;
  final int max;
  final Security Function(String secid, String board) lookup;
  final ValueChanged<bool> onToggle;
  final ValueChanged<Security> onOpen;

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    final textTheme = Theme.of(context).textTheme;
    final movers = [
      for (final p in positions)
        if (p.dayChangePercent case final pct?)
          if (gainers ? pct >= 0.005 : pct <= -0.005) (position: p, percent: pct),
    ]..sort((a, b) => gainers ? b.percent.compareTo(a.percent) : a.percent.compareTo(b.percent));

    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        const _SectionTitle(title: 'Топ изменений за день'),
        const SizedBox(height: 12),
        Pills<bool>(
          values: const [true, false],
          selected: gainers,
          label: (v) => v ? 'Растут' : 'Падают',
          onSelected: onToggle,
          compact: true,
        ),
        const SizedBox(height: 12),
        if (movers.isEmpty)
          Padding(
            padding: const EdgeInsets.symmetric(vertical: 16),
            child: Text(
              gainers
                  ? 'Сегодня в портфеле нет бумаг, которые выросли'
                  : 'Сегодня в портфеле нет бумаг, которые подешевели',
              textAlign: TextAlign.center,
              style: textTheme.bodyMedium?.copyWith(color: scheme.onSurfaceVariant),
            ),
          )
        else
          for (final m in movers.take(max)) ...[
            _MoverTile(
              position: m.position,
              percent: m.percent,
              security: lookup(m.position.secid, m.position.board),
              onTap: onOpen,
            ),
            const SizedBox(height: 8),
          ],
      ],
    );
  }
}

class _MoverTile extends StatelessWidget {
  const _MoverTile({
    required this.position,
    required this.percent,
    required this.security,
    required this.onTap,
  });

  final Position position;
  final double percent;
  final Security security;
  final ValueChanged<Security> onTap;

  @override
  Widget build(BuildContext context) {
    final textTheme = Theme.of(context).textTheme;
    final scheme = Theme.of(context).colorScheme;
    final change = position.dayChange ?? 0;
    return _Tile(
      onTap: () => onTap(security),
      child: Row(
        children: [
          TickerBadge(secid: position.secid, size: 40),
          const SizedBox(width: 14),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  security.title,
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                  style: textTheme.titleSmall?.copyWith(fontWeight: FontWeight.w600),
                ),
                Text(
                  '${position.secid} • ${_quantity(position.quantity)} шт.',
                  style: textTheme.bodySmall?.copyWith(color: scheme.onSurfaceVariant),
                ),
              ],
            ),
          ),
          const SizedBox(width: 12),
          Column(
            crossAxisAlignment: CrossAxisAlignment.end,
            children: [
              _PercentChange(percent: percent, style: textTheme.titleSmall),
              Text(
                formatMoney(change),
                style: textTheme.bodySmall?.copyWith(color: changeColor(context, change)),
              ),
            ],
          ),
        ],
      ),
    );
  }
}

String _quantity(double q) {
  final text = q == q.roundToDouble() ? q.toStringAsFixed(0) : q.toString();
  return text.replaceAll('.', ',');
}

// --- Состав портфеля ---

enum _Part {
  shares('Акции', Color(0xFF5B2A86)),
  funds('Фонды', Color(0xFF0E8C8C)),
  bonds('Облигации', Color(0xFF2F6FDB)),
  other('Другое', Color(0xFFD9822B)),
  cash('Рубли', Color(0xFF8E8E93));

  const _Part(this.title, this.color);

  final String title;
  final Color color;

  static _Part of(String board) => switch (board) {
        'TQBR' => shares,
        'TQTF' => funds,
        'TQOB' || 'TQCB' => bonds,
        _ => other,
      };
}

class _StructureSection extends StatelessWidget {
  const _StructureSection({required this.stats});

  final PortfolioStats stats;

  @override
  Widget build(BuildContext context) {
    final textTheme = Theme.of(context).textTheme;
    final scheme = Theme.of(context).colorScheme;
    final sums = <_Part, double>{};
    for (final p in stats.positions) {
      if (p.value > 0) sums[_Part.of(p.board)] = (sums[_Part.of(p.board)] ?? 0) + p.value;
    }
    if (stats.cashBalance > 0.005) sums[_Part.cash] = stats.cashBalance;
    final total = sums.values.fold(0.0, (a, b) => a + b);
    if (total <= 0) return const SizedBox.shrink();
    final parts = [for (final part in _Part.values) if (sums[part] != null) part];

    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        const _SectionTitle(title: 'Состав портфеля'),
        const SizedBox(height: 14),
        ClipRRect(
          borderRadius: BorderRadius.circular(6),
          child: SizedBox(
            height: 12,
            child: Row(
              children: [
                for (final part in parts)
                  Expanded(
                    // Минимум 1‰, чтобы крошечная доля была видна полоской.
                    flex: (sums[part]! / total * 1000).round().clamp(1, 1000),
                    child: Container(
                      margin: EdgeInsets.only(right: part == parts.last ? 0 : 2),
                      color: part.color,
                    ),
                  ),
              ],
            ),
          ),
        ),
        const SizedBox(height: 12),
        for (final part in parts)
          Padding(
            padding: const EdgeInsets.symmetric(vertical: 6),
            child: Row(
              children: [
                Container(
                  width: 10,
                  height: 10,
                  decoration: BoxDecoration(color: part.color, shape: BoxShape.circle),
                ),
                const SizedBox(width: 10),
                Expanded(
                  child: Text(
                    part.title,
                    maxLines: 1,
                    overflow: TextOverflow.ellipsis,
                    style: textTheme.bodyLarge,
                  ),
                ),
                const SizedBox(width: 8),
                Text(
                  _unsigned(formatMoney(sums[part]!), sums[part]!),
                  maxLines: 1,
                  softWrap: false,
                  style: textTheme.bodyMedium?.copyWith(color: scheme.onSurfaceVariant),
                ),
                const SizedBox(width: 12),
                // Колонка процентов выровнена по правому краю и не переносится:
                // «100%» — самое широкое значение, короче — просто уже.
                ConstrainedBox(
                  constraints: const BoxConstraints(minWidth: 64),
                  child: Text(
                    _unsigned(formatPercent(sums[part]! / total * 100), 1),
                    maxLines: 1,
                    softWrap: false,
                    textAlign: TextAlign.right,
                    style: textTheme.bodyLarge?.copyWith(fontWeight: FontWeight.w600),
                  ),
                ),
              ],
            ),
          ),
      ],
    );
  }
}

// --- Общее ---

class _SectionTitle extends StatelessWidget {
  const _SectionTitle({required this.title, this.action});

  final String title;
  final Widget? action;

  @override
  Widget build(BuildContext context) {
    return Row(
      children: [
        Expanded(
          child: Text(
            title,
            style: Theme.of(context).textTheme.headlineSmall?.copyWith(fontWeight: FontWeight.w700),
          ),
        ),
        if (action != null) action!,
      ],
    );
  }
}

/// Карточка строки списка: скруглённый прямоугольник на приглушённом фоне.
class _Tile extends StatelessWidget {
  const _Tile({required this.child, this.onTap});

  final Widget child;
  final VoidCallback? onTap;

  @override
  Widget build(BuildContext context) {
    return Material(
      color: Theme.of(context).colorScheme.surfaceContainerLow,
      borderRadius: BorderRadius.circular(16),
      clipBehavior: Clip.antiAlias,
      child: InkWell(
        onTap: onTap,
        child: Padding(
          padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 12),
          child: child,
        ),
      ),
    );
  }
}

// --- Даты (по Москве, как и дни графика на бэкенде) ---

/// Настенное время МСК (UTC+3, без перехода на летнее) как UTC-DateTime.
DateTime _msk(DateTime t) => t.toUtc().add(const Duration(hours: 3));

const _monthsShort = [
  'янв', 'фев', 'мар', 'апр', 'май', 'июн',
  'июл', 'авг', 'сен', 'окт', 'ноя', 'дек',
];

const _monthsGenitive = [
  'января', 'февраля', 'марта', 'апреля', 'мая', 'июня',
  'июля', 'августа', 'сентября', 'октября', 'ноября', 'декабря',
];

/// `18 сентября 2026`
String _longDate(DateTime t) {
  final m = _msk(t);
  return '${m.day} ${_monthsGenitive[m.month - 1]} ${m.year}';
}
