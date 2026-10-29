package audit

import (
	"context"
	"database/sql"
	"time"
)

type AuditLog struct {
	ID           int64
	RequestID    string
	ActorID      string
	ActorType    string
	Action       string
	ResourceType string
	ResourceID   string
	BeforeJSON   []byte
	AfterJSON    []byte
	Result       string
	SourceIP     string
	CreatedAt    time.Time
}

type Repository interface {
	Create(context.Context, AuditLog) error
}

type mysqlRepository struct{ db *sql.DB }

func NewMySQLRepository(db *sql.DB) Repository { return &mysqlRepository{db: db} }

func (r *mysqlRepository) Create(ctx context.Context, log AuditLog) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO audit_logs
		(request_id, actor_id, actor_type, action, resource_type, resource_id,
		 before_json, after_json, result, source_ip, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		log.RequestID, log.ActorID, log.ActorType, log.Action, log.ResourceType,
		log.ResourceID, log.BeforeJSON, log.AfterJSON, log.Result, log.SourceIP, log.CreatedAt)
	return err
}
