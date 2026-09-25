package upstream

import (
	"context"
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"

	portfoliopb "invest/backend/services/gateway/internal/portfoliopb"
	securitiesreaderpb "invest/backend/services/gateway/internal/securitiesreaderpb"
	userspb "invest/backend/services/gateway/internal/userspb"
)

type Clients struct {
	Users            userspb.UsersServiceClient
	Portfolio        portfoliopb.PortfolioServiceClient
	SecuritiesReader securitiesreaderpb.SecuritiesReaderServiceClient

	usersConn            *grpc.ClientConn
	portfolioConn        *grpc.ClientConn
	securitiesReaderConn *grpc.ClientConn

	usersHealth            grpc_health_v1.HealthClient
	portfolioHealth        grpc_health_v1.HealthClient
	securitiesReaderHealth grpc_health_v1.HealthClient
}

type Addrs struct {
	Users            string
	Portfolio        string
	SecuritiesReader string
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
	securitiesReaderConn, err := grpc.NewClient(addrs.SecuritiesReader, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		usersConn.Close()
		portfolioConn.Close()
		return nil, fmt.Errorf("dial securities_reader service: %w", err)
	}

	return &Clients{
		Users:            userspb.NewUsersServiceClient(usersConn),
		Portfolio:        portfoliopb.NewPortfolioServiceClient(portfolioConn),
		SecuritiesReader: securitiesreaderpb.NewSecuritiesReaderServiceClient(securitiesReaderConn),

		usersConn:            usersConn,
		portfolioConn:        portfolioConn,
		securitiesReaderConn: securitiesReaderConn,

		usersHealth:            grpc_health_v1.NewHealthClient(usersConn),
		portfolioHealth:        grpc_health_v1.NewHealthClient(portfolioConn),
		securitiesReaderHealth: grpc_health_v1.NewHealthClient(securitiesReaderConn),
	}, nil
}

func (c *Clients) Close() {
	c.usersConn.Close()
	c.portfolioConn.Close()
	c.securitiesReaderConn.Close()
}

type HealthReport map[string]string

func (c *Clients) CheckAll(ctx context.Context) HealthReport {
	problems := HealthReport{}
	for name, client := range map[string]grpc_health_v1.HealthClient{
		"users":             c.usersHealth,
		"portfolio":         c.portfolioHealth,
		"securities_reader": c.securitiesReaderHealth,
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
