import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import '../api/api_client.dart';
import '../api/securities_api.dart';
import '../search/asset_search_screen.dart';
import '../securities/security_widgets.dart';
import 'alert_format.dart';
import 'alerts_store.dart';
import 'price_alert.dart';

/// Создание и редактирование уведомления: бумага → вырастет/упадёт →
/// на сколько (в деньгах или в %) → «Сохранить».
class AlertEditScreen extends StatefulWidget {
  const AlertEditScreen({super.key, this.alert});

  /// null — новое уведомление.
  final PriceAlert? alert;

  @override
  State<AlertEditScreen> createState() => _AlertEditScreenState();
}

class _AlertEditScreenState extends State<AlertEditScreen> {
  static const _quickPercents = [1.0, 3.0, 5.0, 10.0, 20.0];
  static const _maxPercent = 1000.0;

  final _api = SecuritiesApi();
  final _value = TextEditingController();
  final _valueFocus = FocusNode();

  Security? _security;
  bool _priceLoading = false;
  String? _priceError;
  AlertDirection _direction = AlertDirection.up;
  AlertMode _mode = AlertMode.percent;
  bool _saving = false;
  int _priceSeq = 0;

  bool get _editing => widget.alert != null;

  @override
  void initState() {
    super.initState();
    final alert = widget.alert;
    if (alert != null) {
      _security = AlertsStore.instance.priceOf(alert) ?? alert.security;
      _direction = alert.direction;
      _mode = alert.mode;
      _value.text = alertNumberText(alert.value);
      _loadPrice();
    }
  }

  @override
  void dispose() {
    _value.dispose();
    _valueFocus.dispose();
    super.dispose();
  }

  Future<void> _loadPrice() async {
    final s = _security;
    if (s == null) return;
    final seq = ++_priceSeq;
    setState(() {
      _priceLoading = true;
      _priceError = null;
    });
    try {
      final fresh = await _api.price(s.secid, s.board);
      if (!mounted || seq != _priceSeq) return;
      setState(() {
        if (fresh != null) _security = s.withPricesFrom(fresh);
        if (_security?.lastPrice == null) _priceError = 'Нет текущей цены';
      });
    } on ApiException catch (e) {
      if (mounted && seq == _priceSeq) setState(() => _priceError = e.message);
    } catch (_) {
      if (mounted && seq == _priceSeq) setState(() => _priceError = 'Не удалось получить цену');
    } finally {
      if (mounted && seq == _priceSeq) setState(() => _priceLoading = false);
    }
  }

  Future<void> _pickSecurity() async {
    final picked = await Navigator.of(context).push<Security>(
      MaterialPageRoute(builder: (_) => const AssetSearchScreen(pick: true)),
    );
    if (picked == null || !mounted) return;
    setState(() {
      _security = picked;
      _priceError = null;
    });
    if (picked.lastPrice == null) {
      _loadPrice();
    } else if (_value.text.isEmpty) {
      _valueFocus.requestFocus();
    }
  }

  double? get _basePrice {
    final price = _security?.lastPrice;
    if (price != null) return price;
    final alert = widget.alert;
    if (alert != null && _security?.key == alert.security.key) return alert.basePrice;
    return null;
  }

  double? get _parsed => parseAlertNumber(_value.text);

  String? get _valueError {
    if (_value.text.trim().isEmpty) return null;
    final v = _parsed;
    if (v == null) return 'Введите число';
    if (v <= 0) return 'Должно быть больше нуля';
    if (_mode == AlertMode.percent) {
      if (_direction == AlertDirection.down && v >= 100) {
        return 'Цена не может упасть на 100% и больше';
      }
      if (v > _maxPercent) return 'Не больше ${_maxPercent.toInt()}%';
    } else {
      final base = _basePrice;
      if (_direction == AlertDirection.down && base != null && v >= base) {
        return 'Больше текущей цены';
      }
    }
    return null;
  }

  double? get _target {
    final base = _basePrice, v = _parsed;
    if (base == null || v == null || v <= 0 || _valueError != null) return null;
    return PriceAlert.targetFor(basePrice: base, direction: _direction, mode: _mode, value: v);
  }

  bool get _canSave => !_saving && _security != null && _target != null;

  Future<void> _save() async {
    if (!_canSave) return;
    FocusScope.of(context).unfocus();
    setState(() => _saving = true);
    final store = AlertsStore.instance;
    try {
      if (_editing) {
        await store.update(
          widget.alert!.id,
          security: _security!,
          direction: _direction,
          mode: _mode,
          value: _parsed!,
          basePrice: _basePrice!,
        );
      } else {
        await store.create(
          security: _security!,
          direction: _direction,
          mode: _mode,
          value: _parsed!,
          basePrice: _basePrice!,
        );
      }
      if (mounted) Navigator.of(context).pop(true);
    } on ApiException catch (e) {
      _showError(e.message);
    } catch (_) {
      _showError('Не удалось сохранить уведомление');
    } finally {
      if (mounted) setState(() => _saving = false);
    }
  }

  void _showError(String text) {
    if (!mounted) return;
    ScaffoldMessenger.of(context)
      ..hideCurrentSnackBar()
      ..showSnackBar(SnackBar(content: Text(text)));
  }

  Future<void> _delete() async {
    final ok = await confirmAlertDelete(context);
    if (ok != true || !mounted) return;
    try {
      await AlertsStore.instance.remove(widget.alert!.id);
    } on ApiException catch (e) {
      _showError('Не удалось удалить: ${e.message}');
      return;
    } catch (_) {
      _showError('Не удалось удалить уведомление');
      return;
    }
    if (mounted) Navigator.of(context).pop(false);
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(
        title: Text(_editing ? 'Редактирование' : 'Новое уведомление'),
        actions: [
          if (_editing)
            IconButton(
              tooltip: 'Удалить',
              icon: const Icon(Icons.delete_outline_rounded),
              onPressed: _delete,
            ),
        ],
      ),
      body: SafeArea(
        child: Column(
          children: [
            Expanded(
              child: ListView(
                keyboardDismissBehavior: ScrollViewKeyboardDismissBehavior.onDrag,
                padding: const EdgeInsets.fromLTRB(16, 16, 16, 16),
                children: [
                  if (widget.alert?.triggered ?? false) ...[
                    _TriggeredBanner(alert: widget.alert!),
                    const SizedBox(height: 16),
                  ],
                  const _Label('Бумага'),
                  _SecurityField(
                    security: _security,
                    loading: _priceLoading,
                    error: _priceError,
                    onTap: _pickSecurity,
                    onRetry: _loadPrice,
                  ),
                  const SizedBox(height: 24),
                  const _Label('Цена'),
                  Row(
                    children: [
                      for (final d in AlertDirection.values) ...[
                        if (d != AlertDirection.values.first) const SizedBox(width: 12),
                        Expanded(
                          child: _DirectionCard(
                            direction: d,
                            selected: d == _direction,
                            onTap: () => setState(() => _direction = d),
                          ),
                        ),
                      ],
                    ],
                  ),
                  const SizedBox(height: 24),
                  const _Label('На сколько'),
                  _valueInput(context),
                  if (_mode == AlertMode.percent) ...[
                    const SizedBox(height: 12),
                    Wrap(
                      spacing: 8,
                      runSpacing: 8,
                      children: [
                        for (final p in _quickPercents)
                          ActionChip(
                            label: Text('${alertNumberText(p)}%'),
                            onPressed: () => setState(() => _value.text = alertNumberText(p)),
                          ),
                      ],
                    ),
                  ],
                  const SizedBox(height: 24),
                  _Summary(
                    security: _security,
                    direction: _direction,
                    basePrice: _basePrice,
                    target: _target,
                  ),
                ],
              ),
            ),
            Padding(
              padding: const EdgeInsets.fromLTRB(16, 8, 16, 16),
              child: SizedBox(
                width: double.infinity,
                height: 54,
                child: FilledButton(
                  onPressed: _canSave ? _save : null,
                  style: FilledButton.styleFrom(
                    shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(16)),
                  ),
                  child: _saving
                      ? const SizedBox.square(
                          dimension: 22,
                          child: CircularProgressIndicator(strokeWidth: 2.5),
                        )
                      : const Text('Сохранить'),
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }

  Widget _valueInput(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    final s = _security;
    final unit = s == null ? '₽' : alertAmountUnit(s);
    return Row(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Expanded(
          child: TextField(
            controller: _value,
            focusNode: _valueFocus,
            keyboardType: const TextInputType.numberWithOptions(decimal: true),
            inputFormatters: [FilteringTextInputFormatter.allow(RegExp(r'[0-9.,]'))],
            textInputAction: TextInputAction.done,
            onChanged: (_) => setState(() {}),
            onSubmitted: (_) => _save(),
            style: Theme.of(context).textTheme.titleMedium,
            decoration: InputDecoration(
              hintText: _mode == AlertMode.percent ? 'Например, 5' : 'Например, 100',
              prefixText: _direction == AlertDirection.up ? '+ ' : '− ',
              suffixText: _mode == AlertMode.percent ? '%' : unit,
              errorText: _valueError,
              errorMaxLines: 2,
              filled: true,
              fillColor: scheme.surfaceContainerHighest,
              border: OutlineInputBorder(
                borderRadius: BorderRadius.circular(14),
                borderSide: BorderSide.none,
              ),
            ),
          ),
        ),
        const SizedBox(width: 12),
        SizedBox(
          height: 56,
          child: SegmentedButton<AlertMode>(
            showSelectedIcon: false,
            style: SegmentedButton.styleFrom(
              shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(14)),
            ),
            segments: [
              ButtonSegment(value: AlertMode.amount, label: Text(unit)),
              const ButtonSegment(value: AlertMode.percent, label: Text('%')),
            ],
            selected: {_mode},
            onSelectionChanged: (v) => setState(() => _mode = v.first),
          ),
        ),
      ],
    );
  }
}

Future<bool?> confirmAlertDelete(BuildContext context) => showDialog<bool>(
      context: context,
      builder: (context) => AlertDialog(
        title: const Text('Удалить уведомление?'),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(context).pop(false),
            child: const Text('Отмена'),
          ),
          TextButton(
            onPressed: () => Navigator.of(context).pop(true),
            style: TextButton.styleFrom(foregroundColor: Theme.of(context).colorScheme.error),
            child: const Text('Удалить'),
          ),
        ],
      ),
    );

class _Label extends StatelessWidget {
  const _Label(this.text);

  final String text;

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.only(left: 4, bottom: 8),
      child: Text(
        text,
        style: Theme.of(context).textTheme.titleSmall?.copyWith(fontWeight: FontWeight.w700),
      ),
    );
  }
}

class _SecurityField extends StatelessWidget {
  const _SecurityField({
    required this.security,
    required this.loading,
    required this.error,
    required this.onTap,
    required this.onRetry,
  });

  final Security? security;
  final bool loading;
  final String? error;
  final VoidCallback onTap;
  final VoidCallback onRetry;

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    final textTheme = Theme.of(context).textTheme;
    final s = security;
    final price = s?.lastPrice;
    return Material(
      color: scheme.surfaceContainerHighest,
      borderRadius: BorderRadius.circular(16),
      clipBehavior: Clip.antiAlias,
      child: InkWell(
        onTap: onTap,
        child: Padding(
          padding: const EdgeInsets.fromLTRB(14, 12, 8, 12),
          child: Row(
            children: [
              if (s == null)
                Container(
                  width: 44,
                  height: 44,
                  decoration: BoxDecoration(shape: BoxShape.circle, color: scheme.primaryContainer),
                  child: Icon(Icons.search_rounded, color: scheme.onPrimaryContainer),
                )
              else
                TickerBadge(secid: s.secid),
              const SizedBox(width: 14),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      s?.title ?? 'Выберите бумагу',
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                      style: textTheme.titleMedium?.copyWith(fontWeight: FontWeight.w600),
                    ),
                    const SizedBox(height: 2),
                    Text(
                      s == null ? 'Акция, фонд или облигация' : '${s.secid} · ${s.kind}',
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                      style: textTheme.bodySmall?.copyWith(color: scheme.onSurfaceVariant),
                    ),
                  ],
                ),
              ),
              if (s != null) ...[
                if (loading && price == null)
                  const SizedBox.square(
                    dimension: 18,
                    child: CircularProgressIndicator(strokeWidth: 2),
                  )
                else if (price != null)
                  Text(
                    alertPrice(s, price),
                    style: textTheme.titleSmall?.copyWith(fontWeight: FontWeight.w600),
                  )
                else if (error != null)
                  IconButton(
                    tooltip: '$error. Повторить',
                    onPressed: onRetry,
                    icon: Icon(Icons.refresh_rounded, color: scheme.error),
                  ),
              ],
              Icon(Icons.chevron_right_rounded, color: scheme.onSurfaceVariant),
            ],
          ),
        ),
      ),
    );
  }
}

class _DirectionCard extends StatelessWidget {
  const _DirectionCard({required this.direction, required this.selected, required this.onTap});

  final AlertDirection direction;
  final bool selected;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    final color = alertDirectionColor(context, direction);
    final up = direction == AlertDirection.up;
    return AnimatedContainer(
      duration: const Duration(milliseconds: 180),
      decoration: BoxDecoration(
        color: selected ? color.withValues(alpha: 0.12) : scheme.surfaceContainerHighest,
        borderRadius: BorderRadius.circular(16),
        border: Border.all(color: selected ? color : Colors.transparent, width: 1.5),
      ),
      child: Material(
        type: MaterialType.transparency,
        borderRadius: BorderRadius.circular(16),
        clipBehavior: Clip.antiAlias,
        child: InkWell(
          onTap: onTap,
          child: Padding(
            padding: const EdgeInsets.symmetric(vertical: 16, horizontal: 12),
            child: Column(
              children: [
                Icon(
                  alertDirectionIcon(direction),
                  size: 30,
                  color: selected ? color : scheme.onSurfaceVariant,
                ),
                const SizedBox(height: 6),
                Text(
                  up ? 'Вырастет' : 'Упадёт',
                  style: Theme.of(context).textTheme.titleSmall?.copyWith(
                        fontWeight: FontWeight.w600,
                        color: selected ? color : scheme.onSurface,
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

class _Summary extends StatelessWidget {
  const _Summary({
    required this.security,
    required this.direction,
    required this.basePrice,
    required this.target,
  });

  final Security? security;
  final AlertDirection direction;
  final double? basePrice;
  final double? target;

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    final textTheme = Theme.of(context).textTheme;
    final s = security, base = basePrice, t = target;
    final ready = s != null && base != null && t != null;
    final color = alertDirectionColor(context, direction);
    return AnimatedContainer(
      duration: const Duration(milliseconds: 200),
      padding: const EdgeInsets.all(16),
      decoration: BoxDecoration(
        borderRadius: BorderRadius.circular(18),
        gradient: LinearGradient(
          begin: Alignment.topLeft,
          end: Alignment.bottomRight,
          colors: [
            alertGradient.first.withValues(alpha: ready ? 0.12 : 0.06),
            alertGradient.last.withValues(alpha: ready ? 0.12 : 0.06),
          ],
        ),
      ),
      child: Row(
        children: [
          Icon(Icons.notifications_active_rounded, color: ready ? color : scheme.onSurfaceVariant),
          const SizedBox(width: 12),
          Expanded(
            child: !ready
                ? Text(
                    'Выберите бумагу и задайте условие — здесь появится цена, '
                    'при которой придёт уведомление',
                    style: textTheme.bodyMedium?.copyWith(color: scheme.onSurfaceVariant),
                  )
                : Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text(
                        direction == AlertDirection.up
                            ? 'Уведомим, когда цена вырастет до'
                            : 'Уведомим, когда цена упадёт до',
                        style: textTheme.bodyMedium?.copyWith(color: scheme.onSurfaceVariant),
                      ),
                      const SizedBox(height: 2),
                      Text(
                        alertPrice(s, t),
                        style: textTheme.titleLarge?.copyWith(
                          fontWeight: FontWeight.w700,
                          color: color,
                        ),
                      ),
                      const SizedBox(height: 2),
                      Text(
                        'Сейчас ${alertPrice(s, base)}',
                        style: textTheme.bodySmall?.copyWith(color: scheme.onSurfaceVariant),
                      ),
                    ],
                  ),
          ),
        ],
      ),
    );
  }
}

class _TriggeredBanner extends StatelessWidget {
  const _TriggeredBanner({required this.alert});

  final PriceAlert alert;

  @override
  Widget build(BuildContext context) {
    final textTheme = Theme.of(context).textTheme;
    final price = alert.triggeredPrice;
    return Container(
      padding: const EdgeInsets.all(14),
      decoration: BoxDecoration(
        color: alertUpColor.withValues(alpha: 0.1),
        borderRadius: BorderRadius.circular(16),
      ),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          const Icon(Icons.check_circle_rounded, color: alertUpColor),
          const SizedBox(width: 12),
          Expanded(
            child: Text(
              'Сработало ${alertDateTime(alert.triggeredAt!)}'
              '${price == null ? '' : ' по ${alertPrice(alert.security, price)}'}. '
              'После сохранения уведомление снова начнёт следить за ценой.',
              style: textTheme.bodyMedium,
            ),
          ),
        ],
      ),
    );
  }
}
