package pnl

import (
	"sort"
	"time"
)

type DailyClose struct {
	Day   time.Time
	Close float64
}

type ValuePoint struct {
	Day         time.Time
	Value       float64
	NetDeposits float64
}

func ValueHistory(trades []Trade, cash []CashFlow, closes map[string][]DailyClose,
	current map[string]float64, days []time.Time, now time.Time) []ValuePoint {
	type ev struct {
		at    time.Time
		trade *Trade
		cash  *CashFlow
	}
	events := make([]ev, 0, len(trades)+len(cash))
	for i := range trades {
		events = append(events, ev{at: trades[i].ExecutedAt, trade: &trades[i]})
	}
	for i := range cash {
		events = append(events, ev{at: cash[i].OccurredAt, cash: &cash[i]})
	}
	sort.SliceStable(events, func(i, j int) bool { return events[i].at.Before(events[j].at) })

	qty := map[string]float64{}
	lastTradePrice := map[string]float64{}
	redeemedAt := map[string]time.Time{}
	closeIdx := map[string]int{}
	var cashBalance, netDeposits float64

	out := make([]ValuePoint, 0, len(days))
	next := 0
	for di, day := range days {
		end := day.AddDate(0, 0, 1)
		for next < len(events) && events[next].at.Before(end) {
			e := events[next]
			next++
			if t := e.trade; t != nil {
				key := PriceKey(t.SecID, t.Board)
				amount := t.Quantity*t.Price + t.AccruedInterest
				switch t.Side {
				case Buy:
					qty[key] += t.Quantity
					cashBalance -= amount + t.Fee
				case Sell:
					qty[key] -= t.Quantity
					if qty[key] < 0 {
						qty[key] = 0
					}
					cashBalance += amount - t.Fee
				}
				lastTradePrice[key] = t.Price
				continue
			}
			c := e.cash
			cashBalance += c.Amount
			switch c.Type {
			case CashDeposit, CashWithdrawal:
				netDeposits += c.Amount
			case CashRedemption:
				if c.SecID != "" {
					redeemedAt[PriceKey(c.SecID, c.Board)] = c.OccurredAt
				}
			}
		}

		live := di == len(days)-1 && !now.Before(day) && now.Before(end)
		value := cashBalance
		for key, q := range qty {
			if q <= 0 {
				continue
			}
			price, ok, fromCurrent := 0.0, false, false
			if live {
				price, ok = current[key]
				fromCurrent = ok
			}
			var closeDay time.Time
			if !ok {
				list := closes[key]
				i := closeIdx[key]
				for i < len(list) && !list[i].Day.After(day) {
					i++
				}
				closeIdx[key] = i
				if i > 0 {
					price, closeDay, ok = list[i-1].Close, list[i-1].Day, true
				}
			}
			if !ok {
				price, ok = lastTradePrice[key]
			}
			if r, redeemed := redeemedAt[key]; redeemed && !fromCurrent && closeDay.Before(dayStart(r, day.Location())) {
				continue
			}
			if ok {
				value += q * price
			}
		}
		out = append(out, ValuePoint{Day: day, Value: value, NetDeposits: netDeposits})
	}
	return out
}

func HistoryDays(from, to time.Time, loc *time.Location, maxDaily int) []time.Time {
	start := dayStart(from, loc)
	last := dayStart(to, loc)
	if start.After(last) {
		start = last
	}
	var days []time.Time
	for d := start; !d.After(last); d = d.AddDate(0, 0, 1) {
		days = append(days, d)
	}
	if len(days) <= maxDaily {
		return days
	}
	var thinned []time.Time
	for i := len(days) - 1; i >= 0; i -= 7 {
		thinned = append(thinned, days[i])
	}
	for i, j := 0, len(thinned)-1; i < j; i, j = i+1, j-1 {
		thinned[i], thinned[j] = thinned[j], thinned[i]
	}
	return thinned
}

func dayStart(t time.Time, loc *time.Location) time.Time {
	t = t.In(loc)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc)
}
