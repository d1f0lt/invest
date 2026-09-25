import 'package:flutter/material.dart';

import 'api/session.dart';
import 'auth/login_screen.dart';
import 'auth/widgets.dart';
import 'favorites/favorites_store.dart';
import 'main_shell.dart';
import 'portfolio/portfolio_store.dart';

final _navigatorKey = GlobalKey<NavigatorState>();

Future<void> main() async {
  WidgetsFlutterBinding.ensureInitialized();
  // Восстанавливаем сохранённую сессию, чтобы не просить пароль при каждом запуске.
  await Session.instance.restore();
  // Избранное хранится на устройстве — подтягиваем его сразу при запуске.
  await FavoritesStore.instance.load();
  Session.instance.onExpired = () {
    PortfolioStore.instance.reset();
    _navigatorKey.currentState
        ?.pushAndRemoveUntil(fadeRoute(const LoginScreen()), (_) => false);
  };
  runApp(MainApp(loggedIn: Session.instance.isLoggedIn));
}

class MainApp extends StatelessWidget {
  const MainApp({super.key, required this.loggedIn});

  final bool loggedIn;

  @override
  Widget build(BuildContext context) {
    final scheme = ColorScheme.fromSeed(seedColor: const Color(0xFF5B2A86));
    return MaterialApp(
      title: 'Invest',
      navigatorKey: _navigatorKey,
      debugShowCheckedModeBanner: false,
      theme: ThemeData(
        colorScheme: scheme,
        // Верхний бар на всех экранах — слегка серый фон и тонкая линия снизу,
        // чтобы отделялся от содержимого. Цвет не меняется при прокрутке.
        appBarTheme: AppBarTheme(
          backgroundColor: scheme.surfaceContainer,
          surfaceTintColor: Colors.transparent,
          scrolledUnderElevation: 0,
          shape: Border(bottom: BorderSide(color: scheme.outlineVariant.withValues(alpha: 0.6))),
        ),
      ),
      home: loggedIn ? const MainShell() : const LoginScreen(),
    );
  }
}
