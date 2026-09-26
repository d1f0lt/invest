import 'dart:convert';
import 'dart:math';

import 'package:flutter/foundation.dart';
import 'package:shared_preferences/shared_preferences.dart';

import '../api/securities_api.dart';
import 'price_alert.dart';

/// Уведомления о ценах. Заготовка: хранятся на устройстве
/// (SharedPreferences), срабатывание проверяется при обновлении списка
/// по текущим ценам с сервера. Когда появится `notifier`, методы
/// [create]/[update]/[remove]/[refresh] переедут на `/api/v1/alerts`.
class AlertsStore extends ChangeNotifier {
  AlertsStore._();

  static final AlertsStore instance = AlertsStore._();

  static const _alertsKey = 'price_alerts';
  static const _telegramKey = 'telegram_prompt_done';

  final _api = SecuritiesApi();
  final _random = Random();

  List<PriceAlert> _items = const [];
  Map<String, Security> _prices = const {};
  bool _loaded = false;
  bool _telegramPromptDone = false;
  bool _refreshing = false;

  /// Сначала активные, потом сработавшие; внутри — недавно изменённые выше.
  List<PriceAlert> get items => _items;
  List<PriceAlert> get active => _items.where((a) => !a.triggered).toList();
  List<PriceAlert> get triggered => _items.where((a) => a.triggered).toList();
  bool get loaded => _loaded;
  bool get refreshing => _refreshing;

  /// Пользователь уже прошёл экран «Подключите Telegram».
  bool get telegramPromptDone => _telegramPromptDone;

  /// Бумага со свежей ценой (после [refresh]) или null.
  Security? priceOf(PriceAlert alert) => _prices[alert.security.key];

  Future<void> load() async {
    try {
      final prefs = await SharedPreferences.getInstance();
      _telegramPromptDone = prefs.getBool(_telegramKey) ?? false;
      final raw = prefs.getString(_alertsKey);
      if (raw != null) {
        _items = _sorted((jsonDecode(raw) as List<dynamic>)
            .map((e) => PriceAlert.fromJson(e as Map<String, dynamic>))
            .toList());
      }
    } catch (e) {
      debugPrint('alerts: не удалось прочитать ($e)');
      _items = const [];
    }
    _loaded = true;
    notifyListeners();
  }

  /// Кнопка «Подключить Telegram». TODO: `POST /api/v1/telegram/link`
  /// и открыть ссылку на бота — пока просто пускаем дальше.
  Future<void> completeTelegramPrompt() async {
    _telegramPromptDone = true;
    notifyListeners();
    try {
      final prefs = await SharedPreferences.getInstance();
      await prefs.setBool(_telegramKey, true);
    } catch (e) {
      debugPrint('alerts: не удалось сохранить ($e)');
    }
  }

  Future<PriceAlert> create({
    required Security security,
    required AlertDirection direction,
    required AlertMode mode,
    required double value,
    required double basePrice,
  }) async {
    final now = DateTime.now();
    final alert = PriceAlert(
      id: '${now.microsecondsSinceEpoch}-${_random.nextInt(1 << 32)}',
      security: Security.fromJson(security.toJson()),
      direction: direction,
      mode: mode,
      value: value,
      basePrice: basePrice,
      targetPrice: PriceAlert.targetFor(
        basePrice: basePrice,
        direction: direction,
        mode: mode,
        value: value,
      ),
      createdAt: now,
      updatedAt: now,
    );
    _remember(security);
    _items = _sorted([alert, ..._items]);
    notifyListeners();
    await _save();
    return alert;
  }

  /// Новое условие: базовая цена снова текущая, статус снова «активно».
  Future<PriceAlert> update(
    String id, {
    required Security security,
    required AlertDirection direction,
    required AlertMode mode,
    required double value,
    required double basePrice,
  }) async {
    final old = _items.firstWhere((a) => a.id == id);
    final alert = PriceAlert(
      id: id,
      security: Security.fromJson(security.toJson()),
      direction: direction,
      mode: mode,
      value: value,
      basePrice: basePrice,
      targetPrice: PriceAlert.targetFor(
        basePrice: basePrice,
        direction: direction,
        mode: mode,
        value: value,
      ),
      createdAt: old.createdAt,
      updatedAt: DateTime.now(),
    );
    _remember(security);
    _items = _sorted([for (final a in _items) a.id == id ? alert : a]);
    notifyListeners();
    await _save();
    return alert;
  }

  /// Возвращает позицию удалённого — для «Отменить».
  Future<int> remove(String id) async {
    final index = _items.indexWhere((a) => a.id == id);
    if (index < 0) return -1;
    _items = [..._items]..removeAt(index);
    notifyListeners();
    await _save();
    return index;
  }

  Future<void> restore(PriceAlert alert) async {
    if (_items.any((a) => a.id == alert.id)) return;
    _items = _sorted([..._items, alert]);
    notifyListeners();
    await _save();
  }

  /// Подтягивает текущие цены и отмечает сработавшие уведомления.
  /// TODO: когда появится `notifier`, это будет просто `GET /api/v1/alerts`.
  Future<void> refresh() async {
    if (_items.isEmpty || _refreshing) return;
    _refreshing = true;
    notifyListeners();
    try {
      final fresh = await _api.prices(_items.map((a) => a.security.secid));
      _prices = {for (final s in fresh) s.key: s};
      final now = DateTime.now();
      var changed = false;
      final updated = <PriceAlert>[];
      for (final a in _items) {
        final price = _prices[a.security.key]?.lastPrice;
        if (!a.triggered && price != null && a.reachedBy(price)) {
          updated.add(a.markTriggered(price, now));
          changed = true;
        } else {
          updated.add(a);
        }
      }
      _items = _sorted(updated);
      if (changed) await _save();
    } finally {
      _refreshing = false;
      notifyListeners();
    }
  }

  void _remember(Security s) {
    if (s.lastPrice != null) _prices = {..._prices, s.key: s};
  }

  static List<PriceAlert> _sorted(List<PriceAlert> list) => list
    ..sort((a, b) {
      if (a.triggered != b.triggered) return a.triggered ? 1 : -1;
      final at = a.triggeredAt ?? a.updatedAt, bt = b.triggeredAt ?? b.updatedAt;
      return bt.compareTo(at);
    });

  Future<void> _save() async {
    try {
      final prefs = await SharedPreferences.getInstance();
      await prefs.setString(_alertsKey, jsonEncode(_items.map((e) => e.toJson()).toList()));
    } catch (e) {
      debugPrint('alerts: не удалось сохранить ($e)');
    }
  }
}
