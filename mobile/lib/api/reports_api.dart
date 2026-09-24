import 'dart:typed_data';

import 'api_client.dart';

/// Брокер, отчёты которого умеет разбирать бэкенд (`GET /api/v1/brokers`).
class Broker {
  const Broker({
    required this.id,
    required this.name,
    required this.fileFormats,
    this.iconUrl,
    this.color,
  });

  factory Broker.fromJson(Map<String, dynamic> json) => Broker(
        id: json['id'] as String,
        name: json['name'] as String? ?? '',
        fileFormats: [
          for (final f in json['file_formats'] as List<dynamic>? ?? const []) (f as String).toLowerCase(),
        ],
        iconUrl: _nonEmpty(json['icon_url']),
        color: _nonEmpty(json['color']),
      );

  /// Ключ брокера — уходит полем `broker` при загрузке отчёта.
  final String id;
  final String name;

  /// Расширения файлов без точки, в нижнем регистре: `['html']`.
  final List<String> fileFormats;

  /// Полный URL или путь на gateway (`/static/brokers/sber.png`).
  final String? iconUrl;

  /// Фирменный цвет `#RRGGBB` — фон буквы, если иконки нет.
  final String? color;

  /// Расширения для системного выбора файла: к `html` добавляем `htm`.
  List<String> get pickerExtensions => {
        for (final f in fileFormats) ...[f, if (f == 'html') 'htm'],
      }.toList();

  /// «HTML» / «HTML, PDF» — для подписи под кнопкой.
  String get formatsLabel => fileFormats.map((f) => f.toUpperCase()).join(', ');

  bool accepts(String filename) {
    final dot = filename.lastIndexOf('.');
    if (dot < 0) return false;
    return pickerExtensions.contains(filename.substring(dot + 1).toLowerCase());
  }
}

enum ReportImportStatus { queued, processing, done, failed }

/// Загруженный отчёт на пути через парсер: queued → processing → done | failed.
class ReportImport {
  const ReportImport({
    required this.id,
    required this.portfolioId,
    required this.filename,
    required this.status,
    this.error,
    this.tradesCreated = 0,
    this.tradesSkipped = 0,
    this.cashCreated = 0,
    this.cashSkipped = 0,
  });

  factory ReportImport.fromJson(Map<String, dynamic> json) {
    int count(String key) => (json[key] as num?)?.toInt() ?? 0;
    return ReportImport(
      id: json['id'] as String,
      portfolioId: json['portfolio_id'] as String? ?? '',
      filename: json['filename'] as String? ?? '',
      status: switch (json['status']) {
        'processing' => ReportImportStatus.processing,
        'done' => ReportImportStatus.done,
        'failed' => ReportImportStatus.failed,
        _ => ReportImportStatus.queued,
      },
      error: _nonEmpty(json['error']),
      tradesCreated: count('trades_created'),
      tradesSkipped: count('trades_skipped'),
      cashCreated: count('cash_operations_created'),
      cashSkipped: count('cash_operations_skipped'),
    );
  }

  final String id;
  final String portfolioId;
  final String filename;
  final ReportImportStatus status;

  /// Для failed — текст для пользователя (приходит с бэкенда по-русски).
  final String? error;

  final int tradesCreated;
  final int tradesSkipped;
  final int cashCreated;
  final int cashSkipped;

  bool get finished => status == ReportImportStatus.done || status == ReportImportStatus.failed;
}

/// Загрузка отчётов брокера через gateway.
class ReportsApi {
  ReportsApi({ApiClient? client}) : _client = client ?? ApiClient();

  final ApiClient _client;

  Future<List<Broker>> brokers() async {
    final json = await _client.get('/api/v1/brokers', auth: true);
    return [
      for (final b in json as List<dynamic>? ?? const []) Broker.fromJson(b as Map<String, dynamic>),
    ];
  }

  /// Отправляет файл; ответ — импорт в статусе queued, дальше его опрашивают [get].
  Future<ReportImport> upload({
    required String portfolioId,
    required String brokerId,
    required String filename,
    required Uint8List bytes,
  }) async {
    final json = await _client.postMultipart(
      '/api/v1/portfolios/$portfolioId/reports',
      fields: {'broker': brokerId},
      fileField: 'file',
      filename: filename,
      bytes: bytes,
      auth: true,
    );
    return ReportImport.fromJson(json as Map<String, dynamic>);
  }

  Future<ReportImport> get(String portfolioId, String importId) async {
    final json = await _client.get(
      '/api/v1/portfolios/$portfolioId/reports/$importId',
      auth: true,
    );
    return ReportImport.fromJson(json as Map<String, dynamic>);
  }

  /// Абсолютный адрес иконки брокера (или null).
  Uri? iconUri(Broker broker) {
    final url = broker.iconUrl;
    if (url == null) return null;
    return _client.resolve(url);
  }
}

String? _nonEmpty(Object? value) {
  final s = value as String?;
  return s == null || s.trim().isEmpty ? null : s;
}

/// Текст ошибки для пользователя: русское сообщение сервера, если оно есть,
/// иначе общее описание по коду ответа.
String userMessage(ApiException e) {
  final server = e.serverMessage;
  if (server != null && RegExp('[А-Яа-яЁё]').hasMatch(server)) return server;
  return e.message;
}
