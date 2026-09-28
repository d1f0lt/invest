import 'package:flutter/material.dart';

import '../api/securities_api.dart';
import '../portfolio/stats_format.dart';
import '../securities/security_widgets.dart';
import 'price_alert.dart';

const alertUpColor = Color(0xFF1E9E5A);
const alertGradient = [Color(0xFF5B2A86), Color(0xFF0E8C8C)];

Color alertDirectionColor(BuildContext context, AlertDirection d) =>
    d == AlertDirection.up ? alertUpColor : Theme.of(context).colorScheme.error;

IconData alertDirectionIcon(AlertDirection d) =>
    d == AlertDirection.up ? Icons.trending_up_rounded : Icons.trending_down_rounded;

/// Единица «суммы» для бумаги: валюта, у облигаций — пункты (цена в % номинала).
String alertAmountUnit(Security s) => s.isBond ? 'п.' : currencySymbol(s.currency);

String alertPrice(Security s, double value) =>
    formatPrice(value, bond: s.isBond, currency: s.currency);

/// Введённое условие со знаком: `+5%`, `−120,00 ₽`, `+1,5 п.`.
String alertChangeText(Security s, AlertDirection d, AlertMode mode, double value) {
  final signed = d == AlertDirection.up ? value : -value;
  if (mode == AlertMode.percent) return formatPercent(signed);
  final sign = signed > 0 ? '+' : '';
  if (s.isBond) {
    return '$sign${formatPercent(signed).replaceAll('%', '').replaceAll('+', '')} п.';
  }
  return '$sign${formatPrice(signed, bond: false, currency: s.currency)}';
}

const _months = [
  'янв', 'фев', 'мар', 'апр', 'мая', 'июн',
  'июл', 'авг', 'сен', 'окт', 'ноя', 'дек',
];

/// `25 сен в 14:32`; другой год — `25 сен 2025 в 14:32`.
String alertDateTime(DateTime t) {
  final local = t.toLocal();
  final year = local.year == DateTime.now().year ? '' : ' ${local.year}';
  final hh = local.hour.toString().padLeft(2, '0');
  final mm = local.minute.toString().padLeft(2, '0');
  return '${local.day} ${_months[local.month - 1]}$year в $hh:$mm';
}

/// Разбор числа, введённого пользователем: пробелы, запятая вместо точки.
double? parseAlertNumber(String text) =>
    double.tryParse(text.replaceAll(RegExp(r'[\s ]'), '').replaceAll(',', '.'));

/// Число для поля ввода: `5`, `2,5`, `0,015`.
String alertNumberText(double v) {
  final text = v.toStringAsFixed(6).replaceFirst(RegExp(r'\.?0+$'), '');
  return text.replaceAll('.', ',');
}

/// «Красивый плюсик»: круглая кнопка с фирменным градиентом и тенью.
class GradientAddButton extends StatelessWidget {
  const GradientAddButton({super.key, required this.onPressed, this.size = 64});

  final VoidCallback onPressed;
  final double size;

  @override
  Widget build(BuildContext context) {
    return Tooltip(
      message: 'Новое уведомление',
      child: DecoratedBox(
        decoration: BoxDecoration(
          shape: BoxShape.circle,
          gradient: const LinearGradient(
            begin: Alignment.topLeft,
            end: Alignment.bottomRight,
            colors: alertGradient,
          ),
          boxShadow: [
            BoxShadow(
              color: alertGradient.first.withValues(alpha: 0.35),
              blurRadius: 18,
              offset: const Offset(0, 8),
            ),
          ],
        ),
        child: Material(
          type: MaterialType.transparency,
          shape: const CircleBorder(),
          clipBehavior: Clip.antiAlias,
          child: InkWell(
            onTap: onPressed,
            child: SizedBox.square(
              dimension: size,
              child: Icon(Icons.add_rounded, color: Colors.white, size: size * 0.5),
            ),
          ),
        ),
      ),
    );
  }
}
