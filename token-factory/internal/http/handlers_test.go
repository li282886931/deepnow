package http

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type contractBackend struct {
	createTokenCalls int
}

func (b *contractBackend) CreateApp(context.Context, AppInput) (App, error) {
	return App{AppID: "app_1", Name: "Orders", OwnerID: "owner_1", Status: "active"}, nil
}
func (b *contractBackend) ListApps(_ context.Context, q PageQuery) (Page[App], error) {
	return Page[App]{Items: []App{{AppID: "app_1", Name: "Orders", OwnerID: "owner_1", Status: "active"}}, Total: 1, Page: q.Page, PageSize: q.PageSize}, nil
}
func (b *contractBackend) GetApp(context.Context, string) (App, error) {
	return App{AppID: "app_1", Name: "Orders", OwnerID: "owner_1", Status: "active"}, nil
}
func (b *contractBackend) UpdateApp(context.Context, string, AppInput) (App, error) {
	return App{AppID: "app_1", Name: "Orders updated", OwnerID: "owner_1", Status: "active"}, nil
}
func (b *contractBackend) CreateToken(context.Context, string, TokenInput) (TokenResult, error) {
	b.createTokenCalls++
	return TokenResult{TokenID: "tok_1", Status: "active", Secret: "secret-only-once", Metadata: TokenMetadataResponse{
		TokenID: "tok_1", AppID: "app_1", Status: "active",
	}}, nil
}
func (b *contractBackend) ListTokens(context.Context, PageQuery) (Page[TokenMetadataResponse], error) {
	return Page[TokenMetadataResponse]{Items: []TokenMetadataResponse{{TokenID: "tok_1", AppID: "app_1", Status: "active"}}, Total: 1}, nil
}
func (b *contractBackend) GetToken(context.Context, string) (TokenMetadataResponse, error) {
	return TokenMetadataResponse{TokenID: "tok_1", AppID: "app_1", Status: "active"}, nil
}
func (b *contractBackend) RotateToken(context.Context, string, string, RotateInput) (TokenResult, error) {
	return TokenResult{TokenID: "tok_1", Status: "active", Secret: "rotated-secret"}, nil
}
func (b *contractBackend) RevokeToken(context.Context, string, string, RevokeInput) (TokenMetadataResponse, error) {
	return TokenMetadataResponse{TokenID: "tok_1", AppID: "app_1", Status: "revoked"}, nil
}
func (b *contractBackend) ExtendToken(context.Context, string, string, ExtendInput) (TokenMetadataResponse, error) {
	return TokenMetadataResponse{TokenID: "tok_1", AppID: "app_1", Status: "active"}, nil
}
func (b *contractBackend) ListVersions(context.Context, string, PageQuery) (Page[TokenVersionResponse], error) {
	return Page[TokenVersionResponse]{Items: []TokenVersionResponse{{Version: 1, Status: "active"}}, Total: 1}, nil
}
func (b *contractBackend) GetUsage(context.Context, string, PageQuery) (Page[UsageResponse], error) {
	return Page[UsageResponse]{Items: []UsageResponse{{AllowedCalls: 1}}, Total: 1}, nil
}
func (b *contractBackend) ValidateToken(context.Context, ValidateInput) (ValidateResponse, error) {
	return ValidateResponse{Allow: true, Reason: "allowed", TokenID: "tok_1"}, nil
}
func (b *contractBackend) ListApprovals(context.Context, PageQuery) (Page[ApprovalResponse], error) {
	return Page[ApprovalResponse]{Items: []ApprovalResponse{{RequestID: "apr_1", Status: "pending"}}, Total: 1}, nil
}
func (b *contractBackend) GetApproval(context.Context, string) (ApprovalResponse, error) {
	return ApprovalResponse{RequestID: "apr_1", Status: "pending"}, nil
}
func (b *contractBackend) ActApproval(context.Context, string, string, ApprovalActionInput) error {
	return nil
}
func (b *contractBackend) ListAuditLogs(context.Context, PageQuery) (Page[AuditResponse], error) {
	return Page[AuditResponse]{Items: []AuditResponse{{Action: "token.create", After: `{"status":"active"}`}}, Total: 1}, nil
}
func (b *contractBackend) ListRiskEvents(context.Context, PageQuery) (Page[RiskResponse], error) {
	return Page[RiskResponse]{Items: []RiskResponse{{EventID: "risk_1", Level: "high"}}, Total: 1}, nil
}
func (b *contractBackend) ResolveRisk(context.Context, string, string, ResolveRiskInput) error {
	return nil
}
func (b *contractBackend) ListPolicies(context.Context, PageQuery) (Page[PolicyResponse], error) {
	return Page[PolicyResponse]{Items: []PolicyResponse{{PolicyID: "pol_1", Name: "default"}}, Total: 1}, nil
}
func (b *contractBackend) UpdatePolicy(context.Context, string, PolicyInput) (PolicyResponse, error) {
	return PolicyResponse{PolicyID: "pol_1", Name: "updated"}, nil
}

func newContractServer(backend Backend) *httptest.Server {
	return httptest.NewServer(NewRouter(RouterConfig{
		Backend: backend,
		Authenticator: StaticAuthenticator{Actors: map[string]Actor{
			"Bearer test-admin":  {ID: "admin_1", Type: "user", Roles: []string{"admin"}},
			"Bearer test-reader": {ID: "reader_1", Type: "user", Roles: []string{"reader"}},
		}},
	}))
}

func request(t *testing.T, client *http.Client, method, url, body, auth string) (*http.Response, map[string]any) {
	t.Helper()
	req, err := http.NewRequest(method, url, bytes.NewBufferString(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var envelope map[string]any
	if resp.StatusCode == http.StatusNoContent {
		resp.Body.Close()
		return resp, envelope
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		resp.Body.Close()
		t.Fatal(err)
	}
	resp.Body.Close()
	return resp, envelope
}

func TestHTTPContractRequiresAuthenticationAndReturnsRequestID(t *testing.T) {
	server := newContractServer(&contractBackend{})
	defer server.Close()

	resp, body := request(t, server.Client(), http.MethodGet, server.URL+"/api/v1/apps", "", "")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
	if resp.Header.Get("X-Request-ID") == "" || body["request_id"] == "" {
		t.Fatalf("request id missing: header=%q body=%v", resp.Header.Get("X-Request-ID"), body)
	}
	if body["success"] != false {
		t.Fatalf("body = %v, want failure envelope", body)
	}
}

func TestHTTPContractAppsPaginationAndJSONEnvelope(t *testing.T) {
	server := newContractServer(&contractBackend{})
	defer server.Close()

	resp, body := request(t, server.Client(), http.MethodGet, server.URL+"/api/v1/apps?page=2&page_size=10", "", "Bearer test-reader")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if body["request_id"] == nil || body["success"] != true || body["data"] == nil {
		t.Fatalf("body = %v, want success envelope", body)
	}
	data := body["data"].(map[string]any)
	if data["page"] != float64(2) || data["page_size"] != float64(10) || data["total"] != float64(1) {
		t.Fatalf("pagination = %v", data)
	}
}

func TestHTTPContractTokenSecretIsOnlyReturnedOnCreateAndRotate(t *testing.T) {
	server := newContractServer(&contractBackend{})
	defer server.Close()
	auth := "Bearer test-admin"

	resp, create := request(t, server.Client(), http.MethodPost, server.URL+"/api/v1/tokens", `{"app_id":"app_1","name":"orders","expires_at":"2027-01-01T00:00:00Z"}`, auth)
	if resp.StatusCode != http.StatusCreated || !containsJSON(create, "secret-only-once") {
		t.Fatalf("create status/body = %d/%v", resp.StatusCode, create)
	}
	_, detail := request(t, server.Client(), http.MethodGet, server.URL+"/api/v1/tokens/tok_1", "", auth)
	if containsJSON(detail, "secret") || containsJSON(detail, "secret-only-once") {
		t.Fatalf("detail leaked Secret: %v", detail)
	}
	_, list := request(t, server.Client(), http.MethodGet, server.URL+"/api/v1/tokens", "", auth)
	if containsJSON(list, "secret") || containsJSON(list, "secret-only-once") {
		t.Fatalf("list leaked Secret: %v", list)
	}
	_, audit := request(t, server.Client(), http.MethodGet, server.URL+"/api/v1/audit-logs", "", auth)
	if containsJSON(audit, "secret") || containsJSON(audit, "secret-only-once") {
		t.Fatalf("audit leaked Secret: %v", audit)
	}
}

func TestHTTPContractIdempotencyReplaysCreateAndRejectsConflictingPayload(t *testing.T) {
	backend := &contractBackend{}
	server := newContractServer(backend)
	defer server.Close()
	url := server.URL + "/api/v1/tokens"

	req, _ := http.NewRequest(http.MethodPost, url, strings.NewReader(`{"app_id":"app_1","name":"orders","expires_at":"2027-01-01T00:00:00Z"}`))
	req.Header.Set("Authorization", "Bearer test-admin")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "idem-1")
	first, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	firstBody := new(bytes.Buffer)
	_, _ = firstBody.ReadFrom(first.Body)
	first.Body.Close()

	req2, _ := http.NewRequest(http.MethodPost, url, strings.NewReader(`{"app_id":"app_1","name":"different","expires_at":"2027-01-01T00:00:00Z"}`))
	req2.Header.Set("Authorization", "Bearer test-admin")
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Idempotency-Key", "idem-1")
	second, err := server.Client().Do(req2)
	if err != nil {
		t.Fatal(err)
	}
	var secondBody map[string]any
	_ = json.NewDecoder(second.Body).Decode(&secondBody)
	second.Body.Close()
	if first.StatusCode != http.StatusCreated || second.StatusCode != http.StatusConflict || backend.createTokenCalls != 1 {
		t.Fatalf("statuses/calls = %d/%d/%d, body=%v", first.StatusCode, second.StatusCode, backend.createTokenCalls, secondBody)
	}
	if firstBody.Len() == 0 {
		t.Fatal("first response was empty")
	}
}

func TestHTTPContractRegistersAllResourceFamilies(t *testing.T) {
	server := newContractServer(&contractBackend{})
	defer server.Close()
	auth := "Bearer test-admin"
	routes := []struct {
		method string
		path   string
		body   string
		want   int
	}{
		{http.MethodPost, "/api/v1/apps", `{"name":"x"}`, http.StatusCreated},
		{http.MethodPatch, "/api/v1/apps/app_1", `{"name":"x"}`, http.StatusOK},
		{http.MethodPost, "/api/v1/tokens/tok_1/rotate", `{}`, http.StatusOK},
		{http.MethodPost, "/api/v1/tokens/tok_1/revoke", `{"reason":"test"}`, http.StatusOK},
		{http.MethodPost, "/api/v1/tokens/tok_1/extend", `{"expires_at":"2027-01-01T00:00:00Z"}`, http.StatusOK},
		{http.MethodGet, "/api/v1/tokens/tok_1/versions", "", http.StatusOK},
		{http.MethodGet, "/api/v1/tokens/tok_1/usage", "", http.StatusOK},
		{http.MethodPost, "/api/v1/tokens/validate", `{"token":"raw"}`, http.StatusOK},
		{http.MethodGet, "/api/v1/approvals", "", http.StatusOK},
		{http.MethodGet, "/api/v1/approvals/apr_1", "", http.StatusOK},
		{http.MethodPost, "/api/v1/approvals/apr_1/approve", `{}`, http.StatusNoContent},
		{http.MethodPost, "/api/v1/approvals/apr_1/reject", `{}`, http.StatusNoContent},
		{http.MethodPost, "/api/v1/approvals/apr_1/return", `{}`, http.StatusNoContent},
		{http.MethodGet, "/api/v1/risk-events", "", http.StatusOK},
		{http.MethodPost, "/api/v1/risk-events/risk_1/resolve", `{"resolution":"closed"}`, http.StatusNoContent},
		{http.MethodGet, "/api/v1/policies", "", http.StatusOK},
		{http.MethodPut, "/api/v1/policies/pol_1", `{"name":"x"}`, http.StatusOK},
	}
	for _, route := range routes {
		resp, _ := request(t, server.Client(), route.method, server.URL+route.path, route.body, auth)
		if resp.StatusCode != route.want {
			t.Errorf("%s %s status = %d, want %d", route.method, route.path, resp.StatusCode, route.want)
		}
	}
}

func TestHTTPContractMapsDomainErrorsToStatusCodes(t *testing.T) {
	backend := &errorBackend{err: ErrNotFound}
	server := newContractServer(backend)
	defer server.Close()
	resp, _ := request(t, server.Client(), http.MethodGet, server.URL+"/api/v1/tokens/missing", "", "Bearer test-admin")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("not found status = %d", resp.StatusCode)
	}
	backend.err = ErrConflict
	resp, _ = request(t, server.Client(), http.MethodGet, server.URL+"/api/v1/tokens/missing", "", "Bearer test-admin")
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("conflict status = %d", resp.StatusCode)
	}
	backend.err = ErrPolicyDenied
	resp, _ = request(t, server.Client(), http.MethodGet, server.URL+"/api/v1/tokens/missing", "", "Bearer test-admin")
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("policy status = %d", resp.StatusCode)
	}
	backend.err = ErrRateLimited
	resp, _ = request(t, server.Client(), http.MethodGet, server.URL+"/api/v1/tokens/missing", "", "Bearer test-admin")
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("rate status = %d", resp.StatusCode)
	}
	backend.err = ErrDependencyUnavailable
	resp, _ = request(t, server.Client(), http.MethodGet, server.URL+"/api/v1/tokens/missing", "", "Bearer test-admin")
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("dependency status = %d", resp.StatusCode)
	}
}

type errorBackend struct {
	contractBackend
	err error
}

func (b *errorBackend) GetToken(context.Context, string) (TokenMetadataResponse, error) {
	return TokenMetadataResponse{}, b.err
}

func containsJSON(value any, wanted string) bool {
	raw, _ := json.Marshal(value)
	return strings.Contains(strings.ToLower(string(raw)), strings.ToLower(wanted))
}
