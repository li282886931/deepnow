package redis

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func newTestRedis(t *testing.T) (*miniredis.Miniredis, *Client) {
	t.Helper()

	server := miniredis.RunT(t)
	client := NewClient(redis.Options{Addr: server.Addr()}, "tf:")
	t.Cleanup(func() {
		_ = client.Close()
	})
	return server, client
}

func TestTokenCacheUsesTfPrefixAndExpires(t *testing.T) {
	server, client := newTestRedis(t)
	cache := NewTokenCache(client)
	ctx := context.Background()

	err := cache.Set(ctx, "hash-1", TokenCacheEntry{
		TokenID: "token-1",
		Status:  "active",
	}, time.Minute)
	if err != nil {
		t.Fatalf("set token cache: %v", err)
	}

	if !server.Exists("tf:token:cache:hash-1") {
		t.Fatal("expected token cache key with tf prefix")
	}
	if server.Exists("token:cache:hash-1") {
		t.Fatal("token cache key escaped tf prefix")
	}

	server.FastForward(time.Minute)
	_, err = cache.Get(ctx, "hash-1")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("get expired token cache: got %v, want ErrNotFound", err)
	}
}

func TestBlacklistTTLMatchesTokenExpiry(t *testing.T) {
	server, client := newTestRedis(t)
	blacklist := NewBlacklist(client)
	ctx := context.Background()

	expiresAt := time.Now().Add(90 * time.Second)
	if err := blacklist.Add(ctx, "hash-2", "revoked", expiresAt); err != nil {
		t.Fatalf("blacklist token: %v", err)
	}

	key := "tf:token:blacklist:hash-2"
	ttl := server.TTL(key)
	if ttl < 89*time.Second || ttl > 90*time.Second {
		t.Fatalf("blacklist ttl = %s, want approximately 90s", ttl)
	}
	blacklisted, err := blacklist.Contains(ctx, "hash-2")
	if err != nil {
		t.Fatalf("check blacklist: %v", err)
	}
	if !blacklisted {
		t.Fatal("expected token to be blacklisted")
	}
}

func TestQuotaIncrementsAtomically(t *testing.T) {
	server, client := newTestRedis(t)
	quota := NewQuota(client)
	ctx := context.Background()

	const workers = 20
	results := make(chan QuotaResult, workers)
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		go func() {
			result, err := quota.IncrementDaily(ctx, "token-3", time.Date(2026, 10, 29, 0, 0, 0, 0, time.UTC), 100, 1)
			results <- result
			errs <- err
		}()
	}

	var total int64
	for i := 0; i < workers; i++ {
		if err := <-errs; err != nil {
			t.Fatalf("increment quota: %v", err)
		}
	}
	for i := 0; i < workers; i++ {
		total += (<-results).Current
	}
	if total != 210 {
		t.Fatalf("sum of atomic quota results = %d, want 210", total)
	}
	if got, err := server.Get("tf:quota:token-3:20261029"); err != nil || got != "20" {
		t.Fatalf("stored quota = %q, want 20", got)
	}
}

func TestQuotaRejectsWhenDailyLimitExceeded(t *testing.T) {
	_, client := newTestRedis(t)
	quota := NewQuota(client)
	ctx := context.Background()
	day := time.Date(2026, 10, 29, 0, 0, 0, 0, time.UTC)

	if _, err := quota.IncrementDaily(ctx, "token-4", day, 2, 2); err != nil {
		t.Fatalf("first quota increment: %v", err)
	}
	_, err := quota.IncrementDaily(ctx, "token-4", day, 2, 1)
	if !errors.Is(err, ErrQuotaExceeded) {
		t.Fatalf("second quota increment: got %v, want ErrQuotaExceeded", err)
	}
}

func TestRateCounterIncrementsAtomically(t *testing.T) {
	server, client := newTestRedis(t)
	quota := NewQuota(client)
	ctx := context.Background()

	result, err := quota.IncrementRate(ctx, "token-5", "202610291530", 3*time.Second, 2)
	if err != nil {
		t.Fatalf("increment rate counter: %v", err)
	}
	if result.Current != 2 {
		t.Fatalf("rate count = %d, want 2", result.Current)
	}
	if got, err := server.Get("tf:rate:token-5:202610291530"); err != nil || got != "2" {
		t.Fatalf("stored rate count = %q, want 2", got)
	}
}

func TestLockCannotBeAcquiredTwice(t *testing.T) {
	_, client := newTestRedis(t)
	locks := NewLocks(client)
	ctx := context.Background()

	first, err := locks.Acquire(ctx, "token-6", time.Minute)
	if err != nil {
		t.Fatalf("acquire first lock: %v", err)
	}
	defer first.Release(ctx)

	second, err := locks.Acquire(ctx, "token-6", time.Minute)
	if !errors.Is(err, ErrLockNotAcquired) {
		t.Fatalf("acquire second lock: got %v, want ErrLockNotAcquired", err)
	}
	if second != nil {
		t.Fatal("second lock should be nil")
	}
}

func TestLockReleaseChecksOwnership(t *testing.T) {
	_, client := newTestRedis(t)
	locks := NewLocks(client)
	ctx := context.Background()

	lock, err := locks.Acquire(ctx, "token-7", time.Minute)
	if err != nil {
		t.Fatalf("acquire lock: %v", err)
	}
	if err := locks.Release(ctx, "token-7", "different-owner"); !errors.Is(err, ErrLockNotOwned) {
		t.Fatalf("release with different owner: got %v, want ErrLockNotOwned", err)
	}

	again, err := locks.Acquire(ctx, "token-7", time.Minute)
	if !errors.Is(err, ErrLockNotAcquired) || again != nil {
		t.Fatalf("different owner released lock: lock=%v err=%v", again, err)
	}
	if err := lock.Release(ctx); err != nil {
		t.Fatalf("release owned lock: %v", err)
	}
}

func TestRedisErrorsAreTyped(t *testing.T) {
	_, client := newTestRedis(t)
	if err := client.Close(); err != nil {
		t.Fatalf("close redis: %v", err)
	}

	err := NewBlacklist(client).Add(context.Background(), "hash-8", "revoked", time.Now().Add(time.Minute))
	if !errors.Is(err, ErrRedisUnavailable) {
		t.Fatalf("redis error = %v, want ErrRedisUnavailable", err)
	}
}
