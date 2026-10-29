package app

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLoadConfigRequiresDSNAndUsesSafeDefaults(t *testing.T) {
	t.Setenv("TOKEN_FACTORY_HTTP_ADDR", ":8090")
	t.Setenv("TOKEN_FACTORY_MYSQL_DSN", "")
	t.Setenv("TOKEN_FACTORY_SECRET_KEY", "test-secret-key")

	_, err := LoadConfig()
	if err == nil {
		t.Fatal("expected missing MySQL DSN error")
	}
}

func TestLoadConfigRequiresSecretKey(t *testing.T) {
	t.Setenv("TOKEN_FACTORY_MYSQL_DSN", "test-dsn")
	t.Setenv("TOKEN_FACTORY_SECRET_KEY", "")

	_, err := LoadConfig()
	if err == nil {
		t.Fatal("expected missing secret key error")
	}
}

func TestLoadConfigUsesSafeDefaults(t *testing.T) {
	t.Setenv("TOKEN_FACTORY_HTTP_ADDR", "")
	t.Setenv("TOKEN_FACTORY_MYSQL_DSN", "test-dsn")
	t.Setenv("TOKEN_FACTORY_REDIS_ADDR", "")
	t.Setenv("TOKEN_FACTORY_REDIS_PASSWORD", "")
	t.Setenv("TOKEN_FACTORY_REDIS_PREFIX", "")
	t.Setenv("TOKEN_FACTORY_SECRET_KEY", "test-secret-key")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if cfg.HTTPAddr != ":8090" {
		t.Fatalf("HTTPAddr = %q, want %q", cfg.HTTPAddr, ":8090")
	}
	if cfg.RedisAddr != "127.0.0.1:6379" {
		t.Fatalf("RedisAddr = %q, want %q", cfg.RedisAddr, "127.0.0.1:6379")
	}
	if cfg.RedisPrefix != "tf:" {
		t.Fatalf("RedisPrefix = %q, want %q", cfg.RedisPrefix, "tf:")
	}
}

func TestNewServerHealthz(t *testing.T) {
	cfg := Config{HTTPAddr: ":8090"}
	server, cleanup, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}
	t.Cleanup(func() {
		if err := cleanup(); err != nil {
			t.Errorf("cleanup() error = %v", err)
		}
	})

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	server.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got, want := rec.Body.String(), "{\"status\":\"ok\",\"service\":\"token-factory\"}\n"; got != want {
		t.Fatalf("body = %q, want %q", got, want)
	}
}
