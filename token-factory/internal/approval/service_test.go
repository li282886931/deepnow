package approval

import (
	"context"
	"errors"
	"testing"
	"time"

	policy "github.com/li282886931/deepnow/token-factory/internal/policy"
	"github.com/li282886931/deepnow/token-factory/internal/token"
)

type fakeApprovalRepository struct {
	requests []ApprovalRequest
	steps    []ApprovalStep
}

func (r *fakeApprovalRepository) CreateRequest(_ context.Context, request ApprovalRequest) error {
	r.requests = append(r.requests, request)
	return nil
}

func (r *fakeApprovalRepository) AddStep(_ context.Context, step ApprovalStep) error {
	r.steps = append(r.steps, step)
	return nil
}

type fakeIssuer struct {
	calls int
}

func (i *fakeIssuer) Create(_ context.Context, request token.CreateRequest) (token.Result, error) {
	i.calls++
	return token.Result{TokenID: "tok_issued", Metadata: token.TokenMetadata{TokenID: "tok_issued"}}, nil
}

func manualRequest() policy.Request {
	return policy.Request{
		Environment: "prod", Scopes: []string{"read"}, ExpiresAt: time.Now().UTC().Add(time.Hour),
		Quota: policy.Quota{DailyLimit: 100}, ApplicationMember: true, ApplicationOwnerID: "owner-1",
	}
}

func TestSubmitAutomaticallyIssuesLowRiskRequest(t *testing.T) {
	issuer := &fakeIssuer{}
	service := NewService(&fakeApprovalRepository{}, policy.NewEngine(time.Now().UTC()), issuer)
	result, err := service.Submit(context.Background(), SubmitRequest{
		Request: policy.Request{
			Environment: "dev", Scopes: []string{"read"}, ExpiresAt: time.Now().UTC().Add(time.Hour),
			Quota: policy.Quota{DailyLimit: 10}, ApplicationMember: true,
		},
		ApplicantID: "user-1", AppID: "app-1", IdempotencyKey: "issue-1",
	})
	if err != nil {
		t.Fatalf("Submit() error = %v", err)
	}
	if result.Status != StatusApproved || result.TokenID != "tok_issued" || issuer.calls != 1 {
		t.Fatalf("result = %+v, issuer calls = %d", result, issuer.calls)
	}
}

func TestApprovalWorkflowActionsAndSingleFinalIssuance(t *testing.T) {
	repo := &fakeApprovalRepository{}
	issuer := &fakeIssuer{}
	service := NewService(repo, policy.NewEngine(time.Now().UTC()), issuer)
	created, err := service.Submit(context.Background(), SubmitRequest{
		Request: manualRequest(), ApplicantID: "user-1", AppID: "app-1", IdempotencyKey: "issue-2",
	})
	if err != nil {
		t.Fatalf("Submit() error = %v", err)
	}
	if created.Status != StatusPending || len(created.Steps) != 1 {
		t.Fatalf("created = %+v", created)
	}
	requestID := created.RequestID

	if err := service.Transfer(context.Background(), requestID, "owner-1", "owner-2", "out of office"); err != nil {
		t.Fatalf("Transfer() error = %v", err)
	}
	if err := service.AddApprover(context.Background(), requestID, "owner-2", "security-1", "security review"); err != nil {
		t.Fatalf("AddApprover() error = %v", err)
	}
	if err := service.Approve(context.Background(), requestID, "owner-2", "approved"); err != nil {
		t.Fatalf("Approve() first error = %v", err)
	}
	if err := service.Reject(context.Background(), requestID, "owner-2", "denied"); err == nil {
		t.Fatal("Reject() unexpectedly succeeded after first step was approved")
	}
	if err := service.Return(context.Background(), requestID, "owner-2", "missing evidence"); err == nil {
		t.Fatal("Return() unexpectedly succeeded after first step was approved")
	}
	if err := service.Approve(context.Background(), requestID, "security-1", "approved"); err != nil {
		t.Fatalf("Approve() final error = %v", err)
	}
	if issuer.calls != 1 {
		t.Fatalf("issuer calls = %d, want 1", issuer.calls)
	}
	state, err := service.Get(context.Background(), requestID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if state.Status != StatusApproved || state.TokenID != "tok_issued" {
		t.Fatalf("final state = %+v", state)
	}
	if err := service.Approve(context.Background(), requestID, "security-1", "duplicate"); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("duplicate Approve() error = %v, want %v", err, ErrInvalidTransition)
	}
}

func TestApprovalRejectAndReturn(t *testing.T) {
	for _, action := range []string{"reject", "return"} {
		t.Run(action, func(t *testing.T) {
			service := NewService(&fakeApprovalRepository{}, policy.NewEngine(time.Now().UTC()), &fakeIssuer{})
			created, err := service.Submit(context.Background(), SubmitRequest{
				Request: manualRequest(), ApplicantID: "user-1", AppID: "app-1",
			})
			if err != nil {
				t.Fatal(err)
			}
			var actionErr error
			if action == "reject" {
				actionErr = service.Reject(context.Background(), created.RequestID, "owner-1", "no")
			} else {
				actionErr = service.Return(context.Background(), created.RequestID, "owner-1", "fix")
			}
			if actionErr != nil {
				t.Fatalf("%s() error = %v", action, actionErr)
			}
			state, err := service.Get(context.Background(), created.RequestID)
			if err != nil {
				t.Fatal(err)
			}
			want := StatusRejected
			if action == "return" {
				want = StatusReturned
			}
			if state.Status != want {
				t.Fatalf("status = %q, want %q", state.Status, want)
			}
		})
	}
}
