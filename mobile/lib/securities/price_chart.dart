import 'dart:math' as math;

import 'package:flutter/material.dart';

/// Точка графика: время (UTC) и цена.
class ChartPoint {
  const ChartPoint(this.time, this.value);

  final DateTime time;
  final double value;
}

/// Линейный график цены с заливкой, пунктиром уровня начала периода,
/// подписями осей и «прицелом» по касанию/протягиванию пальцем.
///
/// Точки расставлены равномерно по индексу (без разрывов на ночь и
/// выходные), поэтому подписи X — это [xLabel] у выбранных точек.
class PriceChart extends StatefulWidget {
  const PriceChart({
    super.key,
    required this.points,
    required this.xTickKey,
    required this.xLabel,
    required this.yLabel,
    this.reference,
    this.onScrub,
    this.skipFirstTick = false,
    this.height = 260,
  });

  final List<ChartPoint> points;

  /// Пунктирная линия — цена в начале периода.
  final double? reference;

  /// Подпись X ставится там, где ключ точки меняется (новый день, месяц…).
  final Object Function(DateTime time) xTickKey;
  final String Function(DateTime time) xLabel;
  final String Function(double value) yLabel;

  /// Не подписывать первую точку (для месяцев: период начинается с середины).
  final bool skipFirstTick;

  /// Индекс точки под пальцем или null, когда палец убран.
  final ValueChanged<int?>? onScrub;
  final double height;

  @override
  State<PriceChart> createState() => _PriceChartState();
}

class _PriceChartState extends State<PriceChart> {
  static const _bottomAxis = 28.0;
  static const _maxXTicks = 6;

  int? _scrub;

  @override
  void didUpdateWidget(PriceChart old) {
    super.didUpdateWidget(old);
    // Список точек пересоздаётся при каждой перестройке родителя (в том числе
    // от самого «прицела»), поэтому сравниваем по содержимому краёв.
    final a = old.points, b = widget.points;
    final same = a.length == b.length &&
        (a.isEmpty ||
            (a.first.time == b.first.time &&
                a.last.time == b.last.time &&
                a.last.value == b.last.value));
    if (!same) _scrub = null;
  }

  void _scrubAt(double dx, double width) {
    final n = widget.points.length;
    if (n == 0) return;
    final int i = n == 1 ? 0 : math.max(0, math.min(n - 1, (dx / width * (n - 1)).round()));
    if (i == _scrub) return;
    setState(() => _scrub = i);
    widget.onScrub?.call(i);
  }

  void _endScrub() {
    if (_scrub == null) return;
    setState(() => _scrub = null);
    widget.onScrub?.call(null);
  }

  List<int> _xTicks() {
    final pts = widget.points;
    final ticks = <int>[];
    Object? prev;
    for (var i = 0; i < pts.length; i++) {
      final key = widget.xTickKey(pts[i].time);
      if (i == 0 ? !widget.skipFirstTick : key != prev) ticks.add(i);
      prev = key;
    }
    if (ticks.length <= _maxXTicks) return ticks;
    final step = (ticks.length / _maxXTicks).ceil();
    return [for (var i = 0; i < ticks.length; i += step) ticks[i]];
  }

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final scheme = theme.colorScheme;
    final labelStyle = theme.textTheme.labelSmall!.copyWith(color: scheme.onSurfaceVariant);
    return SizedBox(
      height: widget.height,
      child: LayoutBuilder(
        builder: (context, constraints) {
          final width = constraints.maxWidth;
          return GestureDetector(
            behavior: HitTestBehavior.opaque,
            onHorizontalDragStart: (d) => _scrubAt(d.localPosition.dx, width),
            onHorizontalDragUpdate: (d) => _scrubAt(d.localPosition.dx, width),
            onHorizontalDragEnd: (_) => _endScrub(),
            onHorizontalDragCancel: _endScrub,
            onLongPressStart: (d) => _scrubAt(d.localPosition.dx, width),
            onLongPressMoveUpdate: (d) => _scrubAt(d.localPosition.dx, width),
            onLongPressEnd: (_) => _endScrub(),
            child: CustomPaint(
              size: Size(width, widget.height),
              painter: _ChartPainter(
                points: widget.points,
                reference: widget.reference,
                xTicks: _xTicks(),
                xLabel: widget.xLabel,
                yLabel: widget.yLabel,
                scrub: _scrub,
                bottomAxis: _bottomAxis,
                lineColor: scheme.primary,
                labelStyle: labelStyle,
                guideColor: scheme.outline,
                dotBorder: scheme.surface,
                textDirection: Directionality.of(context),
              ),
            ),
          );
        },
      ),
    );
  }
}

class _ChartPainter extends CustomPainter {
  _ChartPainter({
    required this.points,
    required this.reference,
    required this.xTicks,
    required this.xLabel,
    required this.yLabel,
    required this.scrub,
    required this.bottomAxis,
    required this.lineColor,
    required this.labelStyle,
    required this.guideColor,
    required this.dotBorder,
    required this.textDirection,
  });

  final List<ChartPoint> points;
  final double? reference;
  final List<int> xTicks;
  final String Function(DateTime) xLabel;
  final String Function(double) yLabel;
  final int? scrub;
  final double bottomAxis;
  final Color lineColor;
  final TextStyle labelStyle;
  final Color guideColor;
  final Color dotBorder;
  final TextDirection textDirection;

  static const _topPad = 12.0;

  @override
  void paint(Canvas canvas, Size size) {
    if (points.isEmpty) return;
    final plotH = size.height - bottomAxis - _topPad;
    final w = size.width;

    var lo = points.map((p) => p.value).reduce(math.min);
    var hi = points.map((p) => p.value).reduce(math.max);
    if (reference != null) {
      lo = math.min(lo, reference!);
      hi = math.max(hi, reference!);
    }
    if (hi - lo < 1e-9) {
      final pad = hi.abs() * 0.01 + 1e-6;
      lo -= pad;
      hi += pad;
    }
    final ticks = _niceTicks(lo, hi, 5);
    lo = math.min(lo, ticks.first);
    hi = math.max(hi, ticks.last);
    final span = hi - lo;
    // Небольшой запас, чтобы линия не прилипала к краям.
    lo -= span * 0.04;
    hi += span * 0.04;

    double y(double v) => _topPad + (hi - v) / (hi - lo) * plotH;
    double x(int i) => points.length == 1 ? w / 2 : i / (points.length - 1) * w;

    // Подписи оси Y — слева поверх графика, как в приложениях брокеров.
    for (final t in ticks) {
      final ty = y(t);
      if (ty < 0 || ty > _topPad + plotH) continue;
      final tp = _text(yLabel(t));
      tp.paint(canvas, Offset(0, ty - tp.height / 2));
    }

    final line = Path();
    for (var i = 0; i < points.length; i++) {
      final p = Offset(x(i), y(points[i].value));
      if (i == 0) {
        line.moveTo(p.dx, p.dy);
      } else {
        line.lineTo(p.dx, p.dy);
      }
    }

    if (points.length > 1) {
      final bottom = _topPad + plotH;
      final fill = Path.from(line)
        ..lineTo(x(points.length - 1), bottom)
        ..lineTo(x(0), bottom)
        ..close();
      canvas.drawPath(
        fill,
        Paint()
          ..shader = LinearGradient(
            begin: Alignment.topCenter,
            end: Alignment.bottomCenter,
            colors: [lineColor.withValues(alpha: 0.28), lineColor.withValues(alpha: 0.02)],
          ).createShader(Rect.fromLTRB(0, _topPad, w, bottom)),
      );
    }

    if (reference != null) {
      _dashedLine(
        canvas,
        y(reference!),
        w,
        Paint()
          ..color = lineColor.withValues(alpha: 0.6)
          ..strokeWidth = 1,
      );
    }

    canvas.drawPath(
      line,
      Paint()
        ..color = lineColor
        ..style = PaintingStyle.stroke
        ..strokeWidth = 2.2
        ..strokeJoin = StrokeJoin.round
        ..strokeCap = StrokeCap.round,
    );
    if (points.length == 1) {
      canvas.drawCircle(Offset(x(0), y(points[0].value)), 3.5, Paint()..color = lineColor);
    }

    // Подписи оси X.
    for (final i in xTicks) {
      final tp = _text(xLabel(points[i].time));
      final dx = math.max(0.0, math.min(w - tp.width, x(i) - tp.width / 2));
      tp.paint(canvas, Offset(dx, size.height - bottomAxis + 8));
    }

    final s = scrub;
    if (s != null && s < points.length) {
      final p = Offset(x(s), y(points[s].value));
      canvas.drawLine(
        Offset(p.dx, _topPad),
        Offset(p.dx, _topPad + plotH),
        Paint()
          ..color = guideColor.withValues(alpha: 0.7)
          ..strokeWidth = 1,
      );
      canvas.drawCircle(p, 6, Paint()..color = dotBorder);
      canvas.drawCircle(p, 4.5, Paint()..color = lineColor);
    }
  }

  TextPainter _text(String s) =>
      TextPainter(text: TextSpan(text: s, style: labelStyle), textDirection: textDirection)
        ..layout();

  static void _dashedLine(Canvas canvas, double y, double width, Paint paint) {
    const dash = 5.0, gap = 4.0;
    for (var x = 0.0; x < width; x += dash + gap) {
      canvas.drawLine(Offset(x, y), Offset(math.min(x + dash, width), y), paint);
    }
  }

  /// «Круглые» деления оси: шаг 1, 2, 2,5 или 5 × 10^k.
  static List<double> _niceTicks(double lo, double hi, int count) {
    final raw = (hi - lo) / count;
    final mag = math.pow(10, (math.log(raw) / math.ln10).floor()).toDouble();
    final norm = raw / mag;
    final step = (norm <= 1
            ? 1
            : norm <= 2
                ? 2
                : norm <= 2.5
                    ? 2.5
                    : norm <= 5
                        ? 5
                        : 10) *
        mag;
    final first = (lo / step).floor() * step;
    final out = <double>[];
    for (var v = first; v <= hi + step * 0.5; v += step) {
      out.add(double.parse(v.toStringAsPrecision(12)));
    }
    return out;
  }

  @override
  bool shouldRepaint(_ChartPainter old) =>
      !identical(old.points, points) ||
      old.reference != reference ||
      old.scrub != scrub ||
      old.lineColor != lineColor ||
      old.labelStyle != labelStyle;
}
