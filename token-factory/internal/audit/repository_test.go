package audit

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestMySQLAuditRepositoryWritesAuditLog(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	log := AuditLog{
		RequestID: "req_test", ActorID: "user_test", ActorType: "user",
		Action: "token.create", ResourceType: "token", ResourceID: "tok_test",
		BeforeJSON: nil, AfterJSON: []byte(`{"status":"active"}`), Result: "success",
		SourceIP: "127.0.0.1", CreatedAt: time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC),
	}
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO audit_logs")).
		WithArgs(log.RequestID, log.ActorID, log.ActorType, log.Action,
			log.ResourceType, log.ResourceID, log.BeforeJSON, log.AfterJSON,
			log.Result, log.SourceIP, log.CreatedAt).
		WillReturnResult(sqlmock.NewResult(1, 1))
	if err := NewMySQLRepository(db).Create(context.Background(), log); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("repository expectations: %v", err)
	}
}
