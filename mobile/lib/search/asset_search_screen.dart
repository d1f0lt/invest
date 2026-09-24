import 'package:flutter/material.dart';

import '../api/api_client.dart';
import '../api/securities_api.dart';
import 'securities_store.dart';

/// Экран «Поиск активов»: пока ничего не введено — пусто, дальше —
/// бумаги, найденные по тикеру, названию или ISIN.
class AssetSearchScreen extends StatefulWidget {
  const AssetSearchScreen({super.key});

  @override
  State<AssetSearchScreen> createState() => _AssetSearchScreenState();
}

class _AssetSearchScreenState extends State<AssetSearchScreen> {
  final _store = SecuritiesStore.instance;
  final _search = TextEditingController();
  String _query = '';

  List<Security>? _all;
  String? _error;

  @override
  void initState() {
    super.initState();
    // Справочник грузим сразу, пока пользователь набирает запрос.
    _load();
  }

  @override
  void dispose() {
    _search.dispose();
    super.dispose();
  }

  Future<void> _load() async {
    if (_error != null) setState(() => _error = null);
    try {
      final items = await _store.load();
      if (mounted) setState(() => _all = items);
    } on ApiException catch (e) {
      if (mounted) setState(() => _error = e.message);
    } catch (_) {
      if (mounted) setState(() => _error = 'Не удалось загрузить список бумаг');
    }
  }

  void _open(Security s) {
    // TODO: экран бумаги (цена, график по /prices/{secid}/candles).
    ScaffoldMessenger.of(context)
      ..hideCurrentSnackBar()
      ..showSnackBar(SnackBar(content: Text('${s.title} — карточка бумаги скоро')));
  }

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    return Scaffold(
      appBar: AppBar(title: const Text('Поиск активов')),
      body: Column(
        children: [
          Padding(
            padding: const EdgeInsets.fromLTRB(16, 12, 16, 8),
            child: TextField(
              controller: _search,
              autofocus: true,
              onChanged: (v) => setState(() => _query = v.trim()),
              textInputAction: TextInputAction.search,
              autocorrect: false,
              decoration: InputDecoration(
                hintText: 'Название или тикер',
                prefixIcon: const Icon(Icons.search_rounded),
                suffixIcon: _query.isEmpty
                    ? null
                    : IconButton(
                        tooltip: 'Очистить',
                        icon: const Icon(Icons.close_rounded),
                        onPressed: () => setState(() {
                          _search.clear();
                          _query = '';
                        }),
                      ),
                filled: true,
                fillColor: scheme.surfaceContainerHighest,
                isDense: true,
                border: OutlineInputBorder(
                  borderRadius: BorderRadius.circular(14),
                  borderSide: BorderSide.none,
                ),
              ),
            ),
          ),
          Expanded(child: _results()),
        ],
      ),
    );
  }

  Widget _results() {
    // Пока ничего не введено — пустой экран.
    if (_query.isEmpty) return const SizedBox.shrink();

    final all = _all;
    if (all == null) {
      if (_error != null) {
        return _Message(
          text: _error!,
          action: OutlinedButton(onPressed: _load, child: const Text('Повторить')),
        );
      }
      return const Padding(
        padding: EdgeInsets.only(top: 48),
        child: Align(alignment: Alignment.topCenter, child: CircularProgressIndicator()),
      );
    }

    final found = SecuritiesStore.search(all, _query);
    if (found.isEmpty) return const _Message(text: 'Ничего не найдено');

    return ListView.builder(
      keyboardDismissBehavior: ScrollViewKeyboardDismissBehavior.onDrag,
      padding: const EdgeInsets.only(bottom: 24),
      itemCount: found.length,
      itemBuilder: (context, i) => _SecurityTile(security: found[i], onTap: () => _open(found[i])),
    );
  }
}

class _Message extends StatelessWidget {
  const _Message({required this.text, this.action});

  final String text;
  final Widget? action;

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.fromLTRB(32, 48, 32, 0),
      child: Column(
        children: [
          Text(
            text,
            textAlign: TextAlign.center,
            style: TextStyle(color: Theme.of(context).colorScheme.onSurfaceVariant),
          ),
          if (action != null) ...[const SizedBox(height: 12), action!],
        ],
      ),
    );
  }
}

/// Строка результата: значок с тикером, название, «тикер · тип», цена справа.
class _SecurityTile extends StatelessWidget {
  const _SecurityTile({required this.security, required this.onTap});

  final Security security;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    final textTheme = Theme.of(context).textTheme;
    final s = security;
    final price = s.lastPrice;
    return ListTile(
      onTap: onTap,
      contentPadding: const EdgeInsets.symmetric(horizontal: 16, vertical: 2),
      leading: _TickerBadge(secid: s.secid),
      title: Text(s.title, maxLines: 1, overflow: TextOverflow.ellipsis),
      subtitle: Text(
        '${s.secid} · ${s.kind}',
        maxLines: 1,
        overflow: TextOverflow.ellipsis,
        style: textTheme.bodySmall?.copyWith(color: scheme.onSurfaceVariant),
      ),
      trailing: price == null
          ? null
          : Text(
              _formatPrice(price, bond: s.isBond, currency: s.currency),
              style: textTheme.titleSmall?.copyWith(fontWeight: FontWeight.w600),
            ),
    );
  }
}

class _TickerBadge extends StatelessWidget {
  const _TickerBadge({required this.secid});

  final String secid;

  static const _palette = [
    Color(0xFF5B2A86),
    Color(0xFF0E8C8C),
    Color(0xFF1A1F71),
    Color(0xFFB5487A),
    Color(0xFF2F6FDB),
    Color(0xFFD9822B),
  ];

  @override
  Widget build(BuildContext context) {
    final color = _palette[secid.codeUnits.fold(0, (a, c) => a + c) % _palette.length];
    final letters = secid.length > 2 ? secid.substring(0, 2) : secid;
    return CircleAvatar(
      radius: 22,
      backgroundColor: color,
      child: Text(
        letters,
        style: const TextStyle(color: Colors.white, fontWeight: FontWeight.w700, fontSize: 14),
      ),
    );
  }
}

const _nbsp = ' ';

/// Облигации — в % от номинала (`98,53%`), остальное — в валюте:
/// `1 234,50 ₽`; дешёвые бумаги — с нужной точностью (`0,02345 ₽`).
String _formatPrice(double value, {required bool bond, String? currency}) {
  final decimals = bond || value >= 1 ? 2 : 6;
  var text = value.toStringAsFixed(decimals);
  if (decimals > 2) text = text.replaceFirst(RegExp(r'0+$'), '');
  final parts = text.split('.');
  final digits = parts[0];
  final grouped = StringBuffer();
  for (var i = 0; i < digits.length; i++) {
    if (i > 0 && (digits.length - i) % 3 == 0) grouped.write(_nbsp);
    grouped.write(digits[i]);
  }
  final number = parts.length > 1 && parts[1].isNotEmpty ? '$grouped,${parts[1]}' : '$grouped';
  if (bond) return '$number%';
  final symbol = switch (currency) {
    null || 'SUR' || 'RUB' => '₽',
    'USD' => r'$',
    'EUR' => '€',
    'CNY' => '¥',
    _ => currency,
  };
  return '$number$_nbsp$symbol';
}
