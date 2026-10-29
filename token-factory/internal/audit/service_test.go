package audit

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

type recordingRepository struct {
	logs []AuditLog
}

func (r *recordingRepository) Create(_ context.Context, log AuditLog) error {
	r.logs = append(r.logs, log)
	return nil
}

func TestServiceEmitsRequiredActionsWithoutPlaintextSecrets(t *testing.T) {
	repo := &recordingRepository{}
	service := NewService(repo)
	actions := []string{
		ActionTokenCreate,
		ActionSecretCopy,
		ActionTokenValidate,
		ActionTokenRotate,
		ActionTokenRevoke,
		ActionApproval,
		ActionQuotaReject,
		ActionRiskResolve,
	}

	for _, action := range actions {
		t.Run(action, func(t *testing.T) {
			err := service.Emit(context.Background(), Event{
				RequestID:    "req_1",
				ActorID:      "user_1",
				Action:       action,
				ResourceType: "token",
				ResourceID:   "tok_1",
				Before:       map[string]any{"status": "active", "secret": "tf_live_plaintext"},
				After:        map[string]any{"raw_token": "tf_live_plaintext", "secret_ciphertext": "tf_live_plaintext", "note": "copied tf_live_plaintext", "status": "active"},
				Sensitive:    []string{"tf_live_plaintext"},
			})
			if err != nil {
				t.Fatalf("Emit() error = %v", err)
			}
		})
	}

	if len(repo.logs) != len(actions) {
		t.Fatalf("recorded logs = %d, want %d", len(repo.logs), len(actions))
	}
	for _, log := range repo.logs {
		encoded := string(log.BeforeJSON) + string(log.AfterJSON)
		if strings.Contains(encoded, "tf_live_plaintext") ||
			strings.Contains(encoded, `"secret"`) ||
			strings.Contains(encoded, `"raw_token"`) {
			t.Fatalf("audit payload contains secret material: %s", encoded)
		}
		var payload map[string]any
		if len(log.AfterJSON) > 0 && json.Unmarshal(log.AfterJSON, &payload) != nil {
			t.Fatalf("after payload is not valid JSON: %s", log.AfterJSON)
		}
	}
}

func TestServiceConvenienceMethodsEmitAuditEvents(t *testing.T) {
	repo := &recordingRepository{}
	service := NewService(repo)
	ctx := context.Background()

	if err := service.TokenCreate(ctx, "req_create", "user_1", "tok_1", map[string]any{"status": "active"}); err != nil {
		t.Fatal(err)
	}
	if err := service.SecretCopy(ctx, "req_copy", "user_1", "tok_1"); err != nil {
		t.Fatal(err)
	}
	if err := service.TokenValidate(ctx, "req_validate", "caller_1", "tok_1", true, "allowed"); err != nil {
		t.Fatal(err)
	}
	if err := service.TokenRotate(ctx, "req_rotate", "user_1", "tok_1"); err != nil {
		t.Fatal(err)
	}
	if err := service.TokenRevoke(ctx, "req_revoke", "user_1", "tok_1", "compromised"); err != nil {
		t.Fatal(err)
	}
	if err := service.Approval(ctx, "req_approval", "approver_1", "approval_1", "approved"); err != nil {
		t.Fatal(err)
	}
	if err := service.QuotaReject(ctx, "req_quota", "caller_1", "tok_1", "daily limit"); err != nil {
		t.Fatal(err)
	}
	if err := service.RiskResolve(ctx, "req_risk", "security_1", "risk_1", "resolved"); err != nil {
		t.Fatal(err)
	}

	if len(repo.logs) != 8 {
		t.Fatalf("convenience methods emitted %d events, want 8", len(repo.logs))
	}
}
