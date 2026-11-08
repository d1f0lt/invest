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

  static String get assistantBaseUrl {
    const override = String.fromEnvironment('ASSISTANT_BASE_URL');
    if (override.isNotEmpty) return override;
    final base = Uri.parse(apiBaseUrl);
    return base.scheme == 'http' && base.port == 8080
        ? base.replace(port: 8087).toString()
        : apiBaseUrl;
  }

  static const requestTimeout = Duration(seconds: 15);
}
