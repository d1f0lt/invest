import 'package:flutter/material.dart';

class AssistantAnswer extends StatelessWidget {
  const AssistantAnswer(this.text, {super.key});

  final String text;

  static final _heading = RegExp(r'^(#{1,6})[ \t]+(.+)$');
  static final _bullet = RegExp(r'^( {0,12})[-*+][ \t]+');
  static final _inline = RegExp(
    r'(`)([^`]+)\1|'
    r'(\*{1,3})(?!\s)(.*?\S)\3(?!\*)|'
    r'(?<!\w)(_{1,3})(?!\s)(.*?\S)\5(?!\w)|'
    r'(~~)(?!\s)(.*?\S)\7',
  );

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final lines = text.split('\n');

    return SelectableText.rich(
      TextSpan(
        children: [
          for (var index = 0; index < lines.length; index++) ...[
            _line(lines[index], theme),
            if (index < lines.length - 1) const TextSpan(text: '\n'),
          ],
        ],
      ),
    );
  }

  static TextSpan _line(String line, ThemeData theme) {
    final heading = _heading.firstMatch(line);
    final bullet = _bullet.firstMatch(line);
    final text = heading?.group(2) ?? line.substring(bullet?.end ?? 0);
    final headingStyle = heading == null
        ? null
        : (heading.group(1)!.length < 3
                  ? theme.textTheme.titleMedium
                  : theme.textTheme.bodyLarge)
              ?.copyWith(fontWeight: FontWeight.w700);

    final spans = <TextSpan>[];
    if (bullet != null) {
      spans.add(TextSpan(text: '${bullet.group(1)}• '));
    }

    var cursor = 0;
    for (final match in _inline.allMatches(text)) {
      if (match.start > cursor) {
        spans.add(TextSpan(text: text.substring(cursor, match.start)));
      }
      final emphasis = match.group(3) ?? match.group(5);
      final length = emphasis?.length ?? 0;
      spans.add(
        TextSpan(
          text:
              match.group(2) ??
              match.group(4) ??
              match.group(6) ??
              match.group(8),
          style: TextStyle(
            fontWeight: length >= 2 ? FontWeight.w700 : null,
            fontStyle: length.isOdd ? FontStyle.italic : null,
            decoration: match.group(7) == null
                ? null
                : TextDecoration.lineThrough,
            fontFamily: match.group(1) == null ? null : 'monospace',
            backgroundColor: match.group(1) == null
                ? null
                : theme.colorScheme.surfaceContainerHighest,
          ),
        ),
      );
      cursor = match.end;
    }
    if (cursor < text.length) {
      spans.add(TextSpan(text: text.substring(cursor)));
    }
    return TextSpan(style: headingStyle, children: spans);
  }
}
