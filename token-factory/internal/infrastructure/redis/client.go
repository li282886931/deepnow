package redis

import (
	"errors"
	"fmt"
	"strings"

	"github.com/redis/go-redis/v9"
)

var (
	ErrNotFound         = errors.New("redis value not found")
	ErrRedisUnavailable = errors.New("redis unavailable")
	ErrInvalidTTL       = errors.New("redis ttl must be positive")
)

type Client struct {
	rdb    *redis.Client
	prefix string
}

func NewClient(options redis.Options, prefix string) *Client {
	return &Client{
		rdb:    redis.NewClient(&options),
		prefix: normalizePrefix(prefix),
	}
}

func Key(prefix, kind, id string) string {
	return prefix + kind + ":" + id
}

func (c *Client) key(kind, id string) string {
	return Key(c.prefix, kind, id)
}

func (c *Client) Close() error {
	return c.rdb.Close()
}

func redisError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, redis.Nil) {
		return ErrNotFound
	}
	return fmt.Errorf("%w: %v", ErrRedisUnavailable, err)
}

func normalizePrefix(prefix string) string {
	if strings.TrimSpace(prefix) == "" {
		return "tf:"
	}
	return prefix
}
