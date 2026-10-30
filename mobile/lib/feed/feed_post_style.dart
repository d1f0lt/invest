import 'package:flutter/material.dart';

import 'feed_api.dart';

Color feedPositiveColor(ThemeData theme) => theme.brightness == Brightness.dark
    ? const Color(0xFF80DCA4)
    : const Color(0xFF17683A);

Color feedRecommendationColor(ThemeData theme, Recommendation recommendation) =>
    switch (recommendation) {
      Recommendation.buy => feedPositiveColor(theme),
      Recommendation.sell => theme.colorScheme.error,
      Recommendation.hold =>
        theme.brightness == Brightness.dark
            ? const Color(0xFFFFCC80)
            : const Color(0xFF855400),
    };

Color feedChangeColor(ThemeData theme, double change) => switch (change) {
  > 0 => feedPositiveColor(theme),
  < 0 => theme.colorScheme.error,
  _ => theme.colorScheme.onSurfaceVariant,
};

String feedPercentText(double change) {
  final absolute = change.abs();
  final number = absolute == absolute.truncateToDouble()
      ? absolute.toInt().toString()
      : absolute.toString().replaceAll('.', ',');
  final sign = change > 0
      ? '+'
      : change < 0
      ? '−'
      : '';
  return '$sign$number%';
}

class FeedTickerLabel extends StatelessWidget {
  const FeedTickerLabel({super.key, required this.ticker});

  final String ticker;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final colors = theme.colorScheme;
    final palette = [
      (colors.primaryContainer, colors.onPrimaryContainer),
      (colors.secondaryContainer, colors.onSecondaryContainer),
      (colors.tertiaryContainer, colors.onTertiaryContainer),
    ];
    final index = ticker.codeUnits.fold(0, (sum, unit) => sum + unit);
    final (background, foreground) = palette[index % palette.length];
    return DecoratedBox(
      decoration: BoxDecoration(
        color: background,
        borderRadius: BorderRadius.circular(4),
      ),
      child: Padding(
        padding: const EdgeInsets.symmetric(horizontal: 7, vertical: 4),
        child: Text(
          ticker,
          style: theme.textTheme.labelSmall?.copyWith(
            color: foreground,
            fontWeight: FontWeight.w700,
          ),
        ),
      ),
    );
  }
}
