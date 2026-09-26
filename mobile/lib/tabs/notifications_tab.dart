import 'package:flutter/material.dart';

import '../alerts/alert_edit_screen.dart';
import '../alerts/alert_format.dart';
import '../alerts/alerts_store.dart';
import '../alerts/price_alert.dart';
import '../alerts/telegram_connect_view.dart';
import '../api/securities_api.dart';
import '../portfolio/stats_format.dart';
import '../securities/security_widgets.dart';

/// Вкладка «Уведомления»: при первом заходе — предложение подключить
/// Telegram, дальше — список уведомлений о ценах и «+» для нового.
class NotificationsTab extends StatefulWidget {
  const NotificationsTab({super.key});

  @override
  State<NotificationsTab> createState() => _NotificationsTabState();
}

class _NotificationsTabState extends State<NotificationsTab> {
  final _store = AlertsStore.instance;

  @override
  void initState() {
    super.initState();
    _refresh(silent: true);
  }

  Future<void> _refresh({bool silent = false}) async {
    try {
      await _store.refresh();
    } catch (_) {
      if (!silent && mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          const SnackBar(content: Text('Не удалось обновить цены')),
        );
      }
    }
  }

  Future<void> _open([PriceAlert? alert]) async {
    final saved = await Navigator.of(context).push<bool>(
      MaterialPageRoute(builder: (_) => AlertEditScreen(alert: alert)),
    );
    if (saved == true) _refresh(silent: true);
  }

  Future<void> _delete(PriceAlert alert) async {
    final messenger = ScaffoldMessenger.of(context);
    await _store.remove(alert.id);
    messenger
      ..hideCurrentSnackBar()
      ..showSnackBar(
        SnackBar(
          content: Text('Уведомление по ${alert.security.title} удалено'),
          action: SnackBarAction(label: 'Отменить', onPressed: () => _store.restore(alert)),
        ),
      );
  }

  @override
  Widget build(BuildContext context) {
    return ListenableBuilder(
      listenable: _store,
      builder: (context, _) {
        final showList = _store.loaded && _store.telegramPromptDone;
        return Scaffold(
          appBar: AppBar(
            centerTitle: false,
            title: const Text('Уведомления'),
            bottom: PreferredSize(
              preferredSize: const Size.fromHeight(2),
              child: SizedBox(
                height: 2,
                child: showList && _store.refreshing && _store.items.isNotEmpty
                    ? const LinearProgressIndicator()
                    : null,
              ),
            ),
          ),
          floatingActionButton:
              showList && _store.items.isNotEmpty ? GradientAddButton(onPressed: _open) : null,
          body: !_store.loaded
              ? const Center(child: CircularProgressIndicator())
              : !_store.telegramPromptDone
                  ? const TelegramConnectView()
                  : _store.items.isEmpty
                      ? _Empty(onCreate: _open)
                      : _list(),
        );
      },
    );
  }

  Widget _list() {
    final active = _store.active, triggered = _store.triggered;
    return RefreshIndicator(
      onRefresh: _refresh,
      child: ListView(
        physics: const AlwaysScrollableScrollPhysics(),
        padding: const EdgeInsets.fromLTRB(16, 8, 16, 104),
        children: [
          if (active.isNotEmpty) ...[
            _Section(title: 'Активные', count: active.length),
            for (final a in active) _item(a),
          ],
          if (triggered.isNotEmpty) ...[
            _Section(title: 'Сработавшие', count: triggered.length),
            for (final a in triggered) _item(a),
          ],
        ],
      ),
    );
  }

  Widget _item(PriceAlert a) {
    return Padding(
      padding: const EdgeInsets.only(bottom: 10),
      child: Dismissible(
        key: ValueKey(a.id),
        direction: DismissDirection.endToStart,
        onDismissed: (_) => _delete(a),
        background: Container(
          alignment: Alignment.centerRight,
          padding: const EdgeInsets.only(right: 24),
          decoration: BoxDecoration(
            color: Theme.of(context).colorScheme.error,
            borderRadius: BorderRadius.circular(18),
          ),
          child: const Icon(Icons.delete_outline_rounded, color: Colors.white),
        ),
        child: _AlertCard(
          alert: a,
          current: _store.priceOf(a),
          onTap: () => _open(a),
          onDelete: () async {
            if (await confirmAlertDelete(context) == true && mounted) _delete(a);
          },
        ),
      ),
    );
  }
}

class _Section extends StatelessWidget {
  const _Section({required this.title, required this.count});

  final String title;
  final int count;

  @override
  Widget build(BuildContext context) {
    final textTheme = Theme.of(context).textTheme;
    return Padding(
      padding: const EdgeInsets.fromLTRB(4, 12, 4, 10),
      child: Text.rich(
        TextSpan(
          children: [
            TextSpan(text: title),
            TextSpan(
              text: '  $count',
              style: TextStyle(color: Theme.of(context).colorScheme.onSurfaceVariant),
            ),
          ],
        ),
        style: textTheme.titleMedium?.copyWith(fontWeight: FontWeight.w700),
      ),
    );
  }
}

enum _Action { edit, delete }

class _AlertCard extends StatelessWidget {
  const _AlertCard({
    required this.alert,
    required this.current,
    required this.onTap,
    required this.onDelete,
  });

  final PriceAlert alert;
  final Security? current;
  final VoidCallback onTap;
  final VoidCallback onDelete;

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    final textTheme = Theme.of(context).textTheme;
    final a = alert, s = alert.security;
    final color = alertDirectionColor(context, a.direction);
    final up = a.direction == AlertDirection.up;
    return Material(
      color: scheme.surfaceContainerLow,
      borderRadius: BorderRadius.circular(18),
      clipBehavior: Clip.antiAlias,
      child: InkWell(
        onTap: onTap,
        child: Padding(
          padding: const EdgeInsets.fromLTRB(14, 12, 4, 14),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Row(
                children: [
                  Opacity(
                    opacity: a.triggered ? 0.6 : 1,
                    child: TickerBadge(secid: s.secid, size: 42),
                  ),
                  const SizedBox(width: 12),
                  Expanded(
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        Text(
                          s.title,
                          maxLines: 1,
                          overflow: TextOverflow.ellipsis,
                          style: textTheme.titleMedium?.copyWith(fontWeight: FontWeight.w600),
                        ),
                        const SizedBox(height: 2),
                        Row(
                          children: [
                            Icon(alertDirectionIcon(a.direction), size: 16, color: color),
                            const SizedBox(width: 4),
                            Flexible(
                              child: Text(
                                '${up ? 'Вырастет' : 'Упадёт'} до ${alertPrice(s, a.targetPrice)}',
                                maxLines: 1,
                                overflow: TextOverflow.ellipsis,
                                style: textTheme.bodyMedium?.copyWith(
                                  color: color,
                                  fontWeight: FontWeight.w600,
                                ),
                              ),
                            ),
                          ],
                        ),
                      ],
                    ),
                  ),
                  PopupMenuButton<_Action>(
                    tooltip: 'Действия',
                    icon: Icon(Icons.more_vert_rounded, color: scheme.onSurfaceVariant),
                    onSelected: (v) {
                      if (v == _Action.edit) {
                        onTap();
                      } else {
                        onDelete();
                      }
                    },
                    itemBuilder: (_) => const [
                      PopupMenuItem(
                        value: _Action.edit,
                        child: ListTile(
                          contentPadding: EdgeInsets.zero,
                          leading: Icon(Icons.edit_outlined),
                          title: Text('Редактировать'),
                        ),
                      ),
                      PopupMenuItem(
                        value: _Action.delete,
                        child: ListTile(
                          contentPadding: EdgeInsets.zero,
                          leading: Icon(Icons.delete_outline_rounded),
                          title: Text('Удалить'),
                        ),
                      ),
                    ],
                  ),
                ],
              ),
              const SizedBox(height: 10),
              Padding(
                padding: const EdgeInsets.only(right: 10),
                child: Row(
                  children: [
                    _StatusChip(triggered: a.triggered),
                    const SizedBox(width: 10),
                    Expanded(
                      child: Text(
                        _details(),
                        maxLines: 2,
                        overflow: TextOverflow.ellipsis,
                        style: textTheme.bodySmall?.copyWith(color: scheme.onSurfaceVariant),
                      ),
                    ),
                  ],
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }

  String _details() {
    final a = alert, s = alert.security;
    if (a.triggered) {
      final price = a.triggeredPrice;
      return '${alertDateTime(a.triggeredAt!)}'
          '${price == null ? '' : ' по ${alertPrice(s, price)}'}';
    }
    final condition = '${alertChangeText(s, a.direction, a.mode, a.value)} '
        'от ${alertPrice(s, a.basePrice)}';
    final price = current?.lastPrice;
    if (price == null || price == 0) return condition;
    final left = (a.targetPrice - price) / price * 100;
    return '$condition · до цели ${formatPercent(left)}';
  }
}

class _StatusChip extends StatelessWidget {
  const _StatusChip({required this.triggered});

  final bool triggered;

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    final color = triggered ? alertUpColor : scheme.primary;
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 5),
      decoration: BoxDecoration(
        color: color.withValues(alpha: 0.12),
        borderRadius: BorderRadius.circular(20),
      ),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          Icon(
            triggered ? Icons.check_circle_rounded : Icons.visibility_outlined,
            size: 14,
            color: color,
          ),
          const SizedBox(width: 5),
          Text(
            triggered ? 'Сработало' : 'Следим',
            style: Theme.of(context).textTheme.labelMedium?.copyWith(
                  color: color,
                  fontWeight: FontWeight.w600,
                ),
          ),
        ],
      ),
    );
  }
}

class _Empty extends StatelessWidget {
  const _Empty({required this.onCreate});

  final VoidCallback onCreate;

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    final textTheme = Theme.of(context).textTheme;
    return Center(
      child: SingleChildScrollView(
        padding: const EdgeInsets.all(32),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Container(
              width: 88,
              height: 88,
              decoration: BoxDecoration(shape: BoxShape.circle, color: scheme.primaryContainer),
              child: Icon(
                Icons.notifications_active_rounded,
                size: 44,
                color: scheme.onPrimaryContainer,
              ),
            ),
            const SizedBox(height: 20),
            Text(
              'Пока нет уведомлений',
              textAlign: TextAlign.center,
              style: textTheme.titleLarge?.copyWith(fontWeight: FontWeight.w700),
            ),
            const SizedBox(height: 8),
            Text(
              'Выберите бумагу и цену — мы напишем, когда она вырастет или упадёт '
              'до нужного уровня',
              textAlign: TextAlign.center,
              style: textTheme.bodyMedium?.copyWith(color: scheme.onSurfaceVariant),
            ),
            const SizedBox(height: 28),
            GradientAddButton(onPressed: onCreate, size: 72),
            const SizedBox(height: 10),
            Text(
              'Создать уведомление',
              style: textTheme.labelLarge?.copyWith(color: scheme.onSurfaceVariant),
            ),
          ],
        ),
      ),
    );
  }
}
