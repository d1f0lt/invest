import 'package:flutter/material.dart';

import '../api/api_client.dart';
import '../api/portfolio_api.dart';
import '../api/reports_api.dart';
import '../portfolio/portfolio_store.dart';
import 'broker_avatar.dart';
import 'report_upload_screen.dart';

/// Открывает загрузку отчёта в текущий портфель: сначала выбор брокера,
/// затем выбор файла и ожидание разбора.
Future<void> openReportUpload(BuildContext context) async {
  final portfolio = PortfolioStore.instance.current;
  if (portfolio == null) {
    ScaffoldMessenger.of(context)
      ..hideCurrentSnackBar()
      ..showSnackBar(const SnackBar(content: Text('Портфель ещё загружается')));
    return;
  }
  await Navigator.of(context, rootNavigator: true).push(
    MaterialPageRoute<void>(builder: (_) => BrokerSelectScreen(portfolio: portfolio)),
  );
}

/// «Выберите брокера» — список поддерживаемых брокеров с бэкенда.
class BrokerSelectScreen extends StatefulWidget {
  const BrokerSelectScreen({super.key, required this.portfolio});

  final Portfolio portfolio;

  @override
  State<BrokerSelectScreen> createState() => _BrokerSelectScreenState();
}

class _BrokerSelectScreenState extends State<BrokerSelectScreen> {
  final _api = ReportsApi();

  List<Broker>? _brokers;
  String? _error;
  bool _loading = false;

  @override
  void initState() {
    super.initState();
    _load();
  }

  Future<void> _load() async {
    setState(() {
      _loading = true;
      _error = null;
    });
    try {
      final brokers = await _api.brokers();
      if (mounted) setState(() => _brokers = brokers);
    } on ApiException catch (e) {
      if (mounted) setState(() => _error = e.message);
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  void _open(Broker broker) {
    Navigator.of(context).push(
      MaterialPageRoute<void>(
        builder: (_) => ReportUploadScreen(broker: broker, portfolio: widget.portfolio),
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: const Text('Выберите брокера')),
      body: RefreshIndicator(onRefresh: _load, child: _body(context)),
    );
  }

  Widget _body(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    final textTheme = Theme.of(context).textTheme;
    final brokers = _brokers;

    if (brokers == null) {
      if (_loading) return const Center(child: CircularProgressIndicator());
      return _Message(
        icon: Icons.cloud_off_rounded,
        text: _error ?? 'Не удалось загрузить список брокеров',
        action: FilledButton.tonal(onPressed: _load, child: const Text('Повторить')),
      );
    }
    if (brokers.isEmpty) {
      return const _Message(
        icon: Icons.account_balance_outlined,
        text: 'Пока нет брокеров, отчёты которых можно загрузить',
      );
    }

    return ListView(
      physics: const AlwaysScrollableScrollPhysics(),
      padding: const EdgeInsets.fromLTRB(16, 8, 16, 24),
      children: [
        Padding(
          padding: const EdgeInsets.fromLTRB(4, 0, 4, 12),
          child: Text(
            'Отчёт будет добавлен в портфель «${widget.portfolio.displayName}»',
            style: textTheme.bodyMedium?.copyWith(color: scheme.onSurfaceVariant),
          ),
        ),
        for (final broker in brokers) ...[
          Card(
            margin: EdgeInsets.zero,
            elevation: 0,
            color: scheme.surfaceContainerLow,
            clipBehavior: Clip.antiAlias,
            shape: RoundedRectangleBorder(
              borderRadius: BorderRadius.circular(18),
              side: BorderSide(color: scheme.outlineVariant),
            ),
            child: ListTile(
              contentPadding: const EdgeInsets.symmetric(horizontal: 16, vertical: 8),
              leading: BrokerAvatar(broker: broker),
              title: Text(broker.name, style: const TextStyle(fontWeight: FontWeight.w600)),
              subtitle: Text('Отчёт в формате ${broker.formatsLabel}'),
              trailing: const Icon(Icons.chevron_right_rounded),
              onTap: () => _open(broker),
            ),
          ),
          const SizedBox(height: 10),
        ],
      ],
    );
  }
}

class _Message extends StatelessWidget {
  const _Message({required this.icon, required this.text, this.action});

  final IconData icon;
  final String text;
  final Widget? action;

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    // ListView — чтобы работал pull-to-refresh.
    return ListView(
      physics: const AlwaysScrollableScrollPhysics(),
      padding: const EdgeInsets.fromLTRB(32, 96, 32, 32),
      children: [
        Icon(icon, size: 48, color: scheme.onSurfaceVariant),
        const SizedBox(height: 16),
        Text(text, textAlign: TextAlign.center),
        if (action != null) ...[
          const SizedBox(height: 16),
          Center(child: action),
        ],
      ],
    );
  }
}
