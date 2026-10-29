package token

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"
)

var (
	ErrTokenNotFound       = errors.New("token not found")
	ErrConflict            = errors.New("token state conflict")
	ErrIdempotencyConflict = errors.New("idempotency key payload conflict")
)

type Blacklister interface {
	Add(context.Context, string, string, time.Time) error
}

type Lock interface {
	Release(context.Context) error
}

type LockProvider interface {
	Acquire(context.Context, string, time.Duration) (Lock, error)
}

type expiryUpdater interface {
	UpdateExpiry(context.Context, string, time.Time) error
}

type versionUpdater interface {
	UpdateVersion(context.Context, TokenVersion) error
}

type idempotentResult struct {
	fingerprint string
	result      Result
}

type Service struct {
	repo      TokenRepository
	cipher    SecretCipher
	blacklist Blacklister
	locks     LockProvider

	mu          sync.Mutex
	operationMu sync.Mutex
	idempotency map[string]idempotentResult
}

func NewService(repo TokenRepository, cipher SecretCipher, blacklist Blacklister, locks LockProvider) *Service {
	return &Service{
		repo: repo, cipher: cipher, blacklist: blacklist, locks: locks,
		idempotency: make(map[string]idempotentResult),
	}
}

func (s *Service) Create(ctx context.Context, request CreateRequest) (Result, error) {
	s.operationMu.Lock()
	defer s.operationMu.Unlock()
	fingerprint, err := fingerprint("create", request.ActorID, request)
	if err != nil {
		return Result{}, err
	}
	if result, ok, err := s.cached(request.IdempotencyKey, fingerprint); err != nil || ok {
		return result, err
	}
	secret, err := GenerateSecret()
	if err != nil {
		return Result{}, err
	}
	now := time.Now().UTC()
	tokenID, err := identifier("tok_")
	if err != nil {
		return Result{}, err
	}
	if request.ExpiresAt.IsZero() || !request.ExpiresAt.After(now) {
		return Result{}, fmt.Errorf("%w: expiry must be in the future", ErrConflict)
	}
	metadata := Token{
		TokenID: tokenID, AppID: request.AppID, TokenType: request.TokenType,
		Name: request.Name, Status: string(Active), Env: request.Env,
		Scopes: defaultJSON(request.Scopes, "[]"), Quota: defaultJSON(request.Quota, "{}"),
		ExpiresAt: request.ExpiresAt.UTC(), CreatedBy: request.ActorID, OwnerID: request.OwnerID,
		CreatedAt: now, UpdatedAt: now,
	}
	ciphertext, err := s.cipher.Encrypt([]byte(secret))
	if err != nil {
		return Result{}, err
	}
	if err := s.repo.Create(ctx, metadata); err != nil {
		return Result{}, err
	}
	if err := s.repo.AddVersion(ctx, TokenVersion{
		TokenID: tokenID, Version: 1, SecretCiphertext: ciphertext,
		SecretHash: HashSecret(secret), Status: string(Active),
		ValidFrom: now, CreatedAt: now,
	}); err != nil {
		return Result{}, err
	}
	result := Result{TokenID: tokenID, Secret: secret, Metadata: toMetadata(metadata)}
	s.store(request.IdempotencyKey, fingerprint, result)
	return result, nil
}

func (s *Service) Get(ctx context.Context, tokenID string) (TokenMetadata, error) {
	token, err := s.repo.Get(ctx, tokenID)
	if err != nil {
		return TokenMetadata{}, err
	}
	return toMetadata(token), nil
}

func (s *Service) Rotate(ctx context.Context, tokenID string, request RotateRequest) (Result, error) {
	s.operationMu.Lock()
	defer s.operationMu.Unlock()
	fingerprint, err := fingerprint("rotate:"+tokenID, request.ActorID, request)
	if err != nil {
		return Result{}, err
	}
	if result, ok, err := s.cached(request.IdempotencyKey, fingerprint); err != nil || ok {
		return result, err
	}
	token, err := s.repo.Get(ctx, tokenID)
	if err != nil {
		return Result{}, err
	}
	if token.Status != string(Active) {
		return Result{}, fmt.Errorf("%w: cannot rotate %s token", ErrConflict, token.Status)
	}
	if request.GracePeriod <= 0 {
		request.GracePeriod = time.Hour
	}
	lock, err := s.locks.Acquire(ctx, tokenID, 30*time.Second)
	if err != nil {
		return Result{}, err
	}
	defer lock.Release(ctx)
	now := time.Now().UTC()
	if err := s.repo.UpdateStatus(ctx, tokenID, string(Active), string(Rotating)); err != nil {
		return Result{}, mapConflict(err)
	}
	versions, err := s.repo.ListVersions(ctx, tokenID)
	if err != nil {
		return Result{}, err
	}
	if len(versions) == 0 {
		return Result{}, ErrConflict
	}
	versions[len(versions)-1].ValidUntil = ptrTime(now.Add(request.GracePeriod))
	if updater, ok := s.repo.(versionUpdater); ok {
		if err := updater.UpdateVersion(ctx, versions[len(versions)-1]); err != nil {
			return Result{}, err
		}
	}
	secret, err := GenerateSecret()
	if err != nil {
		return Result{}, err
	}
	ciphertext, err := s.cipher.Encrypt([]byte(secret))
	if err != nil {
		return Result{}, err
	}
	version := TokenVersion{
		TokenID: tokenID, Version: versions[len(versions)-1].Version + 1,
		SecretCiphertext: ciphertext, SecretHash: HashSecret(secret),
		Status: string(Active), ValidFrom: now, CreatedAt: now,
	}
	if err := s.repo.AddVersion(ctx, version); err != nil {
		return Result{}, err
	}
	if err := s.repo.UpdateStatus(ctx, tokenID, string(Rotating), string(Active)); err != nil {
		return Result{}, mapConflict(err)
	}
	token.Status = string(Active)
	token.UpdatedAt = now
	result := Result{TokenID: tokenID, Secret: secret, Metadata: toMetadata(token)}
	s.store(request.IdempotencyKey, fingerprint, result)
	return result, nil
}

func (s *Service) Revoke(ctx context.Context, tokenID, reason string) (TokenMetadata, error) {
	token, err := s.repo.Get(ctx, tokenID)
	if err != nil {
		return TokenMetadata{}, err
	}
	if token.Status != string(Active) && token.Status != string(Rotating) {
		return TokenMetadata{}, fmt.Errorf("%w: cannot revoke %s token", ErrConflict, token.Status)
	}
	lock, err := s.locks.Acquire(ctx, tokenID, 30*time.Second)
	if err != nil {
		return TokenMetadata{}, err
	}
	defer lock.Release(ctx)
	if err := s.repo.UpdateStatus(ctx, tokenID, token.Status, string(Revoked)); err != nil {
		return TokenMetadata{}, mapConflict(err)
	}
	versions, err := s.repo.ListVersions(ctx, tokenID)
	if err != nil {
		return TokenMetadata{}, err
	}
	for _, version := range versions {
		if err := s.blacklist.Add(ctx, version.SecretHash, reason, token.ExpiresAt); err != nil {
			return TokenMetadata{}, err
		}
	}
	token.Status = string(Revoked)
	return toMetadata(token), nil
}

func (s *Service) Extend(ctx context.Context, tokenID string, request ExtendRequest) (TokenMetadata, error) {
	s.operationMu.Lock()
	defer s.operationMu.Unlock()
	fingerprint, err := fingerprint("extend:"+tokenID, request.ActorID, request)
	if err != nil {
		return TokenMetadata{}, err
	}
	if result, ok, err := s.cached(request.IdempotencyKey, fingerprint); err != nil || ok {
		return result.Metadata, err
	}
	token, err := s.repo.Get(ctx, tokenID)
	if err != nil {
		return TokenMetadata{}, err
	}
	if token.Status != string(Active) && token.Status != string(Rotating) {
		return TokenMetadata{}, fmt.Errorf("%w: cannot extend %s token", ErrConflict, token.Status)
	}
	if !request.ExpiresAt.After(time.Now().UTC()) {
		return TokenMetadata{}, fmt.Errorf("%w: expiry must be in the future", ErrConflict)
	}
	lock, err := s.locks.Acquire(ctx, tokenID, 30*time.Second)
	if err != nil {
		return TokenMetadata{}, err
	}
	defer lock.Release(ctx)
	updater, ok := s.repo.(expiryUpdater)
	if !ok {
		return TokenMetadata{}, errors.New("token repository does not support expiry updates")
	}
	if err := updater.UpdateExpiry(ctx, tokenID, request.ExpiresAt.UTC()); err != nil {
		return TokenMetadata{}, err
	}
	token.ExpiresAt = request.ExpiresAt.UTC()
	metadata := toMetadata(token)
	s.store(request.IdempotencyKey, fingerprint, Result{TokenID: tokenID, Metadata: metadata})
	return metadata, nil
}

func (s *Service) cached(key, fingerprint string) (Result, bool, error) {
	if key == "" {
		return Result{}, false, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.idempotency[key]
	if !ok {
		return Result{}, false, nil
	}
	if entry.fingerprint != fingerprint {
		return Result{}, false, ErrIdempotencyConflict
	}
	return entry.result, true, nil
}

func (s *Service) store(key, fingerprint string, result Result) {
	if key == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.idempotency[key] = idempotentResult{fingerprint: fingerprint, result: result}
}

func fingerprint(operation, actor string, value any) (string, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return operation + ":" + actor + ":" + string(payload), nil
}

func identifier(prefix string) (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return prefix + hex.EncodeToString(raw[:]), nil
}

func defaultJSON(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func ptrTime(value time.Time) *time.Time { return &value }

func mapConflict(err error) error {
	if errors.Is(err, ErrTokenNotFoundOrStatusMismatch) {
		return ErrConflict
	}
	return err
}

func toMetadata(token Token) TokenMetadata {
	return TokenMetadata{
		TokenID: token.TokenID, AppID: token.AppID, Name: token.Name,
		TokenType: token.TokenType, Status: Status(token.Status), Env: token.Env,
		Scopes: token.Scopes, Quota: token.Quota, ExpiresAt: token.ExpiresAt,
		OwnerID: token.OwnerID, CreatedAt: token.CreatedAt, UpdatedAt: token.UpdatedAt,
	}
}
