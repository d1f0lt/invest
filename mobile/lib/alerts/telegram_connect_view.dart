import 'dart:math' as math;

import 'package:flutter/material.dart';

import 'alerts_store.dart';

const _telegramBlue = Color(0xFF229ED9);
const _telegramLight = Color(0xFF2AABEE);

/// Первый заход на вкладку: предложение подключить Telegram-бота,
/// через которого будут приходить уведомления.
class TelegramConnectView extends StatelessWidget {
  const TelegramConnectView({super.key});

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    final textTheme = Theme.of(context).textTheme;
    return SafeArea(
      child: LayoutBuilder(
        builder: (context, constraints) => SingleChildScrollView(
          padding: const EdgeInsets.fromLTRB(24, 24, 24, 24),
          child: ConstrainedBox(
            constraints: BoxConstraints(minHeight: constraints.maxHeight - 48),
            child: Column(
              mainAxisAlignment: MainAxisAlignment.center,
              children: [
                const _TelegramLogo(),
                const SizedBox(height: 28),
                Text(
                  'Подключите Telegram',
                  textAlign: TextAlign.center,
                  style: textTheme.headlineSmall?.copyWith(fontWeight: FontWeight.w700),
                ),
                const SizedBox(height: 10),
                Text(
                  'Уведомления о ценах будут приходить от нашего бота — '
                  'даже когда приложение закрыто',
                  textAlign: TextAlign.center,
                  style: textTheme.bodyLarge?.copyWith(color: scheme.onSurfaceVariant),
                ),
                const SizedBox(height: 28),
                const _Benefit(
                  icon: Icons.bolt_rounded,
                  text: 'Сообщение придёт, как только цена достигнет цели',
                ),
                const _Benefit(
                  icon: Icons.tune_rounded,
                  text: 'Бот только присылает сообщения — всё настраивается в приложении',
                ),
                const _Benefit(
                  icon: Icons.notifications_off_outlined,
                  text: 'Отключить можно в любой момент',
                ),
                const SizedBox(height: 32),
                SizedBox(
                  width: double.infinity,
                  height: 54,
                  child: FilledButton.icon(
                    onPressed: AlertsStore.instance.completeTelegramPrompt,
                    style: FilledButton.styleFrom(
                      backgroundColor: _telegramBlue,
                      foregroundColor: Colors.white,
                      shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(16)),
                      textStyle: textTheme.titleMedium?.copyWith(fontWeight: FontWeight.w600),
                    ),
                    icon: Transform.rotate(
                      angle: -math.pi / 7,
                      child: const Icon(Icons.send_rounded, size: 20),
                    ),
                    label: const Text('Подключить Telegram'),
                  ),
                ),
              ],
            ),
          ),
        ),
      ),
    );
  }
}

class _TelegramLogo extends StatelessWidget {
  const _TelegramLogo();

  @override
  Widget build(BuildContext context) {
    return SizedBox.square(
      dimension: 132,
      child: Stack(
        alignment: Alignment.center,
        children: [
          Container(
            decoration: BoxDecoration(
              shape: BoxShape.circle,
              color: _telegramLight.withValues(alpha: 0.12),
            ),
          ),
          Container(
            width: 96,
            height: 96,
            decoration: BoxDecoration(
              shape: BoxShape.circle,
              gradient: const LinearGradient(
                begin: Alignment.topCenter,
                end: Alignment.bottomCenter,
                colors: [_telegramLight, _telegramBlue],
              ),
              boxShadow: [
                BoxShadow(
                  color: _telegramBlue.withValues(alpha: 0.35),
                  blurRadius: 24,
                  offset: const Offset(0, 10),
                ),
              ],
            ),
            child: Padding(
              padding: const EdgeInsets.only(left: 6, bottom: 2),
              child: Transform.rotate(
                angle: -math.pi / 7,
                child: const Icon(Icons.send_rounded, color: Colors.white, size: 46),
              ),
            ),
          ),
        ],
      ),
    );
  }
}

class _Benefit extends StatelessWidget {
  const _Benefit({required this.icon, required this.text});

  final IconData icon;
  final String text;

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 6),
      child: Row(
        children: [
          Container(
            width: 40,
            height: 40,
            decoration: BoxDecoration(
              color: _telegramLight.withValues(alpha: 0.12),
              borderRadius: BorderRadius.circular(12),
            ),
            child: Icon(icon, color: _telegramBlue, size: 22),
          ),
          const SizedBox(width: 14),
          Expanded(
            child: Text(
              text,
              style: Theme.of(context).textTheme.bodyMedium?.copyWith(color: scheme.onSurface),
            ),
          ),
        ],
      ),
    );
  }
}
