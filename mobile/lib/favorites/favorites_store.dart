import 'dart:convert';

import 'package:flutter/foundation.dart';
import 'package:shared_preferences/shared_preferences.dart';

import '../api/securities_api.dart';

/// Избранные бумаги. Хранятся только на устройстве (SharedPreferences):
/// список справочных полей бумаги (тикер, board, названия), без цен —
/// цены всегда запрашиваются заново. Новые — в начале списка.
class FavoritesStore extends ChangeNotifier {
  FavoritesStore._();

  static final FavoritesStore instance = FavoritesStore._();

  static const _storageKey = 'favorite_securities';

  List<Security> _items = const [];
  bool _loaded = false;

  List<Security> get items => _items;
  bool get loaded => _loaded;

  bool contains(Security s) => _items.any((e) => e.key == s.key);

  /// Читает список с устройства; вызывается при старте приложения.
  /// Повреждённая запись не роняет запуск — избранное просто пустое.
  Future<void> load() async {
    try {
      final prefs = await SharedPreferences.getInstance();
      final raw = prefs.getString(_storageKey);
      if (raw != null) {
        _items = (jsonDecode(raw) as List<dynamic>)
            .map((e) => Security.fromJson(e as Map<String, dynamic>))
            .where((s) => s.secid.isNotEmpty && s.board.isNotEmpty)
            .toList();
      }
    } catch (e) {
      debugPrint('favorites: не удалось прочитать ($e)');
      _items = const [];
    }
    _loaded = true;
    notifyListeners();
  }

  Future<void> add(Security s) async {
    if (contains(s)) return;
    _items = [_stripPrices(s), ..._items];
    notifyListeners();
    await _save();
  }

  Future<void> remove(Security s) async {
    if (!contains(s)) return;
    _items = _items.where((e) => e.key != s.key).toList();
    notifyListeners();
    await _save();
  }

  /// Возвращает, в избранном ли бумага после переключения.
  Future<bool> toggle(Security s) async {
    if (contains(s)) {
      await remove(s);
      return false;
    }
    await add(s);
    return true;
  }

  /// Вернуть бумагу на прежнее место (отмена удаления).
  Future<void> insertAt(int index, Security s) async {
    if (contains(s)) return;
    final list = [..._items];
    list.insert(index < 0 ? 0 : (index > list.length ? list.length : index), _stripPrices(s));
    _items = list;
    notifyListeners();
    await _save();
  }

  static Security _stripPrices(Security s) => Security.fromJson(s.toJson());

  Future<void> _save() async {
    try {
      final prefs = await SharedPreferences.getInstance();
      await prefs.setString(_storageKey, jsonEncode(_items.map((e) => e.toJson()).toList()));
    } catch (e) {
      debugPrint('favorites: не удалось сохранить ($e)');
    }
  }
}
