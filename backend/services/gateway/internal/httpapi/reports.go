package httpapi

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"invest/backend/services/gateway/internal/auth"
	"invest/backend/services/gateway/internal/objectstore"
	portfoliopb "invest/backend/services/gateway/internal/portfoliopb"
	"invest/backend/services/gateway/internal/queue"
	"invest/backend/services/gateway/internal/task"
)

type ReportsHandler struct {
	Portfolio portfoliopb.PortfolioServiceClient

	UpstreamTimeout time.Duration

	Store  *objectstore.Store
	Queue  *queue.Publisher
	Bucket string

	MaxUploadBytes int64

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
		Bucket:      h.Bucket,
		ObjectKey:   objectKey,
		Filename:    header.Filename,
		ContentType: contentType,
		UploadedAt:  time.Now().UTC(),
	}
	if err := h.Queue.Publish(r.Context(), t); err != nil {
		h.Log.Error("publish report.uploaded task", "error", err, "task_id", taskID, "object_key", objectKey)
		writeError(w, http.StatusBadGateway, "failed to queue the report for parsing")
		return
	}

	h.Log.Info("report queued for parsing", "task_id", taskID, "portfolio_id", portfolioID, "user_id", userID)
	writeJSON(w, http.StatusAccepted, map[string]string{
		"task_id":      taskID,
		"portfolio_id": portfolioID,
		"filename":     header.Filename,
		"status":       "queued",
	})
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
