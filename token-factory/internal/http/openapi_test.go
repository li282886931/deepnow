package http

import (
	"os"
	"strings"
	"testing"
)

func TestOpenAPIContractIsOfflineAndContainsAllHTTPFamilies(t *testing.T) {
	raw, err := os.ReadFile("../../api/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	spec := string(raw)
	for _, required := range []string{
		"openapi: 3.0.3", "bearerAuth", "Idempotency-Key", "request_id",
		"/api/v1/apps", "/api/v1/tokens", "/api/v1/tokens/validate",
		"/api/v1/approvals", "/api/v1/audit-logs", "/api/v1/risk-events", "/api/v1/policies",
		"'400'", "'401'", "'403'", "'404'", "'409'", "'422'", "'429'", "'500'", "'503'",
	} {
		if !strings.Contains(spec, required) {
			t.Errorf("OpenAPI is missing %q", required)
		}
	}
	if strings.Contains(strings.ToLower(spec), "http://validator") || strings.Contains(strings.ToLower(spec), "curl ") {
		t.Fatal("OpenAPI validation must not depend on network access")
	}
}
