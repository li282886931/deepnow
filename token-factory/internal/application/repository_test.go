package application

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestMySQLApplicationRepositoryCreatesAndGetsApplication(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	now := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	app := Application{
		AppID: "app_test", Name: "Orders", Description: "orders", OwnerID: "owner_test",
		DepartmentID: "dept_test", Env: "prod", DataLevel: "internal",
		Status: "active", CreatedAt: now, UpdatedAt: now,
	}
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO applications")).
		WithArgs(app.AppID, app.Name, app.Description, app.OwnerID, app.DepartmentID,
			app.Env, app.DataLevel, app.Status, app.CreatedAt, app.UpdatedAt, nil).
		WillReturnResult(sqlmock.NewResult(1, 1))
	repo := NewMySQLRepository(db)
	if err := repo.Create(context.Background(), app); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	mock.ExpectQuery(regexp.QuoteMeta("SELECT app_id, name")).
		WithArgs(app.AppID).
		WillReturnRows(sqlmock.NewRows([]string{
			"app_id", "name", "description", "owner_id", "department_id", "env",
			"data_level", "status", "created_at", "updated_at", "deleted_at",
		}).AddRow(app.AppID, app.Name, app.Description, app.OwnerID, app.DepartmentID,
			app.Env, app.DataLevel, app.Status, app.CreatedAt, app.UpdatedAt, nil))
	got, err := repo.Get(context.Background(), app.AppID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.AppID != app.AppID || got.OwnerID != app.OwnerID || got.Status != app.Status {
		t.Fatalf("Get() = %+v, want %+v", got, app)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("repository expectations: %v", err)
	}
}
