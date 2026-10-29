package redis

import (
	"context"
	"time"
)

type Blacklist struct {
	client *Client
}

func NewBlacklist(client *Client) *Blacklist {
	return &Blacklist{client: client}
}

func (b *Blacklist) Add(ctx context.Context, tokenHash, reason string, expiresAt time.Time) error {
	ttl := time.Until(expiresAt)
	if ttl <= 0 {
		return ErrInvalidTTL
	}
	return redisError(b.client.rdb.Set(ctx, b.client.key("token:blacklist", tokenHash), reason, ttl).Err())
}

func (b *Blacklist) Contains(ctx context.Context, tokenHash string) (bool, error) {
	exists, err := b.client.rdb.Exists(ctx, b.client.key("token:blacklist", tokenHash)).Result()
	if err := redisError(err); err != nil {
		return false, err
	}
	return exists == 1, nil
}
