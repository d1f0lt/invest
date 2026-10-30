import 'package:flutter/material.dart';

import '../api/api_client.dart';
import '../api/securities_api.dart';
import 'feed_api.dart';
import 'feed_security_picker.dart';

typedef FeedComposeResult = ({FeedKind kind, FeedPost? updated});

class FeedComposeScreen extends StatefulWidget {
  const FeedComposeScreen({super.key, required this.kind, this.post});

  final FeedKind kind;
  final FeedPost? post;

  @override
  State<FeedComposeScreen> createState() => _FeedComposeScreenState();
}

class _FeedComposeScreenState extends State<FeedComposeScreen> {
  final _api = FeedApi();
  final _form = GlobalKey<FormState>();
  final _title = TextEditingController();
  final _body = TextEditingController();
  final _percent = TextEditingController();
  final _securitiesByKind = <FeedKind, List<Security>>{};
  late FeedKind _kind = widget.kind == FeedKind.all
      ? FeedKind.news
      : widget.kind;
  DateTime? _date;
  Recommendation? _recommendation;
  bool _growth = true;
  bool _submitting = false;
  String? _error;

  bool get _editing => widget.post != null;

  @override
  void initState() {
    super.initState();
    final post = widget.post;
    if (post == null) return;
    _title.text = post.title;
    _body.text = post.body;
    _date = post.targetDate;
    _recommendation = post.recommendation;
    _securitiesByKind[post.kind] = [
      for (final ticker in post.tickers)
        Security(secid: ticker, board: post.board),
    ];
    if (post.change case final change?) {
      _growth = change >= 0;
      _percent.text = change.abs().toString();
    }
  }

  bool get _forecast => _kind == FeedKind.forecast;
  List<Security> get _securities => _securitiesByKind[_kind] ?? const [];
  double? get _magnitude =>
      double.tryParse(_percent.text.trim().replaceAll(',', '.'));

  @override
  void dispose() {
    _title.dispose();
    _body.dispose();
    _percent.dispose();
    super.dispose();
  }

  Future<void> _pickDate() async {
    final moscow = DateTime.now().toUtc().add(const Duration(hours: 3));
    final firstDate = DateTime(moscow.year, moscow.month, moscow.day + 1);
    final date = await showDatePicker(
      context: context,
      initialDate: _date != null && !_date!.isBefore(firstDate)
          ? _date
          : firstDate,
      firstDate: firstDate,
      lastDate: DateTime(9999, 12, 31),
    );
    if (date != null && mounted) setState(() => _date = date);
  }

  Future<void> _publish() async {
    if (!_form.currentState!.validate()) return;
    final error = switch ((
      _securities.isEmpty,
      _forecast,
      _date,
      _recommendation,
    )) {
      (true, _, _, _) => 'Выберите бумагу',
      (_, true, null, _) => 'Укажите дату окончания прогноза',
      (_, true, _, null) => 'Выберите рекомендацию',
      _ => null,
    };
    if (error != null) {
      setState(() => _error = error);
      return;
    }
    setState(() {
      _submitting = true;
      _error = null;
    });
    try {
      final updated = await _api.savePost(
        postId: widget.post?.id,
        kind: _kind,
        title: _title.text.trim(),
        body: _body.text.trim(),
        securities: _securities,
        change: _forecast ? _magnitude! * (_growth ? 1 : -1) : null,
        recommendation: _recommendation,
        targetDate: _date,
      );
      if (mounted) Navigator.of(context).pop((kind: _kind, updated: updated));
    } on ApiException catch (e) {
      if (mounted) {
        setState(
          () => _error = e.statusCode == 422
              ? e.serverMessage ?? e.message
              : e.message,
        );
      }
    } catch (_) {
      if (mounted) setState(() => _error = 'Не удалось сохранить публикацию');
    } finally {
      if (mounted) setState(() => _submitting = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    return PopScope(
      canPop: !_submitting,
      child: Scaffold(
        appBar: AppBar(
          title: Text(_editing ? 'Редактирование' : 'Новая публикация'),
        ),
        body: SafeArea(
          child: Form(
            key: _form,
            child: ListView(
              padding: const EdgeInsets.all(20),
              children: [
                Text(
                  'Тип публикации',
                  style: Theme.of(context).textTheme.titleSmall,
                ),
                Wrap(
                  spacing: 8,
                  children: [
                    for (final kind
                        in _editing
                            ? [_kind]
                            : [FeedKind.news, FeedKind.forecast])
                      ChoiceChip(
                        label: Text(kind.postLabel),
                        selected: _kind == kind,
                        onSelected: _submitting || _editing
                            ? null
                            : (_) => setState(() {
                                _kind = kind;
                                _error = null;
                              }),
                      ),
                  ],
                ),
                const SizedBox(height: 12),
                TextFormField(
                  controller: _title,
                  enabled: !_submitting,
                  maxLength: 120,
                  decoration: const InputDecoration(labelText: 'Заголовок'),
                  validator: (value) =>
                      value!.trim().isEmpty ? 'Введите заголовок' : null,
                ),
                const SizedBox(height: 8),
                FeedSecurityPicker(
                  selected: _securities,
                  maximum: _forecast ? 1 : 20,
                  enabled: !_submitting,
                  onChanged: (items) =>
                      setState(() => _securitiesByKind[_kind] = items),
                ),
                const SizedBox(height: 16),
                if (_forecast) ...[
                  Wrap(
                    spacing: 8,
                    children: [
                      for (final growth in [true, false])
                        ChoiceChip(
                          label: Text(growth ? 'Рост' : 'Снижение'),
                          selected: _growth == growth,
                          onSelected: _submitting
                              ? null
                              : (_) => setState(() {
                                  _growth = growth;
                                  if (_recommendation?.allows(
                                        growth ? 1 : -1,
                                      ) ==
                                      false) {
                                    _recommendation = null;
                                  }
                                }),
                        ),
                    ],
                  ),
                  const SizedBox(height: 8),
                  TextFormField(
                    controller: _percent,
                    enabled: !_submitting,
                    keyboardType: const TextInputType.numberWithOptions(
                      decimal: true,
                    ),
                    decoration: const InputDecoration(
                      labelText: 'Изменение цены',
                      suffixText: '%',
                    ),
                    validator: (_) {
                      final value = _magnitude;
                      final maximum = _growth ? 10000 : 100;
                      if (value == null ||
                          !value.isFinite ||
                          value <= 0 ||
                          value > maximum) {
                        return 'Введите число больше 0 и не больше $maximum';
                      }
                      return null;
                    },
                  ),
                  const SizedBox(height: 12),
                  OutlinedButton.icon(
                    onPressed: _submitting ? null : _pickDate,
                    icon: const Icon(Icons.event_outlined),
                    label: Text(
                      _date == null
                          ? 'Дата окончания прогноза'
                          : 'До ${MaterialLocalizations.of(context).formatCompactDate(_date!)}',
                    ),
                  ),
                  const SizedBox(height: 16),
                ],
                TextFormField(
                  controller: _body,
                  enabled: !_submitting,
                  minLines: 4,
                  maxLines: 12,
                  maxLength: 8000,
                  decoration: const InputDecoration(
                    labelText: 'Описание',
                    alignLabelWithHint: true,
                  ),
                  validator: (value) =>
                      value!.trim().isEmpty ? 'Введите описание' : null,
                ),
                if (_forecast) ...[
                  Text(
                    'Рекомендация',
                    style: Theme.of(context).textTheme.titleSmall,
                  ),
                  Wrap(
                    spacing: 8,
                    children: [
                      for (final value in Recommendation.values)
                        ChoiceChip(
                          label: Text(value.label),
                          selected: _recommendation == value,
                          onSelected:
                              _submitting || !value.allows(_growth ? 1 : -1)
                              ? null
                              : (_) => setState(() => _recommendation = value),
                        ),
                    ],
                  ),
                ],
                if (_error != null)
                  Padding(
                    padding: const EdgeInsets.only(top: 12),
                    child: Text(
                      _error!,
                      style: TextStyle(
                        color: Theme.of(context).colorScheme.error,
                      ),
                    ),
                  ),
                const SizedBox(height: 20),
                FilledButton(
                  onPressed: _submitting ? null : _publish,
                  child: Text(
                    _submitting
                        ? 'Сохраняем…'
                        : _editing
                        ? 'Сохранить'
                        : 'Опубликовать',
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
