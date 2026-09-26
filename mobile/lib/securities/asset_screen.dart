import 'package:flutter/material.dart';

import '../api/api_client.dart';
import '../api/securities_api.dart';
import '../favorites/favorites_store.dart';
import '../portfolio/stats_format.dart';
import 'price_chart.dart';
import 'security_widgets.dart';

/// Карточка актива: значок и название, текущая цена с изменением за день,
/// вкладки «Обзор / Дивиденды / В портфеле», на «Обзоре» — график цены
/// с изменением за выбранный период. Звезда в шапке — избранное.
class AssetScreen extends StatefulWidget {
  const AssetScreen({super.key, required this.security});

  /// Можно передать бумагу без цен (например, из избранного) — цена
  /// всё равно запрашивается при открытии.
  final Security security;

  @override
  State<AssetScreen> createState() => _AssetScreenState();
}

enum _Tab { overview, dividends, position }

/// Период на экране. 3м, 6м и 1г строятся из одного ответа `range=year`
/// (дневные свечи), 3м и 6м — отфильтрованные на клиенте.
enum _Period {
  day('1д', CandleRange.day),
  week('7д', CandleRange.week),
  month('1м', CandleRange.month),
  months3('3м', CandleRange.year),
  months6('6м', CandleRange.year),
  year('1г', CandleRange.year),
  years5('5л', CandleRange.fiveYears);

  const _Period(this.label, this.range);

  final String label;
  final CandleRange range;
}

class _AssetScreenState extends State<AssetScreen> {
  final _api = SecuritiesApi();
  final _favorites = FavoritesStore.instance;

  late Security _security = widget.security;
  String? _priceError;

  _Tab _tab = _Tab.overview;
  _Period _period = _Period.day;

  final Map<CandleRange, List<Candle>> _candles = {};
  bool _chartLoading = false;
  String? _chartError;
  int _chartSeq = 0;

  int? _scrub;

  List<SecurityInfoField>? _info;
  bool _infoLoading = false;
  String? _infoError;

  List<Dividend>? _dividends;
  bool _dividendsLoading = false;
  String? _dividendsError;

  @override
  void initState() {
    super.initState();
    _loadPrice();
    _loadChart();
    _loadInfo();
  }

  Future<void> _refresh() async {
    // Старый график остаётся на экране, пока грузится новый.
    _candles.removeWhere((range, _) => range != _period.range);
    await Future.wait([
      _loadPrice(),
      _loadChart(force: true),
      if (_info == null || _info!.isEmpty) _loadInfo(),
      if (_tab == _Tab.dividends) _loadDividends(),
    ]);
  }

  Future<void> _loadDividends() async {
    setState(() {
      _dividendsLoading = true;
      _dividendsError = null;
    });
    try {
      final list = await _api.dividends(_security.secid);
      if (!mounted) return;
      setState(() {
        _dividends = list;
        _dividendsLoading = false;
      });
    } on ApiException catch (e) {
      if (!mounted) return;
      setState(() {
        _dividendsError = e.message;
        _dividendsLoading = false;
      });
    } catch (_) {
      if (!mounted) return;
      setState(() {
        _dividendsError = 'Не удалось загрузить дивиденды';
        _dividendsLoading = false;
      });
    }
  }

  void _selectTab(_Tab t) {
    setState(() => _tab = t);
    if (t == _Tab.dividends && _dividends == null && !_dividendsLoading) _loadDividends();
  }

  Future<void> _loadInfo() async {
    setState(() {
      _infoLoading = true;
      _infoError = null;
    });
    try {
      final info = await _api.info(_security.secid, _security.board);
      if (!mounted) return;
      setState(() {
        _info = info;
        _infoLoading = false;
      });
    } on ApiException catch (e) {
      if (!mounted) return;
      setState(() {
        _infoError = e.message;
        _infoLoading = false;
      });
    } catch (_) {
      if (!mounted) return;
      setState(() {
        _infoError = 'Не удалось загрузить информацию';
        _infoLoading = false;
      });
    }
  }

  Future<void> _loadPrice() async {
    try {
      final fresh = await _api.price(_security.secid, _security.board);
      if (!mounted) return;
      setState(() {
        _priceError = fresh == null ? 'Нет данных о цене' : null;
        if (fresh != null) _security = _security.withPricesFrom(fresh);
      });
    } on ApiException catch (e) {
      if (mounted) setState(() => _priceError = e.message);
    } catch (_) {
      if (mounted) setState(() => _priceError = 'Не удалось обновить цену');
    }
  }

  Future<void> _loadChart({bool force = false}) async {
    final range = _period.range;
    if (!force && _candles.containsKey(range)) {
      setState(() => _chartError = null);
      return;
    }
    final seq = ++_chartSeq;
    setState(() {
      _chartLoading = true;
      _chartError = null;
    });
    try {
      final candles = await _api.candles(_security.secid, _security.board, range);
      if (!mounted || seq != _chartSeq) return;
      setState(() {
        _candles[range] = candles;
        _chartLoading = false;
      });
    } on ApiException catch (e) {
      if (!mounted || seq != _chartSeq) return;
      setState(() {
        _chartError = e.message;
        _chartLoading = false;
      });
    } catch (_) {
      if (!mounted || seq != _chartSeq) return;
      setState(() {
        _chartError = 'Не удалось загрузить график';
        _chartLoading = false;
      });
    }
  }

  void _selectPeriod(_Period p) {
    if (p == _period) return;
    setState(() {
      _period = p;
      _scrub = null;
    });
    _loadChart();
  }

  Future<void> _toggleFavorite() async {
    final added = await _favorites.toggle(_security);
    if (!mounted) return;
    ScaffoldMessenger.of(context)
      ..hideCurrentSnackBar()
      ..showSnackBar(SnackBar(
        content: Text(added ? 'Добавлено в избранное' : 'Удалено из избранного'),
        duration: const Duration(seconds: 2),
      ));
  }

  /// Свечи выбранного периода (для 3м/6м/YTD — отфильтрованный год).
  List<Candle>? _periodCandles() {
    final all = _candles[_period.range];
    if (all == null) return null;
    final now = _msk(DateTime.now());
    final DateTime? from = switch (_period) {
      _Period.months3 => DateTime.utc(now.year, now.month - 3, now.day),
      _Period.months6 => DateTime.utc(now.year, now.month - 6, now.day),
      _ => null,
    };
    if (from == null) return all;
    return all.where((c) => !_msk(c.start).isBefore(from)).toList();
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(
        title: const Text('Актив'),
        actions: [
          ListenableBuilder(
            listenable: _favorites,
            builder: (context, _) {
              final fav = _favorites.contains(_security);
              return IconButton(
                tooltip: fav ? 'Убрать из избранного' : 'Добавить в избранное',
                onPressed: _toggleFavorite,
                icon: Icon(
                  fav ? Icons.star_rounded : Icons.star_outline_rounded,
                  color: Theme.of(context).colorScheme.primary,
                ),
              );
            },
          ),
          const SizedBox(width: 4),
        ],
      ),
      body: RefreshIndicator(
        onRefresh: _refresh,
        child: ListView(
          physics: const AlwaysScrollableScrollPhysics(),
          padding: const EdgeInsets.fromLTRB(16, 20, 16, 32),
          children: [
            _Header(security: _security),
            const SizedBox(height: 20),
            _PriceRow(security: _security, error: _priceError),
            const SizedBox(height: 20),
            Pills<_Tab>(
              values: _Tab.values,
              selected: _tab,
              label: (t) => switch (t) {
                _Tab.overview => 'Обзор',
                _Tab.dividends => 'Дивиденды',
                _Tab.position => 'В портфеле',
              },
              onSelected: _selectTab,
            ),
            const SizedBox(height: 24),
            if (_tab == _Tab.overview)
              ..._overview(context)
            else if (_tab == _Tab.dividends)
              _DividendsSection(
                dividends: _dividends,
                loading: _dividendsLoading,
                error: _dividendsError,
                onRetry: _loadDividends,
                security: _security,
              )
            else
              const _Soon(
                icon: Icons.business_center_outlined,
                text: 'Позиция по бумаге в вашем портфеле появится позже',
              ),
          ],
        ),
      ),
    );
  }

  List<Widget> _overview(BuildContext context) {
    final textTheme = Theme.of(context).textTheme;
    final scheme = Theme.of(context).colorScheme;
    final candles = _periodCandles();
    final s = _security;
    final bond = s.isBond;

    Widget chart;
    Widget? summary;
    if (candles == null || (_chartLoading && candles.isEmpty)) {
      chart = _chartError != null
          ? _ChartMessage(
              text: _chartError!,
              action: OutlinedButton(onPressed: _loadChart, child: const Text('Повторить')),
            )
          : const _ChartMessage(child: CircularProgressIndicator());
    } else if (candles.isEmpty) {
      chart = const _ChartMessage(text: 'Нет данных за этот период');
    } else {
      final points = [for (final c in candles) ChartPoint(c.start, c.close)];
      final intraday = _period == _Period.day;
      // Уровень начала периода: для дня — вчерашнее закрытие, иначе —
      // открытие первой свечи.
      final reference = intraday ? (s.prevClose ?? candles.first.open) : candles.first.open;
      final scrub = _scrub != null && _scrub! < points.length ? _scrub : null;
      final endValue =
          scrub != null ? points[scrub].value : (s.lastPrice ?? points.last.value);
      final change = endValue - reference;
      final percent = reference == 0 ? 0.0 : change / reference * 100;

      final String dates;
      if (scrub != null) {
        dates = _scrubLabel(points[scrub].time);
      } else if (intraday) {
        dates = _fullDate(candles.first.start);
      } else {
        dates = '${_fullDate(candles.first.start)} - ${_fullDate(candles.last.start)}';
      }

      summary = Row(
        children: [
          Expanded(
            child: Text(
              dates,
              maxLines: 1,
              overflow: TextOverflow.ellipsis,
              style: textTheme.titleSmall?.copyWith(fontWeight: FontWeight.w600),
            ),
          ),
          const SizedBox(width: 8),
          PriceChange(change: change, percent: percent, security: s, style: textTheme.titleSmall),
        ],
      );

      final axis = _axisFor(_period);

      chart = Stack(
        children: [
          PriceChart(
            points: points,
            reference: reference,
            xTickKey: axis.key,
            xLabel: axis.label,
            skipFirstTick: axis.skipFirst,
            yLabel: (v) => _axisPrice(v, bond: bond),
            onScrub: (i) => setState(() => _scrub = i),
          ),
          if (_chartLoading)
            const Positioned(left: 0, right: 0, top: 0, child: LinearProgressIndicator(minHeight: 2)),
        ],
      );
    }

    return [
      SizedBox(height: 24, child: summary),
      const SizedBox(height: 12),
      chart,
      const SizedBox(height: 16),
      Pills<_Period>(
        values: _Period.values,
        selected: _period,
        label: (p) => p.label,
        onSelected: _selectPeriod,
        compact: true,
      ),
      if (_chartError != null && candles != null && candles.isNotEmpty) ...[
        const SizedBox(height: 8),
        Text(_chartError!, style: textTheme.bodySmall?.copyWith(color: scheme.error)),
      ],
      const SizedBox(height: 28),
      _AboutSection(
        fields: _info,
        loading: _infoLoading,
        error: _infoError,
        onRetry: _loadInfo,
      ),
    ];
  }
}

class _Header extends StatelessWidget {
  const _Header({required this.security});

  final Security security;

  @override
  Widget build(BuildContext context) {
    final textTheme = Theme.of(context).textTheme;
    final scheme = Theme.of(context).colorScheme;
    final s = security;
    final name = s.secName ?? s.shortName;
    return Row(
      children: [
        TickerBadge(secid: s.secid, size: 60, square: true),
        const SizedBox(width: 16),
        Expanded(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(
                '${s.secid} · MOEX',
                style: textTheme.titleLarge?.copyWith(fontWeight: FontWeight.w600),
              ),
              if (name != null) ...[
                const SizedBox(height: 2),
                Text(
                  name,
                  maxLines: 2,
                  overflow: TextOverflow.ellipsis,
                  style: textTheme.bodyLarge?.copyWith(color: scheme.onSurfaceVariant),
                ),
              ],
            ],
          ),
        ),
      ],
    );
  }
}

/// Текущая цена крупно и рядом — изменение за день.
class _PriceRow extends StatelessWidget {
  const _PriceRow({required this.security, this.error});

  final Security security;
  final String? error;

  @override
  Widget build(BuildContext context) {
    final textTheme = Theme.of(context).textTheme;
    final scheme = Theme.of(context).colorScheme;
    final s = security;
    final price = s.lastPrice;
    final change = s.dayChange, percent = s.dayChangePercent;
    if (price == null) {
      return error == null
          ? const SizedBox(height: 36, child: Align(alignment: Alignment.centerLeft, child: _SmallSpinner()))
          : Text(error!, style: textTheme.bodyMedium?.copyWith(color: scheme.error));
    }
    return Wrap(
      crossAxisAlignment: WrapCrossAlignment.center,
      spacing: 12,
      runSpacing: 4,
      children: [
        Text(
          formatPrice(price, bond: s.isBond, currency: s.currency),
          style: textTheme.headlineMedium?.copyWith(fontWeight: FontWeight.w700),
        ),
        if (change != null && percent != null)
          PriceChange(change: change, percent: percent, security: s, style: textTheme.titleMedium),
      ],
    );
  }
}

class _SmallSpinner extends StatelessWidget {
  const _SmallSpinner();

  @override
  Widget build(BuildContext context) =>
      const SizedBox(width: 22, height: 22, child: CircularProgressIndicator(strokeWidth: 2.5));
}

class _ChartMessage extends StatelessWidget {
  const _ChartMessage({this.text, this.action, this.child});

  final String? text;
  final Widget? action;
  final Widget? child;

  @override
  Widget build(BuildContext context) {
    return SizedBox(
      height: 260,
      child: Center(
        child: child ??
            Column(
              mainAxisSize: MainAxisSize.min,
              children: [
                Text(
                  text ?? '',
                  textAlign: TextAlign.center,
                  style: TextStyle(color: Theme.of(context).colorScheme.onSurfaceVariant),
                ),
                if (action != null) ...[const SizedBox(height: 12), action!],
              ],
            ),
      ),
    );
  }
}

class _Soon extends StatelessWidget {
  const _Soon({required this.icon, required this.text, this.title = 'Скоро'});

  final IconData icon;
  final String text;
  final String title;

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    final textTheme = Theme.of(context).textTheme;
    return Padding(
      padding: const EdgeInsets.fromLTRB(24, 40, 24, 0),
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
        ],
      ),
    );
  }
}

/// Вкладка «Дивиденды»: все выплаты, новые сверху. Уже прошедшие (реестр
/// закрыт) — приглушённым цветом, будущие — обычным; ближайшая будущая
/// выделена плашкой «Ближайшая», сроком и доходностью к текущей цене.
class _DividendsSection extends StatelessWidget {
  const _DividendsSection({
    required this.dividends,
    required this.loading,
    required this.error,
    required this.onRetry,
    required this.security,
  });

  final List<Dividend>? dividends;
  final bool loading;
  final String? error;
  final VoidCallback onRetry;
  final Security security;

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    final textTheme = Theme.of(context).textTheme;
    final list = dividends;

    if (list == null || (list.isEmpty && loading)) {
      if (error != null) {
        return _ChartMessage(
          text: error!,
          action: OutlinedButton(onPressed: onRetry, child: const Text('Повторить')),
        );
      }
      return const _ChartMessage(child: CircularProgressIndicator());
    }
    if (list.isEmpty) {
      return _Soon(
        icon: Icons.payments_outlined,
        title: 'Нет выплат',
        text: security.isBond
            ? 'По облигациям выплачиваются купоны — их покажем позже'
            : 'По этой бумаге дивиденды не выплачивались',
      );
    }

    final now = _msk(DateTime.now());
    final today = DateTime.utc(now.year, now.month, now.day);
    Dividend? nearest;
    for (final d in list) {
      if (!d.date.isBefore(today) && (nearest == null || d.date.isBefore(nearest.date))) {
        nearest = d;
      }
    }

    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Text('Все выплаты', style: textTheme.titleLarge?.copyWith(fontWeight: FontWeight.w700)),
        const SizedBox(height: 4),
        Text(
          'Даты — закрытие реестра, суммы — на одну акцию. Источник: dohod.ru',
          style: textTheme.bodySmall?.copyWith(color: scheme.onSurfaceVariant),
        ),
        if (error != null) ...[
          const SizedBox(height: 8),
          Text(error!, style: textTheme.bodySmall?.copyWith(color: scheme.error)),
        ],
        const SizedBox(height: 8),
        for (var i = 0; i < list.length; i++) ...[
          if (i > 0 && !identical(list[i], nearest) && !identical(list[i - 1], nearest))
            Divider(height: 1, color: scheme.outlineVariant.withValues(alpha: 0.6)),
          _DividendRow(
            dividend: list[i],
            paid: list[i].date.isBefore(today),
            nearest: identical(list[i], nearest),
            today: today,
            price: security.lastPrice,
          ),
        ],
      ],
    );
  }
}

class _DividendRow extends StatelessWidget {
  const _DividendRow({
    required this.dividend,
    required this.paid,
    required this.nearest,
    required this.today,
    required this.price,
  });

  final Dividend dividend;
  final bool paid;
  final bool nearest;
  final DateTime today;
  final double? price;

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    final textTheme = Theme.of(context).textTheme;
    final color = paid ? scheme.onSurfaceVariant.withValues(alpha: 0.75) : scheme.onSurface;
    final weight = paid ? FontWeight.w400 : FontWeight.w600;
    final amount = formatPrice(dividend.value, bond: false, currency: dividend.currency);

    final row = Row(
      children: [
        Icon(Icons.calendar_today_outlined, size: 20, color: nearest ? scheme.primary : color),
        const SizedBox(width: 12),
        Expanded(
          child: Text.rich(
            TextSpan(
              children: [
                TextSpan(
                  text: dividend.forecast ? '~ ${_longDate(dividend.date)}' : _longDate(dividend.date),
                  style: textTheme.bodyLarge?.copyWith(color: color, fontWeight: weight),
                ),
                if (dividend.forecast)
                  TextSpan(
                    text: '  прогноз',
                    style: textTheme.bodySmall?.copyWith(color: scheme.onSurfaceVariant),
                  ),
              ],
            ),
          ),
        ),
        const SizedBox(width: 12),
        Text(amount, style: textTheme.bodyLarge?.copyWith(color: color, fontWeight: weight)),
      ],
    );

    if (!nearest) {
      return Padding(padding: const EdgeInsets.symmetric(vertical: 16), child: row);
    }

    final days = dividend.date.difference(today).inDays;
    final when = switch (days) {
      0 => 'сегодня',
      1 => 'завтра',
      _ => 'через $days ${_daysWord(days)}',
    };
    final p = price;
    final yieldText = p != null && p > 0 && _sameCurrency(dividend.currency)
        ? ' · доходность ${formatPercent(dividend.value / p * 100).replaceFirst('+', '')}'
        : '';

    return Container(
      margin: const EdgeInsets.symmetric(vertical: 6),
      padding: const EdgeInsets.fromLTRB(12, 12, 12, 12),
      decoration: BoxDecoration(
        color: scheme.primaryContainer.withValues(alpha: 0.45),
        borderRadius: BorderRadius.circular(16),
        border: Border.all(color: scheme.primary.withValues(alpha: 0.5)),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          row,
          const SizedBox(height: 8),
          Row(
            children: [
              const SizedBox(width: 32),
              Container(
                padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 3),
                decoration: BoxDecoration(
                  color: scheme.primary,
                  borderRadius: BorderRadius.circular(8),
                ),
                child: Text(
                  'Ближайшая',
                  style: textTheme.labelSmall?.copyWith(
                    color: scheme.onPrimary,
                    fontWeight: FontWeight.w700,
                  ),
                ),
              ),
              const SizedBox(width: 8),
              Expanded(
                child: Text(
                  '$when$yieldText',
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                  style: textTheme.bodySmall?.copyWith(color: scheme.onSurfaceVariant),
                ),
              ),
            ],
          ),
        ],
      ),
    );
  }

  static bool _sameCurrency(String c) => c == 'RUB' || c == 'SUR';

  static String _daysWord(int n) {
    final n100 = n % 100, n10 = n % 10;
    if (n100 >= 11 && n100 <= 14) return 'дней';
    if (n10 == 1) return 'день';
    if (n10 >= 2 && n10 <= 4) return 'дня';
    return 'дней';
  }
}

const _monthsFull = [
  'января', 'февраля', 'марта', 'апреля', 'мая', 'июня',
  'июля', 'августа', 'сентября', 'октября', 'ноября', 'декабря',
];

/// `20 июля 2026 г.`
String _longDate(DateTime d) => '${d.day} ${_monthsFull[d.month - 1]} ${d.year} г.';

/// «О компании»: справочные данные MOEX о бумаге и эмитенте. Сверху —
/// эмитент крупно и тип бумаги, ниже — строки «подпись … значение справа».
class _AboutSection extends StatelessWidget {
  const _AboutSection({
    required this.fields,
    required this.loading,
    required this.error,
    required this.onRetry,
  });

  final List<SecurityInfoField>? fields;
  final bool loading;
  final String? error;
  final VoidCallback onRetry;

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    final textTheme = Theme.of(context).textTheme;
    final list = fields;

    final Widget body;
    if (list != null && list.isNotEmpty) {
      SecurityInfoField? pick(String name) {
        for (final f in list) {
          if (f.name == name) return f;
        }
        return null;
      }

      final issuer = pick('ISSUER');
      final name = pick('NAME');
      final heading = issuer ?? name;
      final type = pick('TYPE');
      final rows = list
          .where((f) => f != heading && f != type && !(issuer == null && f == name))
          .toList();
      final divider = Divider(height: 1, color: scheme.outlineVariant.withValues(alpha: 0.6));

      body = Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          if (heading != null || type != null)
            Padding(
              padding: const EdgeInsets.fromLTRB(0, 14, 0, 12),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  if (heading != null)
                    Text(
                      heading.value,
                      maxLines: 3,
                      overflow: TextOverflow.ellipsis,
                      style: textTheme.titleMedium?.copyWith(fontWeight: FontWeight.w700),
                    ),
                  if (type != null) ...[
                    const SizedBox(height: 2),
                    Text(
                      type.value,
                      style: textTheme.bodyMedium?.copyWith(color: scheme.onSurfaceVariant),
                    ),
                  ],
                ],
              ),
            ),
          for (var i = 0; i < rows.length; i++) ...[
            if (i > 0 || heading != null || type != null) divider,
            _InfoRow(field: rows[i]),
          ],
        ],
      );
    } else if (loading) {
      body = const Padding(
        padding: EdgeInsets.symmetric(vertical: 24),
        child: Center(child: CircularProgressIndicator()),
      );
    } else if (error != null) {
      body = Padding(
        padding: const EdgeInsets.symmetric(vertical: 16),
        child: Column(
          children: [
            Text(error!, style: textTheme.bodyMedium?.copyWith(color: scheme.onSurfaceVariant)),
            const SizedBox(height: 8),
            OutlinedButton(onPressed: onRetry, child: const Text('Повторить')),
          ],
        ),
      );
    } else {
      return const SizedBox.shrink();
    }

    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Text('О компании', style: textTheme.titleLarge?.copyWith(fontWeight: FontWeight.w700)),
        const SizedBox(height: 12),
        Container(
          padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 2),
          decoration: BoxDecoration(
            color: scheme.surfaceContainerLow,
            borderRadius: BorderRadius.circular(20),
            border: Border.all(color: scheme.outlineVariant),
          ),
          child: body,
        ),
      ],
    );
  }
}

/// Строка: подпись слева, значение прижато вправо. Длинное значение
/// переносится максимум на 2 строки (тоже по правому краю), остальное —
/// многоточие; полный текст — по долгому нажатию.
class _InfoRow extends StatelessWidget {
  const _InfoRow({required this.field});

  final SecurityInfoField field;

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    final textTheme = Theme.of(context).textTheme;
    final value = formatInfoValue(field);
    return Tooltip(
      message: value,
      triggerMode: TooltipTriggerMode.longPress,
      child: Padding(
        padding: const EdgeInsets.symmetric(vertical: 13),
        child: Row(
          mainAxisAlignment: MainAxisAlignment.spaceBetween,
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Flexible(
              child: Text(
                field.title,
                style: textTheme.bodyMedium?.copyWith(color: scheme.onSurfaceVariant),
              ),
            ),
            const SizedBox(width: 16),
            Flexible(
              child: Text(
                value,
                textAlign: TextAlign.right,
                maxLines: 2,
                overflow: TextOverflow.ellipsis,
                style: textTheme.bodyMedium?.copyWith(fontWeight: FontWeight.w600),
              ),
            ),
          ],
        ),
      ),
    );
  }
}

/// Значение строки «О компании» для показа: `21 586 948 000 шт.`, `3 ₽`,
/// `11.07.2007`, `Да`.
String formatInfoValue(SecurityInfoField f) {
  String number(String raw) {
    final v = double.tryParse(raw);
    if (v == null) return raw;
    var text = v == v.roundToDouble() ? v.toStringAsFixed(0) : v.toStringAsFixed(4);
    if (text.contains('.')) text = text.replaceFirst(RegExp(r'\.?0+$'), '');
    final parts = text.split('.');
    final negative = parts[0].startsWith('-');
    final digits = negative ? parts[0].substring(1) : parts[0];
    final grouped = StringBuffer(negative ? '−' : '');
    for (var i = 0; i < digits.length; i++) {
      if (i > 0 && (digits.length - i) % 3 == 0) grouped.write('\u00A0');
      grouped.write(digits[i]);
    }
    return parts.length > 1 ? '$grouped,${parts[1]}' : grouped.toString();
  }

  switch (f.type) {
    case 'number':
      final n = number(f.value);
      return f.unit == null ? n : '$n\u00A0${f.unit}';
    case 'money':
      return '${number(f.value)}\u00A0${currencySymbol(f.unit)}';
    case 'percent':
      return '${number(f.value)}%';
    case 'date':
      final d = DateTime.tryParse(f.value);
      if (d == null) return f.value;
      return '${d.day.toString().padLeft(2, '0')}.${d.month.toString().padLeft(2, '0')}.${d.year}';
    case 'bool':
      return f.value == '1' ? 'Да' : 'Нет';
    default:
      return f.value;
  }
}

/// Подписи оси X для периода: где ставить деление и что писать.
class _Axis {
  const _Axis(this.key, this.label, {this.skipFirst = false});

  final Object Function(DateTime time) key;
  final String Function(DateTime time) label;
  final bool skipFirst;
}

_Axis _axisFor(_Period period) {
  switch (period) {
    case _Period.day:
      return _Axis(
        (DateTime t) => _msk(t).hour,
        (DateTime t) => '${_msk(t).hour.toString().padLeft(2, '0')}:00',
        skipFirst: true,
      );
    case _Period.week:
    case _Period.month:
      return _Axis((DateTime t) => _msk(t).day, (DateTime t) => '${_msk(t).day}');
    case _Period.years5:
      return _Axis(
        (DateTime t) => _msk(t).year,
        (DateTime t) => '${_msk(t).year}',
        skipFirst: true,
      );
    case _Period.months3:
    case _Period.months6:
    case _Period.year:
      return _Axis(
        (DateTime t) => _msk(t).month,
        (DateTime t) => _monthsShort[_msk(t).month - 1],
        skipFirst: true,
      );
  }
}

// --- Даты (всё показываем по Москве, время торгов MOEX) ---

/// Настенное время МСК (UTC+3, без перехода на летнее) как UTC-DateTime.
DateTime _msk(DateTime t) => t.toUtc().add(const Duration(hours: 3));

const _monthsGenitive = [
  'янв.', 'февр.', 'мар.', 'апр.', 'мая', 'июн.',
  'июл.', 'авг.', 'сент.', 'окт.', 'нояб.', 'дек.',
];

const _monthsShort = [
  'янв', 'фев', 'мар', 'апр', 'май', 'июн',
  'июл', 'авг', 'сен', 'окт', 'ноя', 'дек',
];

/// `18 сент. 26 г.`
String _fullDate(DateTime t) {
  final m = _msk(t);
  final yy = (m.year % 100).toString().padLeft(2, '0');
  return '${m.day} ${_monthsGenitive[m.month - 1]} $yy г.';
}

/// Подпись точки под пальцем: для часов — дата и время.
String _scrubLabel(DateTime t) {
  final m = _msk(t);
  if (m.hour == 0 && m.minute == 0) return _fullDate(t);
  final hh = m.hour.toString().padLeft(2, '0');
  final mm = m.minute.toString().padLeft(2, '0');
  return '${m.day} ${_monthsGenitive[m.month - 1]}, $hh:$mm';
}

/// Подпись оси Y: без валюты, без лишних нулей (`281`, `280,5`).
String _axisPrice(double v, {required bool bond}) {
  final abs = v.abs();
  final decimals = abs >= 1000 ? 0 : (abs >= 1 ? 2 : 4);
  var text = v.toStringAsFixed(decimals);
  if (text.contains('.')) text = text.replaceFirst(RegExp(r'\.?0+$'), '');
  return text.replaceAll('.', ',') + (bond ? '%' : '');
}
