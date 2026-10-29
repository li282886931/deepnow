package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSystemStatusNeverReturnsUpstreamAPIKey(t *testing.T) {
	setupAdminFeatureDB(t)
	seedPrimaryModel(t, "openai", "sk-live-secret", "gpt-test")

	response := serveAdminRequest(authenticatedRequest(http.MethodGet, "/api/system/status", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.Code)
	}
	if strings.Contains(response.Body.String(), "sk-live-secret") {
		t.Fatal("system status leaked upstream API key")
	}
}

func TestModelListNeverReturnsUpstreamAPIKey(t *testing.T) {
	setupAdminFeatureDB(t)
	seedPrimaryModel(t, "openai", "sk-live-secret", "gpt-test")

	response := serveAdminRequest(authenticatedRequest(http.MethodGet, "/api/models/get", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.Code)
	}
	if strings.Contains(response.Body.String(), "sk-live-secret") {
		t.Fatal("model list leaked upstream API key")
	}
}

func TestLogoutExpiresSessionAndSessionEndpointReflectsState(t *testing.T) {
	setupAdminFeatureDB(t)

	response := serveAdminRequest(authenticatedRequest(http.MethodPost, "/api/logout", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("logout status = %d, want 200", response.Code)
	}
	cookies := response.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("logout did not set a session cookie")
	}
	cookie := cookies[0]
	if cookie.Name != "session" || cookie.MaxAge >= 0 {
		t.Fatalf("logout cookie = %#v, want expired session cookie", cookie)
	}
	if cookie.HttpOnly != true || cookie.SameSite != http.SameSiteLaxMode {
		t.Fatalf("logout cookie flags = %#v, want HttpOnly and SameSite=Lax", cookie)
	}
	if sessionToken != "" {
		t.Fatalf("session token = %q, want empty after logout", sessionToken)
	}

	sessionResponse := serveAdminRequest(authenticatedRequest(http.MethodGet, "/api/session", nil))
	if sessionResponse.Code != http.StatusUnauthorized {
		t.Fatalf("session status = %d, want 401 after logout", sessionResponse.Code)
	}
}

func TestGenerateKeysReturnsPlaintextOnlyInCreationResponse(t *testing.T) {
	setupAdminFeatureDB(t)

	response := serveAdminRequest(authenticatedRequest(http.MethodPost, "/api/keys/generate", nil))
	var body struct {
		Success bool     `json:"success"`
		Data    []string `json:"data"`
	}
	decodeJSON(t, response, &body)
	if response.Code != http.StatusOK || !body.Success || len(body.Data) != 5 {
		t.Fatalf("unexpected generation response: status=%d body=%#v", response.Code, body)
	}

	list := serveAdminRequest(authenticatedRequest(http.MethodGet, "/api/keys/list", nil))
	if strings.Contains(list.Body.String(), body.Data[0]) {
		t.Fatal("key list returned plaintext generated key")
	}
}

func TestKeyListReturnsMetadataAndUsageNeverPlaintext(t *testing.T) {
	setupAdminFeatureDB(t)
	mustExec(t, keyDB, `INSERT INTO access_keys
		(api_key, used, created_at, name, status, last_used_at)
		VALUES ('sk-secret-key', 2, '2026-10-29T10:00:00Z', '研发环境', 'active', '2026-10-29T11:00:00Z')`)
	mustExec(t, keyDB, `INSERT INTO key_usage_daily
		(api_key, usage_date, request_count, input_tokens, output_tokens, error_count)
		VALUES ('sk-secret-key', '2026-10-29', 2, 12, 8, 1)`)

	response := serveAdminRequest(authenticatedRequest(http.MethodGet, "/api/keys/list", nil))
	if response.Code != http.StatusOK || strings.Contains(response.Body.String(), "sk-secret-key") {
		t.Fatalf("unexpected key list response: status=%d body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "研发环境") {
		t.Fatalf("key list omitted metadata: %s", response.Body.String())
	}

	usage := serveAdminRequest(authenticatedRequest(http.MethodGet, "/api/keys/usage?key_id=sk-s*****-key&range=7d", nil))
	if usage.Code != http.StatusOK || strings.Contains(usage.Body.String(), "sk-secret-key") {
		t.Fatalf("unexpected key usage response: status=%d body=%s", usage.Code, usage.Body.String())
	}
	if !strings.Contains(usage.Body.String(), `"input_tokens":12`) {
		t.Fatalf("key usage omitted counters: %s", usage.Body.String())
	}
}

func TestAdminP0P1EndpointsReturnStableEnvelopes(t *testing.T) {
	setupAdminFeatureDB(t)

	requests := []*http.Request{
		authenticatedRequest(http.MethodGet, "/api/rag/documents", nil),
		authenticatedRequest(http.MethodGet, "/api/diagnostics", nil),
		authenticatedRequest(http.MethodGet, "/api/audit/events", nil),
	}
	for _, request := range requests {
		response := serveAdminRequest(request)
		assertAdminEnvelope(t, response)
		if response.Code != http.StatusOK {
			t.Fatalf("%s %s returned %d", request.Method, request.URL.Path, response.Code)
		}
	}
}

func TestAdminEndpointsRejectMalformedInputWithValidationEnvelope(t *testing.T) {
	setupAdminFeatureDB(t)

	tests := []struct {
		name string
		req  *http.Request
	}{
		{
			name: "document id",
			req:  authenticatedRequest(http.MethodDelete, "/api/rag/documents/not-an-id", nil),
		},
		{
			name: "rag test json",
			req:  authenticatedJSONRequest(http.MethodPost, "/api/rag/test", bytes.NewBufferString("{")),
		},
		{
			name: "model test json",
			req:  authenticatedJSONRequest(http.MethodPost, "/api/models/test", bytes.NewBufferString("{}")),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := serveAdminRequest(test.req)
			assertAdminEnvelope(t, response)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400; body = %s", response.Code, response.Body.String())
			}
			var body struct {
				Success bool `json:"success"`
			}
			decodeJSON(t, response, &body)
			if body.Success {
				t.Fatal("validation response reported success")
			}
		})
	}
}

func TestSaveModelRejectsMissingRequiredFields(t *testing.T) {
	setupAdminFeatureDB(t)

	response := serveAdminRequest(authenticatedJSONRequest(
		http.MethodPost,
		"/api/models/save",
		bytes.NewBufferString(`{"provider":"","api_type":"LM","model_name":""}`),
	))
	assertAdminEnvelope(t, response)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", response.Code, response.Body.String())
	}
}

func assertAdminEnvelope(t *testing.T, response *httptest.ResponseRecorder) {
	t.Helper()
	var body map[string]interface{}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode envelope: %v; body = %s", err, response.Body.String())
	}
	for _, field := range []string{"success", "message", "data", "request_id"} {
		if _, ok := body[field]; !ok {
			t.Fatalf("envelope missing %q: %s", field, response.Body.String())
		}
	}
}
