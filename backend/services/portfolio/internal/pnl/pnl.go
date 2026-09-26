package pnl

import (
	"sort"
	"time"
)

type TradeSide string

const (
	Buy  TradeSide = "buy"
	Sell TradeSide = "sell"
)

type Trade struct {
	SecID      string
	Board      string
	Side       TradeSide
	Quantity   float64
	Price      float64
	Fee        float64
	ExecutedAt time.Time

	AccruedInterest float64
}

const (
	CashDeposit    = "deposit"
	CashWithdrawal = "withdrawal"
	CashDividend   = "dividend"
	CashCoupon     = "coupon"
	CashRedemption = "redemption"
	CashTax        = "tax"
	CashFee        = "fee"
	CashOther      = "other"
)

type CashFlow struct {
	Type       string
	Amount     float64
	SecID      string
	Board      string
	OccurredAt time.Time
}

type InstrumentPnL struct {
	SecID string
	Board string

	Quantity float64

	AvgCost float64

	CurrentPrice *float64

	MarketValue *float64

	UnrealizedPnL *float64

	DayChange *float64

	RealizedPnL float64

	Dividends       float64
	Coupons         float64
	AccruedInterest float64
}

func (i InstrumentPnL) TotalPnL() float64 {
	total := i.RealizedPnL + i.Dividends + i.Coupons + i.AccruedInterest
	if i.UnrealizedPnL != nil {
		total += *i.UnrealizedPnL
	}
	return total
}

type Summary struct {
	Instruments        []InstrumentPnL
	TotalRealizedPnL   float64
	TotalUnrealizedPnL float64

	TotalDividends       float64
	TotalCoupons         float64
	TotalAccruedInterest float64
	TotalTaxes           float64
	TotalFees            float64
	TotalOther           float64
	NetDeposits          float64
	CashBalance          float64
	TotalDayChange       float64
}

func (s Summary) TotalPnL() float64 {
	return s.TotalRealizedPnL + s.TotalUnrealizedPnL +
		s.TotalDividends + s.TotalCoupons + s.TotalAccruedInterest +
		s.TotalTaxes + s.TotalFees + s.TotalOther
}

func PriceKey(secid, board string) string { return secid + "/" + board }

type event struct {
	at         time.Time
	trade      *Trade
	redemption float64
}

func Compute(trades []Trade, cash []CashFlow, currentPrices map[string]float64) Summary {
	type acc struct {
		secid, board string
		events       []event
		dividends    float64
		coupons      float64
	}
	byInstrument := map[string]*acc{}
	get := func(secid, board string) *acc {
		key := PriceKey(secid, board)
		a, ok := byInstrument[key]
		if !ok {
			a = &acc{secid: secid, board: board}
			byInstrument[key] = a
		}
		return a
	}

	summary := Summary{}

	for i := range trades {
		t := trades[i]
		a := get(t.SecID, t.Board)
		a.events = append(a.events, event{at: t.ExecutedAt, trade: &t})

		amount := t.Quantity*t.Price + t.AccruedInterest
		switch t.Side {
		case Buy:
			summary.CashBalance -= amount + t.Fee
		case Sell:
			summary.CashBalance += amount - t.Fee
		}
	}

	for _, c := range cash {
		summary.CashBalance += c.Amount
		hasInstrument := c.SecID != ""

		switch c.Type {
		case CashDeposit, CashWithdrawal:
			summary.NetDeposits += c.Amount
		case CashDividend:
			summary.TotalDividends += c.Amount
			if hasInstrument {
				get(c.SecID, c.Board).dividends += c.Amount
			}
		case CashCoupon:
			summary.TotalCoupons += c.Amount
			if hasInstrument {
				get(c.SecID, c.Board).coupons += c.Amount
			}
		case CashRedemption:
			if hasInstrument {
				a := get(c.SecID, c.Board)
				a.events = append(a.events, event{at: c.OccurredAt, redemption: c.Amount})
			} else {

				summary.TotalOther += c.Amount
			}
		case CashTax:
			summary.TotalTaxes += c.Amount
		case CashFee:
			summary.TotalFees += c.Amount
		default:
			summary.TotalOther += c.Amount
		}
	}

	keys := make([]string, 0, len(byInstrument))
	for k := range byInstrument {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, key := range keys {
		a := byInstrument[key]
		sort.SliceStable(a.events, func(i, j int) bool { return a.events[i].at.Before(a.events[j].at) })

		result := computeInstrument(a.secid, a.board, a.events)
		result.Dividends = a.dividends
		result.Coupons = a.coupons

		if price, ok := currentPrices[key]; ok && result.Quantity > 0 {
			p := price
			result.CurrentPrice = &p
			mv := result.Quantity * price
			result.MarketValue = &mv
			upnl := result.Quantity * (price - result.AvgCost)
			result.UnrealizedPnL = &upnl
			summary.TotalUnrealizedPnL += upnl
		}
		summary.TotalRealizedPnL += result.RealizedPnL
		summary.TotalAccruedInterest += result.AccruedInterest
		summary.Instruments = append(summary.Instruments, result)
	}
	return summary
}

func Holdings(summary Summary) []InstrumentPnL {
	var open []InstrumentPnL
	for _, i := range summary.Instruments {
		if i.Quantity > 0 {
			open = append(open, i)
		}
	}
	return open
}

func computeInstrument(secid, board string, events []event) InstrumentPnL {
	res := InstrumentPnL{SecID: secid, Board: board}

	var runningQty, runningCost float64
	for _, e := range events {
		if e.trade == nil {

			runningCost -= e.redemption
			if runningCost < 0 {
				res.RealizedPnL += -runningCost
				runningCost = 0
			}
			continue
		}
		t := e.trade
		switch t.Side {
		case Buy:
			runningQty += t.Quantity
			runningCost += t.Quantity*t.Price + t.Fee
			res.AccruedInterest -= t.AccruedInterest
		case Sell:
			avgCost := 0.0
			if runningQty > 0 {
				avgCost = runningCost / runningQty
			}
			sellQty := t.Quantity
			if sellQty > runningQty {
				sellQty = runningQty
			}
			res.RealizedPnL += sellQty*(t.Price-avgCost) - t.Fee
			res.AccruedInterest += t.AccruedInterest
			runningCost -= sellQty * avgCost
			runningQty -= sellQty
		}
	}

	res.Quantity = runningQty
	if runningQty > 0 {
		res.AvgCost = runningCost / runningQty
	}
	return res
}

func ApplyDayChange(summary *Summary, prevCloses map[string]float64) {
	summary.TotalDayChange = 0
	for i := range summary.Instruments {
		in := &summary.Instruments[i]
		in.DayChange = nil
		prev, ok := prevCloses[PriceKey(in.SecID, in.Board)]
		if !ok || in.CurrentPrice == nil || in.Quantity <= 0 {
			continue
		}
		change := in.Quantity * (*in.CurrentPrice - prev)
		in.DayChange = &change
		summary.TotalDayChange += change
	}
}
