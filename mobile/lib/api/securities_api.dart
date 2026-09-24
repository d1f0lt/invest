import 'api_client.dart';

/// Бумага из справочника price_updater (одна строка на `(secid, board)`).
class Security {
  const Security({
    required this.secid,
    required this.board,
    this.shortName,
    this.secName,
    this.isin,
    this.currency,
    this.lastPrice,
  });

  factory Security.fromJson(Map<String, dynamic> json) => Security(
        secid: json['secid'] as String? ?? '',
        board: json['board'] as String? ?? '',
        shortName: _nonEmpty(json['short_name']),
        secName: _nonEmpty(json['sec_name']),
        isin: _nonEmpty(json['isin']),
        currency: _nonEmpty(json['currency']),
        lastPrice: (json['last_price'] as num?)?.toDouble(),
      );

  final String secid;
  final String board;
  final String? shortName;
  final String? secName;
  final String? isin;
  final String? currency;
  final double? lastPrice;

  /// Название для списка: краткое → полное → тикер.
  String get title => shortName ?? secName ?? secid;

  /// Облигации (TQOB — ОФЗ, TQCB — корпоративные): цена в % от номинала.
  bool get isBond => board == 'TQOB' || board == 'TQCB';

  /// Тип бумаги по режиму торгов MOEX.
  String get kind => switch (board) {
        'TQBR' => 'Акция',
        'TQTF' => 'Фонд',
        'TQOB' => 'ОФЗ',
        'TQCB' => 'Облигация',
        _ => board,
      };

  static String? _nonEmpty(Object? v) {
    final s = (v as String?)?.trim();
    return s == null || s.isEmpty ? null : s;
  }
}

class SecuritiesApi {
  SecuritiesApi({ApiClient? client}) : _client = client ?? ApiClient();

  final ApiClient _client;

  /// `GET /api/v1/prices` без `tickers` → все бумаги с последними ценами.
  Future<List<Security>> all() async {
    final json = await _client.get('/api/v1/prices', auth: true) as Map<String, dynamic>?;
    return (json?['prices'] as List<dynamic>? ?? const [])
        .map((e) => Security.fromJson(e as Map<String, dynamic>))
        .where((s) => s.secid.isNotEmpty)
        .toList();
  }
}
