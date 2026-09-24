import 'dart:typed_data';

import 'package:file_picker/file_picker.dart';
import 'package:flutter/material.dart';

import '../api/api_client.dart';
import '../api/portfolio_api.dart';
import '../api/reports_api.dart';
import '../portfolio/portfolio_store.dart';
import 'broker_avatar.dart';

/// Выбор файла отчёта, отправка и ожидание, пока бэкенд его разберёт.
///
/// После отправки gateway возвращает импорт в статусе `queued`; экран раз в
/// [_pollInterval] спрашивает `GET /portfolios/{id}/reports/{import_id}`,
/// пока статус не станет `done` или `failed`.
class ReportUploadScreen extends StatefulWidget {
  const ReportUploadScreen({super.key, required this.broker, required this.portfolio});

  final Broker broker;
  final Portfolio portfolio;

  @override
  State<ReportUploadScreen> createState() => _ReportUploadScreenState();
}

enum _Stage { pick, uploading, processing, done, failed, slow }

class _ReportUploadScreenState extends State<ReportUploadScreen> {
  static const _pollInterval = Duration(milliseconds: 1500);

  /// Дольше этого не ждём на экране — разбор продолжится на сервере.
  static const _maxWait = Duration(minutes: 2);

  /// Совпадает с MAX_UPLOAD_BYTES gateway по умолчанию.
  static const _maxBytes = 32 * 1024 * 1024;

  final _api = ReportsApi();

  _Stage _stage = _Stage.pick;
  String? _filename;
  String? _pickError;
  String? _failure;
  ReportImport? _result;

  /// Растёт при каждом новом запуске/уходе с экрана — старый цикл опроса
  /// видит, что он больше не актуален, и останавливается.
  int _run = 0;

  @override
  void dispose() {
    _run++;
    super.dispose();
  }

  Future<void> _pick() async {
    setState(() => _pickError = null);
    final PlatformFile? file;
    try {
      file = await FilePicker.pickFile(
        type: FileType.custom,
        allowedExtensions: widget.broker.pickerExtensions,
      );
    } catch (_) {
      if (mounted) setState(() => _pickError = 'Не удалось открыть выбор файла');
      return;
    }
    if (file == null || !mounted) return; // пользователь закрыл выбор

    if (!widget.broker.accepts(file.name)) {
      setState(() => _pickError = 'Нужен файл в формате ${widget.broker.formatsLabel}');
      return;
    }

    final Uint8List bytes;
    try {
      bytes = await file.readAsBytes();
    } catch (_) {
      if (mounted) setState(() => _pickError = 'Не удалось прочитать файл');
      return;
    }
    if (!mounted) return;
    if (bytes.isEmpty) {
      setState(() => _pickError = 'Файл пустой');
      return;
    }
    if (bytes.length > _maxBytes) {
      setState(() => _pickError = 'Файл слишком большой (больше 32 МБ)');
      return;
    }

    await _upload(file.name, bytes);
  }

  Future<void> _upload(String filename, Uint8List bytes) async {
    final run = ++_run;
    setState(() {
      _stage = _Stage.uploading;
      _filename = filename;
      _failure = null;
      _result = null;
    });

    ReportImport imp;
    try {
      imp = await _api.upload(
        portfolioId: widget.portfolio.id,
        brokerId: widget.broker.id,
        filename: filename,
        bytes: bytes,
      );
    } on ApiException catch (e) {
      if (run == _run && mounted) _fail(userMessage(e));
      return;
    }
    if (run != _run || !mounted) return;
    setState(() => _stage = _Stage.processing);

    final deadline = DateTime.now().add(_maxWait);
    while (!imp.finished) {
      await Future<void>.delayed(_pollInterval);
      if (run != _run || !mounted) return;
      if (DateTime.now().isAfter(deadline)) {
        setState(() => _stage = _Stage.slow);
        return;
      }
      try {
        imp = await _api.get(widget.portfolio.id, imp.id);
      } on ApiException catch (e) {
        // Сеть моргнула — пробуем ещё раз; без авторизации ждать нечего.
        if (e.statusCode == 401 || e.statusCode == 404) {
          if (run == _run && mounted) _fail(userMessage(e));
          return;
        }
      }
      if (run != _run || !mounted) return;
    }

    if (imp.status == ReportImportStatus.failed) {
      _fail(imp.error ?? 'Не удалось обработать отчёт');
      return;
    }
    setState(() {
      _stage = _Stage.done;
      _result = imp;
    });
    // Главная сразу покажет новые цифры.
    PortfolioStore.instance.loadStats();
  }

  void _fail(String message) {
    setState(() {
      _stage = _Stage.failed;
      _failure = message;
    });
  }

  void _reset() {
    _run++;
    setState(() {
      _stage = _Stage.pick;
      _filename = null;
      _failure = null;
      _pickError = null;
      _result = null;
    });
  }

  void _finish() {
    PortfolioStore.instance.loadStats();
    Navigator.of(context).popUntil((route) => route.isFirst);
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: const Text('Загрузка отчёта')),
      body: SafeArea(
        child: Padding(
          padding: const EdgeInsets.fromLTRB(20, 8, 20, 16),
          child: AnimatedSwitcher(
            duration: const Duration(milliseconds: 250),
            child: KeyedSubtree(key: ValueKey(_stage), child: _content(context)),
          ),
        ),
      ),
    );
  }

  Widget _content(BuildContext context) => switch (_stage) {
        _Stage.pick => _pickView(context),
        _Stage.uploading => _ProgressView(
            title: 'Отправляем файл…',
            filename: _filename,
          ),
        _Stage.processing => _ProgressView(
            title: 'Разбираем отчёт…',
            subtitle: 'Обычно это занимает несколько секунд',
            filename: _filename,
          ),
        _Stage.done => _doneView(context),
        _Stage.failed => _ResultView(
            icon: Icons.error_outline_rounded,
            iconColor: Theme.of(context).colorScheme.error,
            title: 'Не удалось загрузить отчёт',
            lines: [?_failure],
            primary: FilledButton(onPressed: _reset, child: const Text('Выбрать другой файл')),
          ),
        _Stage.slow => _ResultView(
            icon: Icons.hourglass_top_rounded,
            iconColor: Theme.of(context).colorScheme.primary,
            title: 'Отчёт ещё обрабатывается',
            lines: const [
              'Разбор занимает больше времени, чем обычно. Данные появятся в портфеле, '
                  'как только он закончится — экран можно закрыть.',
            ],
            primary: FilledButton(onPressed: _finish, child: const Text('Готово')),
          ),
      };

  Widget _pickView(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    final textTheme = Theme.of(context).textTheme;
    final broker = widget.broker;
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        const SizedBox(height: 8),
        Row(
          children: [
            BrokerAvatar(broker: broker, size: 40),
            const SizedBox(width: 12),
            Expanded(
              child: Text(
                broker.name,
                style: textTheme.titleMedium?.copyWith(fontWeight: FontWeight.w600),
              ),
            ),
          ],
        ),
        const SizedBox(height: 20),
        Material(
          color: scheme.surfaceContainerLow,
          shape: RoundedRectangleBorder(
            borderRadius: BorderRadius.circular(24),
            side: BorderSide(color: scheme.outlineVariant, width: 1.5),
          ),
          clipBehavior: Clip.antiAlias,
          child: InkWell(
            onTap: _pick,
            child: Padding(
              padding: const EdgeInsets.symmetric(horizontal: 24, vertical: 36),
              child: Column(
                children: [
                  Container(
                    width: 72,
                    height: 72,
                    decoration: BoxDecoration(shape: BoxShape.circle, color: scheme.primaryContainer),
                    child: Icon(Icons.upload_file_rounded, size: 36, color: scheme.onPrimaryContainer),
                  ),
                  const SizedBox(height: 16),
                  Text(
                    'Выберите файл отчёта',
                    textAlign: TextAlign.center,
                    style: textTheme.titleMedium?.copyWith(fontWeight: FontWeight.w700),
                  ),
                  const SizedBox(height: 6),
                  Text(
                    'Брокерский отчёт за любой период. Повторная загрузка '
                    'не создаст дублей',
                    textAlign: TextAlign.center,
                    style: textTheme.bodyMedium?.copyWith(color: scheme.onSurfaceVariant),
                  ),
                ],
              ),
            ),
          ),
        ),
        if (_pickError != null) ...[
          const SizedBox(height: 12),
          Text(
            _pickError!,
            textAlign: TextAlign.center,
            style: TextStyle(color: scheme.error),
          ),
        ],
        const Spacer(),
        Text(
          'Отчёт будет добавлен в портфель «${widget.portfolio.displayName}»',
          textAlign: TextAlign.center,
          style: textTheme.bodySmall?.copyWith(color: scheme.onSurfaceVariant),
        ),
        const SizedBox(height: 12),
        SizedBox(
          height: 52,
          child: FilledButton.icon(
            onPressed: _pick,
            icon: const Icon(Icons.folder_open_rounded),
            label: const Text('Выбрать файл'),
          ),
        ),
        const SizedBox(height: 10),
        Text(
          'Поддерживаемый формат: ${broker.formatsLabel}',
          textAlign: TextAlign.center,
          style: textTheme.labelSmall?.copyWith(color: scheme.onSurfaceVariant),
        ),
      ],
    );
  }

  Widget _doneView(BuildContext context) {
    final r = _result!;
    final created = r.tradesCreated + r.cashCreated;
    final skipped = r.tradesSkipped + r.cashSkipped;
    final String title;
    final lines = <String>[];
    if (created == 0 && skipped == 0) {
      title = 'Отчёт загружен';
      lines.add('В отчёте нет сделок и движений денег за этот период');
    } else if (created == 0) {
      title = 'Этот отчёт уже загружен';
      lines.add('Все операции из него уже есть в портфеле');
    } else {
      title = 'Отчёт загружен';
      if (r.tradesCreated > 0) lines.add('Добавлено ${_count(r.tradesCreated, 'сделка', 'сделки', 'сделок')}');
      if (r.cashCreated > 0) {
        lines.add('Добавлено ${_count(r.cashCreated, 'операция', 'операции', 'операций')} '
            'по счёту: пополнения, дивиденды, купоны, налоги');
      }
      if (skipped > 0) lines.add('Уже были в портфеле и пропущены: $skipped');
    }
    return _ResultView(
      icon: Icons.check_circle_rounded,
      iconColor: const Color(0xFF1E9E5A),
      title: title,
      lines: lines,
      primary: FilledButton(onPressed: _finish, child: const Text('Готово')),
      secondary: TextButton(onPressed: _reset, child: const Text('Загрузить ещё отчёт')),
    );
  }
}

/// «11 сделок», «1 операция», «3 операции».
String _count(int n, String one, String few, String many) {
  final mod10 = n % 10, mod100 = n % 100;
  final word = mod10 == 1 && mod100 != 11
      ? one
      : mod10 >= 2 && mod10 <= 4 && (mod100 < 12 || mod100 > 14)
          ? few
          : many;
  return '$n $word';
}

class _ProgressView extends StatelessWidget {
  const _ProgressView({required this.title, this.subtitle, this.filename});

  final String title;
  final String? subtitle;
  final String? filename;

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    final textTheme = Theme.of(context).textTheme;
    return Center(
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          const SizedBox(width: 56, height: 56, child: CircularProgressIndicator(strokeWidth: 5)),
          const SizedBox(height: 28),
          Text(
            title,
            textAlign: TextAlign.center,
            style: textTheme.titleLarge?.copyWith(fontWeight: FontWeight.w700),
          ),
          if (subtitle != null) ...[
            const SizedBox(height: 8),
            Text(
              subtitle!,
              textAlign: TextAlign.center,
              style: textTheme.bodyMedium?.copyWith(color: scheme.onSurfaceVariant),
            ),
          ],
          if (filename != null) ...[
            const SizedBox(height: 20),
            _FileChip(filename: filename!),
          ],
        ],
      ),
    );
  }
}

class _FileChip extends StatelessWidget {
  const _FileChip({required this.filename});

  final String filename;

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 10),
      decoration: BoxDecoration(
        color: scheme.surfaceContainerHigh,
        borderRadius: BorderRadius.circular(14),
      ),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          Icon(Icons.description_outlined, size: 20, color: scheme.onSurfaceVariant),
          const SizedBox(width: 8),
          Flexible(child: Text(filename, overflow: TextOverflow.ellipsis)),
        ],
      ),
    );
  }
}

class _ResultView extends StatelessWidget {
  const _ResultView({
    required this.icon,
    required this.iconColor,
    required this.title,
    required this.lines,
    required this.primary,
    this.secondary,
  });

  final IconData icon;
  final Color iconColor;
  final String title;
  final List<String> lines;
  final Widget primary;
  final Widget? secondary;

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    final textTheme = Theme.of(context).textTheme;
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Expanded(
          child: Center(
            child: SingleChildScrollView(
              child: Column(
                children: [
                  Icon(icon, size: 72, color: iconColor),
                  const SizedBox(height: 20),
                  Text(
                    title,
                    textAlign: TextAlign.center,
                    style: textTheme.titleLarge?.copyWith(fontWeight: FontWeight.w700),
                  ),
                  for (final line in lines) ...[
                    const SizedBox(height: 8),
                    Text(
                      line,
                      textAlign: TextAlign.center,
                      style: textTheme.bodyMedium?.copyWith(color: scheme.onSurfaceVariant),
                    ),
                  ],
                ],
              ),
            ),
          ),
        ),
        SizedBox(height: 52, child: primary),
        if (secondary != null) ...[
          const SizedBox(height: 8),
          secondary!,
        ],
      ],
    );
  }
}
