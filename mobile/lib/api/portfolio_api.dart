import 'api_client.dart';
import 'auth_api.dart';

class Portfolio {
  const Portfolio({required this.id, required this.name});

  factory Portfolio.fromJson(Map<String, dynamic> json) => Portfolio(
        id: json['id'] as String,
        name: json['name'] as String? ?? '',
      );

  final String id;
  final String name;

  String get displayName => name.trim().isEmpty ? 'Без названия' : name;
}

/// Краткая сводка по портфелю для шапки и списка портфелей.
class PortfolioStats {
  const PortfolioStats({
    this.profit = 0,
    this.profitPercent = 0,
    this.dayProfit = 0,
    this.dayPercent = 0,
    this.yieldPercent = 0,
    this.hasData = false,
  });

  /// Из ответа `GET /portfolios/{id}/pnl`.
  ///
  /// Прибыль — `total_pnl` (сделки + дивиденды/купоны − налоги и комиссии),
  /// проценты считаются от чистых пополнений (`net_deposits`).
  factory PortfolioStats.fromPnL(Map<String, dynamic> json) {
    double value(String key) => (json[key] as num?)?.toDouble() ?? 0;
    final profit = value('total_pnl');
    final deposits = value('net_deposits');
    final percent = deposits > 0 ? profit / deposits * 100 : 0.0;
    // Отчётов пока не храним отдельно — «данные есть», если в портфеле
    // появились позиции или хоть одна денежная операция/сумма.
    final instruments = json['instruments'] as List<dynamic>? ?? const [];
    const totals = [
      'total_pnl', 'total_realized_pnl', 'total_unrealized_pnl', 'total_dividends',
      'total_coupons', 'total_accrued_interest', 'total_taxes', 'total_fees',
      'total_other', 'net_deposits', 'cash_balance',
    ];
    final hasData = instruments.isNotEmpty || totals.any((k) => value(k) != 0);
    return PortfolioStats(
      hasData: hasData,
      profit: profit,
      profitPercent: percent,
      // TODO: изменение за день — в API нет цен закрытия прошлого дня.
      // TODO: доходность — годовая (XIRR по пополнениям), пока = прибыль / пополнения.
      yieldPercent: percent,
    );
  }

  static const zero = PortfolioStats();

  final double profit;
  final double profitPercent;
  final double dayProfit;
  final double dayPercent;
  final double yieldPercent;

  /// Есть ли в портфеле хоть какие-то данные (загружен ли отчёт / внесены сделки).
  final bool hasData;
}

/// Запросы к портфелям через gateway (нужен access-токен).
class PortfolioApi {
  PortfolioApi({ApiClient? client}) : _client = client ?? ApiClient();

  final ApiClient _client;

  String get _token {
    final tokens = Session.tokens;
    if (tokens == null) throw const ApiException('Требуется вход', statusCode: 401);
    return tokens.accessToken;
  }

  /// `GET /api/v1/portfolios` → массив портфелей.
  Future<List<Portfolio>> list() async {
    final json = await _client.get('/api/v1/portfolios', accessToken: _token);
    return (json as List<dynamic>? ?? const [])
        .map((e) => Portfolio.fromJson(e as Map<String, dynamic>))
        .toList();
  }

  /// `POST /api/v1/portfolios` → 201 с созданным портфелем.
  Future<Portfolio> create(String name) async {
    final json = await _client.post(
      '/api/v1/portfolios',
      {'name': name.trim()},
      accessToken: _token,
    );
    return Portfolio.fromJson(json! as Map<String, dynamic>);
  }

  /// `PATCH /api/v1/portfolios/{id}` → переименованный портфель.
  Future<Portfolio> rename(String portfolioId, String name) async {
    final json = await _client.patch(
      '/api/v1/portfolios/${Uri.encodeComponent(portfolioId)}',
      {'name': name.trim()},
      accessToken: _token,
    );
    return Portfolio.fromJson(json! as Map<String, dynamic>);
  }

  /// `GET /api/v1/portfolios/{id}/pnl` → сводка по прибыли.
  Future<PortfolioStats> stats(String portfolioId) async {
    final json = await _client.get(
      '/api/v1/portfolios/${Uri.encodeComponent(portfolioId)}/pnl',
      accessToken: _token,
    );
    return PortfolioStats.fromPnL(json! as Map<String, dynamic>);
  }
}
