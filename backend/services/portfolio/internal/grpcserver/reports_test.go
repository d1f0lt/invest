package grpcserver

import (
	"context"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	"invest/backend/services/portfolio/internal/storage"
	portfoliopb "invest/backend/services/portfolio/proto"
)

func (f *fakeStore) ListBrokers(context.Context) ([]storage.Broker, error) {
	var out []storage.Broker
	for _, b := range f.brokers {
		if b.Enabled {
			out = append(out, b)
		}
	}
	return out, nil
}

func (f *fakeStore) GetBroker(_ context.Context, id string) (storage.Broker, error) {
	b, ok := f.brokers[id]
	if !ok {
		return storage.Broker{}, storage.ErrNotFound
	}
	return b, nil
}

func (f *fakeStore) CreateReportImport(_ context.Context, r storage.ReportImport) (storage.ReportImport, error) {
	if _, ok := f.imports[r.ID]; ok {
		return storage.ReportImport{}, storage.ErrDuplicate
	}
	r.Status = storage.ImportQueued
	r.CreatedAt, r.UpdatedAt = time.Now(), time.Now()
	f.imports[r.ID] = r
	return r, nil
}

func (f *fakeStore) GetReportImport(_ context.Context, id string) (storage.ReportImport, error) {
	r, ok := f.imports[id]
	if !ok {
		return storage.ReportImport{}, storage.ErrNotFound
	}
	return r, nil
}

func (f *fakeStore) ListReportImports(_ context.Context, portfolioID string) ([]storage.ReportImport, error) {
	var out []storage.ReportImport
	for _, r := range f.imports {
		if r.PortfolioID == portfolioID {
			out = append(out, r)
		}
	}
	return out, nil
}

func (f *fakeStore) UpdateReportImportStatus(_ context.Context, id, st, errMsg string) (storage.ReportImport, error) {
	r, ok := f.imports[id]
	if !ok {
		return storage.ReportImport{}, storage.ErrNotFound
	}
	if r.Status == storage.ImportDone || r.Status == storage.ImportFailed {
		return storage.ReportImport{}, storage.ErrImportFinished
	}
	r.Status, r.Error = st, errMsg
	f.imports[id] = r
	return r, nil
}

const importID = "7f1c2d3e-4b5a-4c6d-8e9f-0a1b2c3d4e5f"

func TestListBrokers_OnlyEnabled(t *testing.T) {
	srv := newTestServer(newFakeStore())
	resp, err := srv.ListBrokers(withUserID("u1"), &emptypb.Empty{})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.GetBrokers()) != 1 || resp.GetBrokers()[0].GetId() != "sber" {
		t.Fatalf("brokers = %v, want only sber", resp.GetBrokers())
	}
	if _, err := srv.ListBrokers(context.Background(), &emptypb.Empty{}); status.Code(err) != codes.Unauthenticated {
		t.Errorf("without user: %v, want Unauthenticated", err)
	}
}

func TestCreateReportImport_Validation(t *testing.T) {
	srv := newTestServer(newFakeStore())
	ctx := withUserID("u1")
	p, _ := srv.CreatePortfolio(ctx, &portfoliopb.CreatePortfolioRequest{Name: "p"})

	cases := []struct {
		name string
		req  *portfoliopb.CreateReportImportRequest
		want codes.Code
	}{
		{"ok", &portfoliopb.CreateReportImportRequest{Id: importID, PortfolioId: p.GetId(), BrokerId: "Sber", Filename: "r.HTML"}, codes.OK},
		{"duplicate id", &portfoliopb.CreateReportImportRequest{Id: importID, PortfolioId: p.GetId(), BrokerId: "sber", Filename: "r.html"}, codes.AlreadyExists},
		{"htm", &portfoliopb.CreateReportImportRequest{Id: "7f1c2d3e-4b5a-4c6d-8e9f-0a1b2c3d4e50", PortfolioId: p.GetId(), BrokerId: "sber", Filename: "r.htm"}, codes.OK},
		{"bad id", &portfoliopb.CreateReportImportRequest{Id: "x", PortfolioId: p.GetId(), BrokerId: "sber", Filename: "r.html"}, codes.InvalidArgument},
		{"unknown broker", &portfoliopb.CreateReportImportRequest{Id: "7f1c2d3e-4b5a-4c6d-8e9f-0a1b2c3d4e51", PortfolioId: p.GetId(), BrokerId: "vtb", Filename: "r.html"}, codes.InvalidArgument},
		{"disabled broker", &portfoliopb.CreateReportImportRequest{Id: "7f1c2d3e-4b5a-4c6d-8e9f-0a1b2c3d4e52", PortfolioId: p.GetId(), BrokerId: "old", Filename: "r.html"}, codes.InvalidArgument},
		{"pdf", &portfoliopb.CreateReportImportRequest{Id: "7f1c2d3e-4b5a-4c6d-8e9f-0a1b2c3d4e53", PortfolioId: p.GetId(), BrokerId: "sber", Filename: "r.pdf"}, codes.InvalidArgument},
		{"no extension", &portfoliopb.CreateReportImportRequest{Id: "7f1c2d3e-4b5a-4c6d-8e9f-0a1b2c3d4e54", PortfolioId: p.GetId(), BrokerId: "sber", Filename: "report"}, codes.InvalidArgument},
	}
	for _, c := range cases {
		_, err := srv.CreateReportImport(ctx, c.req)
		if status.Code(err) != c.want {
			t.Errorf("%s: code = %v (%v), want %v", c.name, status.Code(err), err, c.want)
		}
	}

	
	_, err := srv.CreateReportImport(withUserID("u2"), &portfoliopb.CreateReportImportRequest{
		Id: "7f1c2d3e-4b5a-4c6d-8e9f-0a1b2c3d4e55", PortfolioId: p.GetId(), BrokerId: "sber", Filename: "r.html",
	})
	if status.Code(err) != codes.NotFound {
		t.Errorf("foreign portfolio: %v, want NotFound", err)
	}
}

func TestReportImport_Lifecycle(t *testing.T) {
	srv := newTestServer(newFakeStore())
	ctx := withUserID("u1")
	p, _ := srv.CreatePortfolio(ctx, &portfoliopb.CreatePortfolioRequest{Name: "p"})

	created, err := srv.CreateReportImport(ctx, &portfoliopb.CreateReportImportRequest{
		Id: importID, PortfolioId: p.GetId(), BrokerId: "sber", Filename: "r.html",
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.GetStatus() != "queued" {
		t.Fatalf("status = %q, want queued", created.GetStatus())
	}

	
	if _, err := srv.GetReportImport(withUserID("u2"), &portfoliopb.GetReportImportRequest{Id: importID}); status.Code(err) != codes.NotFound {
		t.Errorf("foreign get: %v, want NotFound", err)
	}
	if _, err := srv.UpdateReportImportStatus(withUserID("u2"), &portfoliopb.UpdateReportImportStatusRequest{Id: importID, Status: "failed"}); status.Code(err) != codes.NotFound {
		t.Errorf("foreign update: %v, want NotFound", err)
	}

	if _, err := srv.UpdateReportImportStatus(ctx, &portfoliopb.UpdateReportImportStatusRequest{Id: importID, Status: "queued"}); status.Code(err) != codes.InvalidArgument {
		t.Errorf("status queued: %v, want InvalidArgument", err)
	}
	if _, err := srv.UpdateReportImportStatus(ctx, &portfoliopb.UpdateReportImportStatusRequest{Id: importID, Status: "processing"}); err != nil {
		t.Fatal(err)
	}

	
	req := importReq(p.GetId())
	req.ReportImportId = importID
	res, err := srv.ImportReport(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	got, err := srv.GetReportImport(ctx, &portfoliopb.GetReportImportRequest{Id: importID})
	if err != nil {
		t.Fatal(err)
	}
	if got.GetStatus() != "done" || got.GetTradesCreated() != res.GetTradesCreated() || got.GetCashOperationsCreated() != res.GetCashOperationsCreated() {
		t.Errorf("after import: %v, want done with %v", got, res)
	}

	
	if _, err := srv.UpdateReportImportStatus(ctx, &portfoliopb.UpdateReportImportStatusRequest{Id: importID, Status: "failed"}); status.Code(err) != codes.FailedPrecondition {
		t.Errorf("update finished: %v, want FailedPrecondition", err)
	}

	list, err := srv.ListReportImports(ctx, &portfoliopb.ListReportImportsRequest{PortfolioId: p.GetId()})
	if err != nil || len(list.GetImports()) != 1 {
		t.Errorf("list = %v, %v; want 1 import", list, err)
	}
}

func TestImportReport_ForeignImportID(t *testing.T) {
	srv := newTestServer(newFakeStore())
	ctx := withUserID("u1")
	p1, _ := srv.CreatePortfolio(ctx, &portfoliopb.CreatePortfolioRequest{Name: "p1"})
	p2, _ := srv.CreatePortfolio(ctx, &portfoliopb.CreatePortfolioRequest{Name: "p2"})
	if _, err := srv.CreateReportImport(ctx, &portfoliopb.CreateReportImportRequest{
		Id: importID, PortfolioId: p1.GetId(), BrokerId: "sber", Filename: "r.html",
	}); err != nil {
		t.Fatal(err)
	}
	req := importReq(p2.GetId())
	req.ReportImportId = importID
	if _, err := srv.ImportReport(ctx, req); status.Code(err) != codes.NotFound {
		t.Errorf("import id of another portfolio: %v, want NotFound", err)
	}
}

func TestUpdateReportImportStatus_FailedDefaultsMessage(t *testing.T) {
	srv := newTestServer(newFakeStore())
	ctx := withUserID("u1")
	p, _ := srv.CreatePortfolio(ctx, &portfoliopb.CreatePortfolioRequest{Name: "p"})
	if _, err := srv.CreateReportImport(ctx, &portfoliopb.CreateReportImportRequest{
		Id: importID, PortfolioId: p.GetId(), BrokerId: "sber", Filename: "r.html",
	}); err != nil {
		t.Fatal(err)
	}
	got, err := srv.UpdateReportImportStatus(ctx, &portfoliopb.UpdateReportImportStatusRequest{Id: importID, Status: "failed"})
	if err != nil {
		t.Fatal(err)
	}
	if got.GetStatus() != "failed" || got.GetError() == "" {
		t.Errorf("got %v, want failed with a message", got)
	}
}
