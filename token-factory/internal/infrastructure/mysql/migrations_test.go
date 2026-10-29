package mysql

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestMigrateCreatesSchemaAndRecordsVersion(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	sqlText, err := os.ReadFile("../../../migrations/001_initial.sql")
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	for _, table := range []string{
		"schema_migrations",
		"applications",
		"tokens",
		"token_versions",
		"approval_requests",
		"approval_steps",
		"audit_logs",
	} {
		if !strings.Contains(string(sqlText), "CREATE TABLE IF NOT EXISTS "+table) {
			t.Errorf("migration does not create %s", table)
		}
	}

	mock.ExpectBegin()
	for _, table := range []string{
		"schema_migrations",
		"applications",
		"tokens",
		"token_versions",
		"approval_requests",
		"approval_steps",
		"audit_logs",
	} {
		mock.ExpectExec("CREATE TABLE IF NOT EXISTS " + table).
			WillReturnResult(sqlmock.NewResult(0, 0))
	}
	mock.ExpectQuery("SELECT version FROM schema_migrations WHERE version = \\?").
		WithArgs("001_initial").
		WillReturnError(sql.ErrNoRows)
	mock.ExpectExec("INSERT INTO schema_migrations \\(version, applied_at\\) VALUES \\(\\?, \\?\\)").
		WithArgs("001_initial", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	if err := Migrate(context.Background(), db); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
	mock.ExpectBegin()
	for _, table := range []string{
		"schema_migrations",
		"applications",
		"tokens",
		"token_versions",
		"approval_requests",
		"approval_steps",
		"audit_logs",
	} {
		mock.ExpectExec("CREATE TABLE IF NOT EXISTS " + table).
			WillReturnResult(sqlmock.NewResult(0, 0))
	}
	mock.ExpectQuery("SELECT version FROM schema_migrations WHERE version = \\?").
		WithArgs("001_initial").
		WillReturnRows(sqlmock.NewRows([]string{"version"}).AddRow("001_initial"))
	mock.ExpectCommit()
	if err := Migrate(context.Background(), db); err != nil {
		t.Fatalf("second Migrate() error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("migration expectations: %v", err)
	}
}
