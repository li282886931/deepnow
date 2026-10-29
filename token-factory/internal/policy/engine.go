package policy

import (
	"context"
	"strings"
	"time"
)

type Engine struct {
	now func() time.Time
}

func NewEngine(now time.Time) *Engine {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	return &Engine{now: func() time.Time { return now }}
}

func (e *Engine) Evaluate(_ context.Context, request Request) (Decision, error) {
	decision := Decision{RiskLevel: "low", AutoApprove: true}
	addReason := func(reason string) {
		decision.Reasons = append(decision.Reasons, reason)
		if decision.RiskLevel == "low" {
			decision.RiskLevel = "medium"
		}
	}
	addStep := func(kind, id string) {
		for _, step := range decision.RequiredSteps {
			if step.ApproverType == kind {
				return
			}
		}
		decision.RequiredSteps = append(decision.RequiredSteps, ApprovalStep{
			StepOrder: len(decision.RequiredSteps) + 1, ApproverType: kind, ApproverID: id,
		})
		decision.AutoApprove = false
	}

	env := strings.ToLower(request.Environment)
	if env == "prod" || env == "production" {
		addReason("production environment")
		addStep("application_owner", request.ApplicationOwnerID)
	}
	if request.ExpiresAt.After(e.now().Add(24 * time.Hour)) {
		addReason("long validity period")
		addStep("application_owner", request.ApplicationOwnerID)
	}
	if strings.EqualFold(request.TokenType, "service_token") {
		addReason("service token")
		addStep("application_owner", request.ApplicationOwnerID)
	}
	if !request.ApplicationMember {
		addReason("applicant is not an application member")
		addStep("application_owner", request.ApplicationOwnerID)
	}

	highScope := false
	for _, scope := range request.Scopes {
		switch strings.ToLower(strings.TrimSpace(scope)) {
		case "write", "delete", "admin", "manage", "security", "owner":
			highScope = true
		}
	}
	if highScope || request.SensitiveData {
		addReason("sensitive or elevated scope")
		addStep("application_owner", request.ApplicationOwnerID)
		addStep("security", "")
	}

	if request.ApplicantDepartmentID != "" &&
		request.ApplicationDepartmentID != "" &&
		request.ApplicantDepartmentID != request.ApplicationDepartmentID {
		addReason("cross-department resource access")
		addStep("application_owner", request.ApplicationOwnerID)
		addStep("resource_owner", request.ResourceOwnerID)
	}

	if request.Quota.DailyLimit > 10000 || request.Quota.QPS > 50 {
		addReason("high quota")
		addStep("application_owner", request.ApplicationOwnerID)
		addStep("platform", "")
	}
	if decision.RiskLevel == "medium" && len(decision.RequiredSteps) > 1 {
		decision.RiskLevel = "high"
	}
	return decision, nil
}
