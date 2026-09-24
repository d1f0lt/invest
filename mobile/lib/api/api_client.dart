import 'dart:async';
import 'dart:convert';
import 'dart:io';

import '../config.dart';

/// Ошибка запроса к gateway с текстом, который можно показать пользователю.
class ApiException implements Exception {
  const ApiException(this.message, {this.statusCode, this.serverMessage});

  final String message;
  final int? statusCode;

  /// Поле `error` из тела ответа gateway (`{"error": "..."}`), если было.
  final String? serverMessage;

  @override
  String toString() => 'ApiException($statusCode): $message';
}

/// Минимальный JSON-клиент поверх dart:io (без сторонних пакетов).
class ApiClient {
  ApiClient({String? baseUrl}) : _base = Uri.parse(baseUrl ?? AppConfig.apiBaseUrl);

  final Uri _base;
  final HttpClient _http = HttpClient()..connectionTimeout = AppConfig.requestTimeout;

  /// Возвращает разобранный JSON ответа (объект, массив) или null для пустого тела.
  Future<Object?> post(String path, Map<String, dynamic> body, {String? accessToken}) =>
      _send('POST', path, body: body, accessToken: accessToken);

  Future<Object?> patch(String path, Map<String, dynamic> body, {String? accessToken}) =>
      _send('PATCH', path, body: body, accessToken: accessToken);

  Future<Object?> get(String path, {String? accessToken}) =>
      _send('GET', path, accessToken: accessToken);

  Future<Object?> _send(
    String method,
    String path, {
    Map<String, dynamic>? body,
    String? accessToken,
  }) async {
    final uri = _base.resolve(path);
    try {
      final request = await _http.openUrl(method, uri);
      request.headers.set(HttpHeaders.acceptHeader, 'application/json');
      if (accessToken != null) {
        request.headers.set(HttpHeaders.authorizationHeader, 'Bearer $accessToken');
      }
      if (body != null) {
        request.headers.contentType = ContentType.json;
        request.write(jsonEncode(body));
      }

      final response = await request.close().timeout(AppConfig.requestTimeout);
      final text = await response.transform(utf8.decoder).join();
      final json = text.isEmpty ? null : _tryDecode(text);

      if (response.statusCode >= 200 && response.statusCode < 300) {
        return json;
      }
      final serverMessage = json is Map<String, dynamic> ? json['error'] as String? : null;
      throw ApiException(
        _describeStatus(response.statusCode),
        statusCode: response.statusCode,
        serverMessage: serverMessage,
      );
    } on ApiException {
      rethrow;
    } on TimeoutException {
      throw const ApiException('Сервер не отвечает. Попробуйте ещё раз');
    } on SocketException {
      throw const ApiException('Нет связи с сервером');
    } on HttpException {
      throw const ApiException('Ошибка соединения с сервером');
    }
  }

  static Object? _tryDecode(String text) {
    try {
      return jsonDecode(text);
    } on FormatException {
      return null;
    }
  }

  static String _describeStatus(int code) => switch (code) {
        400 => 'Проверьте введённые данные',
        401 => 'Требуется вход',
        404 => 'Не найдено',
        409 => 'Уже существует',
        >= 500 => 'Сервер временно недоступен',
        _ => 'Ошибка сервера ($code)',
      };
}
