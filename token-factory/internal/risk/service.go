package risk

import (
	"context"
	"errors"
	"time"

	"github.com/li282886931/deepnow/token-factory/internal/audit"
)

type Level string

const (
	Low      Level = "low"
	Medium   Level = "medium"
	High     Level = "high"
	Critical Level = "critical"
)

type Classification struct {
	Score int
	Level Level
}

func Classify(score int) Classification {
	score = clamp(score)
	level := Low
	switch {
	case score >= 81:
		level = Critical
	case score >= 61:
		level = High
	case score >= 31:
		level = Medium
	}
	return Classification{Score: score, Level: level}
}

func Score(assessment Assessment) Classification {
	return Classify(score(assessment))
}

type Assessment struct {
	EventID           string
	RequestID         string
	TokenID           string
	ActorID           string
	Environment       string
	Scopes            []string
	DailyLimit        int64
	SensitiveData     bool
	ValidationDenials int
	UnknownCaller     bool
}

type RiskEvent struct {
	EventID            string
	RequestID          string
	TokenID            string
	ActorID            string
	Score              int
	Level              Level
	Reason             string
	RecommendedActions []string
	TokenAction        string
	ResolvedBy         string
	Resolution         string
	ResolvedAt         *time.Time
	CreatedAt          time.Time
}

type Repository interface {
	Create(context.Context, RiskEvent) error
	Resolve(context.Context, string, string, string, time.Time) error
}

type AuditEmitter interface {
	Emit(context.Context, audit.Event) error
}

type Service struct {
	repo    Repository
	auditor AuditEmitter
	Now     func() time.Time
}

var ErrRiskEventNotFound = errors.New("risk event not found")

func NewService(repo Repository, auditor AuditEmitter) *Service {
	return &Service{repo: repo, auditor: auditor, Now: func() time.Time { return time.Now().UTC() }}
}

func (s *Service) Assess(ctx context.Context, assessment Assessment) (RiskEvent, error) {
	classification := Classify(score(assessment))
	event := RiskEvent{
		EventID: assessment.EventID, RequestID: assessment.RequestID, TokenID: assessment.TokenID,
		ActorID: assessment.ActorID, Score: classification.Score, Level: classification.Level,
		Reason: reason(assessment), RecommendedActions: recommendations(classification.Level),
		CreatedAt: s.Now().UTC(),
	}
	if err := s.repo.Create(ctx, event); err != nil {
		return RiskEvent{}, err
	}
	return event, nil
}

func (s *Service) Resolve(ctx context.Context, eventID, actorID, resolution string) error {
	now := s.Now().UTC()
	if err := s.repo.Resolve(ctx, eventID, actorID, resolution, now); err != nil {
		return err
	}
	if s.auditor == nil {
		return nil
	}
	return s.auditor.Emit(ctx, audit.Event{
		ActorID: actorID, Action: audit.ActionRiskResolve, ResourceType: "risk_event",
		ResourceID: eventID, After: map[string]any{"resolution": resolution},
	})
}

func score(assessment Assessment) int {
	value := 0
	if assessment.Environment == "prod" {
		value += 20
	}
	if assessment.SensitiveData {
		value += 30
	}
	for _, scope := range assessment.Scopes {
		if scope == "admin" || len(scope) >= 5 && scope[:5] == "admin" {
			value += 25
			break
		}
	}
	if assessment.DailyLimit > 0 && assessment.DailyLimit <= 10 {
		value += 15
	}
	if assessment.ValidationDenials >= 3 {
		value += 20
	}
	if assessment.UnknownCaller {
		value += 20
	}
	return clamp(value)
}

func reason(assessment Assessment) string {
	reason := "baseline"
	if assessment.Environment == "prod" {
		reason = "production access"
	}
	if assessment.SensitiveData {
		reason += ", sensitive data"
	}
	return reason
}

func recommendations(level Level) []string {
	switch level {
	case Critical:
		return []string{"require_security_review", "consider_rate_limit", "consider_revoke"}
	case High:
		return []string{"require_security_review", "consider_rate_limit"}
	case Medium:
		return []string{"require_owner_review"}
	default:
		return nil
	}
}

func clamp(score int) int {
	if score < 0 {
		return 0
	}
	if score > 100 {
		return 100
	}
	return score
}
