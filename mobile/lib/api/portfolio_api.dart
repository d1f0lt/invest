import 'api_client.dart';

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
    this.positions = const [],
    this.cashBalance = 0,
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
      positions: instruments
          .map((e) => Position.fromJson(e as Map<String, dynamic>))
          .where((p) => p.quantity > 0)
          .toList(),
      cashBalance: value('cash_balance'),
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

  /// Открытые позиции (количество > 0) — для вкладки «Активы».
  final List<Position> positions;

  /// Свободные рубли на счёте (`cash_balance`).
  final double cashBalance;
}

/// Открытая позиция по бумаге (элемент `instruments` из `/pnl`).
class Position {
  const Position({
    required this.secid,
    required this.board,
    required this.quantity,
    required this.avgCost,
    this.currentPrice,
    this.marketValue,
    this.unrealizedPnl,
  });

  factory Position.fromJson(Map<String, dynamic> json) {
    double? opt(String key) => (json[key] as num?)?.toDouble();
    return Position(
      secid: json['secid'] as String? ?? '',
      board: json['board'] as String? ?? '',
      quantity: opt('quantity') ?? 0,
      avgCost: opt('avg_cost') ?? 0,
      currentPrice: opt('current_price'),
      marketValue: opt('market_value'),
      unrealizedPnl: opt('unrealized_pnl'),
    );
  }

  final String secid;
  final String board;
  final double quantity;

  /// Средняя цена покупки одной бумаги (в валюте, у облигаций — в рублях).
  final double avgCost;

  /// Текущая цена одной бумаги в валюте (у облигаций уже пересчитана из % номинала).
  final double? currentPrice;

  /// quantity × currentPrice. Нет цены — null.
  final double? marketValue;

  /// Изменение стоимости позиции относительно цены покупки.
  final double? unrealizedPnl;

  String get key => '$secid@$board';

  /// Сколько заплачено за текущее количество.
  double get cost => quantity * avgCost;

  /// Текущая стоимость, а если цены нет — стоимость покупки.
  double get value => marketValue ?? cost;

  double? get changePercent {
    final pnl = unrealizedPnl;
    if (pnl == null || cost == 0) return null;
    return pnl / cost * 100;
  }
}

/// Сделка (`GET /portfolios/{id}/trades`).
class Trade {
  const Trade({
    required this.id,
    required this.secid,
    required this.board,
    required this.side,
    required this.quantity,
    required this.price,
    required this.fee,
    required this.currency,
    required this.executedAt,
    this.accruedInterest = 0,
    this.externalId = '',
  });

  factory Trade.fromJson(Map<String, dynamic> json) => Trade(
        id: json['id'] as String? ?? '',
        secid: json['secid'] as String? ?? '',
        board: json['board'] as String? ?? '',
        side: json['side'] as String? ?? '',
        quantity: (json['quantity'] as num?)?.toDouble() ?? 0,
        price: (json['price'] as num?)?.toDouble() ?? 0,
        fee: (json['fee'] as num?)?.toDouble() ?? 0,
        currency: json['currency'] as String? ?? 'RUB',
        executedAt: DateTime.parse(json['executed_at'] as String),
        accruedInterest: (json['accrued_interest'] as num?)?.toDouble() ?? 0,
        externalId: json['external_id'] as String? ?? '',
      );

  final String id;
  final String secid;
  final String board;

  /// `buy` | `sell`.
  final String side;
  final double quantity;
  final double price;
  final double fee;
  final String currency;
  final DateTime executedAt;
  final double accruedInterest;
  final String externalId;

  bool get isBuy => side == 'buy';

  /// Вводный остаток: бумага, которая уже была на счёте на начало периода
  /// отчёта (куплена раньше). Цена — рыночная на ту дату.
  bool get isOpening => externalId.contains(openingMarker);

  /// Движение денег по счёту: покупка — минус (с НКД и комиссией), продажа — плюс.
  double get cashFlow {
    final amount = quantity * price + accruedInterest;
    return isBuy ? -(amount + fee) : amount - fee;
  }
}

/// Денежная операция (`GET /portfolios/{id}/cash-operations`).
/// `amount` со знаком: пополнение/дивиденд — плюс, вывод/налог — минус.
class CashOperation {
  const CashOperation({
    required this.id,
    required this.type,
    required this.amount,
    required this.currency,
    required this.occurredAt,
    this.secid,
    this.board,
    this.description = '',
    this.externalId = '',
  });

  factory CashOperation.fromJson(Map<String, dynamic> json) {
    String? opt(String key) {
      final v = (json[key] as String?)?.trim();
      return v == null || v.isEmpty ? null : v;
    }

    return CashOperation(
      id: json['id'] as String? ?? '',
      type: json['type'] as String? ?? 'other',
      amount: (json['amount'] as num?)?.toDouble() ?? 0,
      currency: json['currency'] as String? ?? 'RUB',
      occurredAt: DateTime.parse(json['occurred_at'] as String),
      secid: opt('secid'),
      board: opt('board'),
      description: opt('description') ?? '',
      externalId: opt('external_id') ?? '',
    );
  }

  final String id;

  /// deposit | withdrawal | dividend | coupon | redemption | tax | fee | other
  final String type;
  final double amount;
  final String currency;
  final DateTime occurredAt;
  final String? secid;
  final String? board;
  final String description;
  final String externalId;

  /// Часть вводного остатка (см. [Trade.isOpening]).
  bool get isOpening => externalId.contains(openingMarker);

  /// Служебное пополнение «на стоимость бумаг» вводного остатка — в ленте
  /// операций не показываем (сами бумаги показаны отдельными строками).
  bool get isOpeningSecurities => isOpening && externalId.contains(':securities:');
}

/// Метка в external_id строк вводного остатка (см. парсер отчётов).
const openingMarker = ':opening:';

/// Запросы к портфелям через gateway (нужен access-токен).
class PortfolioApi {
  PortfolioApi({ApiClient? client}) : _client = client ?? ApiClient();

  final ApiClient _client;

  /// `GET /api/v1/portfolios` → массив портфелей.
  Future<List<Portfolio>> list() async {
    final json = await _client.get('/api/v1/portfolios', auth: true);
    return (json as List<dynamic>? ?? const [])
        .map((e) => Portfolio.fromJson(e as Map<String, dynamic>))
        .toList();
  }

  /// `POST /api/v1/portfolios` → 201 с созданным портфелем.
  Future<Portfolio> create(String name) async {
    final json = await _client.post(
      '/api/v1/portfolios',
      {'name': name.trim()},
      auth: true,
    );
    return Portfolio.fromJson(json! as Map<String, dynamic>);
  }

  /// `PATCH /api/v1/portfolios/{id}` → переименованный портфель.
  Future<Portfolio> rename(String portfolioId, String name) async {
    final json = await _client.patch(
      '/api/v1/portfolios/${Uri.encodeComponent(portfolioId)}',
      {'name': name.trim()},
      auth: true,
    );
    return Portfolio.fromJson(json! as Map<String, dynamic>);
  }

  /// `GET /api/v1/portfolios/{id}/pnl` → сводка по прибыли.
  Future<PortfolioStats> stats(String portfolioId) async {
    final json = await _client.get(
      '/api/v1/portfolios/${Uri.encodeComponent(portfolioId)}/pnl',
      auth: true,
    );
    return PortfolioStats.fromPnL(json! as Map<String, dynamic>);
  }

  /// `GET /api/v1/portfolios/{id}/trades` → все сделки.
  Future<List<Trade>> trades(String portfolioId) async {
    final json = await _client.get(
      '/api/v1/portfolios/${Uri.encodeComponent(portfolioId)}/trades',
      auth: true,
    );
    return (json as List<dynamic>? ?? const [])
        .map((e) => Trade.fromJson(e as Map<String, dynamic>))
        .toList();
  }

  /// `GET /api/v1/portfolios/{id}/cash-operations` → денежные операции.
  Future<List<CashOperation>> cashOperations(String portfolioId) async {
    final json = await _client.get(
      '/api/v1/portfolios/${Uri.encodeComponent(portfolioId)}/cash-operations',
      auth: true,
    );
    return (json as List<dynamic>? ?? const [])
        .map((e) => CashOperation.fromJson(e as Map<String, dynamic>))
        .toList();
  }
}
