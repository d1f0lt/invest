import 'package:flutter/material.dart';

const _nbsp = ' ';
const _positive = Color(0xFF1E9E5A);

/// `1234.5` → `+1 234,50 ₽`; ноль — без знака: `0,00 ₽`.
String formatMoney(double value) {
  final rounded = double.parse(value.toStringAsFixed(2));
  final parts = rounded.abs().toStringAsFixed(2).split('.');
  final digits = parts[0];
  final grouped = StringBuffer();
  for (var i = 0; i < digits.length; i++) {
    if (i > 0 && (digits.length - i) % 3 == 0) grouped.write(_nbsp);
    grouped.write(digits[i]);
  }
  return '${_sign(rounded)}$grouped,${parts[1]}$_nbsp₽';
}

/// `12.3456` → `+12,35%`, `1.5` → `+1,5%`, `0` → `0%`.
String formatPercent(double value) {
  final rounded = double.parse(value.toStringAsFixed(2));
  var text = rounded.abs().toStringAsFixed(2);
  text = text.replaceFirst(RegExp(r'\.?0+$'), '');
  return '${_sign(rounded)}${text.replaceAll('.', ',')}%';
}

String _sign(double v) => v > 0 ? '+' : (v < 0 ? '−' : '');

/// Цвет значения: зелёный — плюс, красный — минус, серый — ноль.
Color changeColor(BuildContext context, double value) {
  final scheme = Theme.of(context).colorScheme;
  final rounded = double.parse(value.toStringAsFixed(2));
  if (rounded > 0) return _positive;
  if (rounded < 0) return scheme.error;
  return scheme.onSurfaceVariant;
}

/// Подпись + значение (деньги и/или процент) — одна колонка метрики.
/// [colored] — красить ли значение по знаку (прибыль и «за день» — да,
/// доходность — нет).
class StatValue extends StatelessWidget {
  const StatValue({
    super.key,
    required this.label,
    this.money,
    required this.percent,
    this.large = false,
    this.colored = true,
  });

  final String label;
  final double? money;
  final double percent;
  final bool large;
  final bool colored;

  @override
  Widget build(BuildContext context) {
    final textTheme = Theme.of(context).textTheme;
    final scheme = Theme.of(context).colorScheme;
    final color = colored ? changeColor(context, money ?? percent) : scheme.onSurface;
    final valueStyle = (large ? textTheme.titleLarge : textTheme.bodyMedium)
        ?.copyWith(fontWeight: FontWeight.w700, color: color);
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      mainAxisSize: MainAxisSize.min,
      children: [
        Text(label, style: textTheme.labelMedium?.copyWith(color: scheme.onSurfaceVariant)),
        const SizedBox(height: 2),
        if (money != null)
          Text(formatMoney(money!), maxLines: 1, overflow: TextOverflow.ellipsis, style: valueStyle),
        Text(
          formatPercent(percent),
          maxLines: 1,
          style: money != null
              ? textTheme.labelMedium?.copyWith(color: color, fontWeight: FontWeight.w600)
              : valueStyle,
        ),
      ],
    );
  }
}
