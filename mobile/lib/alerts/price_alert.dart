import '../api/securities_api.dart';

/// Чего ждём от цены.
enum AlertDirection { up, down }

/// Как пользователь задал изменение: в деньгах (пунктах) или в процентах.
enum AlertMode { amount, percent }

/// Уведомление о цене бумаги. Пока хранится только на устройстве
/// (заготовка под `notifier`): поля совпадают с будущим API —
/// базовая цена, цель, статус «сработало».
class PriceAlert {
  const PriceAlert({
    required this.id,
    required this.security,
    required this.direction,
    required this.mode,
    required this.value,
    required this.basePrice,
    required this.targetPrice,
    required this.createdAt,
    required this.updatedAt,
    this.triggeredAt,
    this.triggeredPrice,
  });

  factory PriceAlert.fromJson(Map<String, dynamic> json) => PriceAlert(
        id: json['id'] as String,
        security: Security.fromJson(json['security'] as Map<String, dynamic>),
        direction: AlertDirection.values.byName(json['direction'] as String),
        mode: AlertMode.values.byName(json['mode'] as String),
        value: (json['value'] as num).toDouble(),
        basePrice: (json['base_price'] as num).toDouble(),
        targetPrice: (json['target_price'] as num).toDouble(),
        createdAt: DateTime.parse(json['created_at'] as String),
        updatedAt: DateTime.parse(json['updated_at'] as String),
        triggeredAt: json['triggered_at'] == null
            ? null
            : DateTime.parse(json['triggered_at'] as String),
        triggeredPrice: (json['triggered_price'] as num?)?.toDouble(),
      );

  final String id;

  /// Только справочные поля бумаги (без цен).
  final Security security;
  final AlertDirection direction;
  final AlertMode mode;

  /// Введённое изменение (всегда > 0): в валюте или в процентах — см. [mode].
  final double value;

  /// Цена на момент создания/последнего редактирования — от неё считается цель.
  final double basePrice;
  final double targetPrice;
  final DateTime createdAt;
  final DateTime updatedAt;
  final DateTime? triggeredAt;
  final double? triggeredPrice;

  bool get triggered => triggeredAt != null;

  /// Изменение цели относительно базовой цены, со знаком.
  double get delta => targetPrice - basePrice;

  /// То же в процентах, со знаком.
  double get deltaPercent => basePrice == 0 ? 0 : delta / basePrice * 100;

  /// Достигла ли цена цели.
  bool reachedBy(double price) =>
      direction == AlertDirection.up ? price >= targetPrice : price <= targetPrice;

  static double targetFor({
    required double basePrice,
    required AlertDirection direction,
    required AlertMode mode,
    required double value,
  }) {
    final change = mode == AlertMode.percent ? basePrice * value / 100 : value;
    final target = direction == AlertDirection.up ? basePrice + change : basePrice - change;
    return double.parse(target.toStringAsFixed(basePrice.abs() >= 1 ? 4 : 8));
  }

  PriceAlert markTriggered(double price, DateTime at) => PriceAlert(
        id: id,
        security: security,
        direction: direction,
        mode: mode,
        value: value,
        basePrice: basePrice,
        targetPrice: targetPrice,
        createdAt: createdAt,
        updatedAt: updatedAt,
        triggeredAt: at,
        triggeredPrice: price,
      );

  Map<String, dynamic> toJson() => {
        'id': id,
        'security': security.toJson(),
        'direction': direction.name,
        'mode': mode.name,
        'value': value,
        'base_price': basePrice,
        'target_price': targetPrice,
        'created_at': createdAt.toIso8601String(),
        'updated_at': updatedAt.toIso8601String(),
        if (triggeredAt != null) 'triggered_at': triggeredAt!.toIso8601String(),
        if (triggeredPrice != null) 'triggered_price': triggeredPrice,
      };
}
