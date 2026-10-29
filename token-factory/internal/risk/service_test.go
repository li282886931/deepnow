package risk

import (
	"context"
	"testing"
	"time"

	"github.com/li282886931/deepnow/token-factory/internal/audit"
)

type recordingRiskRepository struct {
	events []RiskEvent
}

func (r *recordingRiskRepository) Create(_ context.Context, event RiskEvent) error {
	r.events = append(r.events, event)
	return nil
}

func (r *recordingRiskRepository) Resolve(_ context.Context, eventID, actor, resolution string, resolvedAt time.Time) error {
	for index := range r.events {
		if r.events[index].EventID == eventID {
			r.events[index].ResolvedBy = actor
			r.events[index].Resolution = resolution
			r.events[index].ResolvedAt = &resolvedAt
			return nil
		}
	}
	return ErrRiskEventNotFound
}

type recordingAudit struct {
	actions []string
}

func (r *recordingAudit) Emit(_ context.Context, event audit.Event) error {
	r.actions = append(r.actions, event.Action)
	return nil
}

func TestScoreMapsToDocumentedRiskLevels(t *testing.T) {
	tests := []struct {
		name  string
		score int
		level Level
	}{
		{"low boundary", 30, Low},
		{"medium boundary", 31, Medium},
		{"high boundary", 61, High},
		{"critical boundary", 81, Critical},
		{"clamps negative", -10, Low},
		{"clamps maximum", 120, Critical},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := Classify(tc.score)
			if result.Score != clamp(tc.score) || result.Level != tc.level {
				t.Fatalf("Classify(%d) = %+v, want score=%d level=%s", tc.score, result, clamp(tc.score), tc.level)
			}
		})
	}
}

func TestServiceCreatesRiskEventAndResolutionAudit(t *testing.T) {
	repo := &recordingRiskRepository{}
	auditor := &recordingAudit{}
	service := NewService(repo, auditor)

	event, err := service.Assess(context.Background(), Assessment{
		EventID: "risk_1", RequestID: "req_1", TokenID: "tok_1",
		ActorID: "caller_1", Environment: "prod", SensitiveData: true,
		Scopes: []string{"admin:write"}, DailyLimit: 2, ValidationDenials: 4,
	})
	if err != nil {
		t.Fatalf("Assess() error = %v", err)
	}
	if event.Score < 0 || event.Score > 100 || event.Level != Critical {
		t.Fatalf("risk event = %+v, want bounded critical event", event)
	}
	if len(repo.events) != 1 || len(event.RecommendedActions) == 0 {
		t.Fatalf("stored event = %+v, want recommended actions", event)
	}
	if err := service.Resolve(context.Background(), "risk_1", "security_1", "contained"); err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if repo.events[0].ResolvedBy != "security_1" || repo.events[0].Resolution != "contained" ||
		repo.events[0].ResolvedAt == nil {
		t.Fatalf("resolved event = %+v", repo.events[0])
	}
	if len(auditor.actions) != 1 || auditor.actions[0] != audit.ActionRiskResolve {
		t.Fatalf("resolution audit actions = %v", auditor.actions)
	}
}

func TestServiceDoesNotMutateTokensWhenAssessingRisk(t *testing.T) {
	service := NewService(&recordingRiskRepository{}, nil)
	event, err := service.Assess(context.Background(), Assessment{
		EventID: "risk_2", TokenID: "tok_2", Environment: "prod",
	})
	if err != nil {
		t.Fatal(err)
	}
	if event.TokenAction != "" {
		t.Fatalf("Assess() proposed an implicit token mutation: %q", event.TokenAction)
	}
}
