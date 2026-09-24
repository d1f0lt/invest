import '../api/securities_api.dart';

/// Справочник бумаг в памяти: грузится один раз при открытии поиска
/// и переиспользуется [ttl], чтобы не качать его на каждый ввод.
///
/// Бумаг несколько тысяч (TQBR, TQTF, TQOB, TQCB), поиск — на клиенте.
/// TODO: если справочник разрастётся — серверный поиск (`GET /securities?q=`).
class SecuritiesStore {
  SecuritiesStore._();

  static final instance = SecuritiesStore._();

  static const ttl = Duration(minutes: 10);

  final _api = SecuritiesApi();
  List<Security>? _items;
  DateTime? _loadedAt;
  Future<List<Security>>? _loading;

  Future<List<Security>> load() {
    final items = _items;
    if (items != null && DateTime.now().difference(_loadedAt!) < ttl) {
      return Future.value(items);
    }
    return _loading ??= _api.all().then((list) {
      _items = list;
      _loadedAt = DateTime.now();
      return list;
    }).whenComplete(() => _loading = null);
  }

  /// Поиск по тикеру, названию и ISIN без учёта регистра.
  ///
  /// Порядок: точный тикер/ISIN → тикер с начала → название с начала
  /// (или с начала слова) → вхождение где угодно.
  static List<Security> search(List<Security> items, String query, {int limit = 50}) {
    final q = query.trim().toLowerCase().replaceAll('ё', 'е');
    if (q.isEmpty) return const [];

    final ranked = <(int, Security)>[];
    for (final s in items) {
      final rank = _rank(s, q);
      if (rank != null) ranked.add((rank, s));
    }
    ranked.sort((a, b) {
      final byRank = a.$1.compareTo(b.$1);
      if (byRank != 0) return byRank;
      // Внутри группы — сначала акции, потом фонды, потом облигации; затем по тикеру.
      final byKind = _boardOrder(a.$2.board).compareTo(_boardOrder(b.$2.board));
      if (byKind != 0) return byKind;
      return a.$2.secid.compareTo(b.$2.secid);
    });
    return [for (final r in ranked.take(limit)) r.$2];
  }

  static int? _rank(Security s, String q) {
    final secid = s.secid.toLowerCase();
    final isin = s.isin?.toLowerCase();
    if (secid == q || isin == q) return 0;
    if (secid.startsWith(q)) return 1;
    final names = [
      if (s.shortName != null) _norm(s.shortName!),
      if (s.secName != null) _norm(s.secName!),
    ];
    if (names.any((n) => n.startsWith(q))) return 2;
    if (names.any((n) => n.contains(' $q') || n.contains('"$q') || n.contains('«$q'))) {
      return 3;
    }
    if (secid.contains(q) || names.any((n) => n.contains(q))) return 4;
    if (isin != null && q.length >= 4 && isin.startsWith(q)) return 4;
    return null;
  }

  static String _norm(String s) => s.toLowerCase().replaceAll('ё', 'е');

  static int _boardOrder(String board) => switch (board) {
        'TQBR' => 0,
        'TQTF' => 1,
        'TQOB' => 2,
        'TQCB' => 3,
        _ => 4,
      };
}
