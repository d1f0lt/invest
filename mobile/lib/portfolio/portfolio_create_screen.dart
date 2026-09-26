import 'package:flutter/material.dart';

import '../api/api_client.dart';
import 'portfolio_picker.dart';
import 'portfolio_store.dart';

/// Открывает экран создания портфеля.
Future<void> openPortfolioCreate(BuildContext context) {
  return Navigator.of(context).push(
    MaterialPageRoute<void>(builder: (_) => const PortfolioCreateScreen()),
  );
}

/// «Новый портфель»: название и переключатель «Составной портфель». Во включённом
/// состоянии ниже — все обычные портфели пользователя с переключателями; нужно
/// выбрать хотя бы [minMembers], и новый портфель будет собран из них.
class PortfolioCreateScreen extends StatefulWidget {
  const PortfolioCreateScreen({super.key});

  /// Совпадает с проверкой на бэкенде (portfolio: minCompositeMembers).
  static const minMembers = 2;

  @override
  State<PortfolioCreateScreen> createState() => _PortfolioCreateScreenState();
}

class _PortfolioCreateScreenState extends State<PortfolioCreateScreen> {
  /// Совпадает с ограничением на бэкенде (portfolio: maxPortfolioNameLen).
  static const _maxLength = 100;

  final _store = PortfolioStore.instance;
  final _name = TextEditingController();

  bool _composite = false;

  /// Выбранные портфели в порядке выбора — в нём же их покажет составной.
  final List<String> _selected = [];

  bool _saving = false;
  String? _error;

  @override
  void initState() {
    super.initState();
    if (!_store.loaded && !_store.loading) _store.load();
  }

  @override
  void dispose() {
    _name.dispose();
    super.dispose();
  }

  bool get _canCreate {
    if (_saving || _name.text.trim().isEmpty) return false;
    return !_composite || _selected.length >= PortfolioCreateScreen.minMembers;
  }

  void _toggleMember(String id, bool on) {
    setState(() {
      _error = null;
      _selected.remove(id);
      if (on) _selected.add(id);
    });
  }

  Future<void> _create() async {
    if (!_canCreate) return;
    FocusScope.of(context).unfocus();
    setState(() {
      _saving = true;
      _error = null;
    });
    try {
      final available = {for (final p in _store.portfolios) p.id};
      await _store.create(
        _name.text,
        memberIds: _composite ? _selected.where(available.contains).toList() : const [],
      );
      if (mounted) Navigator.of(context).pop();
    } on ApiException catch (e) {
      if (mounted) setState(() => _error = e.message);
    } finally {
      if (mounted) setState(() => _saving = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: const Text('Новый портфель')),
      body: SafeArea(
        child: Column(
          children: [
            Expanded(
              child: ListenableBuilder(
                listenable: _store,
                builder: (context, _) => _form(context),
              ),
            ),
            Padding(
              padding: const EdgeInsets.fromLTRB(16, 8, 16, 16),
              child: SizedBox(
                width: double.infinity,
                height: 52,
                child: FilledButton(
                  onPressed: _canCreate ? _create : null,
                  style: FilledButton.styleFrom(
                    shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(14)),
                  ),
                  child: _saving
                      ? const SizedBox.square(
                          dimension: 22,
                          child: CircularProgressIndicator(strokeWidth: 2.5),
                        )
                      : const Text('Создать'),
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }

  Widget _form(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    final textTheme = Theme.of(context).textTheme;
    return ListView(
      padding: const EdgeInsets.fromLTRB(16, 8, 16, 8),
      children: [
        TextField(
          controller: _name,
          autofocus: true,
          maxLength: _maxLength,
          textCapitalization: TextCapitalization.sentences,
          textInputAction: TextInputAction.done,
          onChanged: (_) => setState(() => _error = null),
          onSubmitted: (_) => _create(),
          decoration: InputDecoration(
            labelText: 'Название',
            hintText: _composite ? 'Например, «Все счета»' : 'Например, «ИИС»',
            border: OutlineInputBorder(borderRadius: BorderRadius.circular(14)),
          ),
        ),
        const SizedBox(height: 4),
        Card(
          margin: EdgeInsets.zero,
          elevation: 0,
          color: scheme.surfaceContainerLow,
          shape: RoundedRectangleBorder(
            borderRadius: BorderRadius.circular(14),
            side: BorderSide(color: scheme.outlineVariant),
          ),
          clipBehavior: Clip.antiAlias,
          child: SwitchListTile(
            value: _composite,
            onChanged: _saving
                ? null
                : (v) => setState(() {
                      _composite = v;
                      _error = null;
                    }),
            secondary: const PortfolioAvatar(composite: true),
            title: const Text('Составной портфель'),
            subtitle: const Text('Объединяет несколько ваших портфелей в один'),
          ),
        ),
        if (_composite) ..._members(context),
        if (_error != null) ...[
          const SizedBox(height: 16),
          Text(
            _error!,
            textAlign: TextAlign.center,
            style: textTheme.bodyMedium?.copyWith(color: scheme.error),
          ),
        ],
      ],
    );
  }

  List<Widget> _members(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    final textTheme = Theme.of(context).textTheme;
    final candidates = _store.portfolios.where((p) => !p.isComposite).toList();
    const min = PortfolioCreateScreen.minMembers;

    final Widget body;
    if (candidates.isEmpty && _store.loading) {
      body = const Padding(
        padding: EdgeInsets.all(24),
        child: Center(child: CircularProgressIndicator()),
      );
    } else if (candidates.length < min) {
      body = Padding(
        padding: const EdgeInsets.symmetric(vertical: 8),
        child: Text(
          'Чтобы собрать составной портфель, нужно хотя бы $min обычных. '
          'Сначала создайте ещё один портфель и загрузите в него отчёт',
          style: textTheme.bodyMedium?.copyWith(color: scheme.onSurfaceVariant),
        ),
      );
    } else {
      body = Card(
        margin: EdgeInsets.zero,
        elevation: 0,
        color: scheme.surfaceContainerLow,
        shape: RoundedRectangleBorder(
          borderRadius: BorderRadius.circular(14),
          side: BorderSide(color: scheme.outlineVariant),
        ),
        clipBehavior: Clip.antiAlias,
        child: Column(
          children: [
            for (final (i, p) in candidates.indexed) ...[
              if (i > 0) Divider(height: 1, indent: 72, color: scheme.outlineVariant),
              SwitchListTile(
                value: _selected.contains(p.id),
                onChanged: _saving ? null : (v) => _toggleMember(p.id, v),
                secondary: const PortfolioAvatar(),
                title: Text(p.displayName, maxLines: 1, overflow: TextOverflow.ellipsis),
              ),
            ],
          ],
        ),
      );
    }

    final count = _selected.length;
    return [
      const SizedBox(height: 20),
      Padding(
        padding: const EdgeInsets.fromLTRB(4, 0, 4, 8),
        child: Row(
          children: [
            Expanded(
              child: Text(
                'Портфели',
                style: textTheme.titleSmall?.copyWith(fontWeight: FontWeight.w700),
              ),
            ),
            if (candidates.length >= min)
              Text(
                count < min ? 'Выберите хотя бы $min' : 'Выбрано: $count',
                style: textTheme.bodySmall?.copyWith(
                  color: count < min ? scheme.onSurfaceVariant : scheme.primary,
                ),
              ),
          ],
        ),
      ),
      body,
    ];
  }
}
