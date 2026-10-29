package token

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

var ErrTokenNotFoundOrStatusMismatch = errors.New("token not found or status mismatch")

const (
	StatusDraft           = "draft"
	StatusPendingApproval = "pending_approval"
	StatusActive          = "active"
	StatusRotating        = "rotating"
	StatusExpired         = "expired"
	StatusRevoked         = "revoked"
)

type Token struct {
	ID          int64
	TokenID     string
	AppID       string
	TokenType   string
	Name        string
	Status      string
	Env         string
	Scopes      string
	Quota       string
	ExpiresAt   time.Time
	CreatedBy   string
	OwnerID     string
	RiskLevel   string
	RotateAfter *time.Time
	LastUsedAt  *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
	DeletedAt   *time.Time
}

type TokenVersion struct {
	ID               int64
	TokenID          string
	Version          int
	SecretCiphertext []byte
	SecretHash       string
	Status           string
	ValidFrom        time.Time
	ValidUntil       *time.Time
	CreatedAt        time.Time
}

type TokenRepository interface {
	Create(context.Context, Token) error
	Get(context.Context, string) (Token, error)
	UpdateStatus(context.Context, string, string, string) error
	AddVersion(context.Context, TokenVersion) error
	ListVersions(context.Context, string) ([]TokenVersion, error)
}

type mysqlTokenRepository struct{ db *sql.DB }

func NewMySQLTokenRepository(db *sql.DB) TokenRepository {
	return &mysqlTokenRepository{db: db}
}

func (r *mysqlTokenRepository) Create(ctx context.Context, token Token) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO tokens
		(token_id, app_id, token_type, name, status, env, scopes, quota, expires_at,
		 created_by, owner_id, risk_level, rotate_after, last_used_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		token.TokenID, token.AppID, token.TokenType, token.Name, token.Status, token.Env,
		token.Scopes, token.Quota, token.ExpiresAt, token.CreatedBy, token.OwnerID,
		token.RiskLevel, token.RotateAfter, token.LastUsedAt, token.CreatedAt, token.UpdatedAt)
	return err
}

func (r *mysqlTokenRepository) Get(ctx context.Context, tokenID string) (Token, error) {
	var token Token
	err := r.db.QueryRowContext(ctx, `SELECT token_id, app_id, token_type, name, status, env,
		scopes, quota, expires_at, created_by, owner_id, risk_level, rotate_after, last_used_at,
		created_at, updated_at FROM tokens WHERE token_id = ? AND deleted_at IS NULL`, tokenID).Scan(
		&token.TokenID, &token.AppID, &token.TokenType, &token.Name, &token.Status, &token.Env,
		&token.Scopes, &token.Quota, &token.ExpiresAt, &token.CreatedBy, &token.OwnerID,
		&token.RiskLevel, &token.RotateAfter, &token.LastUsedAt, &token.CreatedAt, &token.UpdatedAt)
	return token, err
}

func (r *mysqlTokenRepository) UpdateStatus(ctx context.Context, tokenID, from, to string) error {
	result, err := r.db.ExecContext(ctx, `UPDATE tokens SET status = ?, updated_at = ?
		WHERE token_id = ? AND status = ? AND deleted_at IS NULL`,
		to, time.Now().UTC(), tokenID, from)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return ErrTokenNotFoundOrStatusMismatch
	}
	return nil
}

func (r *mysqlTokenRepository) AddVersion(ctx context.Context, version TokenVersion) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO token_versions
		(token_id, version, secret_ciphertext, secret_hash, status, valid_from, valid_until, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, version.TokenID, version.Version,
		version.SecretCiphertext, version.SecretHash, version.Status, version.ValidFrom,
		version.ValidUntil, version.CreatedAt)
	return err
}

func (r *mysqlTokenRepository) ListVersions(ctx context.Context, tokenID string) ([]TokenVersion, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, token_id, version, secret_ciphertext,
		secret_hash, status, valid_from, valid_until, created_at FROM token_versions
		WHERE token_id = ? ORDER BY version ASC`, tokenID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var versions []TokenVersion
	for rows.Next() {
		var version TokenVersion
		if err := rows.Scan(&version.ID, &version.TokenID, &version.Version,
			&version.SecretCiphertext, &version.SecretHash, &version.Status, &version.ValidFrom,
			&version.ValidUntil, &version.CreatedAt); err != nil {
			return nil, err
		}
		versions = append(versions, version)
	}
	return versions, rows.Err()
}
