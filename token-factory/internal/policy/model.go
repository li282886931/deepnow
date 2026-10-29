package policy

import "time"

type Quota struct {
	DailyLimit int64
	QPS        int64
}

type Request struct {
	Environment             string
	TokenType               string
	Scopes                  []string
	ExpiresAt               time.Time
	Quota                   Quota
	SensitiveData           bool
	ApplicationMember       bool
	ApplicantDepartmentID   string
	ApplicationDepartmentID string
	ApplicationOwnerID      string
	ResourceOwnerID         string
}

type ApprovalStep struct {
	StepOrder    int
	ApproverType string
	ApproverID   string
}

type Decision struct {
	RiskLevel     string
	AutoApprove   bool
	RequiredSteps []ApprovalStep
	Reasons       []string
}
