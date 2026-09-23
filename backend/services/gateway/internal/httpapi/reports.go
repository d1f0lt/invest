package httpapi

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"invest/backend/services/gateway/internal/auth"
	portfoliopb "invest/backend/services/gateway/internal/portfoliopb"
	"invest/backend/services/gateway/internal/task"
)

// ReportStore is the object-storage side of the upload flow (implemented by
// *objectstore.Store). An interface - and not the concrete MinIO type - so
// this handler can be unit-tested without a live bucket.
type ReportStore interface {
	Upload(ctx context.Context, bucket, key string, r io.Reader, size int64, contentType string) error
	Remove(ctx context.Context, bucket, key string) error
}

// ReportQueue is the task-publishing side of the upload flow (implemented by
// *queue.Publisher).
type ReportQueue interface {
	Publish(ctx context.Context, t task.ReportUploaded) error
}

type ReportsHandler struct {
	Portfolio portfoliopb.PortfolioServiceClient

	UpstreamTimeout time.Duration

	Store  ReportStore
	Queue  ReportQueue
	Bucket string

	MaxUploadBytes int64

	// CleanupTimeout bounds the compensating object deletion after a failed
	// publish. Separate from UpstreamTimeout: it runs on a request-detached
	// context, so the request's own deadline must not apply.
	CleanupTimeout time.Duration

	Log *slog.Logger
}

func (h *ReportsHandler) Upload(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFromContext(r.Context())
	portfolioID := r.PathValue("id")

	if !h.verifyOwnership(w, r.Context(), userID, portfolioID) {
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, h.MaxUploadBytes)
	if err := r.ParseMultipartForm(h.MaxUploadBytes); err != nil {
		writeError(w, http.StatusBadRequest, "invalid multipart form, or file exceeds the upload size limit")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, `missing "file" form field`)
		return
	}
	defer file.Close()

	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	broker := normalizeBroker(r.FormValue("broker"))
	if broker == "" {
		writeError(w, http.StatusBadRequest, `missing "broker" form field`)
		return
	}

	taskID := uuid.NewString()

	objectKey := fmt.Sprintf("%s/%s_%s", portfolioID, taskID, sanitizeFilename(header.Filename))

	if err := h.Store.Upload(r.Context(), h.Bucket, objectKey, file, header.Size, contentType); err != nil {
		h.Log.Error("upload report to minio", "error", err, "task_id", taskID)
		writeError(w, http.StatusBadGateway, "failed to store the report")
		return
	}

	t := task.ReportUploaded{
		TaskID:      taskID,
		UserID:      userID,
		PortfolioID: portfolioID,
		Broker:      broker,
		Bucket:      h.Bucket,
		ObjectKey:   objectKey,
		Filename:    header.Filename,
		ContentType: contentType,
		UploadedAt:  time.Now().UTC(),
	}
	if err := h.Queue.Publish(r.Context(), t); err != nil {
		h.Log.Error("publish report.uploaded task", "error", err, "task_id", taskID, "object_key", objectKey)
		h.rollbackUpload(r.Context(), objectKey, taskID)
		writeError(w, http.StatusBadGateway, "failed to queue the report for parsing")
		return
	}

	h.Log.Info("report queued for parsing", "task_id", taskID, "portfolio_id", portfolioID, "user_id", userID, "broker", broker)
	writeJSON(w, http.StatusAccepted, map[string]string{
		"task_id":      taskID,
		"portfolio_id": portfolioID,
		"filename":     header.Filename,
		"broker":       broker,
		"status":       "queued",
	})
}

// rollbackUpload compensates a failed publish by deleting the object that was
// already stored, so a rejected upload never leaves an orphan in MinIO. The
// context is deliberately detached from the request (WithoutCancel): by the
// time the publish failed, the client may be gone or the request deadline
// expired, and a cancelled context would abort the cleanup itself. Best
// effort - if the removal also fails, the object is logged as a manual
// cleanup candidate (bucket + object_key) instead of being retried here.
func (h *ReportsHandler) rollbackUpload(reqCtx context.Context, objectKey, taskID string) {
	timeout := h.CleanupTimeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(reqCtx), timeout)
	defer cancel()

	if err := h.Store.Remove(ctx, h.Bucket, objectKey); err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			h.Log.Error("rollback of report upload timed out, object orphaned - delete manually",
				"error", err, "task_id", taskID, "bucket", h.Bucket, "object_key", objectKey)
			return
		}
		h.Log.Error("rollback of report upload failed, object orphaned - delete manually",
			"error", err, "task_id", taskID, "bucket", h.Bucket, "object_key", objectKey)
		return
	}
	h.Log.Warn("rolled back report upload after failed publish", "task_id", taskID, "object_key", objectKey)
}

func (h *ReportsHandler) verifyOwnership(w http.ResponseWriter, ctx context.Context, userID, portfolioID string) bool {
	ctx, cancel := context.WithTimeout(ctx, h.UpstreamTimeout)
	defer cancel()
	ctx = auth.WithUserID(ctx, userID)

	_, err := h.Portfolio.GetPortfolio(ctx, &portfoliopb.GetPortfolioRequest{Id: portfolioID})
	if err == nil {
		return true
	}

	if status.Code(err) == codes.NotFound {
		writeError(w, http.StatusNotFound, "portfolio not found")
		return false
	}
	h.Log.Error("check portfolio ownership", "error", err)
	writeError(w, http.StatusBadGateway, "portfolio service unavailable")
	return false
}

func normalizeBroker(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	fields := strings.Fields(name)
	return strings.Join(fields, "-")
}

func sanitizeFilename(name string) string {
	name = strings.ReplaceAll(name, "\\", "/")
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = "report"
	}
	return name
}
