package redis

import (
	"context"
	"errors"
	"fmt"
	"time"
)

var (
	ErrQuotaExceeded = errors.New("daily quota exceeded")
	ErrInvalidAmount = errors.New("counter amount must be positive")
)

type QuotaResult struct {
	Current int64
}

type Quota struct {
	client *Client
}

func NewQuota(client *Client) *Quota {
	return &Quota{client: client}
}

var incrementQuotaScript = `
local current = tonumber(redis.call("GET", KEYS[1]) or "0")
local amount = tonumber(ARGV[1])
local limit = tonumber(ARGV[2])
if limit > 0 and current + amount > limit then
  return -1
end
local next = redis.call("INCRBY", KEYS[1], amount)
if current == 0 then
  redis.call("EXPIRE", KEYS[1], ARGV[3])
end
return next
`

func (q *Quota) IncrementDaily(ctx context.Context, tokenID string, day time.Time, limit, amount int64) (QuotaResult, error) {
	if amount <= 0 {
		return QuotaResult{}, ErrInvalidAmount
	}
	key := q.client.key("quota:"+tokenID, day.UTC().Format("20060102"))
	result, err := q.client.rdb.Eval(ctx, incrementQuotaScript, []string{key}, amount, limit, int64((48*time.Hour)/time.Second)).Int64()
	if err := redisError(err); err != nil {
		return QuotaResult{}, err
	}
	if result == -1 {
		return QuotaResult{}, ErrQuotaExceeded
	}
	return QuotaResult{Current: result}, nil
}

var incrementRateScript = `
local next = redis.call("INCRBY", KEYS[1], ARGV[1])
if next == tonumber(ARGV[1]) then
  redis.call("EXPIRE", KEYS[1], ARGV[2])
end
return next
`

func (q *Quota) IncrementRate(ctx context.Context, tokenID, window string, ttl time.Duration, amount int64) (QuotaResult, error) {
	if amount <= 0 {
		return QuotaResult{}, ErrInvalidAmount
	}
	if ttl <= 0 {
		return QuotaResult{}, ErrInvalidTTL
	}
	key := q.client.key("rate:"+tokenID, window)
	result, err := q.client.rdb.Eval(ctx, incrementRateScript, []string{key}, amount, int64(ttl/time.Second)).Int64()
	if err := redisError(err); err != nil {
		return QuotaResult{}, fmt.Errorf("increment rate counter: %w", err)
	}
	return QuotaResult{Current: result}, nil
}
