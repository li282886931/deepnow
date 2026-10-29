package mysql

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

var migrationStatements = []string{
	`CREATE TABLE IF NOT EXISTS schema_migrations (
		version VARCHAR(128) PRIMARY KEY,
		applied_at DATETIME(6) NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS applications (
		id BIGINT PRIMARY KEY AUTO_INCREMENT, app_id VARCHAR(64) NOT NULL UNIQUE,
		name VARCHAR(128) NOT NULL, description TEXT NOT NULL, owner_id VARCHAR(128) NOT NULL,
		department_id VARCHAR(128) NOT NULL, env VARCHAR(16) NOT NULL,
		data_level VARCHAR(32) NOT NULL, status VARCHAR(32) NOT NULL DEFAULT 'active',
		created_at DATETIME(6) NOT NULL, updated_at DATETIME(6) NOT NULL,
		deleted_at DATETIME(6) NULL, INDEX idx_app_owner (owner_id),
		INDEX idx_app_department (department_id)
	)`,
	`CREATE TABLE IF NOT EXISTS tokens (
		id BIGINT PRIMARY KEY AUTO_INCREMENT, token_id VARCHAR(64) NOT NULL UNIQUE,
		app_id VARCHAR(64) NOT NULL, token_type VARCHAR(32) NOT NULL,
		name VARCHAR(128) NOT NULL, status VARCHAR(32) NOT NULL, env VARCHAR(16) NOT NULL,
		scopes JSON NOT NULL, quota JSON NOT NULL, expires_at DATETIME(6) NOT NULL,
		created_by VARCHAR(128) NOT NULL, owner_id VARCHAR(128) NOT NULL,
		risk_level VARCHAR(16) NOT NULL, rotate_after DATETIME(6) NULL,
		last_used_at DATETIME(6) NULL, created_at DATETIME(6) NOT NULL,
		updated_at DATETIME(6) NOT NULL, deleted_at DATETIME(6) NULL,
		INDEX idx_tokens_app_status (app_id, status), INDEX idx_tokens_expires (expires_at),
		INDEX idx_tokens_risk (risk_level)
	)`,
	`CREATE TABLE IF NOT EXISTS token_versions (
		id BIGINT PRIMARY KEY AUTO_INCREMENT, token_id VARCHAR(64) NOT NULL, version INT NOT NULL,
		secret_ciphertext VARBINARY(4096) NOT NULL, secret_hash CHAR(64) NOT NULL,
		status VARCHAR(32) NOT NULL, valid_from DATETIME(6) NOT NULL,
		valid_until DATETIME(6) NULL, created_at DATETIME(6) NOT NULL,
		UNIQUE KEY uk_token_version (token_id, version), INDEX idx_secret_hash (secret_hash)
	)`,
	`CREATE TABLE IF NOT EXISTS approval_requests (
		id BIGINT PRIMARY KEY AUTO_INCREMENT, request_id VARCHAR(64) NOT NULL UNIQUE,
		request_type VARCHAR(32) NOT NULL, token_id VARCHAR(64) NULL,
		app_id VARCHAR(64) NOT NULL, applicant_id VARCHAR(128) NOT NULL,
		payload JSON NOT NULL, risk_level VARCHAR(16) NOT NULL, status VARCHAR(32) NOT NULL,
		created_at DATETIME(6) NOT NULL, updated_at DATETIME(6) NOT NULL,
		INDEX idx_approval_status (status, created_at)
	)`,
	`CREATE TABLE IF NOT EXISTS approval_steps (
		id BIGINT PRIMARY KEY AUTO_INCREMENT, request_id VARCHAR(64) NOT NULL,
		step_order INT NOT NULL, approver_type VARCHAR(32) NOT NULL, approver_id VARCHAR(128) NULL,
		status VARCHAR(32) NOT NULL, comment TEXT NOT NULL, handled_at DATETIME(6) NULL,
		UNIQUE KEY uk_approval_step (request_id, step_order)
	)`,
	`CREATE TABLE IF NOT EXISTS audit_logs (
		id BIGINT PRIMARY KEY AUTO_INCREMENT, request_id VARCHAR(64) NOT NULL,
		actor_id VARCHAR(128) NOT NULL, actor_type VARCHAR(32) NOT NULL,
		action VARCHAR(64) NOT NULL, resource_type VARCHAR(32) NOT NULL,
		resource_id VARCHAR(128) NOT NULL, before_json JSON NULL, after_json JSON NULL,
		result VARCHAR(16) NOT NULL, source_ip VARCHAR(64) NOT NULL, created_at DATETIME(6) NOT NULL,
		INDEX idx_audit_resource (resource_type, resource_id), INDEX idx_audit_created (created_at)
	)`,
}

func Migrate(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	rollback := func(err error) error {
		_ = tx.Rollback()
		return err
	}
	for _, statement := range migrationStatements {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return rollback(err)
		}
	}
	var version string
	err = tx.QueryRowContext(ctx,
		"SELECT version FROM schema_migrations WHERE version = ? FOR UPDATE", "001_initial",
	).Scan(&version)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO schema_migrations (version, applied_at) VALUES (?, ?)",
			"001_initial", time.Now().UTC(),
		); err != nil {
			return rollback(err)
		}
	case err != nil:
		return rollback(err)
	}
	return tx.Commit()
}
