package token

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestMySQLTokenRepositoryRoundTripsMetadata(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	now := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	expires := now.Add(24 * time.Hour)
	token := Token{
		TokenID:   "tok_test",
		AppID:     "app_test",
		TokenType: "service_token",
		Name:      "orders",
		Status:    StatusActive,
		Env:       "prod",
		Scopes:    `["order.read"]`,
		Quota:     `{"qps":100}`,
		ExpiresAt: expires,
		CreatedBy: "user_test",
		OwnerID:   "owner_test",
		RiskLevel: "medium",
		CreatedAt: now,
		UpdatedAt: now,
	}

	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO tokens")).
		WithArgs(
			token.TokenID, token.AppID, token.TokenType, token.Name, token.Status,
			token.Env, token.Scopes, token.Quota, token.ExpiresAt, token.CreatedBy,
			token.OwnerID, token.RiskLevel, nil, nil, token.CreatedAt, token.UpdatedAt,
		).
		WillReturnResult(sqlmock.NewResult(1, 1))
	if err := NewMySQLTokenRepository(db).Create(context.Background(), token); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	rows := sqlmock.NewRows([]string{
		"token_id", "app_id", "token_type", "name", "status", "env", "scopes",
		"quota", "expires_at", "created_by", "owner_id", "risk_level",
		"rotate_after", "last_used_at", "created_at", "updated_at",
	}).AddRow(
		token.TokenID, token.AppID, token.TokenType, token.Name, token.Status,
		token.Env, token.Scopes, token.Quota, token.ExpiresAt, token.CreatedBy,
		token.OwnerID, token.RiskLevel, nil, nil, token.CreatedAt, token.UpdatedAt,
	)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT token_id, app_id")).
		WithArgs(token.TokenID).
		WillReturnRows(rows)
	got, err := NewMySQLTokenRepository(db).Get(context.Background(), token.TokenID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.TokenID != token.TokenID || got.Status != token.Status ||
		got.Scopes != token.Scopes || got.Quota != token.Quota ||
		!got.ExpiresAt.Equal(token.ExpiresAt) || got.OwnerID != token.OwnerID {
		t.Fatalf("Get() = %+v, want metadata from %+v", got, token)
	}

	mock.ExpectExec(regexp.QuoteMeta("UPDATE tokens SET status = ?, updated_at = ?")).
		WithArgs(StatusExpired, sqlmock.AnyArg(), token.TokenID, StatusActive).
		WillReturnResult(sqlmock.NewResult(0, 1))
	if err := NewMySQLTokenRepository(db).UpdateStatus(
		context.Background(), token.TokenID, StatusActive, StatusExpired,
	); err != nil {
		t.Fatalf("UpdateStatus() error = %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("repository expectations: %v", err)
	}
}

func TestMySQLTokenRepositoryRejectsUnexpectedStatusTransition(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectExec(regexp.QuoteMeta("UPDATE tokens SET status = ?, updated_at = ?")).
		WithArgs(StatusExpired, sqlmock.AnyArg(), "tok_test", StatusActive).
		WillReturnResult(sqlmock.NewResult(0, 0))

	err = NewMySQLTokenRepository(db).UpdateStatus(
		context.Background(), "tok_test", StatusActive, StatusExpired,
	)
	if err != ErrTokenNotFoundOrStatusMismatch {
		t.Fatalf("UpdateStatus() error = %v, want %v", err, ErrTokenNotFoundOrStatusMismatch)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("repository expectations: %v", err)
	}
}

func TestMySQLTokenRepositoryListsVersions(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	now := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	rows := sqlmock.NewRows([]string{
		"id", "token_id", "version", "secret_ciphertext", "secret_hash",
		"status", "valid_from", "valid_until", "created_at",
	}).AddRow(1, "tok_test", 1, []byte("ciphertext"), "hash", StatusActive, now, nil, now)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, token_id, version")).
		WithArgs("tok_test").
		WillReturnRows(rows)

	versions, err := NewMySQLTokenRepository(db).ListVersions(context.Background(), "tok_test")
	if err != nil {
		t.Fatalf("ListVersions() error = %v", err)
	}
	if len(versions) != 1 || versions[0].Version != 1 ||
		string(versions[0].SecretCiphertext) != "ciphertext" {
		t.Fatalf("ListVersions() = %+v", versions)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("repository expectations: %v", err)
	}
}
