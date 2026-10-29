package application

import (
	"context"
	"database/sql"
	"strings"
	"time"
)

type Application struct {
	ID           int64
	AppID        string
	Name         string
	Description  string
	OwnerID      string
	DepartmentID string
	Env          string
	DataLevel    string
	Status       string
	CreatedAt    time.Time
	UpdatedAt    time.Time
	DeletedAt    *time.Time
}

type ApplicationFilter struct {
	OwnerID string
	Status  string
	Limit   int
	Offset  int
}

type Page[T any] struct {
	Items  []T
	Total  int
	Limit  int
	Offset int
}

type ApplicationRepository interface {
	Create(context.Context, Application) error
	Get(context.Context, string) (Application, error)
	List(context.Context, ApplicationFilter) (Page[Application], error)
}

type mysqlRepository struct{ db *sql.DB }

func NewMySQLRepository(db *sql.DB) ApplicationRepository { return &mysqlRepository{db: db} }

func (r *mysqlRepository) Create(ctx context.Context, app Application) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO applications
		(app_id, name, description, owner_id, department_id, env, data_level, status, created_at, updated_at, deleted_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		app.AppID, app.Name, app.Description, app.OwnerID, app.DepartmentID, app.Env,
		app.DataLevel, app.Status, app.CreatedAt, app.UpdatedAt, app.DeletedAt)
	return err
}

func (r *mysqlRepository) Get(ctx context.Context, appID string) (Application, error) {
	var app Application
	err := r.db.QueryRowContext(ctx, `SELECT app_id, name, description, owner_id, department_id,
		env, data_level, status, created_at, updated_at, deleted_at
		FROM applications WHERE app_id = ? AND deleted_at IS NULL`, appID).Scan(
		&app.AppID, &app.Name, &app.Description, &app.OwnerID, &app.DepartmentID,
		&app.Env, &app.DataLevel, &app.Status, &app.CreatedAt, &app.UpdatedAt, &app.DeletedAt)
	return app, err
}

func (r *mysqlRepository) List(ctx context.Context, filter ApplicationFilter) (Page[Application], error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	if filter.Offset < 0 {
		filter.Offset = 0
	}
	conditions := []string{"deleted_at IS NULL"}
	args := make([]any, 0, 2)
	if filter.OwnerID != "" {
		conditions = append(conditions, "owner_id = ?")
		args = append(args, filter.OwnerID)
	}
	if filter.Status != "" {
		conditions = append(conditions, "status = ?")
		args = append(args, filter.Status)
	}
	where := strings.Join(conditions, " AND ")

	var total int
	countArgs := append([]any(nil), args...)
	if err := r.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM applications WHERE "+where, countArgs...,
	).Scan(&total); err != nil {
		return Page[Application]{}, err
	}
	rows, err := r.db.QueryContext(ctx, `SELECT app_id, name, description, owner_id, department_id,
		env, data_level, status, created_at, updated_at, deleted_at FROM applications
		WHERE `+where+` ORDER BY created_at DESC LIMIT ? OFFSET ?`,
		append(args, limit, filter.Offset)...)
	if err != nil {
		return Page[Application]{}, err
	}
	defer rows.Close()
	page := Page[Application]{Limit: limit, Offset: filter.Offset, Total: total}
	for rows.Next() {
		var app Application
		if err := rows.Scan(&app.AppID, &app.Name, &app.Description, &app.OwnerID,
			&app.DepartmentID, &app.Env, &app.DataLevel, &app.Status, &app.CreatedAt,
			&app.UpdatedAt, &app.DeletedAt); err != nil {
			return Page[Application]{}, err
		}
		page.Items = append(page.Items, app)
	}
	return page, rows.Err()
}
