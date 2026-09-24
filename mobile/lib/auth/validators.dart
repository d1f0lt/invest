final _emailRe = RegExp(r'^[^@\s]+@[^@\s]+\.[^@\s]+$');

String? validateRequired(String? value) =>
    (value == null || value.trim().isEmpty) ? 'Заполните поле' : null;

/// Та же проверка, что у сервиса users (`emailRE`).
String? validateEmail(String? v) {
  final value = v?.trim() ?? '';
  if (value.isEmpty) return 'Заполните поле';
  if (!_emailRe.hasMatch(value)) return 'Некорректный email';
  return null;
}
