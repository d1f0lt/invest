package task

import "time"

type ReportUploaded struct {
	TaskID string `json:"task_id"`

	UserID string `json:"user_id"`

	PortfolioID string `json:"portfolio_id"`

	Bucket    string `json:"bucket"`
	ObjectKey string `json:"object_key"`

	Filename    string `json:"filename,omitempty"`
	ContentType string `json:"content_type,omitempty"`

	UploadedAt time.Time `json:"uploaded_at"`
}
