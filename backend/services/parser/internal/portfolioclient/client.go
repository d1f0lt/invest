








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






func New(addr string) (*Client, error) {
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("dial portfolio service: %w", err)
	}
	return &Client{conn: conn, api: portfoliopb.NewPortfolioServiceClient(conn)}, nil
}

func (c *Client) Close() error { return c.conn.Close() }



type ImportResult struct {
	TradesCreated, TradesSkipped int
	CashCreated, CashSkipped     int
}








func (c *Client) ImportReport(ctx context.Context, userID, portfolioID string, r parsing.Report) (ImportResult, error) {
	ctx = authmd.WithUserID(ctx, userID)

	req := &portfoliopb.ImportReportRequest{
		PortfolioId:    portfolioID,
		Trades:         make([]*portfoliopb.TradeInput, 0, len(r.Trades)),
		CashOperations: make([]*portfoliopb.CashOperationInput, 0, len(r.CashOperations)),
	}
	for _, t := range r.Trades {
		in := &portfoliopb.TradeInput{
			Secid:           t.SecID,
			Board:           t.Board,
			Side:            t.Side,
			Quantity:        t.Quantity,
			Price:           t.Price,
			Fee:             t.Fee,
			Currency:        t.Currency,
			AccruedInterest: t.AccruedInterest,
			ExternalId:      t.ExternalID,
		}
		if t.ExecutedAt != nil {
			in.ExecutedAt = timestamppb.New(*t.ExecutedAt)
		}
		req.Trades = append(req.Trades, in)
	}
	for _, op := range r.CashOperations {
		req.CashOperations = append(req.CashOperations, &portfoliopb.CashOperationInput{
			Type:        op.Type,
			Amount:      op.Amount,
			Currency:    op.Currency,
			OccurredAt:  timestamppb.New(op.Date),
			Secid:       op.SecID,
			Board:       op.Board,
			Description: op.Description,
			ExternalId:  op.ExternalID,
		})
	}

	callCtx, cancel := context.WithTimeout(ctx, submitTimeout(len(r.Trades)+len(r.CashOperations)))
	defer cancel()

	resp, err := c.api.ImportReport(callCtx, req)
	if err != nil {
		return ImportResult{}, fmt.Errorf("import report (%d trades, %d cash operations): %w", len(r.Trades), len(r.CashOperations), err)
	}
	return ImportResult{
		TradesCreated: int(resp.GetTradesCreated()),
		TradesSkipped: int(resp.GetTradesSkipped()),
		CashCreated:   int(resp.GetCashOperationsCreated()),
		CashSkipped:   int(resp.GetCashOperationsSkipped()),
	}, nil
}




func submitTimeout(rows int) time.Duration {
	return 30*time.Second + time.Duration(rows)*200*time.Millisecond
}
