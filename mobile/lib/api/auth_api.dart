import 'api_client.dart';

/// Пара токенов из `POST /api/v1/login` и `POST /api/v1/refresh`.
class AuthTokens {
  const AuthTokens({
    required this.accessToken,
    required this.expiresAt,
    required this.refreshToken,
    required this.refreshTokenExpiresAt,
    required this.userId,
  });

  factory AuthTokens.fromJson(Map<String, dynamic> json) => AuthTokens(
        accessToken: json['access_token'] as String,
        expiresAt: DateTime.parse(json['expires_at'] as String),
        refreshToken: json['refresh_token'] as String,
        refreshTokenExpiresAt: DateTime.parse(json['refresh_token_expires_at'] as String),
        userId: json['user_id'] as String,
      );

  Map<String, dynamic> toJson() => {
        'access_token': accessToken,
        'expires_at': expiresAt.toUtc().toIso8601String(),
        'refresh_token': refreshToken,
        'refresh_token_expires_at': refreshTokenExpiresAt.toUtc().toIso8601String(),
        'user_id': userId,
      };

  final String accessToken;
  final DateTime expiresAt;
  final String refreshToken;
  final DateTime refreshTokenExpiresAt;
  final String userId;
}

class UserProfile {
  const UserProfile({required this.id, required this.email, this.username});

  factory UserProfile.fromJson(Map<String, dynamic> json) => UserProfile(
        id: json['id'] as String,
        email: json['email'] as String,
        username: json['username'] as String?,
      );

  final String id;
  final String email;
  final String? username;
}

/// Запросы к публичным маршрутам gateway: регистрация, вход, выход.
class AuthApi {
  AuthApi({ApiClient? client}) : _client = client ?? ApiClient();

  final ApiClient _client;

  /// `POST /api/v1/login` — вход по email и паролю.
  Future<AuthTokens> login({required String email, required String password}) async {
    try {
      final json = await _client.post('/api/v1/login', {
        'email': email.trim(),
        'password': password,
      });
      return AuthTokens.fromJson(json! as Map<String, dynamic>);
    } on ApiException catch (e) {
      if (e.statusCode == 401) {
        throw const ApiException('Неверный email или пароль', statusCode: 401);
      }
      rethrow;
    }
  }

  /// `POST /api/v1/users` → 201 с созданным пользователем.
  Future<void> register({
    required String email,
    required String username,
    required String password,
  }) async {
    try {
      await _client.post('/api/v1/users', {
        'email': email.trim(),
        'username': username.trim(),
        'password': password,
      });
    } on ApiException catch (e) {
      throw switch ((e.statusCode, e.serverMessage)) {
        (409, 'email already registered') =>
          const ApiException('Этот email уже зарегистрирован', statusCode: 409),
        (409, 'username already taken') =>
          const ApiException('Этот логин уже занят', statusCode: 409),
        (400, 'invalid email') => const ApiException('Некорректный email', statusCode: 400),
        (400, _) => ApiException('Проверьте данные: ${e.serverMessage ?? ''}', statusCode: 400),
        _ => e,
      };
    }
  }

  /// `GET /api/v1/me` — текущий пользователь.
  Future<UserProfile> me() async {
    final json = await _client.get('/api/v1/me', auth: true);
    return UserProfile.fromJson(json! as Map<String, dynamic>);
  }

  /// `PATCH /api/v1/me` — меняет email и/или логин (переданные поля).
  Future<UserProfile> updateMe({String? email, String? username}) async {
    try {
      final json = await _client.patch('/api/v1/me', {
        if (email != null) 'email': email.trim(),
        if (username != null) 'username': username.trim(),
      }, auth: true);
      return UserProfile.fromJson(json! as Map<String, dynamic>);
    } on ApiException catch (e) {
      throw switch ((e.statusCode, e.serverMessage)) {
        (409, 'email already registered') =>
          const ApiException('Этот email уже зарегистрирован', statusCode: 409),
        (409, 'username already taken') =>
          const ApiException('Этот логин уже занят', statusCode: 409),
        (400, 'invalid email') => const ApiException('Некорректный email', statusCode: 400),
        _ => e,
      };
    }
  }

  /// `POST /api/v1/me/password` — смена пароля. Сервер отзывает все сессии
  /// и возвращает новую пару токенов для этого устройства.
  Future<AuthTokens> changePassword({
    required String currentPassword,
    required String newPassword,
  }) async {
    try {
      final json = await _client.post('/api/v1/me/password', {
        'current_password': currentPassword,
        'new_password': newPassword,
      }, auth: true);
      return AuthTokens.fromJson(json! as Map<String, dynamic>);
    } on ApiException catch (e) {
      throw switch (e.statusCode) {
        403 => const ApiException('Неверный текущий пароль', statusCode: 403),
        400 => const ApiException('Новый пароль слишком короткий', statusCode: 400),
        _ => e,
      };
    }
  }

  /// `DELETE /api/v1/me` — удаление аккаунта с подтверждением паролем.
  Future<void> deleteMe(String password) async {
    try {
      await _client.delete('/api/v1/me', body: {'password': password}, auth: true);
    } on ApiException catch (e) {
      if (e.statusCode == 403) {
        throw const ApiException('Неверный пароль', statusCode: 403);
      }
      rethrow;
    }
  }

  /// `POST /api/v1/logout` — отзывает refresh-токен.
  Future<void> logout(String refreshToken) =>
      _client.post('/api/v1/logout', {'refresh_token': refreshToken});
}
