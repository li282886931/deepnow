package validation

import (
	"context"
	"errors"
	"net"
	"strings"
	"time"

	"github.com/li282886931/deepnow/token-factory/internal/token"
)

var (
	ErrCacheMiss     = errors.New("validation cache miss")
	ErrTokenNotFound = errors.New("token not found")
	ErrQuotaExceeded = errors.New("quota exceeded")
	ErrDependency    = errors.New("validation dependency unavailable")
)

const (
	ReasonAllowed               = "allowed"
	ReasonUnknownToken          = "unknown_token"
	ReasonBlacklisted           = "blacklisted"
	ReasonExpired               = "expired"
	ReasonRevoked               = "revoked"
	ReasonScopeDenied           = "scope_denied"
	ReasonIPDenied              = "ip_denied"
	ReasonEnvironmentDenied     = "environment_denied"
	ReasonServiceDenied         = "service_denied"
	ReasonQuotaExceeded         = "quota_exceeded"
	ReasonDependencyUnavailable = "dependency_unavailable"
)

type ValidateRequest struct {
	RawToken      string
	RequiredScope string
	CallerIP      string
	CallerService string
	Environment   string
}

type ValidateResult struct {
	Allow     bool
	Reason    string
	TokenID   string
	AppID     string
	Scopes    []string
	ExpiresAt time.Time
}

type TokenRecord struct {
	TokenID         string
	AppID           string
	Status          string
	Env             string
	Scopes          []string
	AllowedIPs      []string
	AllowedServices []string
	DailyLimit      int64
	QPSLimit        int64
	ExpiresAt       time.Time
}

type TokenStore interface {
	FindByHash(context.Context, string) (TokenRecord, error)
	Get(context.Context, string) (TokenRecord, error)
}

type Cache interface {
	Get(context.Context, string) (TokenRecord, error)
	Set(context.Context, string, TokenRecord, time.Duration) error
}

type Blacklist interface {
	Contains(context.Context, string) (bool, error)
}

type Quota interface {
	Consume(context.Context, string, time.Time, int64, int64) error
}

type Service struct {
	store     TokenStore
	cache     Cache
	blacklist Blacklist
	quota     Quota
	Now       func() time.Time
}

func NewService(store TokenStore, cache Cache, blacklist Blacklist, quota Quota) *Service {
	return &Service{
		store: store, cache: cache, blacklist: blacklist, quota: quota,
		Now: func() time.Time { return time.Now().UTC() },
	}
}

func (s *Service) Validate(ctx context.Context, request ValidateRequest) ValidateResult {
	if strings.TrimSpace(request.RawToken) == "" {
		return denied(ReasonUnknownToken)
	}
	hash := token.HashSecret(request.RawToken)

	blacklisted, err := s.blacklist.Contains(ctx, hash)
	if err != nil {
		return denied(ReasonDependencyUnavailable)
	}
	if blacklisted {
		return denied(ReasonBlacklisted)
	}

	record, err := s.cache.Get(ctx, hash)
	if err != nil {
		if !errors.Is(err, ErrCacheMiss) {
			return denied(ReasonDependencyUnavailable)
		}
		record, err = s.store.FindByHash(ctx, hash)
		if err != nil {
			return s.storeFailure(err)
		}
	} else if record.TokenID == "" {
		return denied(ReasonUnknownToken)
	}

	now := s.Now().UTC()
	if record.Status != token.StatusActive {
		if record.Status == token.StatusRevoked {
			return denied(ReasonRevoked)
		}
		return denied(ReasonUnknownToken)
	}
	if !record.ExpiresAt.After(now) {
		return denied(ReasonExpired)
	}
	if request.RequiredScope != "" && !contains(record.Scopes, request.RequiredScope) {
		return denied(ReasonScopeDenied)
	}
	if !ipAllowed(request.CallerIP, record.AllowedIPs) {
		return denied(ReasonIPDenied)
	}
	if record.Env != "" && request.Environment != record.Env {
		return denied(ReasonEnvironmentDenied)
	}
	if len(record.AllowedServices) > 0 && !contains(record.AllowedServices, request.CallerService) {
		return denied(ReasonServiceDenied)
	}
	if err := s.quota.Consume(ctx, record.TokenID, now, record.DailyLimit, record.QPSLimit); err != nil {
		if errors.Is(err, ErrQuotaExceeded) {
			return denied(ReasonQuotaExceeded)
		}
		return denied(ReasonDependencyUnavailable)
	}

	_ = s.cache.Set(ctx, hash, record, cacheTTL(now, record.ExpiresAt))
	return ValidateResult{
		Allow: true, Reason: ReasonAllowed, TokenID: record.TokenID,
		AppID: record.AppID, Scopes: append([]string(nil), record.Scopes...),
		ExpiresAt: record.ExpiresAt,
	}
}

func (s *Service) storeFailure(err error) ValidateResult {
	if errors.Is(err, ErrTokenNotFound) {
		return denied(ReasonUnknownToken)
	}
	return denied(ReasonDependencyUnavailable)
}

func denied(reason string) ValidateResult {
	return ValidateResult{Allow: false, Reason: reason}
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func ipAllowed(raw string, allowlist []string) bool {
	if len(allowlist) == 0 {
		return true
	}
	ip := net.ParseIP(raw)
	if ip == nil {
		return false
	}
	for _, allowed := range allowlist {
		if allowedIP := net.ParseIP(allowed); allowedIP != nil && allowedIP.Equal(ip) {
			return true
		}
		if _, network, err := net.ParseCIDR(allowed); err == nil && network.Contains(ip) {
			return true
		}
	}
	return false
}

func cacheTTL(now, expiresAt time.Time) time.Duration {
	ttl := time.Minute
	if until := expiresAt.Sub(now); until < ttl {
		ttl = until
	}
	if ttl <= 0 {
		return time.Second
	}
	return ttl
}
