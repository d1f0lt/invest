import 'dart:async';

import 'package:flutter/material.dart';

import '../api/api_client.dart';
import '../api/securities_api.dart';
import '../securities/asset_screen.dart';
import '../securities/security_widgets.dart';

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
    Navigator.of(context).push(
      MaterialPageRoute<void>(builder: (_) => AssetScreen(security: s)),
    );
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
          SecurityTile(security: _results[i], onTap: () => _open(_results[i])),
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
