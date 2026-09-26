final _loginRe = RegExp(r'^[a-zA-Z0-9_.]+$');
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

String? validateLogin(String? v) {
  final value = v?.trim() ?? '';
  if (value.isEmpty) return 'Заполните поле';
  if (value.length < 3) return 'Минимум 3 символа';
  if (!_loginRe.hasMatch(value)) return 'Только латиница, цифры, «_» и «.»';
  return null;
}

/// Минимальная длина совпадает с сервисом users (`minPasswordLength`).
String? validatePassword(String? v) {
  final value = v ?? '';
  if (value.isEmpty) return 'Заполните поле';
  if (value.length < 8) return 'Минимум 8 символов';
  return null;
}
