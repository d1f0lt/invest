package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	portfoliopb "invest/backend/services/gateway/internal/portfoliopb"
	"invest/backend/services/gateway/internal/task"
)

type fakeReportStore struct {
	uploaded     map[string][]byte
	uploadedType map[string]string
	uploadErr    error

	removedKeys []string
	removeErr   error

	orphaned bool
}

func newFakeReportStore() *fakeReportStore {
	return &fakeReportStore{
		uploaded:     map[string][]byte{},
		uploadedType: map[string]string{},
	}
}

func (f *fakeReportStore) Upload(_ context.Context, bucket, key string, r io.Reader, size int64, contentType string) error {
	if f.uploadErr != nil {
		return f.uploadErr
	}
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r); err != nil {
		return err
	}
	f.uploaded[bucket+"/"+key] = buf.Bytes()
	f.uploadedType[bucket+"/"+key] = contentType
	return nil
}

func (f *fakeReportStore) Remove(ctx context.Context, bucket, key string) error {
	if ctx.Err() != nil {
		f.orphaned = true
	}
	if f.removeErr != nil {
		return f.removeErr
	}
	f.removedKeys = append(f.removedKeys, bucket+"/"+key)
	return nil
}

type fakeReportQueue struct {
	published []task.ReportUploaded
	err       error

	onCancel func()
}

func (f *fakeReportQueue) Publish(_ context.Context, t task.ReportUploaded) error {
	if f.onCancel != nil {
		f.onCancel()
	}
	if f.err != nil {
		return f.err
	}
	f.published = append(f.published, t)
	return nil
}

type fakePortfolioClient struct {
	portfoliopb.PortfolioServiceClient
	createErr error

	created []*portfoliopb.CreateReportImportRequest
	updates []*portfoliopb.UpdateReportImportStatusRequest
	imports map[string]*portfoliopb.ReportImport
}

func (f *fakePortfolioClient) CreateReportImport(_ context.Context, in *portfoliopb.CreateReportImportRequest, _ ...grpc.CallOption) (*portfoliopb.ReportImport, error) {
	if f.createErr != nil {
		return nil, f.createErr
	}
	f.created = append(f.created, in)
	imp := &portfoliopb.ReportImport{
		Id: in.GetId(), PortfolioId: in.GetPortfolioId(), BrokerId: in.GetBrokerId(),
		Filename: in.GetFilename(), Status: "queued",
	}
	if f.imports == nil {
		f.imports = map[string]*portfoliopb.ReportImport{}
	}
	f.imports[in.GetId()] = imp
	return imp, nil
}

func (f *fakePortfolioClient) UpdateReportImportStatus(ctx context.Context, in *portfoliopb.UpdateReportImportStatusRequest, _ ...grpc.CallOption) (*portfoliopb.ReportImport, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	f.updates = append(f.updates, in)
	return &portfoliopb.ReportImport{Id: in.GetId(), Status: in.GetStatus(), Error: in.GetError()}, nil
}

func (f *fakePortfolioClient) GetReportImport(_ context.Context, in *portfoliopb.GetReportImportRequest, _ ...grpc.CallOption) (*portfoliopb.ReportImport, error) {
	imp, ok := f.imports[in.GetId()]
	if !ok {
		return nil, status.Error(codes.NotFound, "report import not found")
	}
	return imp, nil
}

const (
	testBucket     = "reports"
	testPortfolio  = "p-123"
	testUploadSize = 1 << 20
)

func newUploadRequest(t *testing.T, userID string, fieldname, filename string, content []byte) *http.Request {
	t.Helper()
	return newUploadRequestWithBroker(t, userID, fieldname, filename, content, "tinkoff")
}

func newUploadRequestWithBroker(t *testing.T, userID string, fieldname, filename string, content []byte, broker string) *http.Request {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	if broker != "" {
		if err := mw.WriteField("broker", broker); err != nil {
			t.Fatalf("write broker field: %v", err)
		}
	}
	fw, err := mw.CreateFormFile(fieldname, filename)
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := fw.Write(content); err != nil {
		t.Fatalf("write form file: %v", err)
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/portfolios/"+testPortfolio+"/reports", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.SetPathValue("id", testPortfolio)
	ctx := context.WithValue(req.Context(), userIDContextKey, userID)
	return req.WithContext(ctx)
}

func newTestReportsHandler(store ReportStore, queue ReportQueue, portfolio portfoliopb.PortfolioServiceClient) *ReportsHandler {
	return &ReportsHandler{
		Portfolio:       portfolio,
		UpstreamTimeout: 5 * time.Second,
		Store:           store,
		Queue:           queue,
		Bucket:          testBucket,
		MaxUploadBytes:  testUploadSize,
		Log:             testLogger(),
	}
}

func TestUpload_HappyPath_StoresQueuesAndReturns202(t *testing.T) {
	store := newFakeReportStore()
	queue := &fakeReportQueue{}
	client := &fakePortfolioClient{}
	h := newTestReportsHandler(store, queue, client)

	content := []byte("broker report bytes")
	req := newUploadRequest(t, "user-1", "file", "report.pdf", content)
	rec := httptest.NewRecorder()

	h.Upload(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202; body: %s", rec.Code, rec.Body.String())
	}
	if len(queue.published) != 1 {
		t.Fatalf("published %d tasks, want 1", len(queue.published))
	}
	pub := queue.published[0]
	if pub.UserID != "user-1" || pub.PortfolioID != testPortfolio {
		t.Errorf("published task = %+v, want user-1/%s", pub, testPortfolio)
	}
	if pub.Broker != "tinkoff" {
		t.Errorf("published broker = %q, want %q", pub.Broker, "tinkoff")
	}
	if pub.Bucket != testBucket {
		t.Errorf("published bucket = %q, want %q", pub.Bucket, testBucket)
	}

	if got := store.uploaded[testBucket+"/"+pub.ObjectKey]; !bytes.Equal(got, content) {
		t.Errorf("stored object = %q, want %q (key %q)", got, content, pub.ObjectKey)
	}
	if len(store.removedKeys) != 0 {
		t.Errorf("Remove called on the happy path: %v", store.removedKeys)
	}
	var resp reportImportResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.ID != pub.TaskID || resp.TaskID != pub.TaskID || resp.Status != "queued" {
		t.Errorf("response = %+v, want queued import with id = task id %q", resp, pub.TaskID)
	}
	if len(client.created) != 1 || client.created[0].GetId() != pub.TaskID || client.created[0].GetFilename() != "report.pdf" {
		t.Errorf("CreateReportImport calls = %v", client.created)
	}
}

func TestUpload_PublishFails_RollsBackStoredObject(t *testing.T) {
	store := newFakeReportStore()
	queue := &fakeReportQueue{err: errors.New("rabbit down")}
	client := &fakePortfolioClient{}
	h := newTestReportsHandler(store, queue, client)

	req := newUploadRequest(t, "user-1", "file", "report.pdf", []byte("bytes"))
	rec := httptest.NewRecorder()

	h.Upload(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502; body: %s", rec.Code, rec.Body.String())
	}
	if len(store.uploaded) != 1 {
		t.Fatalf("expected the object to have been stored first, got %v", store.uploaded)
	}
	if len(store.removedKeys) != 1 {
		t.Fatalf("removed = %v, want exactly the stored key", store.removedKeys)
	}
	var storedKey string
	for k := range store.uploaded {
		storedKey = k
	}
	if store.removedKeys[0] != storedKey {
		t.Errorf("removed key = %q, want the stored key %q", store.removedKeys[0], storedKey)
	}
	if store.orphaned {
		t.Error("rollback ran with a cancelled context")
	}
	if len(client.updates) != 1 || client.updates[0].GetStatus() != "failed" {
		t.Errorf("status updates = %v, want the import marked failed", client.updates)
	}
}

func TestUpload_StoreFails_MarksImportFailed(t *testing.T) {
	store := newFakeReportStore()
	store.uploadErr = errors.New("minio down")
	queue := &fakeReportQueue{}
	client := &fakePortfolioClient{}
	h := newTestReportsHandler(store, queue, client)

	rec := httptest.NewRecorder()
	h.Upload(rec, newUploadRequest(t, "user-1", "file", "report.pdf", []byte("bytes")))

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", rec.Code)
	}
	if len(queue.published) != 0 {
		t.Error("published a task for a file that wasn't stored")
	}
	if len(client.updates) != 1 || client.updates[0].GetStatus() != "failed" {
		t.Errorf("status updates = %v, want the import marked failed", client.updates)
	}
}

func TestUpload_PublishFailsAfterClientDisconnect_RollbackStillRuns(t *testing.T) {
	store := newFakeReportStore()
	ctx, cancel := context.WithCancel(context.Background())
	queue := &fakeReportQueue{err: errors.New("rabbit down"), onCancel: cancel}
	h := newTestReportsHandler(store, queue, &fakePortfolioClient{})

	req := newUploadRequest(t, "user-1", "file", "report.pdf", []byte("bytes")).WithContext(ctx)
	rec := httptest.NewRecorder()

	h.Upload(rec, req)

	if store.orphaned {
		t.Error("Remove saw a cancelled context - rollback is not detached from the request")
	}
	if len(store.removedKeys) != 1 {
		t.Errorf("removed = %v, want exactly one rollback despite cancelled request", store.removedKeys)
	}
	if rec.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", rec.Code)
	}
}

func TestUpload_PublishFailsAndRollbackFails_StillResponds502(t *testing.T) {
	store := newFakeReportStore()
	store.removeErr = errors.New("minio down too")
	queue := &fakeReportQueue{err: errors.New("rabbit down")}
	h := newTestReportsHandler(store, queue, &fakePortfolioClient{})

	req := newUploadRequest(t, "user-1", "file", "report.pdf", []byte("bytes"))
	rec := httptest.NewRecorder()

	h.Upload(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 502; body: %s", rec.Code, rec.Body.String())
	}
	if len(store.removedKeys) != 0 {
		t.Errorf("removed = %v, want none (Remove errored)", store.removedKeys)
	}
}

func TestUpload_PortfolioNotFound_404WithoutTouchingStore(t *testing.T) {
	store := newFakeReportStore()
	queue := &fakeReportQueue{}
	client := &fakePortfolioClient{createErr: status.Error(codes.NotFound, "portfolio not found")}
	h := newTestReportsHandler(store, queue, client)

	req := newUploadRequest(t, "user-1", "file", "report.pdf", []byte("bytes"))
	rec := httptest.NewRecorder()

	h.Upload(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
	if len(store.uploaded) != 0 || len(queue.published) != 0 {
		t.Errorf("store/queue touched on unauthorized upload: %v / %v", store.uploaded, queue.published)
	}
}

func TestUpload_PortfolioServiceUnavailable_502WithoutTouchingStore(t *testing.T) {
	store := newFakeReportStore()
	queue := &fakeReportQueue{}
	client := &fakePortfolioClient{createErr: status.Error(codes.Unavailable, "connection refused")}
	h := newTestReportsHandler(store, queue, client)

	req := newUploadRequest(t, "user-1", "file", "report.pdf", []byte("bytes"))
	rec := httptest.NewRecorder()

	h.Upload(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", rec.Code)
	}
	if len(store.uploaded) != 0 {
		t.Errorf("stored something while portfolio service was down: %v", store.uploaded)
	}
}

func TestUpload_MissingFileField_400(t *testing.T) {
	store := newFakeReportStore()
	queue := &fakeReportQueue{}
	h := newTestReportsHandler(store, queue, &fakePortfolioClient{})

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	if err := mw.WriteField("not-file", "x"); err != nil {
		t.Fatalf("write field: %v", err)
	}
	mw.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/portfolios/"+testPortfolio+"/reports", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.SetPathValue("id", testPortfolio)
	req = req.WithContext(context.WithValue(req.Context(), userIDContextKey, "user-1"))
	rec := httptest.NewRecorder()

	h.Upload(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
	if len(store.uploaded) != 0 || len(queue.published) != 0 {
		t.Error("store/queue touched on invalid form")
	}
}

func TestUpload_MissingBrokerField_400WithoutTouchingStore(t *testing.T) {
	store := newFakeReportStore()
	queue := &fakeReportQueue{}
	h := newTestReportsHandler(store, queue, &fakePortfolioClient{})

	req := newUploadRequestWithBroker(t, "user-1", "file", "report.pdf", []byte("bytes"), "")
	rec := httptest.NewRecorder()

	h.Upload(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400; body: %s", rec.Code, rec.Body.String())
	}
	if len(store.uploaded) != 0 || len(queue.published) != 0 {
		t.Error("store/queue touched on missing broker")
	}
}

func TestUpload_BrokerIsNormalized(t *testing.T) {
	store := newFakeReportStore()
	queue := &fakeReportQueue{}
	h := newTestReportsHandler(store, queue, &fakePortfolioClient{})

	req := newUploadRequestWithBroker(t, "user-1", "file", "report.pdf", []byte("bytes"), "  Тинькофф  Инвестиции ")
	rec := httptest.NewRecorder()

	h.Upload(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202; body: %s", rec.Code, rec.Body.String())
	}
	if got := queue.published[0].Broker; got != "тинькофф-инвестиции" {
		t.Errorf("published broker = %q, want %q", got, "тинькофф-инвестиции")
	}
}

func TestUpload_UnsupportedFormat_400WithMessage(t *testing.T) {
	store := newFakeReportStore()
	queue := &fakeReportQueue{}
	msg := "Неподдерживаемый формат файла. Нужен HTML"
	h := newTestReportsHandler(store, queue, &fakePortfolioClient{createErr: status.Error(codes.InvalidArgument, msg)})

	rec := httptest.NewRecorder()
	h.Upload(rec, newUploadRequest(t, "user-1", "file", "report.pdf", []byte("bytes")))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	var body errorResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body.Error != msg {
		t.Errorf("error = %q, want %q", body.Error, msg)
	}
	if len(store.uploaded) != 0 || len(queue.published) != 0 {
		t.Error("store/queue touched for a rejected file")
	}
}

func TestGetReportImport_OtherPortfolio404(t *testing.T) {
	client := &fakePortfolioClient{imports: map[string]*portfoliopb.ReportImport{
		"imp-1": {Id: "imp-1", PortfolioId: testPortfolio, Status: "done", TradesCreated: 3},
	}}
	h := newTestReportsHandler(newFakeReportStore(), &fakeReportQueue{}, client)

	get := func(portfolioID, id string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/portfolios/"+portfolioID+"/reports/"+id, nil)
		req.SetPathValue("id", portfolioID)
		req.SetPathValue("report_id", id)
		req = req.WithContext(context.WithValue(req.Context(), userIDContextKey, "user-1"))
		rec := httptest.NewRecorder()
		h.Get(rec, req)
		return rec
	}

	rec := get(testPortfolio, "imp-1")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var resp reportImportResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Status != "done" || resp.TradesCreated != 3 {
		t.Errorf("response = %+v", resp)
	}

	if rec := get("p-other", "imp-1"); rec.Code != http.StatusNotFound {
		t.Errorf("other portfolio: status = %d, want 404", rec.Code)
	}
	if rec := get(testPortfolio, "missing"); rec.Code != http.StatusNotFound {
		t.Errorf("missing import: status = %d, want 404", rec.Code)
	}
}

func TestBrokerIcon(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/static/brokers/ICONS.md", nil)
	req.SetPathValue("file", "ICONS.md")
	rec := httptest.NewRecorder()
	handleBrokerIcon(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("existing file: status = %d, want 200", rec.Code)
	}

	for _, name := range []string{"nope.png", "../static.go"} {
		req := httptest.NewRequest(http.MethodGet, "/static/brokers/x", nil)
		req.SetPathValue("file", name)
		rec := httptest.NewRecorder()
		handleBrokerIcon(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Errorf("%q: status = %d, want 404", name, rec.Code)
		}
	}
}

func TestSanitizeFilename(t *testing.T) {
	cases := []struct{ in, want string }{
		{"report.pdf", "report.pdf"},
		{"../../../etc/passwd", "passwd"},
		{`..\..\windows\system32\report.pdf`, "report.pdf"},
		{"   spaced name.pdf  ", "spaced name.pdf"},
		{"", "report"},
		{"   ", "report"},
		{"/", "report"},
	}
	for _, c := range cases {
		if got := sanitizeFilename(c.in); got != c.want {
			t.Errorf("sanitizeFilename(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
