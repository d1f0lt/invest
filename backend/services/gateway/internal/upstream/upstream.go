package upstream

import (
	"context"
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"

	portfoliopb "invest/backend/services/gateway/internal/portfoliopb"
	pricereaderpb "invest/backend/services/gateway/internal/pricereaderpb"
	userspb "invest/backend/services/gateway/internal/userspb"
)

type Clients struct {
	Users       userspb.UsersServiceClient
	Portfolio   portfoliopb.PortfolioServiceClient
	PriceReader pricereaderpb.PriceReaderServiceClient

	usersConn       *grpc.ClientConn
	portfolioConn   *grpc.ClientConn
	priceReaderConn *grpc.ClientConn

	usersHealth       grpc_health_v1.HealthClient
	portfolioHealth   grpc_health_v1.HealthClient
	priceReaderHealth grpc_health_v1.HealthClient
}

type Addrs struct {
	Users       string
	Portfolio   string
	PriceReader string
}

func Dial(addrs Addrs) (*Clients, error) {
	usersConn, err := grpc.NewClient(addrs.Users, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("dial users service: %w", err)
	}
	portfolioConn, err := grpc.NewClient(addrs.Portfolio, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		usersConn.Close()
		return nil, fmt.Errorf("dial portfolio service: %w", err)
	}
	priceReaderConn, err := grpc.NewClient(addrs.PriceReader, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		usersConn.Close()
		portfolioConn.Close()
		return nil, fmt.Errorf("dial price_reader service: %w", err)
	}

	return &Clients{
		Users:       userspb.NewUsersServiceClient(usersConn),
		Portfolio:   portfoliopb.NewPortfolioServiceClient(portfolioConn),
		PriceReader: pricereaderpb.NewPriceReaderServiceClient(priceReaderConn),

		usersConn:       usersConn,
		portfolioConn:   portfolioConn,
		priceReaderConn: priceReaderConn,

		usersHealth:       grpc_health_v1.NewHealthClient(usersConn),
		portfolioHealth:   grpc_health_v1.NewHealthClient(portfolioConn),
		priceReaderHealth: grpc_health_v1.NewHealthClient(priceReaderConn),
	}, nil
}

func (c *Clients) Close() {
	c.usersConn.Close()
	c.portfolioConn.Close()
	c.priceReaderConn.Close()
}

type HealthReport map[string]string

func (c *Clients) CheckAll(ctx context.Context) HealthReport {
	problems := HealthReport{}
	for name, client := range map[string]grpc_health_v1.HealthClient{
		"users":        c.usersHealth,
		"portfolio":    c.portfolioHealth,
		"price_reader": c.priceReaderHealth,
	} {
		if err := checkOne(ctx, client); err != nil {
			problems[name] = err.Error()
		}
	}
	return problems
}

func checkOne(ctx context.Context, client grpc_health_v1.HealthClient) error {
	resp, err := client.Check(ctx, &grpc_health_v1.HealthCheckRequest{})
	if err != nil {
		return err
	}
	if resp.GetStatus() != grpc_health_v1.HealthCheckResponse_SERVING {
		return fmt.Errorf("status %s", resp.GetStatus())
	}
	return nil
}
