import 'dart:async';
import 'dart:convert';
import 'dart:io';
import 'dart:math';
import 'dart:typed_data';

import '../config.dart';
import 'session.dart';

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
  ///
  /// `auth: true` — подставить access-токен из [Session] (обновив его при
  /// необходимости) и при 401 один раз повторить запрос с новым токеном.
  Future<Object?> post(String path, Map<String, dynamic> body, {bool auth = false}) =>
      _send('POST', path, body: body, auth: auth);

  Future<Object?> patch(String path, Map<String, dynamic> body, {bool auth = false}) =>
      _send('PATCH', path, body: body, auth: auth);

  Future<Object?> get(String path, {bool auth = false}) => _send('GET', path, auth: auth);

  Future<Object?> delete(String path, {Map<String, dynamic>? body, bool auth = false}) =>
      _send('DELETE', path, body: body, auth: auth);

  /// `multipart/form-data`: текстовые поля + один файл (загрузка отчёта).
  Future<Object?> postMultipart(
    String path, {
    required Map<String, String> fields,
    required String fileField,
    required String filename,
    required Uint8List bytes,
    bool auth = false,
    Duration timeout = const Duration(seconds: 60),
  }) {
    final boundary = 'invest-${Random.secure().nextInt(1 << 32).toRadixString(16)}'
        '${DateTime.now().microsecondsSinceEpoch}';
    // Имя файла в кавычках заголовка: без кавычек и переводов строк.
    final safeName = filename.replaceAll(RegExp(r'["\r\n]'), '_');
    final head = StringBuffer();
    fields.forEach((name, value) {
      head
        ..write('--$boundary\r\n')
        ..write('Content-Disposition: form-data; name="$name"\r\n\r\n')
        ..write('$value\r\n');
    });
    head
      ..write('--$boundary\r\n')
      ..write('Content-Disposition: form-data; name="$fileField"; filename="$safeName"\r\n')
      ..write('Content-Type: ${_contentTypeFor(filename)}\r\n\r\n');
    // Собираем тело один раз: при повторе после обновления токена оно нужно снова.
    final data = (BytesBuilder(copy: false)
          ..add(utf8.encode(head.toString()))
          ..add(bytes)
          ..add(utf8.encode('\r\n--$boundary--\r\n')))
        .takeBytes();

    return _send(
      'POST',
      path,
      auth: auth,
      timeout: timeout,
      writeBody: (request) {
        request.headers.contentType =
            ContentType('multipart', 'form-data', parameters: {'boundary': boundary});
        request.contentLength = data.length;
        request.add(data);
      },
    );
  }

  /// Полный адрес для пути на gateway (например, иконки брокера).
  Uri resolve(String path) => _base.resolve(path);

  static String _contentTypeFor(String filename) {
    final name = filename.toLowerCase();
    if (name.endsWith('.html') || name.endsWith('.htm')) return 'text/html';
    return 'application/octet-stream';
  }

  Future<Object?> _send(
    String method,
    String path, {
    Map<String, dynamic>? body,
    bool auth = false,
    Duration timeout = AppConfig.requestTimeout,
    void Function(HttpClientRequest request)? writeBody,
  }) async {
    if (!auth) {
      return _sendOnce(method, path, body: body, timeout: timeout, writeBody: writeBody);
    }
    final session = Session.instance;
    final token = await session.accessToken();
    try {
      return await _sendOnce(method, path,
          body: body, accessToken: token, timeout: timeout, writeBody: writeBody);
    } on ApiException catch (e) {
      if (e.statusCode != 401) rethrow;
      // Токен мог истечь/быть отозван раньше срока — обновляем и пробуем ещё раз.
      final fresh = await session.refresh(rejectedAccessToken: token);
      return _sendOnce(method, path,
          body: body, accessToken: fresh.accessToken, timeout: timeout, writeBody: writeBody);
    }
  }

  Future<Object?> _sendOnce(
    String method,
    String path, {
    Map<String, dynamic>? body,
    String? accessToken,
    Duration timeout = AppConfig.requestTimeout,
    void Function(HttpClientRequest request)? writeBody,
  }) async {
    final uri = _base.resolve(path);
    try {
      final request = await _http.openUrl(method, uri);
      request.headers.set(HttpHeaders.acceptHeader, 'application/json');
      if (accessToken != null) {
        request.headers.set(HttpHeaders.authorizationHeader, 'Bearer $accessToken');
      }
      if (writeBody != null) {
        writeBody(request);
      } else if (body != null) {
        request.headers.contentType = ContentType.json;
        request.write(jsonEncode(body));
      }

      final response = await request.close().timeout(timeout);
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
        403 => 'Недостаточно прав',
        404 => 'Не найдено',
        409 => 'Уже существует',
        413 => 'Файл слишком большой',
        >= 500 => 'Сервер временно недоступен',
        _ => 'Ошибка сервера ($code)',
      };
}
