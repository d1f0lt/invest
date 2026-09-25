import 'dart:async';

import 'package:flutter/material.dart';

import '../api/api_client.dart';
import '../api/securities_api.dart';

/// Экран «Поиск активов»: пока ничего не введено — пусто, дальше —
/// бумаги, найденные сервером (securities_reader) по тикеру, названию или ISIN.
class AssetSearchScreen extends StatefulWidget {
  const AssetSearchScreen({super.key});

  @override
  State<AssetSearchScreen> createState() => _AssetSearchScreenState();
}

class _AssetSearchScreenState extends State<AssetSearchScreen> {
  /// Пауза после последнего нажатия, прежде чем идти на сервер.
  static const _debounce = Duration(milliseconds: 300);

  final _api = SecuritiesApi();
  final _search = TextEditingController();
  Timer? _timer;

  String _query = '';
  /// Запрос, для которого показаны [_results] (могут быть от прошлого ввода,
  /// пока грузится новый).
  String? _shownQuery;
  List<Security> _results = const [];
  bool _loading = false;
  String? _error;
  /// Номер последнего запроса: ответы на устаревшие игнорируются.
  int _seq = 0;

  @override
  void dispose() {
    _timer?.cancel();
    _search.dispose();
    super.dispose();
  }

  void _onChanged(String value) {
    final q = value.trim();
    if (q == _query) return;
    _timer?.cancel();
    setState(() {
      _query = q;
      _error = null;
      if (q.isEmpty) {
        _seq++; // отменяем ответ на ещё идущий запрос
        _loading = false;
        _results = const [];
        _shownQuery = null;
      } else {
        _loading = true;
      }
    });
    if (q.isNotEmpty) _timer = Timer(_debounce, _run);
  }

  Future<void> _run() async {
    final q = _query;
    if (q.isEmpty) return;
    final seq = ++_seq;
    setState(() {
      _loading = true;
      _error = null;
    });
    try {
      final found = await _api.search(q);
      if (!mounted || seq != _seq) return;
      setState(() {
        _results = found;
        _shownQuery = q;
        _loading = false;
      });
    } on ApiException catch (e) {
      if (!mounted || seq != _seq) return;
      setState(() {
        _error = e.message;
        _loading = false;
      });
    } catch (_) {
      if (!mounted || seq != _seq) return;
      setState(() {
        _error = 'Не удалось выполнить поиск';
        _loading = false;
      });
    }
  }

  void _clear() {
    _search.clear();
    _onChanged('');
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
              onChanged: _onChanged,
              onSubmitted: (_) {
                // Enter — искать сразу, не дожидаясь паузы.
                if (_query.isNotEmpty && (_timer?.isActive ?? false)) {
                  _timer!.cancel();
                  _run();
                }
              },
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
                        onPressed: _clear,
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
          // Тонкая полоска загрузки, пока поверх прошлых результатов ищутся новые.
          SizedBox(
            height: 2,
            child: _loading && _shownQuery != null ? const LinearProgressIndicator() : null,
          ),
          Expanded(child: _body()),
        ],
      ),
    );
  }

  Widget _body() {
    // Пока ничего не введено — пустой экран.
    if (_query.isEmpty) return const SizedBox.shrink();

    if (_error != null) {
      return _Message(
        text: _error!,
        action: OutlinedButton(onPressed: _run, child: const Text('Повторить')),
      );
    }
    if (_shownQuery == null) {
      // Первый поиск — результатов ещё нет.
      return const Padding(
        padding: EdgeInsets.only(top: 48),
        child: Align(alignment: Alignment.topCenter, child: CircularProgressIndicator()),
      );
    }
    if (_results.isEmpty) {
      return _loading ? const SizedBox.shrink() : const _Message(text: 'Ничего не найдено');
    }

    return ListView.builder(
      keyboardDismissBehavior: ScrollViewKeyboardDismissBehavior.onDrag,
      padding: const EdgeInsets.only(bottom: 24),
      itemCount: _results.length,
      itemBuilder: (context, i) =>
          _SecurityTile(security: _results[i], onTap: () => _open(_results[i])),
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
