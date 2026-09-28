import 'dart:async';

import 'package:flutter/foundation.dart';
import 'package:shared_preferences/shared_preferences.dart';

import '../api/alerts_api.dart';
import '../api/api_client.dart';
import '../api/securities_api.dart';
import 'price_alert.dart';

/// Уведомления о ценах и привязка Telegram. Всё хранится на сервере
/// (`notifier` через gateway): сервер сам следит за ценой и присылает
/// сообщение в Telegram, приложение только показывает и настраивает.
class AlertsStore extends ChangeNotifier {
  AlertsStore._();

  static final AlertsStore instance = AlertsStore._();

  /// Пользователь нажал «Позже» на экране «Подключите Telegram».
  static const _skipKey = 'telegram_prompt_skipped';
  static const _pollEvery = Duration(seconds: 3);
  static const _pollFor = Duration(minutes: 3);

  final _api = AlertsApi();

  List<PriceAlert> _items = const [];
  bool _loaded = false;
  bool _refreshing = false;
  String? _error;
  TelegramStatus? _telegram;
  bool _telegramSkipped = false;
  bool _skipLoaded = false;
  Timer? _poll;
  DateTime? _pollUntil;
  int _generation = 0;

  /// Сначала активные, потом сработавшие; внутри — недавно изменённые выше.
  List<PriceAlert> get items => _items;
  List<PriceAlert> get active => _items.where((a) => !a.triggered).toList();
  List<PriceAlert> get triggered => _items.where((a) => a.triggered).toList();

  /// Список хотя бы раз загрузился с сервера.
  bool get loaded => _loaded;
  bool get refreshing => _refreshing;

  /// Ошибка загрузки, пока список ни разу не загрузился.
  String? get error => _error;

  /// null — статус ещё не известен (не загрузился).
  TelegramStatus? get telegram => _telegram;
  bool get telegramLinked => _telegram?.linked ?? false;

  /// Ждём, пока пользователь нажмёт Start в боте.
  bool get waitingForTelegram => _poll != null;

  /// Показать экран «Подключите Telegram» вместо списка: бот есть,
  /// Telegram не привязан и пользователь не нажимал «Позже».
  bool get showTelegramPrompt =>
      _skipLoaded &&
      _telegram != null &&
      _telegram!.botEnabled &&
      !_telegram!.linked &&
      !_telegramSkipped;

  /// Бумага со свежей ценой (с последней загрузки списка) или null.
  Security? priceOf(PriceAlert alert) =>
      alert.security.lastPrice == null ? null : alert.security;

  /// Загружает уведомления и статус Telegram. Ошибку загрузки списка
  /// пробрасывает (если список уже был — старый остаётся на экране).
  Future<void> refresh() async {
    if (_refreshing) return;
    final gen = _generation;
    _refreshing = true;
    notifyListeners();
    try {
      await _loadSkip();
      final results = await Future.wait([
        _api.list(),
        _api.telegramStatus().then<TelegramStatus?>((s) => s).catchError((Object e) {
          debugPrint('alerts: статус Telegram не загрузился ($e)');
          return _telegram;
        }),
      ]);
      if (gen != _generation) return;
      _items = _sorted((results[0] as List<AlertDto>).map(PriceAlert.fromDto).toList());
      _setTelegram(results[1] as TelegramStatus?);
      _loaded = true;
      _error = null;
    } on ApiException catch (e) {
      if (gen == _generation && !_loaded) _error = e.message;
      rethrow;
    } catch (e) {
      if (gen == _generation && !_loaded) _error = 'Не удалось загрузить уведомления';
      rethrow;
    } finally {
      if (gen == _generation) {
        _refreshing = false;
        notifyListeners();
      }
    }
  }

  /// Только статус Telegram (после возврата из Telegram в приложение).
  Future<void> refreshTelegram() async {
    final gen = _generation;
    try {
      final status = await _api.telegramStatus();
      if (gen != _generation) return;
      _setTelegram(status);
      notifyListeners();
    } catch (e) {
      debugPrint('alerts: статус Telegram не загрузился ($e)');
    }
  }

  Future<PriceAlert> create({
    required Security security,
    required AlertDirection direction,
    required AlertMode mode,
    required double value,
    required double basePrice,
  }) async {
    final dto = await _api.create(
      secid: security.secid,
      board: security.board,
      target: PriceAlert.apiTarget(
        basePrice: basePrice,
        direction: direction,
        mode: mode,
        value: value,
      ),
    );
    final alert = PriceAlert.fromDto(dto);
    _items = _sorted([alert, ..._items.where((a) => a.id != alert.id)]);
    notifyListeners();
    return alert;
  }

  /// Новое условие (и, возможно, бумага): базовая цена снова текущая,
  /// статус снова «активно».
  Future<PriceAlert> update(
    String id, {
    required Security security,
    required AlertDirection direction,
    required AlertMode mode,
    required double value,
    required double basePrice,
  }) async {
    final old = _items.where((a) => a.id == id).firstOrNull;
    final moved = old == null || old.security.key != security.key;
    final dto = await _api.update(
      id,
      secid: moved ? security.secid : null,
      board: moved ? security.board : null,
      target: PriceAlert.apiTarget(
        basePrice: basePrice,
        direction: direction,
        mode: mode,
        value: value,
      ),
    );
    final alert = PriceAlert.fromDto(dto);
    _items = _sorted([for (final a in _items) a.id == id ? alert : a]);
    notifyListeners();
    return alert;
  }

  /// Убирает из списка сразу, на сервере удаляет следом; при ошибке
  /// возвращает обратно и пробрасывает её.
  Future<void> remove(String id) async {
    final index = _items.indexWhere((a) => a.id == id);
    if (index < 0) return;
    final removed = _items[index];
    _items = [..._items]..removeAt(index);
    notifyListeners();
    try {
      await _api.delete(id);
    } on ApiException catch (e) {
      if (e.statusCode == 404) return;
      _items = _sorted([..._items, removed]);
      notifyListeners();
      rethrow;
    }
  }

  /// «Отменить» удаление: на сервере это новое уведомление с тем же
  /// условием (базовая цена — текущая).
  Future<void> restore(PriceAlert alert) async {
    final dto = await _api.create(
      secid: alert.security.secid,
      board: alert.security.board,
      target: alert.restoreTarget,
    );
    _items = _sorted([..._items, PriceAlert.fromDto(dto)]);
    notifyListeners();
  }

  /// Одноразовая ссылка на бота. После неё ждём привязку: опрашиваем
  /// статус несколько минут, пока пользователь не нажмёт Start.
  Future<Uri> telegramLink() async {
    final url = await _api.telegramLink();
    _startPolling();
    return url;
  }

  Future<void> disconnectTelegram() async {
    await _api.unlinkTelegram();
    _setTelegram(TelegramStatus(linked: false, botEnabled: _telegram?.botEnabled ?? true));
    await _setSkipped(true);
    notifyListeners();
  }

  /// «Позже» на экране «Подключите Telegram».
  Future<void> skipTelegramPrompt() async {
    await _setSkipped(true);
    notifyListeners();
  }

  /// Выход из аккаунта.
  void reset() {
    _generation++;
    _stopPolling();
    _items = const [];
    _loaded = false;
    _refreshing = false;
    _error = null;
    _telegram = null;
    notifyListeners();
  }

  void _setTelegram(TelegramStatus? status) {
    _telegram = status;
    if (status?.linked ?? false) _stopPolling();
  }

  void _startPolling() {
    _pollUntil = DateTime.now().add(_pollFor);
    if (_poll != null) return;
    _poll = Timer.periodic(_pollEvery, (_) async {
      if (DateTime.now().isAfter(_pollUntil!)) {
        _stopPolling();
        notifyListeners();
        return;
      }
      await refreshTelegram();
    });
    notifyListeners();
  }

  void _stopPolling() {
    _poll?.cancel();
    _poll = null;
  }

  Future<void> _loadSkip() async {
    if (_skipLoaded) return;
    try {
      final prefs = await SharedPreferences.getInstance();
      _telegramSkipped = prefs.getBool(_skipKey) ?? false;
    } catch (e) {
      debugPrint('alerts: не удалось прочитать настройки ($e)');
    }
    _skipLoaded = true;
  }

  Future<void> _setSkipped(bool value) async {
    _telegramSkipped = value;
    _skipLoaded = true;
    try {
      final prefs = await SharedPreferences.getInstance();
      await prefs.setBool(_skipKey, value);
    } catch (e) {
      debugPrint('alerts: не удалось сохранить настройки ($e)');
    }
  }

  static List<PriceAlert> _sorted(List<PriceAlert> list) => list
    ..sort((a, b) {
      if (a.triggered != b.triggered) return a.triggered ? 1 : -1;
      final at = a.triggeredAt ?? a.updatedAt, bt = b.triggeredAt ?? b.updatedAt;
      return bt.compareTo(at);
    });
}
