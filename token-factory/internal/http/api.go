package http

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
)

var (
	ErrNotFound              = errors.New("resource not found")
	ErrConflict              = errors.New("conflict")
	ErrPolicyDenied          = errors.New("policy denied")
	ErrRateLimited           = errors.New("rate limited")
	ErrDependencyUnavailable = errors.New("dependency unavailable")
	ErrForbidden             = errors.New("forbidden")
	ErrInvalidRequest        = errors.New("invalid request")
)

type PageQuery struct {
	Page     int
	PageSize int
}

type Page[T any] struct {
	Items    []T `json:"items"`
	Total    int `json:"total"`
	Page     int `json:"page"`
	PageSize int `json:"page_size"`
}

type App struct {
	AppID        string `json:"app_id"`
	Name         string `json:"name"`
	Description  string `json:"description,omitempty"`
	OwnerID      string `json:"owner_id"`
	DepartmentID string `json:"department_id,omitempty"`
	Env          string `json:"env,omitempty"`
	DataLevel    string `json:"data_level,omitempty"`
	Status       string `json:"status"`
}

type AppInput struct {
	Name         string `json:"name"`
	Description  string `json:"description"`
	OwnerID      string `json:"owner_id"`
	DepartmentID string `json:"department_id"`
	Env          string `json:"env"`
	DataLevel    string `json:"data_level"`
}

type TokenInput struct {
	AppID       string         `json:"app_id"`
	Name        string         `json:"name"`
	TokenType   string         `json:"token_type"`
	Env         string         `json:"env"`
	Scopes      []string       `json:"scopes"`
	ExpiresAt   string         `json:"expires_at"`
	IPWhitelist []string       `json:"ip_whitelist"`
	Quota       map[string]int `json:"quota"`
	Reason      string         `json:"reason"`
}

type TokenMetadataResponse struct {
	TokenID   string `json:"token_id"`
	AppID     string `json:"app_id"`
	Name      string `json:"name,omitempty"`
	TokenType string `json:"token_type,omitempty"`
	Status    string `json:"status"`
	Env       string `json:"env,omitempty"`
	Scopes    any    `json:"scopes,omitempty"`
	Quota     any    `json:"quota,omitempty"`
	ExpiresAt string `json:"expires_at,omitempty"`
	OwnerID   string `json:"owner_id,omitempty"`
}

type TokenResult struct {
	TokenID   string                `json:"token_id"`
	Status    string                `json:"status"`
	RiskLevel string                `json:"risk_level,omitempty"`
	Secret    string                `json:"secret,omitempty"`
	Metadata  TokenMetadataResponse `json:"metadata,omitempty"`
}

type RotateInput struct {
	GracePeriodSeconds int `json:"grace_period_seconds"`
}
type RevokeInput struct {
	Reason string `json:"reason"`
}
type ExtendInput struct {
	ExpiresAt string `json:"expires_at"`
}
type TokenVersionResponse struct {
	Version int    `json:"version"`
	Status  string `json:"status"`
}
type UsageResponse struct {
	Day          string `json:"day,omitempty"`
	AllowedCalls int64  `json:"allowed_calls"`
	DeniedCalls  int64  `json:"denied_calls,omitempty"`
	TokenUnits   int64  `json:"token_units,omitempty"`
}
type ValidateInput struct {
	Token         string `json:"token"`
	RequiredScope string `json:"required_scope"`
	CallerIP      string `json:"caller_ip"`
	CallerService string `json:"caller_service"`
	Environment   string `json:"environment"`
}
type ValidateResponse struct {
	Allow     bool     `json:"allow"`
	Reason    string   `json:"reason"`
	TokenID   string   `json:"token_id,omitempty"`
	AppID     string   `json:"app_id,omitempty"`
	Scopes    []string `json:"scopes,omitempty"`
	ExpiresAt string   `json:"expires_at,omitempty"`
}
type ApprovalResponse struct {
	RequestID string `json:"request_id"`
	AppID     string `json:"app_id,omitempty"`
	TokenID   string `json:"token_id,omitempty"`
	Status    string `json:"status"`
	RiskLevel string `json:"risk_level,omitempty"`
}
type ApprovalActionInput struct {
	Comment     string `json:"comment"`
	NewApprover string `json:"new_approver"`
}
type AuditResponse struct {
	RequestID    string `json:"request_id,omitempty"`
	ActorID      string `json:"actor_id,omitempty"`
	Action       string `json:"action"`
	ResourceType string `json:"resource_type,omitempty"`
	ResourceID   string `json:"resource_id,omitempty"`
	After        string `json:"after,omitempty"`
	Result       string `json:"result,omitempty"`
}
type RiskResponse struct {
	EventID string `json:"event_id"`
	TokenID string `json:"token_id,omitempty"`
	Level   string `json:"level"`
	Score   int    `json:"score,omitempty"`
	Reason  string `json:"reason,omitempty"`
	Status  string `json:"status,omitempty"`
}
type ResolveRiskInput struct {
	Resolution string `json:"resolution"`
}
type PolicyResponse struct {
	PolicyID string `json:"policy_id"`
	Name     string `json:"name"`
	Status   string `json:"status,omitempty"`
	Config   any    `json:"config,omitempty"`
}
type PolicyInput struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Config any    `json:"config"`
}

type Backend interface {
	CreateApp(context.Context, AppInput) (App, error)
	ListApps(context.Context, PageQuery) (Page[App], error)
	GetApp(context.Context, string) (App, error)
	UpdateApp(context.Context, string, AppInput) (App, error)
	CreateToken(context.Context, string, TokenInput) (TokenResult, error)
	ListTokens(context.Context, PageQuery) (Page[TokenMetadataResponse], error)
	GetToken(context.Context, string) (TokenMetadataResponse, error)
	RotateToken(context.Context, string, string, RotateInput) (TokenResult, error)
	RevokeToken(context.Context, string, string, RevokeInput) (TokenMetadataResponse, error)
	ExtendToken(context.Context, string, string, ExtendInput) (TokenMetadataResponse, error)
	ListVersions(context.Context, string, PageQuery) (Page[TokenVersionResponse], error)
	GetUsage(context.Context, string, PageQuery) (Page[UsageResponse], error)
	ValidateToken(context.Context, ValidateInput) (ValidateResponse, error)
	ListApprovals(context.Context, PageQuery) (Page[ApprovalResponse], error)
	GetApproval(context.Context, string) (ApprovalResponse, error)
	ActApproval(context.Context, string, string, ApprovalActionInput) error
	ListAuditLogs(context.Context, PageQuery) (Page[AuditResponse], error)
	ListRiskEvents(context.Context, PageQuery) (Page[RiskResponse], error)
	ResolveRisk(context.Context, string, string, ResolveRiskInput) error
	ListPolicies(context.Context, PageQuery) (Page[PolicyResponse], error)
	UpdatePolicy(context.Context, string, PolicyInput) (PolicyResponse, error)
}

type Actor struct {
	ID    string
	Type  string
	Roles []string
}

type Authenticator interface {
	Authenticate(*http.Request) (Actor, bool)
}

type StaticAuthenticator struct {
	Actors map[string]Actor
}

func (a StaticAuthenticator) Authenticate(r *http.Request) (Actor, bool) {
	actor, ok := a.Actors[r.Header.Get("Authorization")]
	return actor, ok
}

type LocalAuthenticator struct{}

func (LocalAuthenticator) Authenticate(r *http.Request) (Actor, bool) {
	if strings.TrimSpace(r.Header.Get("Authorization")) == "" {
		return Actor{}, false
	}
	return Actor{ID: "local-admin", Type: "user", Roles: []string{"admin"}}, true
}

type RouterConfig struct {
	Backend       Backend
	Authenticator Authenticator
}

type actorKey struct{}
type requestIDKey struct{}
type requestBodyKey struct{}

func actorFromContext(ctx context.Context) Actor {
	actor, _ := ctx.Value(actorKey{}).(Actor)
	return actor
}
func requestIDFromContext(ctx context.Context) string {
	value, _ := ctx.Value(requestIDKey{}).(string)
	return value
}

func bodyFromContext(ctx context.Context) []byte {
	body, _ := ctx.Value(requestBodyKey{}).([]byte)
	return body
}

func parsePage(r *http.Request) PageQuery {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	size, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}
	return PageQuery{Page: page, PageSize: size}
}

func canWrite(actor Actor) bool {
	for _, role := range actor.Roles {
		if role == "admin" || role == "operator" {
			return true
		}
	}
	return false
}

func routeID(path, prefix string) string {
	return strings.Trim(strings.TrimPrefix(path, prefix), "/")
}
