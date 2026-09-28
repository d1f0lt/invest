/// Настройки приложения.
///
/// Адрес gateway можно переопределить при запуске, не меняя код:
///   flutter run --dart-define=API_BASE_URL=http://10.0.2.2:8080
/// (10.0.2.2 — это localhost компьютера из Android-эмулятора;
/// iOS-симулятор видит localhost напрямую.)
class AppConfig {
  static const apiBaseUrl = String.fromEnvironment(
    'API_BASE_URL',
    defaultValue: 'http://localhost:8080',
  );

  static const requestTimeout = Duration(seconds: 15);
}
