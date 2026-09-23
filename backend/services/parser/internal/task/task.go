package task

import "time"

type ReportUploaded struct {
	TaskID string `json:"task_id"`

	UserID string `json:"user_id"`

	PortfolioID string `json:"portfolio_id"`

	// Broker is the report's broker name, chosen by the user at upload
	// time and normalized by the gateway (lowercase, spaces -> "-").
	// The parsing dispatcher looks up the concrete parser by this key;
	// an empty or unknown broker fails the task (not requeued - the
	// task will never become parseable by retrying). Added 2026-09-23;
	// see architecture-decisions.md, "parser: асинхронный разбор
	// отчётов".
	Broker string `json:"broker"`

	Bucket    string `json:"bucket"`
	ObjectKey string `json:"object_key"`

	Filename    string `json:"filename,omitempty"`
	ContentType string `json:"content_type,omitempty"`

	UploadedAt time.Time `json:"uploaded_at"`
}
