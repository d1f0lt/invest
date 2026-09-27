import '../api/alerts_api.dart';
import '../api/securities_api.dart';

/// Чего ждём от цены.
enum AlertDirection { up, down }

/// Как пользователь задал изменение: в деньгах (пунктах) или в процентах.
enum AlertMode { amount, percent }

/// Уведомление о цене бумаги (хранится в `notifier`, приходит в Telegram).
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

  /// Из ответа сервера. Режим ввода восстанавливается по `input_percent`:
  /// есть — пользователь вводил проценты, нет — сумму.
  factory PriceAlert.fromDto(AlertDto d) {
    final percent = d.inputPercent;
    return PriceAlert(
      id: d.id,
      security: Security(
        secid: d.secid,
        board: d.board,
        shortName: d.shortName.isEmpty ? null : d.shortName,
        currency: d.currency.isEmpty ? null : d.currency,
        lastPrice: d.currentPrice,
      ),
      direction: d.above ? AlertDirection.up : AlertDirection.down,
      mode: percent != null ? AlertMode.percent : AlertMode.amount,
      value: percent != null ? percent.abs() : _round((d.targetPrice - d.basePrice).abs()),
      basePrice: d.basePrice,
      targetPrice: d.targetPrice,
      createdAt: d.createdAt,
      updatedAt: d.updatedAt,
      triggeredAt: d.triggeredAt,
      triggeredPrice: d.triggeredPrice,
    );
  }

  final String id;

  /// Бумага; `lastPrice` — текущая цена на момент последней загрузки списка.
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

  /// Текущая цена бумаги (с последней загрузки списка).
  double? get currentPrice => security.lastPrice;

  /// Изменение цели относительно базовой цены, со знаком.
  double get delta => targetPrice - basePrice;

  /// То же в процентах, со знаком.
  double get deltaPercent => basePrice == 0 ? 0 : delta / basePrice * 100;

  /// Цель, которую покажет приложение до сохранения. Сервер считает её сам
  /// от своей текущей цены (и округляет до шага цены бумаги).
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

  /// Что отправить на сервер: проценты уходят процентами (со знаком),
  /// сумма — готовой целевой ценой.
  static AlertTarget apiTarget({
    required double basePrice,
    required AlertDirection direction,
    required AlertMode mode,
    required double value,
  }) {
    if (mode == AlertMode.percent) {
      return AlertTarget.percent(direction == AlertDirection.up ? value : -value);
    }
    return AlertTarget.price(
      targetFor(basePrice: basePrice, direction: direction, mode: mode, value: value),
    );
  }

  /// Условие этого уведомления для повторного создания (кнопка «Отменить»).
  AlertTarget get restoreTarget => mode == AlertMode.percent
      ? AlertTarget.percent(direction == AlertDirection.up ? value : -value)
      : AlertTarget.price(targetPrice);

  static double _round(double v) => double.parse(v.toStringAsFixed(6));
}
