package redis

import (
	"context"
	"encoding/json"
	"time"
)

type TokenCacheEntry struct {
	TokenID   string    `json:"token_id"`
	Version   int       `json:"version,omitempty"`
	Status    string    `json:"status"`
	Scopes    []string  `json:"scopes,omitempty"`
	AppID     string    `json:"app_id,omitempty"`
	ExpiresAt time.Time `json:"expires_at,omitempty"`
}

type TokenCache struct {
	client *Client
}

func NewTokenCache(client *Client) *TokenCache {
	return &TokenCache{client: client}
}

func (c *TokenCache) Set(ctx context.Context, tokenHash string, entry TokenCacheEntry, ttl time.Duration) error {
	if ttl <= 0 {
		return ErrInvalidTTL
	}
	payload, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	return redisError(c.client.rdb.Set(ctx, c.client.key("token:cache", tokenHash), payload, ttl).Err())
}

func (c *TokenCache) Get(ctx context.Context, tokenHash string) (TokenCacheEntry, error) {
	payload, err := c.client.rdb.Get(ctx, c.client.key("token:cache", tokenHash)).Bytes()
	if err := redisError(err); err != nil {
		return TokenCacheEntry{}, err
	}

	var entry TokenCacheEntry
	if err := json.Unmarshal(payload, &entry); err != nil {
		return TokenCacheEntry{}, err
	}
	return entry, nil
}

func (c *TokenCache) Delete(ctx context.Context, tokenHash string) error {
	return redisError(c.client.rdb.Del(ctx, c.client.key("token:cache", tokenHash)).Err())
}
