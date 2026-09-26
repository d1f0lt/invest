import 'package:flutter/material.dart';

import 'payouts.dart';

const _receivedColor = Color(0xFF9366F0);
const _declaredColor = Color(0xFF5CA8F2);
const _forecastColor = Color(0xFFB5D5F7);

Color payoutColor(PayoutStatus status) => switch (status) {
      PayoutStatus.received => _receivedColor,
      PayoutStatus.declared => _declaredColor,
      PayoutStatus.forecast => _forecastColor,
    };

String payoutStatusLabel(PayoutStatus status) => switch (status) {
      PayoutStatus.received => 'Получены',
      PayoutStatus.declared => 'Объявлены',
      PayoutStatus.forecast => 'Прогноз',
    };

const shortMonths = [
  'янв.', 'февр.', 'март', 'апр.', 'май', 'июнь',
  'июль', 'авг.', 'сент.', 'окт.', 'нояб.', 'дек.',
];

/// Сумма выплат за месяц по состояниям.
class MonthTotal {
  MonthTotal(this.month);

  final DateTime month;
  final Map<PayoutStatus, double> byStatus = {};

  double get total => byStatus.values.fold(0.0, (a, b) => a + b);
}

/// Группирует выплаты по месяцам (только месяцы, где что-то есть).
List<MonthTotal> monthTotals(List<Payout> payouts) {
  final result = <MonthTotal>[];
  for (final p in payouts) {
    final month = DateTime(p.date.year, p.date.month);
    if (result.isEmpty || result.last.month != month) result.add(MonthTotal(month));
    final bucket = result.last.byStatus;
    bucket[p.status] = (bucket[p.status] ?? 0) + p.amount;
  }
  return result;
}

/// `3100` → `3,1K`, `194.5` → `194,5`, `1250000` → `1,3M`.
String compactAmount(double value) {
  String fmt(double v) {
    final text = v.toStringAsFixed(v.abs() >= 100 ? 0 : 1);
    return text.replaceFirst(RegExp(r'\.0$'), '').replaceAll('.', ',');
  }

  final abs = value.abs();
  if (abs >= 1e6) return '${fmt(value / 1e6)}M';
  if (abs >= 1e3) return '${fmt(value / 1e3)}K';
  return fmt(value);
}

/// Столбики по месяцам: полученные снизу, объявленные и прогноз — сверху.
class PayoutsChart extends StatelessWidget {
  const PayoutsChart({super.key, required this.months, this.onTap});

  final List<MonthTotal> months;
  final ValueChanged<DateTime>? onTap;

  static const _gridHeight = 170.0;
  static const _valueSpace = 30.0;
  static const _monthSpace = 26.0;

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    final textTheme = Theme.of(context).textTheme;
    final maxTotal = months.fold(0.0, (m, e) => e.total > m ? e.total : m);
    return SizedBox(
      height: _valueSpace + _gridHeight + _monthSpace,
      child: Stack(
        children: [
          Positioned(
            left: 0,
            right: 0,
            bottom: _monthSpace,
            height: _gridHeight,
            child: CustomPaint(painter: _GridPainter(scheme.outlineVariant)),
          ),
          Row(
            crossAxisAlignment: CrossAxisAlignment.end,
            children: [
              for (final m in months)
                Expanded(
                  child: GestureDetector(
                    behavior: HitTestBehavior.opaque,
                    onTap: onTap == null ? null : () => onTap!(m.month),
                    child: Column(
                      mainAxisAlignment: MainAxisAlignment.end,
                      children: [
                        Text(
                          compactAmount(m.total),
                          maxLines: 1,
                          style: textTheme.labelSmall?.copyWith(color: scheme.onSurfaceVariant),
                        ),
                        const SizedBox(height: 6),
                        _Bar(
                          month: m,
                          height: maxTotal <= 0
                              ? 0
                              : (m.total / maxTotal * _gridHeight).clamp(4.0, _gridHeight),
                        ),
                        SizedBox(
                          height: _monthSpace,
                          child: Align(
                            alignment: Alignment.bottomCenter,
                            child: Text(
                              shortMonths[m.month.month - 1],
                              maxLines: 1,
                              style:
                                  textTheme.labelSmall?.copyWith(color: scheme.onSurfaceVariant),
                            ),
                          ),
                        ),
                      ],
                    ),
                  ),
                ),
            ],
          ),
        ],
      ),
    );
  }
}

class _Bar extends StatelessWidget {
  const _Bar({required this.month, required this.height});

  final MonthTotal month;
  final double height;

  @override
  Widget build(BuildContext context) {
    final total = month.total;
    final parts = [
      for (final s in PayoutStatus.values)
        if ((month.byStatus[s] ?? 0) > 0) (s, month.byStatus[s]!),
    ].reversed.toList();
    return LayoutBuilder(
      builder: (context, constraints) {
        final width = (constraints.maxWidth * 0.6).clamp(8.0, 64.0);
        return ClipRRect(
          borderRadius: const BorderRadius.vertical(top: Radius.circular(6)),
          child: SizedBox(
            width: width,
            height: height,
            child: Column(
              children: [
                for (final (status, value) in parts)
                  SizedBox(
                    height: total <= 0 ? 0 : height * value / total,
                    width: width,
                    child: ColoredBox(color: payoutColor(status)),
                  ),
              ],
            ),
          ),
        );
      },
    );
  }
}

class _GridPainter extends CustomPainter {
  _GridPainter(this.color);

  final Color color;

  @override
  void paint(Canvas canvas, Size size) {
    final paint = Paint()
      ..color = color
      ..strokeWidth = 1;
    const lines = 5;
    const dash = 4.0, gap = 4.0;
    for (var i = 0; i <= lines; i++) {
      final y = size.height * i / lines;
      for (var x = 0.0; x < size.width; x += dash + gap) {
        canvas.drawLine(Offset(x, y), Offset((x + dash).clamp(0.0, size.width), y), paint);
      }
    }
  }

  @override
  bool shouldRepaint(_GridPainter old) => old.color != color;
}

/// Легенда: кружок цвета + подпись.
class PayoutsLegend extends StatelessWidget {
  const PayoutsLegend({super.key, required this.statuses});

  final Iterable<PayoutStatus> statuses;

  @override
  Widget build(BuildContext context) {
    final textTheme = Theme.of(context).textTheme;
    return Wrap(
      alignment: WrapAlignment.center,
      spacing: 20,
      runSpacing: 6,
      children: [
        for (final s in statuses)
          Row(
            mainAxisSize: MainAxisSize.min,
            children: [
              Container(
                width: 14,
                height: 14,
                decoration: BoxDecoration(
                  color: payoutColor(s),
                  borderRadius: BorderRadius.circular(4),
                ),
              ),
              const SizedBox(width: 6),
              Text(payoutStatusLabel(s), style: textTheme.bodyMedium),
            ],
          ),
      ],
    );
  }
}
