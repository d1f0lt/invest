import 'package:flutter/material.dart';

import '../api/api_client.dart';

/// Подписанное поле профиля: значение + карандаш. По нажатию на карандаш
/// превращается в поле ввода с кнопками «сохранить» / «отмена».
class EditableField extends StatefulWidget {
  const EditableField({
    super.key,
    required this.label,
    required this.value,
    required this.icon,
    required this.onSave,
    this.validator,
    this.keyboardType,
    this.autofillHints,
  });

  final String label;
  final String? value;
  final IconData icon;

  /// Сохраняет новое значение; [ApiException] показывается под полем.
  final Future<void> Function(String value) onSave;
  final FormFieldValidator<String>? validator;
  final TextInputType? keyboardType;
  final Iterable<String>? autofillHints;

  @override
  State<EditableField> createState() => _EditableFieldState();
}

class _EditableFieldState extends State<EditableField> {
  final _controller = TextEditingController();
  bool _editing = false;
  bool _saving = false;
  String? _error;

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  void _startEditing() {
    _controller.text = widget.value ?? '';
    _controller.selection = TextSelection.collapsed(offset: _controller.text.length);
    setState(() {
      _editing = true;
      _error = null;
    });
  }

  void _cancel() {
    FocusScope.of(context).unfocus();
    setState(() {
      _editing = false;
      _error = null;
    });
  }

  Future<void> _save() async {
    if (_saving) return;
    final value = _controller.text.trim();
    final error = widget.validator?.call(value);
    if (error != null) {
      setState(() => _error = error);
      return;
    }
    if (value == (widget.value ?? '').trim()) {
      _cancel();
      return;
    }
    setState(() {
      _saving = true;
      _error = null;
    });
    try {
      await widget.onSave(value);
      if (!mounted) return;
      FocusScope.of(context).unfocus();
      setState(() => _editing = false);
    } on ApiException catch (e) {
      if (mounted) setState(() => _error = e.message);
    } finally {
      if (mounted) setState(() => _saving = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    return AnimatedSize(
      duration: const Duration(milliseconds: 200),
      alignment: Alignment.topCenter,
      child: _editing ? _buildEditor(context) : _buildView(context),
    );
  }

  Widget _buildView(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    final textTheme = Theme.of(context).textTheme;
    final value = widget.value;
    return Padding(
      padding: const EdgeInsets.fromLTRB(16, 10, 4, 10),
      child: Row(
        children: [
          Icon(widget.icon, color: scheme.onSurfaceVariant),
          const SizedBox(width: 16),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  widget.label,
                  style: textTheme.labelMedium?.copyWith(color: scheme.onSurfaceVariant),
                ),
                const SizedBox(height: 2),
                Text(
                  (value == null || value.isEmpty) ? 'Не указан' : value,
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                  style: textTheme.bodyLarge?.copyWith(
                    color: (value == null || value.isEmpty) ? scheme.onSurfaceVariant : null,
                  ),
                ),
              ],
            ),
          ),
          IconButton(
            onPressed: _startEditing,
            icon: const Icon(Icons.edit_outlined),
            tooltip: 'Изменить',
          ),
        ],
      ),
    );
  }

  Widget _buildEditor(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.fromLTRB(12, 12, 4, 12),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Expanded(
            child: TextField(
              controller: _controller,
              autofocus: true,
              enabled: !_saving,
              autocorrect: false,
              keyboardType: widget.keyboardType,
              autofillHints: widget.autofillHints,
              textInputAction: TextInputAction.done,
              onChanged: (_) {
                if (_error != null) setState(() => _error = null);
              },
              onSubmitted: (_) => _save(),
              decoration: InputDecoration(
                labelText: widget.label,
                prefixIcon: Icon(widget.icon),
                errorText: _error,
                errorMaxLines: 2,
                isDense: true,
                border: OutlineInputBorder(borderRadius: BorderRadius.circular(14)),
              ),
            ),
          ),
          const SizedBox(width: 4),
          Padding(
            padding: const EdgeInsets.only(top: 2),
            child: _saving
                ? const SizedBox.square(
                    dimension: 48,
                    child: Padding(
                      padding: EdgeInsets.all(14),
                      child: CircularProgressIndicator(strokeWidth: 2.5),
                    ),
                  )
                : IconButton(
                    onPressed: _save,
                    icon: const Icon(Icons.check_rounded),
                    tooltip: 'Сохранить',
                  ),
          ),
          Padding(
            padding: const EdgeInsets.only(top: 2),
            child: IconButton(
              onPressed: _saving ? null : _cancel,
              icon: const Icon(Icons.close_rounded),
              tooltip: 'Отмена',
            ),
          ),
        ],
      ),
    );
  }
}
