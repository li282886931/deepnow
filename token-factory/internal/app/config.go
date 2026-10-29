package app

import (
	"errors"
	"os"
	"strings"
)

var (
	errMissingMySQLDSN  = errors.New("TOKEN_FACTORY_MYSQL_DSN is required")
	errMissingSecretKey = errors.New("TOKEN_FACTORY_SECRET_KEY is required")
)

type Config struct {
	HTTPAddr      string
	MySQLDSN      string
	RedisAddr     string
	RedisPassword string
	RedisPrefix   string
	SecretKey     []byte
}

func LoadConfig() (Config, error) {
	mysqlDSN := strings.TrimSpace(os.Getenv("TOKEN_FACTORY_MYSQL_DSN"))
	if mysqlDSN == "" {
		return Config{}, errMissingMySQLDSN
	}

	secretKey := strings.TrimSpace(os.Getenv("TOKEN_FACTORY_SECRET_KEY"))
	if secretKey == "" {
		return Config{}, errMissingSecretKey
	}

	return Config{
		HTTPAddr:      envOrDefault("TOKEN_FACTORY_HTTP_ADDR", ":8090"),
		MySQLDSN:      mysqlDSN,
		RedisAddr:     envOrDefault("TOKEN_FACTORY_REDIS_ADDR", "127.0.0.1:6379"),
		RedisPassword: os.Getenv("TOKEN_FACTORY_REDIS_PASSWORD"),
		RedisPrefix:   envOrDefault("TOKEN_FACTORY_REDIS_PREFIX", "tf:"),
		SecretKey:     []byte(secretKey),
	}, nil
}

func envOrDefault(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}
