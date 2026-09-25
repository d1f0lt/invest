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

	"invest/backend/services/gateway/internal/auth"
	portfoliopb "invest/backend/services/gateway/internal/portfoliopb"
	"invest/backend/services/gateway/internal/task"
)




type ReportStore interface {
	Upload(ctx context.Context, bucket, key string, r io.Reader, size int64, contentType string) error
	Remove(ctx context.Context, bucket, key string) error
}



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

	
	
	
	CleanupTimeout time.Duration

	Log *slog.Logger
}

func (h *ReportsHandler) Upload(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFromContext(r.Context())
	portfolioID := r.PathValue("id")

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
	filename := sanitizeFilename(header.Filename)

	
	
	
	
	imp, err := h.createImport(r.Context(), userID, &portfoliopb.CreateReportImportRequest{
		Id:          taskID,
		PortfolioId: portfolioID,
		BrokerId:    broker,
		Filename:    filename,
	})
	if err != nil {
		writeUpstreamError(w, h.Log, err)
		return
	}

	objectKey := fmt.Sprintf("%s/%s_%s", portfolioID, taskID, filename)

	if err := h.Store.Upload(r.Context(), h.Bucket, objectKey, file, header.Size, contentType); err != nil {
		h.Log.Error("upload report to minio", "error", err, "task_id", taskID)
		h.failImport(r.Context(), userID, taskID, "Не удалось сохранить файл. Попробуйте ещё раз")
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
		h.failImport(r.Context(), userID, taskID, "Не удалось поставить отчёт в очередь. Попробуйте ещё раз")
		writeError(w, http.StatusBadGateway, "failed to queue the report for parsing")
		return
	}

	h.Log.Info("report queued for parsing", "task_id", taskID, "portfolio_id", portfolioID, "user_id", userID, "broker", broker)
	writeJSON(w, http.StatusAccepted, toReportImportResponse(imp))
}



func (h *ReportsHandler) Get(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFromContext(r.Context())
	ctx, cancel := context.WithTimeout(r.Context(), h.UpstreamTimeout)
	defer cancel()
	ctx = auth.WithUserID(ctx, userID)

	imp, err := h.Portfolio.GetReportImport(ctx, &portfoliopb.GetReportImportRequest{Id: r.PathValue("report_id")})
	if err != nil {
		writeUpstreamError(w, h.Log, err)
		return
	}
	if imp.GetPortfolioId() != r.PathValue("id") {
		writeError(w, http.StatusNotFound, "report import not found")
		return
	}
	writeJSON(w, http.StatusOK, toReportImportResponse(imp))
}

func (h *ReportsHandler) List(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFromContext(r.Context())
	ctx, cancel := context.WithTimeout(r.Context(), h.UpstreamTimeout)
	defer cancel()
	ctx = auth.WithUserID(ctx, userID)

	resp, err := h.Portfolio.ListReportImports(ctx, &portfoliopb.ListReportImportsRequest{PortfolioId: r.PathValue("id")})
	if err != nil {
		writeUpstreamError(w, h.Log, err)
		return
	}
	out := make([]reportImportResponse, 0, len(resp.GetImports()))
	for _, imp := range resp.GetImports() {
		out = append(out, toReportImportResponse(imp))
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *ReportsHandler) createImport(reqCtx context.Context, userID string, req *portfoliopb.CreateReportImportRequest) (*portfoliopb.ReportImport, error) {
	ctx, cancel := context.WithTimeout(reqCtx, h.UpstreamTimeout)
	defer cancel()
	return h.Portfolio.CreateReportImport(auth.WithUserID(ctx, userID), req)
}




func (h *ReportsHandler) failImport(reqCtx context.Context, userID, taskID, message string) {
	timeout := h.CleanupTimeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(reqCtx), timeout)
	defer cancel()

	_, err := h.Portfolio.UpdateReportImportStatus(auth.WithUserID(ctx, userID), &portfoliopb.UpdateReportImportStatusRequest{
		Id:     taskID,
		Status: "failed",
		Error:  message,
	})
	if err != nil {
		h.Log.Error("mark report import failed", "error", err, "task_id", taskID)
	}
}

type reportImportResponse struct {
	ID          string `json:"id"`
	TaskID      string `json:"task_id"`
	PortfolioID string `json:"portfolio_id"`
	BrokerID    string `json:"broker_id"`
	Filename    string `json:"filename"`
	Status      string `json:"status"`
	Error       string `json:"error,omitempty"`

	TradesCreated         int32 `json:"trades_created"`
	TradesSkipped         int32 `json:"trades_skipped"`
	CashOperationsCreated int32 `json:"cash_operations_created"`
	CashOperationsSkipped int32 `json:"cash_operations_skipped"`

	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
}

func toReportImportResponse(r *portfoliopb.ReportImport) reportImportResponse {
	out := reportImportResponse{
		ID:                    r.GetId(),
		TaskID:                r.GetId(),
		PortfolioID:           r.GetPortfolioId(),
		BrokerID:              r.GetBrokerId(),
		Filename:              r.GetFilename(),
		Status:                r.GetStatus(),
		Error:                 r.GetError(),
		TradesCreated:         r.GetTradesCreated(),
		TradesSkipped:         r.GetTradesSkipped(),
		CashOperationsCreated: r.GetCashOperationsCreated(),
		CashOperationsSkipped: r.GetCashOperationsSkipped(),
		CreatedAt:             r.GetCreatedAt().AsTime(),
		UpdatedAt:             r.GetUpdatedAt().AsTime(),
	}
	if r.FinishedAt != nil {
		t := r.GetFinishedAt().AsTime()
		out.FinishedAt = &t
	}
	return out
}








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
