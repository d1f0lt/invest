import '../api/portfolio_api.dart';
import '../api/securities_api.dart';

/// Состояние выплаты в календаре.
enum PayoutStatus { received, declared, forecast }

/// Одна выплата дивидендов: полученная (из операций портфеля) или
/// ожидаемая (объявленная/прогнозная по данным dohod.ru × текущее количество).
class Payout {
  const Payout({
    required this.date,
    required this.secid,
    required this.board,
    required this.amount,
    required this.status,
    this.quantity,
    this.perShare,
    this.awaiting = false,
  });

  /// Локальная дата без времени: для полученных — дата зачисления,
  /// для ожидаемых — дата закрытия реестра.
  final DateTime date;
  final String? secid;
  final String? board;

  /// Сумма в рублях (у ожидаемых — с учётом налога, если он включён).
  final double amount;
  final PayoutStatus status;

  /// Для ожидаемых: сколько бумаг в портфеле и дивиденд на одну бумагу (до налога).
  final double? quantity;
  final double? perShare;

  /// Реестр уже закрыт, а зачисления в операциях ещё нет.
  final bool awaiting;
}

/// Период, за который показывается календарь: календарный год или
/// «на год вперёд» (12 месяцев с сегодняшнего дня).
class CalendarPeriod {
  const CalendarPeriod.year(int this.year) : ahead = false;
  const CalendarPeriod.ahead()
      : year = null,
        ahead = true;

  final int? year;
  final bool ahead;

  String get label => ahead ? 'На год вперёд' : '$year';

  DateTime start(DateTime today) => ahead ? today : DateTime(year!);

  DateTime end(DateTime today) =>
      ahead ? DateTime(today.year + 1, today.month, today.day) : DateTime(year! + 1);

  bool contains(DateTime date, DateTime today) =>
      !date.isBefore(start(today)) && date.isBefore(end(today));

  @override
  bool operator ==(Object other) =>
      other is CalendarPeriod && other.year == year && other.ahead == ahead;

  @override
  int get hashCode => Object.hash(year, ahead);
}

/// НДФЛ для резидента, удерживается с дивидендов российских эмитентов.
const dividendTaxRate = 0.13;

/// Сколько дней после закрытия реестра ждём зачисления, прежде чем
/// перестать показывать выплату как ожидаемую (по закону — до 25 рабочих дней).
const _awaitDays = 45;

DateTime dateOnly(DateTime d) => DateTime(d.year, d.month, d.day);

/// Собирает все выплаты: полученные дивиденды из [operations] и будущие
/// (или ещё не зачисленные) по открытым позициям [positions].
List<Payout> buildPayouts({
  required List<CashOperation> operations,
  required List<Position> positions,
  required Map<String, List<Dividend>> dividends,
  required DateTime today,
  required bool withTax,
}) {
  final received = <Payout>[
    for (final op in operations)
      if (op.type == 'dividend' && op.amount != 0)
        Payout(
          date: dateOnly(op.occurredAt.toLocal()),
          secid: op.secid,
          board: op.board,
          amount: op.amount,
          status: PayoutStatus.received,
        ),
  ];

  final lastReceived = <String, DateTime>{};
  for (final p in received) {
    final secid = p.secid;
    if (secid == null) continue;
    final prev = lastReceived[secid];
    if (prev == null || p.date.isAfter(prev)) lastReceived[secid] = p.date;
  }

  final keep = withTax ? 1 - dividendTaxRate : 1.0;
  final expected = <Payout>[];
  for (final position in positions) {
    if (position.quantity <= 0) continue;
    final seen = <DateTime>{};
    for (final d in dividends[position.secid] ?? const <Dividend>[]) {
      final date = DateTime(d.date.year, d.date.month, d.date.day);
      if (!seen.add(date) || d.value <= 0) continue;
      final future = !date.isBefore(today);
      final paid = lastReceived[position.secid];
      final awaiting = !future &&
          today.difference(date).inDays <= _awaitDays &&
          (paid == null || paid.isBefore(date));
      if (!future && !awaiting) continue;
      expected.add(Payout(
        date: date,
        secid: position.secid,
        board: position.board,
        amount: d.value * position.quantity * keep,
        status: d.forecast ? PayoutStatus.forecast : PayoutStatus.declared,
        quantity: position.quantity,
        perShare: d.value,
        awaiting: awaiting,
      ));
    }
  }

  return [...received, ...expected]..sort((a, b) {
      final byDate = a.date.compareTo(b.date);
      return byDate != 0 ? byDate : b.amount.compareTo(a.amount);
    });
}

/// Периоды для переключателя: текущий год, «на год вперёд», будущие годы
/// с выплатами, затем прошлые годы с полученными дивидендами (новые первыми).
List<CalendarPeriod> periodsFor(List<Payout> payouts, DateTime today) {
  final years = payouts.map((p) => p.date.year).toSet();
  final future = years.where((y) => y > today.year).toList()..sort();
  final past = years.where((y) => y < today.year).toList()..sort((a, b) => b - a);
  return [
    CalendarPeriod.year(today.year),
    const CalendarPeriod.ahead(),
    for (final y in future) CalendarPeriod.year(y),
    for (final y in past) CalendarPeriod.year(y),
  ];
}
