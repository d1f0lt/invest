import 'package:flutter/material.dart';

import '../api/securities_api.dart';
import '../portfolio/stats_format.dart';

const _nbsp = ' ';

/// Облигации — в % от номинала (`98,53%`), остальное — в валюте:
/// `1 234,50 ₽`; дешёвые бумаги — с нужной точностью (`0,02345 ₽`).
String formatPrice(double value, {required bool bond, String? currency}) {
  final abs = value.abs();
  final decimals = bond || abs >= 1 || abs == 0 ? 2 : 6;
  var text = abs.toStringAsFixed(decimals);
  if (decimals > 2) text = text.replaceFirst(RegExp(r'0+$'), '');
  final parts = text.split('.');
  final digits = parts[0];
  final grouped = StringBuffer();
  for (var i = 0; i < digits.length; i++) {
    if (i > 0 && (digits.length - i) % 3 == 0) grouped.write(_nbsp);
    grouped.write(digits[i]);
  }
  final sign = value < 0 ? '−' : '';
  final number =
      parts.length > 1 && parts[1].isNotEmpty ? '$sign$grouped,${parts[1]}' : '$sign$grouped';
  if (bond) return '$number%';
  return '$number$_nbsp${currencySymbol(currency)}';
}

String currencySymbol(String? currency) => switch (currency) {
      null || 'SUR' || 'RUB' => '₽',
      'USD' => r'$',
      'EUR' => '€',
      'CNY' => '¥',
      _ => currency,
    };

/// Изменение цены: `+0,76 ₽ ▲ 0,27%` / `−1,84 ₽ ▼ 0,66%`, цветом по знаку.
/// У облигаций цена и так в процентах номинала — показываем только `▲ 0,27%`.
class PriceChange extends StatelessWidget {
  const PriceChange({
    super.key,
    required this.change,
    required this.percent,
    required this.security,
    this.style,
    this.compact = false,
  });

  final double change;
  final double percent;
  final Security security;
  final TextStyle? style;

  /// Только процент со знаком (для строк списков).
  final bool compact;

  @override
  Widget build(BuildContext context) {
    final rounded = double.parse(percent.toStringAsFixed(2));
    final color = changeColor(context, percent);
    final base = (style ?? Theme.of(context).textTheme.bodyMedium)!
        .copyWith(color: color, fontWeight: FontWeight.w500);
    if (compact) return Text(formatPercent(percent), style: base);

    final pct = formatPercent(percent.abs()).replaceFirst('+', '');
    final icon = rounded > 0
        ? Icons.arrow_drop_up_rounded
        : rounded < 0
            ? Icons.arrow_drop_down_rounded
            : null;
    final size = (base.fontSize ?? 14) * 1.5;
    return Text.rich(
      TextSpan(
        style: base,
        children: [
          if (!security.isBond)
            TextSpan(
              text: '${change > 0 ? '+' : ''}'
                  '${formatPrice(change, bond: false, currency: security.currency)} ',
            ),
          if (icon != null)
            WidgetSpan(
              alignment: PlaceholderAlignment.middle,
              child: Icon(icon, size: size, color: color),
            ),
          TextSpan(text: pct),
        ],
      ),
      maxLines: 1,
      overflow: TextOverflow.ellipsis,
    );
  }
}

/// Значок бумаги: цветная плашка с первыми буквами тикера.
class TickerBadge extends StatelessWidget {
  const TickerBadge({super.key, required this.secid, this.size = 44, this.square = false});

  final String secid;
  final double size;

  /// Скруглённый квадрат (карточка актива) вместо круга (списки).
  final bool square;

  static const _palette = [
    Color(0xFF5B2A86),
    Color(0xFF0E8C8C),
    Color(0xFF1A1F71),
    Color(0xFFB5487A),
    Color(0xFF2F6FDB),
    Color(0xFFD9822B),
  ];

  @override
  Widget build(BuildContext context) {
    final color = _palette[secid.codeUnits.fold(0, (a, c) => a + c) % _palette.length];
    final letters = secid.length > 2 ? secid.substring(0, 2) : secid;
    return Container(
      width: size,
      height: size,
      alignment: Alignment.center,
      decoration: BoxDecoration(
        color: square ? null : color,
        gradient: square
            ? LinearGradient(
                begin: Alignment.topLeft,
                end: Alignment.bottomRight,
                colors: [color, Color.lerp(color, Colors.white, 0.35)!],
              )
            : null,
        shape: square ? BoxShape.rectangle : BoxShape.circle,
        borderRadius: square ? BorderRadius.circular(size * 0.26) : null,
      ),
      child: Text(
        letters,
        style: TextStyle(
          color: Colors.white,
          fontWeight: FontWeight.w700,
          fontSize: size * 0.32,
        ),
      ),
    );
  }
}

/// Строка бумаги в списке: значок, название, «тикер · тип»; справа цена
/// и изменение за день.
class SecurityTile extends StatelessWidget {
  const SecurityTile({super.key, required this.security, required this.onTap});

  final Security security;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    final textTheme = Theme.of(context).textTheme;
    final s = security;
    final price = s.lastPrice;
    final change = s.dayChange, percent = s.dayChangePercent;
    return ListTile(
      onTap: onTap,
      contentPadding: const EdgeInsets.symmetric(horizontal: 16, vertical: 2),
      leading: TickerBadge(secid: s.secid),
      title: Text(s.title, maxLines: 1, overflow: TextOverflow.ellipsis),
      subtitle: Text(
        '${s.secid} · ${s.kind}',
        maxLines: 1,
        overflow: TextOverflow.ellipsis,
        style: textTheme.bodySmall?.copyWith(color: scheme.onSurfaceVariant),
      ),
      trailing: price == null
          ? null
          : Column(
              mainAxisAlignment: MainAxisAlignment.center,
              crossAxisAlignment: CrossAxisAlignment.end,
              mainAxisSize: MainAxisSize.min,
              children: [
                Text(
                  formatPrice(price, bond: s.isBond, currency: s.currency),
                  style: textTheme.titleSmall?.copyWith(fontWeight: FontWeight.w600),
                ),
                if (change != null && percent != null)
                  PriceChange(
                    change: change,
                    percent: percent,
                    security: s,
                    compact: true,
                    style: textTheme.bodySmall,
                  ),
              ],
            ),
    );
  }
}
