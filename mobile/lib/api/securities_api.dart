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
    this.prevClose,
  });

  factory Security.fromJson(Map<String, dynamic> json) => Security(
        secid: json['secid'] as String? ?? '',
        board: json['board'] as String? ?? '',
        shortName: _nonEmpty(json['short_name']),
        secName: _nonEmpty(json['sec_name']),
        isin: _nonEmpty(json['isin']),
        currency: _nonEmpty(json['currency']),
        lastPrice: (json['last_price'] as num?)?.toDouble(),
        prevClose: (json['prev_close'] as num?)?.toDouble(),
      );

  final String secid;
  final String board;
  final String? shortName;
  final String? secName;
  final String? isin;
  final String? currency;
  final double? lastPrice;

  /// Цена закрытия предыдущего торгового дня (`prev_close` из API).
  final double? prevClose;

  /// Ключ бумаги: один тикер может торговаться на нескольких board.
  String get key => '$secid@$board';

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

  /// Изменение цены за день (к закрытию прошлого торгового дня).
  double? get dayChange {
    final last = lastPrice, prev = prevClose;
    if (last == null || prev == null) return null;
    return last - prev;
  }

  /// То же в процентах.
  double? get dayChangePercent {
    final change = dayChange, prev = prevClose;
    if (change == null || prev == null || prev == 0) return null;
    return change / prev * 100;
  }

  /// Для хранения в избранном — только справочные поля, цены всегда свежие.
  Map<String, dynamic> toJson() => {
        'secid': secid,
        'board': board,
        if (shortName != null) 'short_name': shortName,
        if (secName != null) 'sec_name': secName,
        if (isin != null) 'isin': isin,
        if (currency != null) 'currency': currency,
      };

  /// Справочные поля отсюда, цены — из [fresh] (ответ сервера).
  Security withPricesFrom(Security fresh) => Security(
        secid: secid,
        board: board,
        shortName: fresh.shortName ?? shortName,
        secName: fresh.secName ?? secName,
        isin: fresh.isin ?? isin,
        currency: fresh.currency ?? currency,
        lastPrice: fresh.lastPrice,
        prevClose: fresh.prevClose,
      );

  static String? _nonEmpty(Object? v) {
    final s = (v as String?)?.trim();
    return s == null || s.isEmpty ? null : s;
  }
}

/// Период графика в API (`range` у `/prices/{secid}/candles`).
enum CandleRange {
  day('day'),
  week('week'),
  month('month'),
  year('year'),
  fiveYears('5y'),
  all('all');

  const CandleRange(this.apiName);

  final String apiName;
}

class Candle {
  const Candle({
    required this.start,
    required this.open,
    required this.high,
    required this.low,
    required this.close,
  });

  factory Candle.fromJson(Map<String, dynamic> json) => Candle(
        start: DateTime.parse(json['start'] as String),
        open: (json['open'] as num).toDouble(),
        high: (json['high'] as num).toDouble(),
        low: (json['low'] as num).toDouble(),
        close: (json['close'] as num).toDouble(),
      );

  /// Начало свечи (UTC).
  final DateTime start;
  final double open;
  final double high;
  final double low;
  final double close;
}

/// Строка раздела «О компании» (`GET /securities/{secid}/info`).
class SecurityInfoField {
  const SecurityInfoField({
    required this.name,
    required this.title,
    required this.value,
    required this.type,
    this.unit,
  });

  factory SecurityInfoField.fromJson(Map<String, dynamic> json) => SecurityInfoField(
        name: json['name'] as String? ?? '',
        title: json['title'] as String? ?? '',
        value: json['value'] as String? ?? '',
        type: json['type'] as String? ?? 'text',
        unit: json['unit'] as String?,
      );

  /// Стабильный ключ: ISSUER, NAME, ISIN, LOTSIZE, FACEVALUE, MATDATE…
  final String name;
  final String title;

  /// Сырое значение: числа с точкой, даты `YYYY-MM-DD`, bool — `1`/`0`.
  final String value;

  /// text | number | money | percent | date | bool
  final String type;

  /// Код валюты для money (`SUR`, `USD`) или единица (`шт.`).
  final String? unit;
}

/// Дивиденд на одну акцию по дате закрытия реестра (MOEX).
class Dividend {
  const Dividend({
    required this.date,
    required this.value,
    required this.currency,
    this.forecast = false,
  });

  factory Dividend.fromJson(Map<String, dynamic> json) {
    final d = DateTime.parse(json['registry_close_date'] as String);
    return Dividend(
      date: DateTime.utc(d.year, d.month, d.day),
      value: (json['value'] as num).toDouble(),
      currency: json['currency'] as String? ?? 'RUB',
      forecast: json['forecast'] as bool? ?? false,
    );
  }

  /// Дата закрытия реестра (только дата, в UTC-полночь).
  final DateTime date;
  final double value;
  final String currency;

  /// Прогноз аналитиков, а не объявленная выплата.
  final bool forecast;
}

class SecuritiesApi {
  SecuritiesApi({ApiClient? client}) : _client = client ?? ApiClient();

  final ApiClient _client;

  /// `GET /api/v1/securities?q=...&limit=...` — поиск по тикеру, названию
  /// и ISIN в securities_reader. Порядок — лучшие совпадения первыми.
  Future<List<Security>> search(String query, {int limit = 30}) async {
    final q = Uri.encodeQueryComponent(query.trim());
    final json = await _client.get('/api/v1/securities?q=$q&limit=$limit', auth: true)
        as Map<String, dynamic>?;
    return _parseList(json?['securities']);
  }

  /// `GET /api/v1/prices?tickers=A,B` — текущие цены (все board этих тикеров).
  Future<List<Security>> prices(Iterable<String> tickers) async {
    final list = tickers.toSet().toList();
    if (list.isEmpty) return const [];
    final q = Uri.encodeQueryComponent(list.join(','));
    final json = await _client.get('/api/v1/prices?tickers=$q', auth: true)
        as Map<String, dynamic>?;
    return _parseList(json?['prices']);
  }

  /// Свежая цена одной бумаги или null, если сервер её не знает.
  Future<Security?> price(String secid, String board) async {
    final found = await prices([secid]);
    for (final s in found) {
      if (s.board == board) return s;
    }
    return null;
  }

  /// `GET /api/v1/prices/{secid}/candles?range=...&board=...` — свечи для
  /// графика, старые первыми.
  Future<List<Candle>> candles(String secid, String board, CandleRange range) async {
    final path = '/api/v1/prices/${Uri.encodeComponent(secid)}/candles'
        '?range=${range.apiName}&board=${Uri.encodeQueryComponent(board)}';
    final json = await _client.get(path, auth: true) as Map<String, dynamic>?;
    return (json?['candles'] as List<dynamic>? ?? const [])
        .map((e) => Candle.fromJson(e as Map<String, dynamic>))
        .toList();
  }

  /// `GET /api/v1/securities/{secid}/info?board=...` — справочные данные
  /// о бумаге и эмитенте (MOEX), уже в порядке показа.
  Future<List<SecurityInfoField>> info(String secid, String board) async {
    final path = '/api/v1/securities/${Uri.encodeComponent(secid)}/info'
        '?board=${Uri.encodeQueryComponent(board)}';
    final json = await _client.get(path, auth: true) as Map<String, dynamic>?;
    return (json?['fields'] as List<dynamic>? ?? const [])
        .map((e) => SecurityInfoField.fromJson(e as Map<String, dynamic>))
        .where((f) => f.value.isNotEmpty)
        .toList();
  }

  /// `GET /api/v1/securities/{secid}/dividends` — история дивидендов,
  /// новые первыми (включая объявленные с датой в будущем).
  Future<List<Dividend>> dividends(String secid) async {
    final path = '/api/v1/securities/${Uri.encodeComponent(secid)}/dividends';
    final json = await _client.get(path, auth: true) as Map<String, dynamic>?;
    return (json?['dividends'] as List<dynamic>? ?? const [])
        .map((e) => Dividend.fromJson(e as Map<String, dynamic>))
        .toList();
  }

  static List<Security> _parseList(Object? raw) => (raw as List<dynamic>? ?? const [])
      .map((e) => Security.fromJson(e as Map<String, dynamic>))
      .where((s) => s.secid.isNotEmpty)
      .toList();
}
