package token

import "time"

type Status string

const (
	Draft           Status = StatusDraft
	PendingApproval Status = StatusPendingApproval
	Active          Status = StatusActive
	Rotating        Status = StatusRotating
	Expired         Status = StatusExpired
	Revoked         Status = StatusRevoked
)

type CreateRequest struct {
	ActorID        string
	IdempotencyKey string
	AppID          string
	Name           string
	TokenType      string
	Env            string
	Scopes         string
	Quota          string
	ExpiresAt      time.Time
	OwnerID        string
}

type RotateRequest struct {
	ActorID        string
	IdempotencyKey string
	GracePeriod    time.Duration
}

type ExtendRequest struct {
	ActorID        string
	IdempotencyKey string
	ExpiresAt      time.Time
}

type TokenMetadata struct {
	TokenID   string    `json:"token_id"`
	AppID     string    `json:"app_id"`
	Name      string    `json:"name"`
	TokenType string    `json:"token_type"`
	Status    Status    `json:"status"`
	Env       string    `json:"env"`
	Scopes    string    `json:"scopes"`
	Quota     string    `json:"quota"`
	ExpiresAt time.Time `json:"expires_at"`
	OwnerID   string    `json:"owner_id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Result struct {
	TokenID  string        `json:"token_id"`
	Secret   string        `json:"secret,omitempty"`
	Metadata TokenMetadata `json:"metadata"`
}
