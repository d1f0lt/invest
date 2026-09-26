package parsing

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"invest/backend/services/parser/internal/task"
)


type Trade struct {
	SecID    string
	Board    string
	Side     string 
	Quantity float64
	
	
	
	Price float64
	
	
	Fee float64
	
	
	AccruedInterest float64
	Currency        string

	ExecutedAt *time.Time

	
	
	
	ExternalID string

	// Название и ISIN из отчёта — чтобы portfolio мог завести бумагу в
	// справочнике, если биржа о ней не знает.
	SecurityName string
	ISIN         string
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



type CashOperation struct {
	Type     string
	Amount   float64 
	Currency string
	Date     time.Time
	
	
	SecID       string
	Board       string
	Description string
	ExternalID  string
}


type Report struct {
	Trades         []Trade
	CashOperations []CashOperation

	// AccountKey — общий префикс ExternalID всех строк отчёта
	// ("sber:<счёт>"), PeriodStart — начало периода отчёта. Нужны portfolio,
	// чтобы решить, применять ли вводный остаток (см. OpeningMarker).
	AccountKey  string
	PeriodStart *time.Time
}

// OpeningMarker — часть ExternalID строк «вводного остатка»:
// "<AccountKey>:opening:<YYYY-MM-DD>:...". Это бумаги и деньги, которые
// лежали на счёте на начало периода отчёта (куплены/внесены раньше):
// бумаги — покупки по рыночной цене на начало периода, деньги — пополнения.
// portfolio применяет их, только если по счёту нет более ранней истории.
const OpeningMarker = ":opening:"


func (r Report) Empty() bool { return len(r.Trades) == 0 && len(r.CashOperations) == 0 }

var ErrNotImplemented = errors.New("parsing: not implemented")





type ErrUnsupportedBroker struct{ Broker string }

func (e *ErrUnsupportedBroker) Error() string {
	return fmt.Sprintf("parsing: no parser registered for broker %q", e.Broker)
}

type Parser interface {
	Parse(t task.ReportUploaded, data []byte) (Report, error)
}






type Dispatcher struct {
	parsers map[string]Parser
}

func NewDispatcher() *Dispatcher {
	return &Dispatcher{parsers: map[string]Parser{}}
}




func (d *Dispatcher) Register(broker string, p Parser) {
	d.parsers[normalizeBroker(broker)] = p
}

func (d *Dispatcher) Parse(t task.ReportUploaded, data []byte) (Report, error) {
	p, ok := d.parsers[normalizeBroker(t.Broker)]
	if !ok {
		return Report{}, &ErrUnsupportedBroker{Broker: t.Broker}
	}
	return p.Parse(t, data)
}


func (d *Dispatcher) SupportedBrokers() []string {
	out := make([]string, 0, len(d.parsers))
	for k := range d.parsers {
		out = append(out, k)
	}
	return out
}

func normalizeBroker(name string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(name))), "-")
}

type Stub struct{}

func (Stub) Parse(task.ReportUploaded, []byte) (Report, error) {
	return Report{}, ErrNotImplemented
}
