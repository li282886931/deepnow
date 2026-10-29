package main

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	_ "modernc.org/sqlite"
)

func setupGatewayTestDB(t *testing.T) {
	t.Helper()

	var err error
	sysDB, err = sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sys db: %v", err)
	}
	keyDB, err = sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open key db: %v", err)
	}
	ragDB, err = sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open rag db: %v", err)
	}
	t.Cleanup(func() {
		sysDB.Close()
		keyDB.Close()
		ragDB.Close()
		authKeys = sync.Map{}
		generateEmbeddingForSearch = generateEmbedding
	})

	mustExec(t, sysDB, `CREATE TABLE model_configs (provider TEXT, api_type TEXT, api_key TEXT, model_name TEXT, ip_port TEXT, PRIMARY KEY(provider, api_type))`)
	mustExec(t, sysDB, `CREATE TABLE system_rules (key TEXT PRIMARY KEY, value TEXT)`)
	mustExec(t, keyDB, `CREATE TABLE access_keys (api_key TEXT PRIMARY KEY, used INTEGER, created_at DATETIME)`)
	mustExec(t, ragDB, `CREATE TABLE knowledge_index (id INTEGER PRIMARY KEY, chunk TEXT, source_file TEXT, embedding_json TEXT)`)
	if err := migrateSystemSchema(sysDB); err != nil {
		t.Fatalf("migrate system schema: %v", err)
	}
	if err := migrateKeySchema(keyDB); err != nil {
		t.Fatalf("migrate key schema: %v", err)
	}
	if err := migrateRAGSchema(ragDB); err != nil {
		t.Fatalf("migrate rag schema: %v", err)
	}
	mustExec(t, sysDB, `INSERT INTO model_configs (provider, api_type, api_key, model_name, ip_port) VALUES ('LM Studio', 'LM', 'local-key', 'local-model', '127.0.0.1:9')`)
	mustExec(t, sysDB, `INSERT INTO system_rules (key, value) VALUES ('active_lm_pri', 'LM Studio')`)
	mustExec(t, sysDB, `INSERT INTO system_rules (key, value) VALUES ('rag_slider', '0')`)
	mustExec(t, keyDB, `INSERT INTO access_keys (api_key, used, created_at) VALUES ('sk-test', 0, datetime('now'))`)
	authKeys.Store("sk-test", 0)
}

func mustExec(t *testing.T, db *sql.DB, query string) {
	t.Helper()
	if _, err := db.Exec(query); err != nil {
		t.Fatalf("exec %q: %v", query, err)
	}
}

func TestIncrementKeyUsageIsAtomic(t *testing.T) {
	setupGatewayTestDB(t)

	const calls = 50
	var wg sync.WaitGroup
	wg.Add(calls)
	for i := 0; i < calls; i++ {
		go func() {
			defer wg.Done()
			if err := incrementKeyUsage("sk-test"); err != nil {
				t.Errorf("incrementKeyUsage: %v", err)
			}
		}()
	}
	wg.Wait()

	used, ok := authKeys.Load("sk-test")
	if !ok {
		t.Fatal("key missing from cache")
	}
	if used.(int) != calls {
		t.Fatalf("cache used count = %d, want %d", used.(int), calls)
	}

	var dbUsed int
	if err := keyDB.QueryRow(`SELECT used FROM access_keys WHERE api_key='sk-test'`).Scan(&dbUsed); err != nil {
		t.Fatalf("read db count: %v", err)
	}
	if dbUsed != calls {
		t.Fatalf("db used count = %d, want %d", dbUsed, calls)
	}
}

func TestGatewayRejectsInvalidJSON(t *testing.T) {
	setupGatewayTestDB(t)

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader("{"))
	req.Header.Set("Authorization", "Bearer sk-test")
	w := httptest.NewRecorder()

	handleGatewayCompletions(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusBadRequest, w.Body.String())
	}
}

func TestGatewayStreamsUnknownWhenStrictRAGMisses(t *testing.T) {
	setupGatewayTestDB(t)
	generateEmbeddingForSearch = func(string, *ModelConfig) (string, error) {
		return `[1,0,0]`, nil
	}
	mustExec(t, sysDB, `INSERT OR REPLACE INTO model_configs (provider, api_type, api_key, model_name, ip_port) VALUES ('Jina AI', 'Embedding', 'embedding-key', 'jina-embeddings-v3', '')`)
	mustExec(t, sysDB, `INSERT OR REPLACE INTO system_rules (key, value) VALUES ('active_embedding', 'Jina AI')`)
	mustExec(t, sysDB, `INSERT OR REPLACE INTO system_rules (key, value) VALUES ('rag_slider', '2')`)

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"x","stream":true,"messages":[{"role":"user","content":"unknown"}]}`))
	req.Header.Set("Authorization", "Bearer sk-test")
	w := httptest.NewRecorder()

	handleGatewayCompletions(w, req)

	if got := w.Header().Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("content-type = %q, want text/event-stream", got)
	}
	body := w.Body.String()
	if !strings.Contains(body, `data: {"choices":[{"delta":{"content":"_UNKNOWN_"}}]}`) {
		t.Fatalf("stream body missing delta unknown: %s", body)
	}
	if !strings.Contains(body, "data: [DONE]") {
		t.Fatalf("stream body missing done: %s", body)
	}
}
