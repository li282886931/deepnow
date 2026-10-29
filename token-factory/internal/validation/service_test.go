package validation

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/li282886931/deepnow/token-factory/internal/token"
)

type validationStore struct {
	record     TokenRecord
	findErr    error
	findHash   string
	findCalls  int
	lookupByID map[string]TokenRecord
	getCalls   int
}

func (s *validationStore) FindByHash(_ context.Context, hash string) (TokenRecord, error) {
	s.findCalls++
	s.findHash = hash
	if s.findErr != nil {
		return TokenRecord{}, s.findErr
	}
	return s.record, nil
}

func (s *validationStore) Get(_ context.Context, tokenID string) (TokenRecord, error) {
	s.getCalls++
	record, ok := s.lookupByID[tokenID]
	if !ok {
		return TokenRecord{}, ErrTokenNotFound
	}
	return record, nil
}

type validationCache struct {
	record TokenRecord
	err    error
	set    int
}

func (c *validationCache) Get(context.Context, string) (TokenRecord, error) {
	if c.err != nil {
		return TokenRecord{}, c.err
	}
	return c.record, nil
}

func (c *validationCache) Set(context.Context, string, TokenRecord, time.Duration) error {
	c.set++
	return nil
}

type validationBlacklist struct {
	contains bool
	err      error
	calls    int
}

func (b *validationBlacklist) Contains(context.Context, string) (bool, error) {
	b.calls++
	return b.contains, b.err
}

type validationQuota struct {
	err     error
	tokenID string
	day     time.Time
	daily   int64
	qps     int64
}

func (q *validationQuota) Consume(_ context.Context, tokenID string, day time.Time, daily, qps int64) error {
	q.tokenID, q.day, q.daily, q.qps = tokenID, day, daily, qps
	return q.err
}

func activeRecord(expiresAt time.Time) TokenRecord {
	return TokenRecord{
		TokenID: "tok_1", AppID: "app_1", Status: "active", Env: "prod",
		Scopes: []string{"read", "write"}, AllowedIPs: []string{"10.0.0.0/8"},
		AllowedServices: []string{"billing"}, DailyLimit: 100, QPSLimit: 10,
		ExpiresAt: expiresAt,
	}
}

func TestValidateAllowsMatchingTokenPolicyAndQuota(t *testing.T) {
	now := time.Date(2026, 10, 29, 23, 30, 0, 0, time.UTC)
	store := &validationStore{record: activeRecord(now.Add(time.Hour))}
	quota := &validationQuota{}
	service := NewService(store, &validationCache{err: ErrCacheMiss}, &validationBlacklist{}, quota)
	service.Now = func() time.Time { return now }

	result := service.Validate(context.Background(), ValidateRequest{
		RawToken: "secret-value", RequiredScope: "read", CallerIP: "10.20.30.40",
		CallerService: "billing", Environment: "prod",
	})
	if !result.Allow || result.TokenID != "tok_1" || result.AppID != "app_1" {
		t.Fatalf("Validate() = %+v, want allowed token", result)
	}
	if store.findHash != token.HashSecret("secret-value") {
		t.Fatalf("lookup hash = %q, want SHA-256 token hash", store.findHash)
	}
	if !quota.day.Equal(now) || quota.daily != 100 || quota.qps != 10 {
		t.Fatalf("quota request = day %s daily=%d qps=%d", quota.day, quota.daily, quota.qps)
	}
}

func TestValidateRejectsPolicyFailuresWithStableReasons(t *testing.T) {
	now := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name   string
		record TokenRecord
		req    ValidateRequest
		reason string
	}{
		{"expired", activeRecord(now.Add(-time.Minute)), ValidateRequest{Environment: "prod"}, ReasonExpired},
		{"revoked", func() TokenRecord { r := activeRecord(now.Add(time.Hour)); r.Status = "revoked"; return r }(), ValidateRequest{Environment: "prod"}, ReasonRevoked},
		{"scope", activeRecord(now.Add(time.Hour)), ValidateRequest{RequiredScope: "admin", Environment: "prod"}, ReasonScopeDenied},
		{"ip", activeRecord(now.Add(time.Hour)), ValidateRequest{CallerIP: "192.0.2.10", Environment: "prod"}, ReasonIPDenied},
		{"environment", activeRecord(now.Add(time.Hour)), ValidateRequest{CallerIP: "10.20.30.40", Environment: "staging"}, ReasonEnvironmentDenied},
		{"service", activeRecord(now.Add(time.Hour)), ValidateRequest{CallerIP: "10.20.30.40", CallerService: "search", Environment: "prod"}, ReasonServiceDenied},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store := &validationStore{record: tc.record}
			service := NewService(store, &validationCache{err: ErrCacheMiss}, &validationBlacklist{}, &validationQuota{})
			service.Now = func() time.Time { return now }
			tc.req.RawToken = "secret-value"
			result := service.Validate(context.Background(), tc.req)
			if result.Allow || result.Reason != tc.reason {
				t.Fatalf("Validate() = %+v, want denied reason %q", result, tc.reason)
			}
		})
	}
}

func TestValidateRejectsUnknownHash(t *testing.T) {
	store := &validationStore{findErr: ErrTokenNotFound}
	service := NewService(store, &validationCache{err: ErrCacheMiss}, &validationBlacklist{}, &validationQuota{})
	result := service.Validate(context.Background(), ValidateRequest{RawToken: "unknown"})
	if result.Allow || result.Reason != ReasonUnknownToken {
		t.Fatalf("Validate() = %+v, want unknown token denial", result)
	}
}

func TestValidateChecksBlacklistBeforeCache(t *testing.T) {
	blacklist := &validationBlacklist{contains: true}
	cache := &validationCache{record: activeRecord(time.Now().Add(time.Hour))}
	service := NewService(&validationStore{}, cache, blacklist, &validationQuota{})
	result := service.Validate(context.Background(), ValidateRequest{RawToken: "secret-value"})
	if result.Allow || result.Reason != ReasonBlacklisted {
		t.Fatalf("Validate() = %+v, want blacklist denial", result)
	}
	if blacklist.calls != 1 || cache.set != 0 {
		t.Fatalf("blacklist/cache calls = %d/%d, want blacklist first", blacklist.calls, cache.set)
	}
}

func TestValidateFailsClosedWhenRedisDependencyFails(t *testing.T) {
	redisErr := errors.New("redis unavailable")
	t.Run("blacklist", func(t *testing.T) {
		service := NewService(&validationStore{record: activeRecord(time.Now().Add(time.Hour))},
			&validationCache{record: activeRecord(time.Now().Add(time.Hour))},
			&validationBlacklist{err: redisErr}, &validationQuota{})
		result := service.Validate(context.Background(), ValidateRequest{RawToken: "secret-value"})
		if result.Allow || result.Reason != ReasonDependencyUnavailable {
			t.Fatalf("Validate() = %+v, want fail-closed dependency denial", result)
		}
	})
	t.Run("quota", func(t *testing.T) {
		service := NewService(&validationStore{record: activeRecord(time.Now().Add(time.Hour))},
			&validationCache{err: ErrCacheMiss}, &validationBlacklist{},
			&validationQuota{err: redisErr})
		result := service.Validate(context.Background(), ValidateRequest{RawToken: "secret-value", CallerIP: "10.20.30.40", CallerService: "billing", Environment: "prod"})
		if result.Allow || result.Reason != ReasonDependencyUnavailable {
			t.Fatalf("Validate() = %+v, want fail-closed quota denial", result)
		}
	})
	t.Run("cache", func(t *testing.T) {
		service := NewService(&validationStore{record: activeRecord(time.Now().Add(time.Hour))},
			&validationCache{err: redisErr}, &validationBlacklist{}, &validationQuota{})
		result := service.Validate(context.Background(), ValidateRequest{
			RawToken: "secret-value", CallerIP: "10.20.30.40",
			CallerService: "billing", Environment: "prod",
		})
		if result.Allow || result.Reason != ReasonDependencyUnavailable {
			t.Fatalf("Validate() = %+v, want fail-closed cache denial", result)
		}
	})
}

func TestValidateRejectsQuotaExceeded(t *testing.T) {
	service := NewService(&validationStore{record: activeRecord(time.Now().Add(time.Hour))},
		&validationCache{err: ErrCacheMiss}, &validationBlacklist{},
		&validationQuota{err: ErrQuotaExceeded})
	result := service.Validate(context.Background(), ValidateRequest{RawToken: "secret-value", CallerIP: "10.20.30.40", CallerService: "billing", Environment: "prod"})
	if result.Allow || result.Reason != ReasonQuotaExceeded {
		t.Fatalf("Validate() = %+v, want quota denial", result)
	}
}

func TestValidateRejectsMalformedCallerIP(t *testing.T) {
	store := &validationStore{record: activeRecord(time.Now().Add(time.Hour))}
	service := NewService(store, &validationCache{err: ErrCacheMiss}, &validationBlacklist{}, &validationQuota{})
	result := service.Validate(context.Background(), ValidateRequest{
		RawToken: "secret-value", CallerIP: "not-an-ip", Environment: "prod",
	})
	if result.Allow || result.Reason != ReasonIPDenied {
		t.Fatalf("Validate() = %+v, want malformed IP denial", result)
	}
	if net.ParseIP("not-an-ip") != nil {
		t.Fatal("test IP unexpectedly parsed")
	}
}
