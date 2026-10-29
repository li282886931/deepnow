package http

import (
	"context"
	"fmt"
	"sync"
)

// MemoryBackend keeps the standalone server usable before external dependencies are configured.
// Production composition can replace it with adapters backed by the existing repositories.
type MemoryBackend struct {
	mu        sync.Mutex
	next      int
	apps      map[string]App
	tokens    map[string]TokenResult
	approvals map[string]ApprovalResponse
	policies  map[string]PolicyResponse
}

func NewMemoryBackend() *MemoryBackend {
	return &MemoryBackend{
		apps: make(map[string]App), tokens: make(map[string]TokenResult),
		approvals: make(map[string]ApprovalResponse), policies: make(map[string]PolicyResponse),
	}
}

func (b *MemoryBackend) id(prefix string) string {
	b.next++
	return fmt.Sprintf("%s_%d", prefix, b.next)
}
func page[T any](items []T, q PageQuery) Page[T] {
	start := (q.Page - 1) * q.PageSize
	if start > len(items) {
		start = len(items)
	}
	end := start + q.PageSize
	if end > len(items) {
		end = len(items)
	}
	return Page[T]{Items: items[start:end], Total: len(items), Page: q.Page, PageSize: q.PageSize}
}
func (b *MemoryBackend) CreateApp(_ context.Context, in AppInput) (App, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	id := b.id("app")
	app := App{AppID: id, Name: in.Name, Description: in.Description, OwnerID: in.OwnerID, DepartmentID: in.DepartmentID, Env: in.Env, DataLevel: in.DataLevel, Status: "active"}
	b.apps[id] = app
	return app, nil
}
func (b *MemoryBackend) ListApps(_ context.Context, q PageQuery) (Page[App], error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	items := make([]App, 0, len(b.apps))
	for _, item := range b.apps {
		items = append(items, item)
	}
	return page(items, q), nil
}
func (b *MemoryBackend) GetApp(_ context.Context, id string) (App, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	item, ok := b.apps[id]
	if !ok {
		return App{}, ErrNotFound
	}
	return item, nil
}
func (b *MemoryBackend) UpdateApp(_ context.Context, id string, in AppInput) (App, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	item, ok := b.apps[id]
	if !ok {
		return App{}, ErrNotFound
	}
	if in.Name != "" {
		item.Name = in.Name
	}
	if in.Description != "" {
		item.Description = in.Description
	}
	b.apps[id] = item
	return item, nil
}
func (b *MemoryBackend) CreateToken(_ context.Context, actor string, in TokenInput) (TokenResult, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	id := b.id("tok")
	secret := "tf_local_" + id
	item := TokenResult{TokenID: id, Status: "active", Secret: secret, Metadata: TokenMetadataResponse{TokenID: id, AppID: in.AppID, Name: in.Name, TokenType: in.TokenType, Status: "active", Env: in.Env, Scopes: in.Scopes, Quota: in.Quota, ExpiresAt: in.ExpiresAt}}
	b.tokens[id] = item
	_ = actor
	return item, nil
}
func (b *MemoryBackend) ListTokens(_ context.Context, q PageQuery) (Page[TokenMetadataResponse], error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	items := make([]TokenMetadataResponse, 0, len(b.tokens))
	for _, item := range b.tokens {
		items = append(items, item.Metadata)
	}
	return page(items, q), nil
}
func (b *MemoryBackend) GetToken(_ context.Context, id string) (TokenMetadataResponse, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	item, ok := b.tokens[id]
	if !ok {
		return TokenMetadataResponse{}, ErrNotFound
	}
	return item.Metadata, nil
}
func (b *MemoryBackend) RotateToken(_ context.Context, id, _ string, _ RotateInput) (TokenResult, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	item, ok := b.tokens[id]
	if !ok {
		return TokenResult{}, ErrNotFound
	}
	item.Secret = "tf_rotated_" + id
	b.tokens[id] = item
	return item, nil
}
func (b *MemoryBackend) RevokeToken(_ context.Context, id, _ string, in RevokeInput) (TokenMetadataResponse, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	item, ok := b.tokens[id]
	if !ok {
		return TokenMetadataResponse{}, ErrNotFound
	}
	item.Metadata.Status = "revoked"
	b.tokens[id] = item
	_ = in
	return item.Metadata, nil
}
func (b *MemoryBackend) ExtendToken(_ context.Context, id, _ string, in ExtendInput) (TokenMetadataResponse, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	item, ok := b.tokens[id]
	if !ok {
		return TokenMetadataResponse{}, ErrNotFound
	}
	item.Metadata.ExpiresAt = in.ExpiresAt
	b.tokens[id] = item
	return item.Metadata, nil
}
func (b *MemoryBackend) ListVersions(context.Context, string, PageQuery) (Page[TokenVersionResponse], error) {
	return Page[TokenVersionResponse]{Items: []TokenVersionResponse{}, Page: 1, PageSize: 20}, nil
}
func (b *MemoryBackend) GetUsage(context.Context, string, PageQuery) (Page[UsageResponse], error) {
	return Page[UsageResponse]{Items: []UsageResponse{}, Page: 1, PageSize: 20}, nil
}
func (b *MemoryBackend) ValidateToken(context.Context, ValidateInput) (ValidateResponse, error) {
	return ValidateResponse{Allow: true, Reason: "allowed"}, nil
}
func (b *MemoryBackend) ListApprovals(_ context.Context, q PageQuery) (Page[ApprovalResponse], error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	items := make([]ApprovalResponse, 0, len(b.approvals))
	for _, item := range b.approvals {
		items = append(items, item)
	}
	return page(items, q), nil
}
func (b *MemoryBackend) GetApproval(_ context.Context, id string) (ApprovalResponse, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	item, ok := b.approvals[id]
	if !ok {
		return ApprovalResponse{}, ErrNotFound
	}
	return item, nil
}
func (b *MemoryBackend) ActApproval(_ context.Context, id, action string, _ ApprovalActionInput) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	item, ok := b.approvals[id]
	if !ok {
		return ErrNotFound
	}
	item.Status = action + "d"
	b.approvals[id] = item
	return nil
}
func (b *MemoryBackend) ListAuditLogs(context.Context, PageQuery) (Page[AuditResponse], error) {
	return Page[AuditResponse]{Items: []AuditResponse{}, Page: 1, PageSize: 20}, nil
}
func (b *MemoryBackend) ListRiskEvents(context.Context, PageQuery) (Page[RiskResponse], error) {
	return Page[RiskResponse]{Items: []RiskResponse{}, Page: 1, PageSize: 20}, nil
}
func (b *MemoryBackend) ResolveRisk(context.Context, string, string, ResolveRiskInput) error {
	return nil
}
func (b *MemoryBackend) ListPolicies(_ context.Context, q PageQuery) (Page[PolicyResponse], error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	items := make([]PolicyResponse, 0, len(b.policies))
	for _, item := range b.policies {
		items = append(items, item)
	}
	return page(items, q), nil
}
func (b *MemoryBackend) UpdatePolicy(_ context.Context, id string, in PolicyInput) (PolicyResponse, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	item := PolicyResponse{PolicyID: id, Name: in.Name, Status: in.Status, Config: in.Config}
	b.policies[id] = item
	return item, nil
}
