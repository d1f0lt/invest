import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:url_launcher/url_launcher.dart';

import '../api/api_client.dart';
import 'alerts_store.dart';

/// «Подключить Telegram»: берёт у сервера одноразовую ссылку на бота и
/// открывает чат с ним — сразу в приложении Telegram (`tg://resolve`),
/// а если его нет — ссылкой t.me в браузере. Пользователю остаётся нажать
/// «Start»; привязку приложение увидит само, когда он вернётся.
Future<void> connectTelegram(BuildContext context) async {
  final messenger = ScaffoldMessenger.of(context);
  final Uri link;
  try {
    link = await AlertsStore.instance.telegramLink();
  } on ApiException catch (e) {
    messenger.showSnackBar(SnackBar(content: Text(e.message)));
    return;
  } catch (_) {
    messenger.showSnackBar(
      const SnackBar(content: Text('Не удалось получить ссылку на бота')),
    );
    return;
  }

  if (await _open(link)) return;
  if (!context.mounted) return;
  await showDialog<void>(
    context: context,
    builder: (context) => AlertDialog(
      title: const Text('Не получилось открыть Telegram'),
      content: SelectableText(
        'Откройте ссылку на устройстве с Telegram и нажмите «Start»:\n\n$link',
      ),
      actions: [
        TextButton(
          onPressed: () {
            Clipboard.setData(ClipboardData(text: link.toString()));
            Navigator.of(context).pop();
            messenger.showSnackBar(const SnackBar(content: Text('Ссылка скопирована')));
          },
          child: const Text('Скопировать'),
        ),
        TextButton(
          onPressed: () => Navigator.of(context).pop(),
          child: const Text('Закрыть'),
        ),
      ],
    ),
  );
}

Future<bool> _open(Uri link) async {
  final app = _appLink(link);
  if (app != null) {
    try {
      if (await launchUrl(app, mode: LaunchMode.externalApplication)) return true;
    } catch (_) {
      // Telegram не установлен — ниже откроем t.me в браузере.
    }
  }
  try {
    return await launchUrl(link, mode: LaunchMode.externalApplication);
  } catch (_) {
    return false;
  }
}

/// `https://t.me/<bot>?start=<token>` → `tg://resolve?domain=<bot>&start=<token>`.
Uri? _appLink(Uri link) {
  if (link.host != 't.me' || link.pathSegments.length != 1) return null;
  return Uri(
    scheme: 'tg',
    host: 'resolve',
    queryParameters: {
      'domain': link.pathSegments.first,
      if (link.queryParameters['start'] != null) 'start': link.queryParameters['start']!,
    },
  );
}
