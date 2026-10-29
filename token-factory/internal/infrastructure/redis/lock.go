package redis

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"
)

var (
	ErrLockNotAcquired = errors.New("lock not acquired")
	ErrLockNotOwned    = errors.New("lock is owned by another caller")
)

type Locks struct {
	client *Client
}

type Lock struct {
	locks   *Locks
	tokenID string
	owner   string
}

func NewLocks(client *Client) *Locks {
	return &Locks{client: client}
}

func (l *Locks) Acquire(ctx context.Context, tokenID string, ttl time.Duration) (*Lock, error) {
	if ttl <= 0 {
		return nil, ErrInvalidTTL
	}
	owner, err := lockOwner()
	if err != nil {
		return nil, err
	}
	ok, err := l.client.rdb.SetNX(ctx, l.client.key("lock:token", tokenID), owner, ttl).Result()
	if err := redisError(err); err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrLockNotAcquired
	}
	return &Lock{locks: l, tokenID: tokenID, owner: owner}, nil
}

func (l *Locks) Release(ctx context.Context, tokenID, owner string) error {
	result, err := l.client.rdb.Eval(ctx, releaseLockScript, []string{l.client.key("lock:token", tokenID)}, owner).Int64()
	if err := redisError(err); err != nil {
		return err
	}
	if result == 0 {
		return ErrLockNotOwned
	}
	return nil
}

func (l *Lock) Release(ctx context.Context) error {
	return l.locks.Release(ctx, l.tokenID, l.owner)
}

func lockOwner() (string, error) {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes[:]), nil
}

var releaseLockScript = `
if redis.call("GET", KEYS[1]) == ARGV[1] then
  return redis.call("DEL", KEYS[1])
end
return 0
`
