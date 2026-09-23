// Package portfolioclient calls the portfolio service's gRPC API to
// submit parsed trades. Replaces the old HTTP client (one POST
// /portfolios/{id}/trades per trade, X-User-ID header) with the
// equivalent gRPC calls - see architecture-decisions.md, "перевод
// внутреннего взаимодействия сервисов на gRPC". Since 2026-09-23 it
// submits the whole parsed batch in a single CreateTrades call instead
// of one CreateTrade per trade, which is also atomic on the portfolio
// side (single DB transaction) - see architecture-decisions.md, "parser:
// асинхронный разбор отчётов".
package portfolioclient

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/timestamppb"

	"invest/backend/services/parser/internal/authmd"
	"invest/backend/services/parser/internal/parsing"
	portfoliopb "invest/backend/services/parser/internal/portfoliopb"
)

type Client struct {
	conn *grpc.ClientConn
	api  portfoliopb.PortfolioServiceClient
}

// New dials addr (host:port, e.g. "portfolio:8083") once and reuses the
// connection - gRPC's ClientConn already pools/multiplexes streams
// internally, unlike the old *http.Client which needed nothing special
// either, but this makes the change explicit: one long-lived connection
// per parser process, not one dial per trade.
func New(addr string) (*Client, error) {
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("dial portfolio service: %w", err)
	}
	return &Client{conn: conn, api: portfoliopb.NewPortfolioServiceClient(conn)}, nil
}

func (c *Client) Close() error { return c.conn.Close() }

// SubmitTrades creates all trades in one CreateTrades call (portfolio
// inserts them in a single DB transaction, so the batch is all-or-nothing
// - a mid-batch failure no longer leaves the earlier trades applied,
// which used to make a requeued task duplicate them). userID is
// attached as "x-user-id" gRPC metadata (was: the X-User-ID HTTP
// header; see internal/authmd), which is what makes the portfolio
// service accept the trades as belonging to that user's portfolio.
func (c *Client) SubmitTrades(ctx context.Context, userID, portfolioID string, trades []parsing.Trade) error {
	ctx = authmd.WithUserID(ctx, userID)

	req := &portfoliopb.CreateTradesRequest{
		PortfolioId: portfolioID,
		Trades:      make([]*portfoliopb.TradeInput, 0, len(trades)),
	}
	for _, t := range trades {
		in := &portfoliopb.TradeInput{
			Secid:    t.SecID,
			Board:    t.Board,
			Side:     t.Side,
			Quantity: t.Quantity,
			Price:    t.Price,
			Fee:      t.Fee,
			Currency: t.Currency,
		}
		if t.ExecutedAt != nil {
			in.ExecutedAt = timestamppb.New(*t.ExecutedAt)
		}
		req.Trades = append(req.Trades, in)
	}

	callCtx, cancel := context.WithTimeout(ctx, submitTimeout(len(trades)))
	defer cancel()

	_, err := c.api.CreateTrades(callCtx, req)
	if err != nil {
		return fmt.Errorf("submit %d trades: %w", len(trades), err)
	}
	return nil
}

// submitTimeout bounds the whole batch: a fixed base (handshake, any
// retry) plus per-trade headroom, so a large report can't be killed by
// a per-trade-sized deadline but a runaway batch still gets cut off.
func submitTimeout(tradeCount int) time.Duration {
	return 30*time.Second + time.Duration(tradeCount)*2*time.Second
}
