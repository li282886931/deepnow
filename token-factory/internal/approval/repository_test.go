package approval

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestMySQLApprovalRepositoryCreatesRequestAndSteps(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	now := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	request := ApprovalRequest{
		RequestID: "req_test", RequestType: "token_issue", AppID: "app_test",
		ApplicantID: "user_test", Payload: `{"scope":"read"}`, RiskLevel: "medium",
		Status: "pending", CreatedAt: now, UpdatedAt: now,
	}
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO approval_requests")).
		WithArgs(request.RequestID, request.RequestType, nil, request.AppID,
			request.ApplicantID, request.Payload, request.RiskLevel, request.Status,
			request.CreatedAt, request.UpdatedAt).
		WillReturnResult(sqlmock.NewResult(1, 1))
	if err := NewMySQLRepository(db).CreateRequest(context.Background(), request); err != nil {
		t.Fatalf("CreateRequest() error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("repository expectations: %v", err)
	}
}
