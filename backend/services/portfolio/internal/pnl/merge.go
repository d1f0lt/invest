package pnl

import "sort"

func Merge(summaries []Summary) Summary {
	if len(summaries) == 1 {
		return summaries[0]
	}

	var out Summary
	byKey := map[string]*InstrumentPnL{}
	var keys []string
	for _, s := range summaries {
		out.TotalRealizedPnL += s.TotalRealizedPnL
		out.TotalUnrealizedPnL += s.TotalUnrealizedPnL
		out.TotalDividends += s.TotalDividends
		out.TotalCoupons += s.TotalCoupons
		out.TotalAccruedInterest += s.TotalAccruedInterest
		out.TotalTaxes += s.TotalTaxes
		out.TotalFees += s.TotalFees
		out.TotalOther += s.TotalOther
		out.NetDeposits += s.NetDeposits
		out.CashBalance += s.CashBalance
		out.TotalDayChange += s.TotalDayChange

		for _, in := range s.Instruments {
			key := PriceKey(in.SecID, in.Board)
			m, ok := byKey[key]
			if !ok {
				m = &InstrumentPnL{SecID: in.SecID, Board: in.Board}
				byKey[key] = m
				keys = append(keys, key)
			}
			mergeInstrument(m, in)
		}
	}

	sort.Strings(keys)
	out.Instruments = make([]InstrumentPnL, 0, len(keys))
	for _, key := range keys {
		out.Instruments = append(out.Instruments, *byKey[key])
	}
	return out
}

func mergeInstrument(m *InstrumentPnL, in InstrumentPnL) {
	cost := m.Quantity*m.AvgCost + in.Quantity*in.AvgCost
	m.Quantity += in.Quantity
	m.AvgCost = 0
	if m.Quantity > 0 {
		m.AvgCost = cost / m.Quantity
	}

	if in.CurrentPrice != nil {
		p := *in.CurrentPrice
		m.CurrentPrice = &p
	}
	m.MarketValue = addOptional(m.MarketValue, in.MarketValue)
	m.UnrealizedPnL = addOptional(m.UnrealizedPnL, in.UnrealizedPnL)
	m.DayChange = addOptional(m.DayChange, in.DayChange)

	m.RealizedPnL += in.RealizedPnL
	m.Dividends += in.Dividends
	m.Coupons += in.Coupons
	m.AccruedInterest += in.AccruedInterest
}

func addOptional(a, b *float64) *float64 {
	if b == nil {
		return a
	}
	sum := *b
	if a != nil {
		sum += *a
	}
	return &sum
}

func MergeValueHistory(histories [][]ValuePoint) []ValuePoint {
	if len(histories) == 0 {
		return nil
	}
	out := make([]ValuePoint, len(histories[0]))
	copy(out, histories[0])
	for _, h := range histories[1:] {
		for i := range out {
			if i < len(h) {
				out[i].Value += h[i].Value
				out[i].NetDeposits += h[i].NetDeposits
			}
		}
	}
	return out
}
