package approval

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/li282886931/deepnow/token-factory/internal/policy"
	"github.com/li282886931/deepnow/token-factory/internal/token"
)

const (
	StatusPending  = "pending"
	StatusApproved = "approved"
	StatusRejected = "rejected"
	StatusReturned = "returned"
	StatusQueued   = "queued"
	StatusHandled  = "handled"
)

var (
	ErrApprovalNotFound   = errors.New("approval request not found")
	ErrInvalidTransition  = errors.New("invalid approval transition")
	ErrNotCurrentApprover = errors.New("actor is not the current approver")
)

type TokenIssuer interface {
	Create(context.Context, token.CreateRequest) (token.Result, error)
}

type SubmitRequest struct {
	policy.Request
	RequestID      string
	AppID          string
	ApplicantID    string
	TokenID        *string
	Name           string
	TokenType      string
	Scopes         string
	Quota          string
	IdempotencyKey string
}

type State struct {
	Request   ApprovalRequest
	Steps     []ApprovalStep
	TokenID   string
	Result    token.Result
	Status    string
	RequestID string
}

type Service struct {
	repo   ApprovalRepository
	engine *policy.Engine
	issuer TokenIssuer

	mu     sync.Mutex
	states map[string]*State
}

func NewService(repo ApprovalRepository, engine *policy.Engine, issuer TokenIssuer) *Service {
	return &Service{repo: repo, engine: engine, issuer: issuer, states: make(map[string]*State)}
}

func (s *Service) Submit(ctx context.Context, request SubmitRequest) (State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	decision, err := s.engine.Evaluate(ctx, request.Request)
	if err != nil {
		return State{}, err
	}
	if decision.AutoApprove {
		result, err := s.issue(ctx, request)
		if err != nil {
			return State{}, err
		}
		return cloneState(State{Request: ApprovalRequest{RequestID: request.RequestID, AppID: request.AppID,
			ApplicantID: request.ApplicantID, Status: StatusApproved, RiskLevel: decision.RiskLevel},
			TokenID: result.TokenID, Result: result, Status: StatusApproved, RequestID: request.RequestID}), nil
	}

	requestID := request.RequestID
	if requestID == "" {
		requestID = newRequestID()
	}
	payload, err := json.Marshal(request)
	if err != nil {
		return State{}, err
	}
	now := time.Now().UTC()
	stored := ApprovalRequest{
		RequestID: requestID, RequestType: "token_issue", TokenID: request.TokenID,
		AppID: request.AppID, ApplicantID: request.ApplicantID, Payload: string(payload),
		RiskLevel: decision.RiskLevel, Status: StatusPending, CreatedAt: now, UpdatedAt: now,
	}
	state := &State{Request: stored}
	for index, required := range decision.RequiredSteps {
		status := StatusQueued
		if index == 0 {
			status = StatusPending
		}
		state.Steps = append(state.Steps, ApprovalStep{
			RequestID: requestID, StepOrder: required.StepOrder, ApproverType: required.ApproverType,
			ApproverID: nullable(required.ApproverID), Status: status,
		})
	}
	if err := s.repo.CreateRequest(ctx, stored); err != nil {
		return State{}, err
	}
	for _, step := range state.Steps {
		if err := s.repo.AddStep(ctx, step); err != nil {
			return State{}, err
		}
	}
	s.states[requestID] = state
	return cloneState(*state), nil
}

func (s *Service) Get(_ context.Context, requestID string) (State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, ok := s.states[requestID]
	if !ok {
		return State{}, ErrApprovalNotFound
	}
	return cloneState(*state), nil
}

func (s *Service) Approve(ctx context.Context, requestID, actor, comment string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, step, err := s.current(requestID, actor)
	if err != nil {
		return err
	}
	step.Status, step.Comment, step.HandledAt = StatusHandled, comment, ptrNow()
	if next := nextPending(state.Steps, step.StepOrder); next != nil {
		next.Status = StatusPending
		state.Request.UpdatedAt = time.Now().UTC()
		return nil
	}
	request := decodeSubmit(state.Request.Payload)
	result, err := s.issue(ctx, request)
	if err != nil {
		step.Status = StatusPending
		return err
	}
	state.Result, state.TokenID = result, result.TokenID
	state.Request.Status, state.Request.UpdatedAt = StatusApproved, time.Now().UTC()
	state.Status = StatusApproved
	return nil
}

func (s *Service) Reject(_ context.Context, requestID, actor, comment string) error {
	return s.finish(requestID, actor, comment, StatusRejected)
}

func (s *Service) Return(_ context.Context, requestID, actor, comment string) error {
	return s.finish(requestID, actor, comment, StatusReturned)
}

func (s *Service) finish(requestID, actor, comment, status string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, step, err := s.current(requestID, actor)
	if err != nil {
		return err
	}
	step.Status, step.Comment, step.HandledAt = status, comment, ptrNow()
	state.Request.Status, state.Request.UpdatedAt = status, time.Now().UTC()
	state.Status = status
	return nil
}

func (s *Service) Transfer(_ context.Context, requestID, actor, newApprover, comment string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, step, err := s.current(requestID, actor)
	if err != nil {
		return err
	}
	step.ApproverID, step.Comment = nullable(newApprover), comment
	return nil
}

func (s *Service) AddApprover(_ context.Context, requestID, actor, newApprover, comment string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, current, err := s.current(requestID, actor)
	if err != nil {
		return err
	}
	insertAt := current.StepOrder
	state.Steps = append(state.Steps, ApprovalStep{})
	copy(state.Steps[insertAt+1:], state.Steps[insertAt:])
	state.Steps[insertAt] = ApprovalStep{
		RequestID: requestID, StepOrder: current.StepOrder + 1, ApproverType: "added",
		ApproverID: nullable(newApprover), Status: StatusQueued, Comment: comment,
	}
	for index := range state.Steps {
		state.Steps[index].StepOrder = index + 1
	}
	return nil
}

func (s *Service) current(requestID, actor string) (*State, *ApprovalStep, error) {
	state, ok := s.states[requestID]
	if !ok {
		return nil, nil, ErrApprovalNotFound
	}
	if state.Request.Status != StatusPending {
		return nil, nil, ErrInvalidTransition
	}
	for index := range state.Steps {
		step := &state.Steps[index]
		if step.Status != StatusPending {
			continue
		}
		if step.ApproverID != nil && *step.ApproverID != "" && *step.ApproverID != actor {
			return nil, nil, ErrNotCurrentApprover
		}
		return state, step, nil
	}
	return nil, nil, ErrInvalidTransition
}

func (s *Service) issue(ctx context.Context, request SubmitRequest) (token.Result, error) {
	if s.issuer == nil {
		return token.Result{}, errors.New("token issuer is not configured")
	}
	scopes := request.Scopes
	if scopes == "" {
		encoded, _ := json.Marshal(request.Request.Scopes)
		scopes = string(encoded)
	}
	quota := request.Quota
	if quota == "" {
		encoded, _ := json.Marshal(request.Request.Quota)
		quota = string(encoded)
	}
	return s.issuer.Create(ctx, token.CreateRequest{
		ActorID: request.ApplicantID, IdempotencyKey: request.IdempotencyKey, AppID: request.AppID,
		Name: request.Name, TokenType: request.TokenType, Env: request.Environment,
		Scopes: scopes, Quota: quota, ExpiresAt: request.ExpiresAt,
	})
}

func nextPending(steps []ApprovalStep, currentOrder int) *ApprovalStep {
	for index := range steps {
		if steps[index].StepOrder > currentOrder && steps[index].Status == StatusQueued {
			return &steps[index]
		}
	}
	return nil
}

func decodeSubmit(payload string) SubmitRequest {
	var request SubmitRequest
	_ = json.Unmarshal([]byte(payload), &request)
	return request
}

func cloneState(state State) State {
	state.Status = state.Request.Status
	state.RequestID = state.Request.RequestID
	state.Steps = append([]ApprovalStep(nil), state.Steps...)
	return state
}

func nullable(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func ptrNow() *time.Time {
	now := time.Now().UTC()
	return &now
}

func newRequestID() string {
	var raw [12]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return fmt.Sprintf("req_%d", time.Now().UnixNano())
	}
	return "req_" + hex.EncodeToString(raw[:])
}
