import 'package:flutter/material.dart';

import '../api/reports_api.dart';

/// Иконка брокера: картинка с бэкенда, а если её нет или она не загрузилась —
/// первая буква названия на фирменном цвете.
class BrokerAvatar extends StatelessWidget {
  const BrokerAvatar({super.key, required this.broker, this.size = 44});

  final Broker broker;
  final double size;

  static final _api = ReportsApi();

  @override
  Widget build(BuildContext context) {
    final uri = _api.iconUri(broker);
    final fallback = _Letter(broker: broker, size: size);
    return ClipRRect(
      borderRadius: BorderRadius.circular(size * 0.28),
      child: SizedBox.square(
        dimension: size,
        child: uri == null
            ? fallback
            : Image.network(
                uri.toString(),
                width: size,
                height: size,
                fit: BoxFit.cover,
                errorBuilder: (_, _, _) => fallback,
                loadingBuilder: (_, child, progress) => progress == null ? child : fallback,
              ),
      ),
    );
  }
}

class _Letter extends StatelessWidget {
  const _Letter({required this.broker, required this.size});

  final Broker broker;
  final double size;

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    final color = parseHexColor(broker.color) ?? scheme.primary;
    final letter = broker.name.trim().isEmpty ? '?' : broker.name.trim()[0].toUpperCase();
    return ColoredBox(
      color: color,
      child: Center(
        child: Text(
          letter,
          style: TextStyle(
            color: Colors.white,
            fontSize: size * 0.46,
            fontWeight: FontWeight.w700,
          ),
        ),
      ),
    );
  }
}

/// `#RRGGBB` → Color, иначе null.
Color? parseHexColor(String? hex) {
  if (hex == null) return null;
  final value = hex.replaceFirst('#', '');
  if (value.length != 6) return null;
  final rgb = int.tryParse(value, radix: 16);
  return rgb == null ? null : Color(0xFF000000 | rgb);
}
