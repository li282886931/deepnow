package audit

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

const (
	ActionTokenCreate   = "token.create"
	ActionSecretCopy    = "token.secret_copy"
	ActionTokenValidate = "token.validate"
	ActionTokenRotate   = "token.rotate"
	ActionTokenRevoke   = "token.revoke"
	ActionApproval      = "approval.action"
	ActionQuotaReject   = "quota.reject"
	ActionRiskResolve   = "risk.resolve"
)

var ErrInvalidEvent = errors.New("invalid audit event")

type Event struct {
	RequestID    string
	ActorID      string
	ActorType    string
	Action       string
	ResourceType string
	ResourceID   string
	Before       any
	After        any
	Result       string
	SourceIP     string
	Sensitive    []string
	CreatedAt    time.Time
}

type Service struct {
	repo Repository
	Now  func() time.Time
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo, Now: func() time.Time { return time.Now().UTC() }}
}

func (s *Service) Emit(ctx context.Context, event Event) error {
	if s == nil || s.repo == nil || event.Action == "" || event.ResourceType == "" {
		return ErrInvalidEvent
	}
	before, err := safeJSON(event.Before, event.Sensitive)
	if err != nil {
		return err
	}
	after, err := safeJSON(event.After, event.Sensitive)
	if err != nil {
		return err
	}
	createdAt := event.CreatedAt
	if createdAt.IsZero() {
		createdAt = s.Now()
	}
	result := event.Result
	if result == "" {
		result = "success"
	}
	return s.repo.Create(ctx, AuditLog{
		RequestID: event.RequestID, ActorID: event.ActorID, ActorType: event.ActorType,
		Action: event.Action, ResourceType: event.ResourceType, ResourceID: event.ResourceID,
		BeforeJSON: before, AfterJSON: after, Result: result, SourceIP: event.SourceIP,
		CreatedAt: createdAt.UTC(),
	})
}

func (s *Service) TokenCreate(ctx context.Context, requestID, actorID, tokenID string, after any) error {
	return s.Emit(ctx, Event{RequestID: requestID, ActorID: actorID, Action: ActionTokenCreate,
		ResourceType: "token", ResourceID: tokenID, After: after})
}

func (s *Service) SecretCopy(ctx context.Context, requestID, actorID, tokenID string) error {
	return s.Emit(ctx, Event{RequestID: requestID, ActorID: actorID, Action: ActionSecretCopy,
		ResourceType: "token", ResourceID: tokenID})
}

func (s *Service) TokenValidate(ctx context.Context, requestID, actorID, tokenID string, allowed bool, reason string) error {
	result := "denied"
	if allowed {
		result = "success"
	}
	return s.Emit(ctx, Event{RequestID: requestID, ActorID: actorID, Action: ActionTokenValidate,
		ResourceType: "token", ResourceID: tokenID, After: map[string]any{"allowed": allowed, "reason": reason}, Result: result})
}

func (s *Service) TokenRotate(ctx context.Context, requestID, actorID, tokenID string) error {
	return s.Emit(ctx, Event{RequestID: requestID, ActorID: actorID, Action: ActionTokenRotate,
		ResourceType: "token", ResourceID: tokenID})
}

func (s *Service) TokenRevoke(ctx context.Context, requestID, actorID, tokenID, reason string) error {
	return s.Emit(ctx, Event{RequestID: requestID, ActorID: actorID, Action: ActionTokenRevoke,
		ResourceType: "token", ResourceID: tokenID, After: map[string]any{"reason": reason}})
}

func (s *Service) Approval(ctx context.Context, requestID, actorID, approvalID, result string) error {
	return s.Emit(ctx, Event{RequestID: requestID, ActorID: actorID, Action: ActionApproval,
		ResourceType: "approval", ResourceID: approvalID, Result: result})
}

func (s *Service) QuotaReject(ctx context.Context, requestID, actorID, tokenID, reason string) error {
	return s.Emit(ctx, Event{RequestID: requestID, ActorID: actorID, Action: ActionQuotaReject,
		ResourceType: "token", ResourceID: tokenID, After: map[string]any{"reason": reason}, Result: "denied"})
}

func (s *Service) RiskResolve(ctx context.Context, requestID, actorID, eventID, resolution string) error {
	return s.Emit(ctx, Event{RequestID: requestID, ActorID: actorID, Action: ActionRiskResolve,
		ResourceType: "risk_event", ResourceID: eventID, After: map[string]any{"resolution": resolution}})
}

func safeJSON(value any, sensitive []string) ([]byte, error) {
	if value == nil {
		return nil, nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil, err
	}
	decoded = redact(decoded, sensitive)
	return json.Marshal(decoded)
}

func redact(value any, sensitive []string) any {
	switch current := value.(type) {
	case map[string]any:
		for key, child := range current {
			lower := strings.ToLower(key)
			if lower == "secret" || lower == "raw_token" || lower == "plaintext" ||
				lower == "ciphertext" || lower == "secret_ciphertext" ||
				lower == "secret_plaintext" || lower == "authorization" || lower == "token" {
				delete(current, key)
				continue
			}
			current[key] = redact(child, sensitive)
		}
		return current
	case []any:
		for index, child := range current {
			current[index] = redact(child, sensitive)
		}
		return current
	case string:
		for _, secret := range sensitive {
			if secret != "" {
				current = strings.ReplaceAll(current, secret, "[REDACTED]")
			}
		}
		return current
	}
	return value
}
