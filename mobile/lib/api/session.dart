import 'dart:async';
import 'dart:convert';

import 'package:flutter_secure_storage/flutter_secure_storage.dart';

import 'api_client.dart';
import 'auth_api.dart';

/// Текущая сессия пользователя: токены в памяти + копия в защищённом
/// хранилище (Keychain / Android Keystore), чтобы вход переживал перезапуск.
///
/// Access-токен живёт недолго (ACCESS_TOKEN_TTL, по умолчанию 30 мин), поэтому
/// перед запросом [accessToken] при необходимости обновляет пару через
/// `POST /api/v1/refresh`. Сервер ротирует refresh-токен и при повторном
/// использовании старого отзывает всё семейство — поэтому одновременные
/// обновления схлопываются в один запрос.
class Session {
  Session._();

  static final Session instance = Session._();

  static const _storageKey = 'auth_tokens';

  /// Обновляем заранее, чтобы токен не истёк «в полёте».
  static const _refreshMargin = Duration(seconds: 60);

  // С v10 на Android по умолчанию используются шифры на базе Keystore
  // (encryptedSharedPreferences устарел), отдельные опции не нужны.
  final FlutterSecureStorage _storage = const FlutterSecureStorage();
  final ApiClient _client = ApiClient();

  AuthTokens? _tokens;
  Future<AuthTokens>? _refreshing;

  /// Вызывается, когда сессию восстановить нельзя (refresh-токен истёк или
  /// отозван) — приложение должно показать экран входа.
  void Function()? onExpired;

  AuthTokens? get tokens => _tokens;
  bool get isLoggedIn => _tokens != null;

  /// Читает сохранённые токены при старте приложения.
  Future<void> restore() async {
    try {
      final raw = await _storage.read(key: _storageKey);
      if (raw == null) return;
      final tokens = AuthTokens.fromJson(jsonDecode(raw) as Map<String, dynamic>);
      if (tokens.refreshTokenExpiresAt.isBefore(DateTime.now())) {
        await _storage.delete(key: _storageKey);
        return;
      }
      _tokens = tokens;
    } catch (_) {
      // Повреждённая запись или недоступное хранилище — просто просим войти.
      _tokens = null;
      await _safeDelete();
    }
  }

  /// Сохраняет новую пару токенов (после входа или обновления).
  Future<void> save(AuthTokens tokens) async {
    _tokens = tokens;
    try {
      await _storage.write(key: _storageKey, value: jsonEncode(tokens.toJson()));
    } catch (_) {
      // Не смогли записать — сессия будет жить до перезапуска.
    }
  }

  /// Локальный выход: забывает токены и возвращает refresh-токен (для logout).
  Future<String?> clear() async {
    final refresh = _tokens?.refreshToken;
    _tokens = null;
    _refreshing = null;
    await _safeDelete();
    return refresh;
  }

  /// Действующий access-токен; при необходимости обновляет пару.
  Future<String> accessToken() async {
    final tokens = _tokens;
    if (tokens == null) throw const ApiException('Требуется вход', statusCode: 401);
    if (DateTime.now().isBefore(tokens.expiresAt.subtract(_refreshMargin))) {
      return tokens.accessToken;
    }
    return (await refresh()).accessToken;
  }

  /// Обновляет пару токенов. [rejectedAccessToken] — токен, на который сервер
  /// ответил 401: если пара уже сменилась, повторно не обновляем.
  Future<AuthTokens> refresh({String? rejectedAccessToken}) {
    final current = _tokens;
    if (current == null) {
      return Future.error(const ApiException('Требуется вход', statusCode: 401));
    }
    if (rejectedAccessToken != null && current.accessToken != rejectedAccessToken) {
      return Future.value(current);
    }
    return _refreshing ??= _doRefresh(current).whenComplete(() => _refreshing = null);
  }

  Future<AuthTokens> _doRefresh(AuthTokens current) async {
    try {
      final json = await _client.post('/api/v1/refresh', {
        'refresh_token': current.refreshToken,
      });
      final fresh = AuthTokens.fromJson(json! as Map<String, dynamic>);
      // Пока шёл запрос, пользователь мог выйти — тогда не воскрешаем сессию.
      if (!identical(_tokens, current)) {
        throw const ApiException('Требуется вход', statusCode: 401);
      }
      await save(fresh);
      return fresh;
    } on ApiException catch (e) {
      // 400/401 — refresh-токен недействителен; сеть/5xx — сессию не трогаем.
      if (e.statusCode == 401 || e.statusCode == 400) {
        if (identical(_tokens, current)) {
          await clear();
          onExpired?.call();
        }
        throw const ApiException('Сессия истекла, войдите снова', statusCode: 401);
      }
      rethrow;
    }
  }

  Future<void> _safeDelete() async {
    try {
      await _storage.delete(key: _storageKey);
    } catch (_) {}
  }
}
