package main

// Shared test fixtures for the admin contract tests.

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

func setupAdminFeatureDB(t *testing.T) {
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
		sessionToken = ""
	})

	mustExec(t, sysDB, `CREATE TABLE admin_users (username TEXT PRIMARY KEY, password TEXT)`)
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
	sessionToken = "session-under-test"
}

func seedPrimaryModel(t *testing.T, provider, apiKey, modelName string) {
	t.Helper()
	mustExec(t, sysDB, `INSERT INTO model_configs (provider, api_type, api_key, model_name, ip_port) VALUES ('`+
		provider+`', 'LM', '`+apiKey+`', '`+modelName+`', '')`)
	mustExec(t, sysDB, `INSERT INTO system_rules (key, value) VALUES ('active_lm_pri', '`+provider+`')`)
}

func authenticatedRequest(method, path string, body io.Reader) *http.Request {
	req := httptest.NewRequest(method, path, body)
	req.AddCookie(&http.Cookie{Name: "session", Value: sessionToken})
	return req
}

func authenticatedJSONRequest(method, path string, body io.Reader) *http.Request {
	req := authenticatedRequest(method, path, body)
	req.Header.Set("Content-Type", "application/json")
	return req
}

func serveAdminRequest(r *http.Request) *httptest.ResponseRecorder {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/system/status", requireAdmin(handleGetSystemStatus))
	mux.HandleFunc("/api/models/get", requireAdmin(handleGetModelConfigs))
	mux.HandleFunc("/api/keys/generate", requireAdmin(handleGenerateKeys))
	mux.HandleFunc("/api/keys/list", requireAdmin(handleListKeys))
	mux.HandleFunc("/api/keys/usage", requireAdmin(handleKeyUsage))
	mux.HandleFunc("/api/models/save", requireAdmin(handleSaveModelConfig))
	mux.HandleFunc("/api/models/test", requireAdmin(handleModelTest))
	mux.HandleFunc("/api/rag/documents", requireAdmin(handleRAGDocuments))
	mux.HandleFunc("/api/rag/documents/", requireAdmin(handleDeleteRAGDocument))
	mux.HandleFunc("/api/rag/test", requireAdmin(handleRAGTest))
	mux.HandleFunc("/api/diagnostics", requireAdmin(handleDiagnostics))
	mux.HandleFunc("/api/audit/events", requireAdmin(handleAuditEvents))
	mux.HandleFunc("/api/logout", handleAdminLogout)
	mux.HandleFunc("/api/session", requireAdmin(handleAdminSession))
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, r)
	return response
}

func decodeJSON(t *testing.T, response *httptest.ResponseRecorder, target any) {
	t.Helper()
	if err := json.NewDecoder(bytes.NewReader(response.Body.Bytes())).Decode(target); err != nil {
		t.Fatalf("decode JSON response: %v; body = %s", err, response.Body.String())
	}
}
