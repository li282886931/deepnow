package policy

import (
	"context"
	"testing"
	"time"
)

func TestEngineEvaluateApprovalPolicy(t *testing.T) {
	now := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name    string
		request Request
		risk    string
		auto    bool
		steps   []string
	}{
		{
			name: "automatic approval",
			request: Request{
				Environment: "dev", Scopes: []string{"read"},
				ExpiresAt: now.Add(12 * time.Hour), Quota: Quota{DailyLimit: 100},
				ApplicationMember: true,
			},
			risk: "low", auto: true,
		},
		{
			name: "production requires application owner",
			request: Request{
				Environment: "prod", Scopes: []string{"read"}, ExpiresAt: now.Add(time.Hour),
				Quota: Quota{DailyLimit: 100}, ApplicationMember: true, ApplicationOwnerID: "owner-1",
			},
			risk: "medium", steps: []string{"application_owner"},
		},
		{
			name: "high scope requires security",
			request: Request{
				Environment: "dev", Scopes: []string{"read", "write", "admin"},
				ExpiresAt: now.Add(time.Hour), Quota: Quota{DailyLimit: 100},
				ApplicationMember: true, ApplicationOwnerID: "owner-1",
			},
			risk: "high", steps: []string{"application_owner", "security"},
		},
		{
			name: "cross department requires resource owner",
			request: Request{
				Environment: "test", Scopes: []string{"read"}, ExpiresAt: now.Add(time.Hour),
				Quota: Quota{DailyLimit: 100}, ApplicationMember: true,
				ApplicantDepartmentID: "dept-a", ApplicationDepartmentID: "dept-b",
				ApplicationOwnerID: "owner-1", ResourceOwnerID: "resource-1",
			},
			risk: "high", steps: []string{"application_owner", "resource_owner"},
		},
		{
			name: "high quota requires platform",
			request: Request{
				Environment: "test", Scopes: []string{"read"}, ExpiresAt: now.Add(time.Hour),
				Quota: Quota{DailyLimit: 100000, QPS: 100}, ApplicationMember: true,
				ApplicationOwnerID: "owner-1",
			},
			risk: "high", steps: []string{"application_owner", "platform"},
		},
	}

	engine := NewEngine(now)
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			decision, err := engine.Evaluate(context.Background(), test.request)
			if err != nil {
				t.Fatalf("Evaluate() error = %v", err)
			}
			if decision.RiskLevel != test.risk || decision.AutoApprove != test.auto {
				t.Fatalf("decision = %+v, want risk=%q auto=%v", decision, test.risk, test.auto)
			}
			if len(decision.RequiredSteps) != len(test.steps) {
				t.Fatalf("steps = %+v, want %v", decision.RequiredSteps, test.steps)
			}
			for i, step := range decision.RequiredSteps {
				if step.ApproverType != test.steps[i] {
					t.Errorf("step[%d].ApproverType = %q, want %q", i, step.ApproverType, test.steps[i])
				}
			}
		})
	}
}
