import 'package:flutter/foundation.dart';

import '../api/api_client.dart';
import '../api/portfolio_api.dart';

/// Список портфелей пользователя и текущий выбранный — общий для всех вкладок.
class PortfolioStore extends ChangeNotifier {
  PortfolioStore._();

  static final instance = PortfolioStore._();

  /// Имя портфеля, который создаётся автоматически, если у пользователя нет ни одного.
  static const defaultName = 'Мой портфель';

  final _api = PortfolioApi();

  List<Portfolio> portfolios = const [];
  Portfolio? current;
  bool loading = false;
  bool loaded = false;
  String? error;

  /// Сводка по каждому портфелю (по id). Нет записи — показываем нули.
  Map<String, PortfolioStats> stats = const {};

  PortfolioStats statsFor(String id) => stats[id] ?? PortfolioStats.zero;

  /// Сводка уже загружена (успешно) — можно решать, пустой ли портфель.
  bool hasStats(String id) => stats.containsKey(id);

  /// Сводка ещё грузится (или не загрузилась) — отличаем от «портфель пуст».
  bool statsLoading = false;

  Future<void> load() async {
    if (loading) return;
    loading = true;
    error = null;
    notifyListeners();
    try {
      var list = await _api.list();
      // У нового пользователя сразу есть портфель по умолчанию.
      if (list.isEmpty) list = [await _api.create(defaultName)];
      portfolios = list;
      loaded = true;
      statsLoading = true; // сразу, чтобы главная не мигнула нулевой сводкой
      // Сохраняем выбор, если такой портфель ещё есть, иначе — первый.
      current = portfolios.where((p) => p.id == current?.id).firstOrNull ??
          portfolios.firstOrNull;
    } on ApiException catch (e) {
      error = e.message;
    } finally {
      loading = false;
      notifyListeners();
    }
    if (loaded) await loadStats();
  }

  /// Подгружает сводку по всем портфелям параллельно. Ошибка по одному
  /// портфелю не мешает остальным — у него просто останутся нули.
  Future<void> loadStats() async {
    statsLoading = true;
    notifyListeners();
    final entries = await Future.wait(portfolios.map((p) async {
      try {
        return MapEntry(p.id, await _api.stats(p.id));
      } on ApiException {
        return null;
      }
    }));
    stats = {
      ...stats,
      for (final e in entries)
        if (e != null) e.key: e.value,
    };
    statsLoading = false;
    notifyListeners();
  }

  void select(Portfolio portfolio) {
    current = portfolio;
    notifyListeners();
  }

  /// Создаёт портфель и сразу делает его текущим. Ошибки пробрасывает вызывающему.
  Future<Portfolio> create(String name) async {
    final created = await _api.create(name);
    portfolios = [...portfolios, created];
    current = created;
    notifyListeners();
    return created;
  }

  /// Переименовывает портфель. Ошибки пробрасывает вызывающему.
  Future<void> rename(String id, String name) async {
    final updated = await _api.rename(id, name);
    portfolios = [for (final p in portfolios) p.id == id ? updated : p];
    if (current?.id == id) current = updated;
    notifyListeners();
  }

  /// Вызывается при выходе из аккаунта.
  void reset() {
    portfolios = const [];
    current = null;
    loaded = false;
    error = null;
    stats = const {};
    statsLoading = false;
    notifyListeners();
  }
}
