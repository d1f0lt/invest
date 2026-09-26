import '../api/securities_api.dart';

/// Кэш справочных данных о бумагах (название, валюта) по `secid@board`.
/// В портфеле и операциях приходят только тикеры — названия подтягиваем
/// через `GET /prices?tickers=…` один раз за запуск.
class SecurityDirectory {
  SecurityDirectory._();

  static final instance = SecurityDirectory._();

  final _api = SecuritiesApi();
  final Map<String, Security> _byKey = {};
  final Set<String> _requested = {};

  /// Бумага из кэша или «голая» (только тикер), если её ещё нет.
  Security lookup(String secid, String board) =>
      _byKey['$secid@$board'] ?? Security(secid: secid, board: board);

  /// Догружает недостающие тикеры. Ошибки глотает — останутся тикеры
  /// вместо названий, в следующий раз попробуем снова. true — что-то добавилось.
  Future<bool> ensure(Iterable<String> secids) async {
    final missing = secids.where((s) => s.isNotEmpty && !_requested.contains(s)).toSet();
    if (missing.isEmpty) return false;
    _requested.addAll(missing);
    try {
      final found = await _api.prices(missing);
      for (final s in found) {
        _byKey[s.key] = s;
      }
      return found.isNotEmpty;
    } catch (_) {
      _requested.removeAll(missing);
      return false;
    }
  }
}
