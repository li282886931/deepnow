package approval

import (
	"context"
	"database/sql"
	"time"
)

type ApprovalRequest struct {
	ID          int64
	RequestID   string
	RequestType string
	TokenID     *string
	AppID       string
	ApplicantID string
	Payload     string
	RiskLevel   string
	Status      string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type ApprovalStep struct {
	ID           int64
	RequestID    string
	StepOrder    int
	ApproverType string
	ApproverID   *string
	Status       string
	Comment      string
	HandledAt    *time.Time
}

type ApprovalRepository interface {
	CreateRequest(context.Context, ApprovalRequest) error
	AddStep(context.Context, ApprovalStep) error
}

type mysqlRepository struct{ db *sql.DB }

func NewMySQLRepository(db *sql.DB) ApprovalRepository { return &mysqlRepository{db: db} }

func (r *mysqlRepository) CreateRequest(ctx context.Context, request ApprovalRequest) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO approval_requests
		(request_id, request_type, token_id, app_id, applicant_id, payload, risk_level, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		request.RequestID, request.RequestType, request.TokenID, request.AppID,
		request.ApplicantID, request.Payload, request.RiskLevel, request.Status,
		request.CreatedAt, request.UpdatedAt)
	return err
}

func (r *mysqlRepository) AddStep(ctx context.Context, step ApprovalStep) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO approval_steps
		(request_id, step_order, approver_type, approver_id, status, comment, handled_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, step.RequestID, step.StepOrder, step.ApproverType,
		step.ApproverID, step.Status, step.Comment, step.HandledAt)
	return err
}
