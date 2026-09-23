package httpapi

import (
	"bytes"
	"context"
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

// --- fakes -----------------------------------------------------------------

type fakeReportStore struct {
	uploaded     map[string][]byte
	uploadedType map[string]string
	uploadErr    error

	removedKeys []string
	removeErr   error
	// orphaned is set when Remove is called with an already-cancelled
	// context: the rollback must run detached from the request context, so
	// in a correct handler this never happens.
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
	// onCancel, when set, is invoked at the start of Publish - used to
	// simulate the client disconnecting while the publish is in flight.
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

// fakePortfolioClient only implements GetPortfolio (used by verifyOwnership);
// every other method panics through the embedded nil interface, which is fine
// because the upload handler never calls them.
type fakePortfolioClient struct {
	portfoliopb.PortfolioServiceClient
	getPortfolioErr error
}

func (f *fakePortfolioClient) GetPortfolio(ctx context.Context, in *portfoliopb.GetPortfolioRequest, opts ...grpc.CallOption) (*portfoliopb.Portfolio, error) {
	if f.getPortfolioErr != nil {
		return nil, f.getPortfolioErr
	}
	return &portfoliopb.Portfolio{Id: in.GetId(), Name: "Основной"}, nil
}

// --- helpers ---------------------------------------------------------------

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

// --- tests -----------------------------------------------------------------

func TestUpload_HappyPath_StoresQueuesAndReturns202(t *testing.T) {
	store := newFakeReportStore()
	queue := &fakeReportQueue{}
	h := newTestReportsHandler(store, queue, &fakePortfolioClient{})

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
	// exactly one object stored under the published key
	if got := store.uploaded[testBucket+"/"+pub.ObjectKey]; !bytes.Equal(got, content) {
		t.Errorf("stored object = %q, want %q (key %q)", got, content, pub.ObjectKey)
	}
	if len(store.removedKeys) != 0 {
		t.Errorf("Remove called on the happy path: %v", store.removedKeys)
	}
}

func TestUpload_PublishFails_RollsBackStoredObject(t *testing.T) {
	store := newFakeReportStore()
	queue := &fakeReportQueue{err: errors.New("rabbit down")}
	h := newTestReportsHandler(store, queue, &fakePortfolioClient{})

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
}

func TestUpload_PublishFailsAfterClientDisconnect_RollbackStillRuns(t *testing.T) {
	store := newFakeReportStore()
	ctx, cancel := context.WithCancel(context.Background())
	queue := &fakeReportQueue{err: errors.New("rabbit down"), onCancel: cancel}
	h := newTestReportsHandler(store, queue, &fakePortfolioClient{})

	req := newUploadRequest(t, "user-1", "file", "report.pdf", []byte("bytes")).WithContext(ctx)
	rec := httptest.NewRecorder()

	h.Upload(rec, req)

	// The client's context was cancelled while publishing; the compensating
	// Remove must still have been attempted with a live (detached) context,
	// otherwise the object would be orphaned.
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

	// A failed rollback is logged (bucket+object_key for manual cleanup) but
	// must not change the client-facing response or panic.
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
	client := &fakePortfolioClient{getPortfolioErr: status.Error(codes.NotFound, "portfolio not found")}
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
	client := &fakePortfolioClient{getPortfolioErr: status.Error(codes.Unavailable, "connection refused")}
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

	// broker="" -> the field is not written to the form at all
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
