import 'package:flutter/material.dart';

import '../api/api_client.dart';
import '../api/securities_api.dart';
import '../search/asset_search_screen.dart';
import '../securities/asset_screen.dart';
import '../securities/security_widgets.dart';
import 'favorites_store.dart';

/// Экран «Избранное»: бумаги из [FavoritesStore] (хранятся на устройстве)
/// со свежими ценами и изменением за день. Свайп влево — убрать.
class FavoritesScreen extends StatefulWidget {
  const FavoritesScreen({super.key});

  @override
  State<FavoritesScreen> createState() => _FavoritesScreenState();
}

class _FavoritesScreenState extends State<FavoritesScreen> {
  final _api = SecuritiesApi();
  final _store = FavoritesStore.instance;

  /// Свежие цены по ключу бумаги (`secid@board`).
  Map<String, Security> _prices = const {};
  bool _loading = false;
  String? _error;

  @override
  void initState() {
    super.initState();
    _store.addListener(_onStoreChanged);
    _loadPrices();
  }

  @override
  void dispose() {
    _store.removeListener(_onStoreChanged);
    super.dispose();
  }

  void _onStoreChanged() {
    // Добавили бумагу (например, из карточки) — подтянем её цену.
    if (_store.items.any((s) => !_prices.containsKey(s.key))) _loadPrices();
  }

  Future<void> _loadPrices() async {
    final items = _store.items;
    if (items.isEmpty) return;
    setState(() {
      _loading = true;
      _error = null;
    });
    try {
      final found = await _api.prices(items.map((s) => s.secid));
      if (!mounted) return;
      setState(() {
        _prices = {for (final s in found) s.key: s};
        _loading = false;
      });
    } on ApiException catch (e) {
      if (!mounted) return;
      setState(() {
        _error = e.message;
        _loading = false;
      });
    } catch (_) {
      if (!mounted) return;
      setState(() {
        _error = 'Не удалось обновить цены';
        _loading = false;
      });
    }
  }

  Security _withPrice(Security s) {
    final fresh = _prices[s.key];
    return fresh == null ? s : s.withPricesFrom(fresh);
  }

  void _open(Security s) {
    Navigator.of(context).push(
      MaterialPageRoute<void>(builder: (_) => AssetScreen(security: s)),
    );
  }

  Future<void> _remove(Security s) async {
    final index = _store.items.indexWhere((e) => e.key == s.key);
    await _store.remove(s);
    if (!mounted) return;
    ScaffoldMessenger.of(context)
      ..hideCurrentSnackBar()
      ..showSnackBar(SnackBar(
        content: Text('${s.title} — удалено из избранного'),
        action: SnackBarAction(label: 'Вернуть', onPressed: () => _store.insertAt(index, s)),
      ));
  }

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    return Scaffold(
      appBar: AppBar(title: const Text('Избранное')),
      body: ListenableBuilder(
        listenable: _store,
        builder: (context, _) {
          final items = _store.items;
          if (items.isEmpty) return _Empty(onSearch: _openSearch);
          return RefreshIndicator(
            onRefresh: _loadPrices,
            child: ListView(
              physics: const AlwaysScrollableScrollPhysics(),
              padding: const EdgeInsets.only(top: 8, bottom: 24),
              children: [
                SizedBox(
                  height: 2,
                  child: _loading && _prices.isNotEmpty ? const LinearProgressIndicator() : null,
                ),
                if (_error != null)
                  Padding(
                    padding: const EdgeInsets.fromLTRB(16, 4, 16, 8),
                    child: Text(_error!, style: TextStyle(color: scheme.error)),
                  ),
                for (final s in items)
                  Dismissible(
                    key: ValueKey(s.key),
                    direction: DismissDirection.endToStart,
                    background: Container(
                      color: scheme.errorContainer,
                      alignment: Alignment.centerRight,
                      padding: const EdgeInsets.only(right: 24),
                      child: Icon(Icons.star_outline_rounded, color: scheme.onErrorContainer),
                    ),
                    onDismissed: (_) => _remove(s),
                    child: SecurityTile(security: _withPrice(s), onTap: () => _open(s)),
                  ),
              ],
            ),
          );
        },
      ),
    );
  }

  void _openSearch() {
    Navigator.of(context).push(
      MaterialPageRoute<void>(builder: (_) => const AssetSearchScreen()),
    );
  }
}

class _Empty extends StatelessWidget {
  const _Empty({required this.onSearch});

  final VoidCallback onSearch;

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    final textTheme = Theme.of(context).textTheme;
    return Padding(
      padding: const EdgeInsets.fromLTRB(32, 72, 32, 16),
      child: Column(
        children: [
          Container(
            width: 88,
            height: 88,
            decoration: BoxDecoration(shape: BoxShape.circle, color: scheme.primaryContainer),
            child: Icon(Icons.star_rounded, size: 44, color: scheme.onPrimaryContainer),
          ),
          const SizedBox(height: 20),
          Text(
            'В избранном пока пусто',
            textAlign: TextAlign.center,
            style: textTheme.titleLarge?.copyWith(fontWeight: FontWeight.w700),
          ),
          const SizedBox(height: 8),
          Text(
            'Откройте карточку актива и нажмите на звезду — '
            'бумага появится здесь',
            textAlign: TextAlign.center,
            style: textTheme.bodyMedium?.copyWith(color: scheme.onSurfaceVariant),
          ),
          const SizedBox(height: 24),
          FilledButton.icon(
            onPressed: onSearch,
            icon: const Icon(Icons.search_rounded),
            label: const Text('Найти актив'),
          ),
        ],
      ),
    );
  }
}
