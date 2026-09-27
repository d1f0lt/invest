import 'api_client.dart';

/// Уведомление о цене в том виде, как его отдаёт gateway (`/api/v1/alerts`,
/// сервис `notifier`).
class AlertDto {
  const AlertDto({
    required this.id,
    required this.secid,
    required this.board,
    required this.shortName,
    required this.currency,
    required this.priceInPercent,
    required this.above,
    required this.basePrice,
    required this.targetPrice,
    required this.createdAt,
    required this.updatedAt,
    this.inputPercent,
    this.currentPrice,
    this.triggeredAt,
    this.triggeredPrice,
  });

  factory AlertDto.fromJson(Map<String, dynamic> json) => AlertDto(
        id: json['id'] as String,
        secid: json['secid'] as String? ?? '',
        board: json['board'] as String? ?? '',
        shortName: json['short_name'] as String? ?? '',
        currency: json['currency'] as String? ?? '',
        priceInPercent: json['price_in_percent'] as bool? ?? false,
        above: json['direction'] != 'below',
        basePrice: (json['base_price'] as num).toDouble(),
        targetPrice: (json['target_price'] as num).toDouble(),
        inputPercent: (json['input_percent'] as num?)?.toDouble(),
        currentPrice: (json['current_price'] as num?)?.toDouble(),
        createdAt: DateTime.parse(json['created_at'] as String),
        updatedAt: DateTime.parse(json['updated_at'] as String),
        triggeredAt: json['triggered_at'] == null
            ? null
            : DateTime.parse(json['triggered_at'] as String),
        triggeredPrice: (json['triggered_price'] as num?)?.toDouble(),
      );

  final String id;
  final String secid;
  final String board;
  final String shortName;
  final String currency;
  final bool priceInPercent;

  /// `direction == "above"`: ждём роста до цели, иначе — падения.
  final bool above;
  final double basePrice;
  final double targetPrice;

  /// Процент, который ввёл пользователь; null — цель задана ценой.
  final double? inputPercent;
  final double? currentPrice;
  final DateTime createdAt;
  final DateTime updatedAt;
  final DateTime? triggeredAt;
  final double? triggeredPrice;
}

/// Цель уведомления: ровно одно из двух — цена или процент от текущей цены.
class AlertTarget {
  const AlertTarget.price(double this.price) : percent = null;
  const AlertTarget.percent(double this.percent) : price = null;

  final double? price;

  /// Со знаком: 5 — рост на 5%, -3 — падение на 3%.
  final double? percent;

  Map<String, dynamic> toJson() =>
      price != null ? {'target_price': price} : {'change_percent': percent};
}

/// Статус привязки Telegram (`GET /api/v1/telegram`).
class TelegramStatus {
  const TelegramStatus({
    required this.linked,
    required this.botEnabled,
    this.username,
    this.linkedAt,
  });

  factory TelegramStatus.fromJson(Map<String, dynamic> json) => TelegramStatus(
        linked: json['linked'] as bool? ?? false,
        botEnabled: json['bot_enabled'] as bool? ?? false,
        username: _nonEmpty(json['username'] as String?),
        linkedAt: json['linked_at'] == null ? null : DateTime.parse(json['linked_at'] as String),
      );

  final bool linked;

  /// false — на сервере не настроен бот (`TELEGRAM_BOT_TOKEN`).
  final bool botEnabled;
  final String? username;
  final DateTime? linkedAt;

  static String? _nonEmpty(String? s) => s == null || s.trim().isEmpty ? null : s.trim();
}

class AlertsApi {
  AlertsApi({ApiClient? client}) : _client = client ?? ApiClient();

  final ApiClient _client;

  Future<List<AlertDto>> list() async {
    final json = await _client.get('/api/v1/alerts', auth: true) as Map<String, dynamic>;
    return (json['alerts'] as List<dynamic>? ?? const [])
        .map((e) => AlertDto.fromJson(e as Map<String, dynamic>))
        .toList();
  }

  Future<AlertDto> create({
    required String secid,
    required String board,
    required AlertTarget target,
  }) =>
      _guard(() async {
        final json = await _client.post(
          '/api/v1/alerts',
          {'secid': secid, 'board': board, ...target.toJson()},
          auth: true,
        );
        return AlertDto.fromJson(json as Map<String, dynamic>);
      });

  /// Новая цель (и, если передана, новая бумага): уведомление снова активно.
  Future<AlertDto> update(
    String id, {
    required AlertTarget target,
    String? secid,
    String? board,
  }) =>
      _guard(() async {
        final json = await _client.put(
          '/api/v1/alerts/${Uri.encodeComponent(id)}',
          {
            if (secid != null) 'secid': secid,
            if (board != null) 'board': board,
            ...target.toJson(),
          },
          auth: true,
        );
        return AlertDto.fromJson(json as Map<String, dynamic>);
      });

  Future<void> delete(String id) =>
      _guard(() => _client.delete('/api/v1/alerts/${Uri.encodeComponent(id)}', auth: true));

  Future<TelegramStatus> telegramStatus() async {
    final json = await _client.get('/api/v1/telegram', auth: true) as Map<String, dynamic>;
    return TelegramStatus.fromJson(json);
  }

  /// Одноразовая ссылка `https://t.me/<bot>?start=<token>` (живёт ~15 минут).
  Future<Uri> telegramLink() => _guard(() async {
        final json = await _client.post('/api/v1/telegram/link', const {}, auth: true)
            as Map<String, dynamic>;
        return Uri.parse(json['url'] as String);
      });

  Future<void> unlinkTelegram() => _guard(() => _client.delete('/api/v1/telegram', auth: true));

  /// Ошибки notifier приходят на английском — переводим понятные пользователю.
  static Future<T> _guard<T>(Future<T> Function() call) async {
    try {
      return await call();
    } on ApiException catch (e) {
      final text = _translate(e);
      if (text == null) rethrow;
      throw ApiException(text, statusCode: e.statusCode, serverMessage: e.serverMessage);
    }
  }

  static String? _translate(ApiException e) {
    final m = e.serverMessage ?? '';
    if (m.contains('no current price')) return 'У бумаги пока нет текущей цены';
    if (m.contains('equals the current price')) return 'Цель совпадает с текущей ценой';
    if (m.contains('too many alerts')) return 'Слишком много уведомлений — удалите лишние';
    if (m.contains('unknown security')) return 'Бумага не найдена';
    if (m.contains('several boards')) return 'Бумага торгуется в нескольких режимах — выберите её заново';
    if (m.contains('bot is not configured')) return 'Telegram-бот пока не настроен на сервере';
    if (m.contains('bot is not ready')) return 'Telegram-бот ещё запускается — попробуйте через минуту';
    if (m.contains('alert not found')) return 'Уведомление не найдено — обновите список';
    if (m.contains('change_percent') || m.contains('target price') || m.contains('target_price')) {
      return 'Проверьте условие уведомления';
    }
    return null;
  }
}
