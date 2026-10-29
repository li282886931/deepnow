package token

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	tfcrypto "github.com/li282886931/deepnow/token-factory/internal/infrastructure/crypto"
)

type fakeTokenRepository struct {
	mu       sync.Mutex
	tokens   map[string]Token
	versions map[string][]TokenVersion
}

func newFakeTokenRepository() *fakeTokenRepository {
	return &fakeTokenRepository{
		tokens:   make(map[string]Token),
		versions: make(map[string][]TokenVersion),
	}
}

func (r *fakeTokenRepository) Create(_ context.Context, token Token) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.tokens[token.TokenID]; exists {
		return errors.New("duplicate token")
	}
	r.tokens[token.TokenID] = token
	return nil
}

func (r *fakeTokenRepository) Get(_ context.Context, tokenID string) (Token, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	token, ok := r.tokens[tokenID]
	if !ok {
		return Token{}, ErrTokenNotFound
	}
	return token, nil
}

func (r *fakeTokenRepository) UpdateStatus(_ context.Context, tokenID, from, to string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	token, ok := r.tokens[tokenID]
	if !ok || token.Status != from {
		return ErrTokenNotFoundOrStatusMismatch
	}
	token.Status = to
	token.UpdatedAt = time.Now().UTC()
	r.tokens[tokenID] = token
	return nil
}

func (r *fakeTokenRepository) AddVersion(_ context.Context, version TokenVersion) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.versions[version.TokenID] = append(r.versions[version.TokenID], version)
	return nil
}

func (r *fakeTokenRepository) ListVersions(_ context.Context, tokenID string) ([]TokenVersion, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	versions := append([]TokenVersion(nil), r.versions[tokenID]...)
	return versions, nil
}

func (r *fakeTokenRepository) UpdateExpiry(_ context.Context, tokenID string, expiresAt time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	token, ok := r.tokens[tokenID]
	if !ok {
		return ErrTokenNotFound
	}
	token.ExpiresAt = expiresAt
	r.tokens[tokenID] = token
	return nil
}

func (r *fakeTokenRepository) UpdateVersion(_ context.Context, version TokenVersion) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for index := range r.versions[version.TokenID] {
		if r.versions[version.TokenID][index].Version == version.Version {
			r.versions[version.TokenID][index] = version
			return nil
		}
	}
	return ErrTokenNotFound
}

type fakeBlacklist struct {
	mu       sync.Mutex
	hashes   []string
	expiries []time.Time
}

func (b *fakeBlacklist) Add(_ context.Context, hash, _ string, expiresAt time.Time) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.hashes = append(b.hashes, hash)
	b.expiries = append(b.expiries, expiresAt)
	return nil
}

type fakeLock struct{}

func (fakeLock) Release(context.Context) error { return nil }

type fakeLocks struct{}

func (fakeLocks) Acquire(context.Context, string, time.Duration) (Lock, error) {
	return fakeLock{}, nil
}

func newTestService(t *testing.T, repo *fakeTokenRepository, blacklist *fakeBlacklist) *Service {
	t.Helper()
	cipher, err := tfcrypto.NewAESGCM([]byte(strings.Repeat("k", 32)))
	if err != nil {
		t.Fatal(err)
	}
	return NewService(repo, cipher, blacklist, fakeLocks{})
}

func TestCreateTokenReturnsPlaintextOnlyOnce(t *testing.T) {
	repo := newFakeTokenRepository()
	service := newTestService(t, repo, &fakeBlacklist{})

	result, err := service.Create(context.Background(), CreateRequest{
		ActorID:   "user_1",
		AppID:     "app_1",
		Name:      "billing",
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if result.Secret == "" || !strings.HasPrefix(result.Secret, "tf_live_") {
		t.Fatalf("Create() returned invalid one-time secret %q", result.Secret)
	}
	if _, err := service.Get(context.Background(), result.TokenID); err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	metadata, err := json.Marshal(result.Metadata)
	if err != nil {
		t.Fatalf("Marshal(metadata) error = %v", err)
	}
	if strings.Contains(string(metadata), result.Secret) {
		t.Fatalf("metadata contains plaintext secret: %s", metadata)
	}
}

func TestCreateTokenStoresCiphertextAndHashOnly(t *testing.T) {
	repo := newFakeTokenRepository()
	service := newTestService(t, repo, &fakeBlacklist{})

	result, err := service.Create(context.Background(), CreateRequest{
		ActorID:   "user_1",
		AppID:     "app_1",
		Name:      "search",
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	versions := repo.versions[result.TokenID]
	if len(versions) != 1 || len(versions[0].SecretCiphertext) == 0 ||
		versions[0].SecretHash == "" || string(versions[0].SecretCiphertext) == result.Secret {
		t.Fatalf("stored version exposes plaintext: %+v", versions)
	}
	if versions[0].SecretHash != HashSecret(result.Secret) {
		t.Fatalf("SecretHash = %q, want hash of generated secret", versions[0].SecretHash)
	}
}

func TestRotateGracefulKeepsOldVersionUntilGraceEnd(t *testing.T) {
	repo := newFakeTokenRepository()
	service := newTestService(t, repo, &fakeBlacklist{})
	created, err := service.Create(context.Background(), CreateRequest{
		ActorID: "user_1", AppID: "app_1", Name: "orders",
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	rotated, err := service.Rotate(context.Background(), created.TokenID, RotateRequest{
		ActorID: "user_1", GracePeriod: 30 * time.Minute,
	})
	if err != nil {
		t.Fatalf("Rotate() error = %v", err)
	}
	if rotated.Secret == "" || rotated.Secret == created.Secret {
		t.Fatal("Rotate() did not issue a new one-time secret")
	}
	versions := repo.versions[created.TokenID]
	if len(versions) != 2 || versions[0].Status != string(Active) ||
		versions[0].ValidUntil == nil || versions[1].Status != string(Active) {
		t.Fatalf("Rotate() versions = %+v", versions)
	}
}

func TestRevokeUpdatesStatusAndBlacklist(t *testing.T) {
	repo := newFakeTokenRepository()
	blacklist := &fakeBlacklist{}
	service := newTestService(t, repo, blacklist)
	created, err := service.Create(context.Background(), CreateRequest{
		ActorID: "user_1", AppID: "app_1", Name: "payments",
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if _, err := service.Revoke(context.Background(), created.TokenID, "compromised"); err != nil {
		t.Fatalf("Revoke() error = %v", err)
	}
	token, _ := repo.Get(context.Background(), created.TokenID)
	if token.Status != string(Revoked) {
		t.Fatalf("status = %q, want %q", token.Status, Revoked)
	}
	if len(blacklist.hashes) != 1 || blacklist.hashes[0] != HashSecret(created.Secret) {
		t.Fatalf("blacklist hashes = %+v", blacklist.hashes)
	}
}

func TestInvalidStateTransitionReturnsConflict(t *testing.T) {
	repo := newFakeTokenRepository()
	service := newTestService(t, repo, &fakeBlacklist{})
	repo.tokens["tok_revoked"] = Token{TokenID: "tok_revoked", Status: string(Revoked)}

	_, err := service.Rotate(context.Background(), "tok_revoked", RotateRequest{ActorID: "user_1"})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("Rotate() error = %v, want %v", err, ErrConflict)
	}
}

func TestIdempotencyKeyDoesNotCreateDuplicateToken(t *testing.T) {
	repo := newFakeTokenRepository()
	service := newTestService(t, repo, &fakeBlacklist{})
	request := CreateRequest{
		ActorID: "user_1", IdempotencyKey: "idem-1", AppID: "app_1",
		Name: "notifications", ExpiresAt: time.Now().UTC().Add(time.Hour),
	}
	first, err := service.Create(context.Background(), request)
	if err != nil {
		t.Fatalf("first Create() error = %v", err)
	}
	second, err := service.Create(context.Background(), request)
	if err != nil {
		t.Fatalf("second Create() error = %v", err)
	}
	if first.TokenID != second.TokenID || first.Secret != second.Secret {
		t.Fatalf("idempotent results differ: first=%+v second=%+v", first, second)
	}
	if len(repo.tokens) != 1 {
		t.Fatalf("created %d tokens, want 1", len(repo.tokens))
	}

	request.Name = "different-payload"
	if _, err := service.Create(context.Background(), request); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("conflicting Create() error = %v, want %v", err, ErrIdempotencyConflict)
	}
}

func TestConcurrentIdempotentCreatesIssueOneToken(t *testing.T) {
	repo := newFakeTokenRepository()
	service := newTestService(t, repo, &fakeBlacklist{})
	request := CreateRequest{
		ActorID: "user_1", IdempotencyKey: "concurrent-1", AppID: "app_1",
		Name: "concurrent", ExpiresAt: time.Now().UTC().Add(time.Hour),
	}
	results := make(chan Result, 2)
	errorsCh := make(chan error, 2)
	var wait sync.WaitGroup
	for i := 0; i < 2; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			result, err := service.Create(context.Background(), request)
			results <- result
			errorsCh <- err
		}()
	}
	wait.Wait()
	close(results)
	close(errorsCh)
	var first Result
	for result := range results {
		if first.TokenID == "" {
			first = result
			continue
		}
		if result.TokenID != first.TokenID || result.Secret != first.Secret {
			t.Fatalf("concurrent results differ: first=%+v result=%+v", first, result)
		}
	}
	for err := range errorsCh {
		if err != nil {
			t.Fatalf("concurrent Create() error = %v", err)
		}
	}
	if len(repo.tokens) != 1 {
		t.Fatalf("created %d tokens, want 1", len(repo.tokens))
	}
}

func TestExtendUpdatesExpiry(t *testing.T) {
	repo := newFakeTokenRepository()
	service := newTestService(t, repo, &fakeBlacklist{})
	created, err := service.Create(context.Background(), CreateRequest{
		ActorID: "user_1", AppID: "app_1", Name: "reports",
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	next := time.Now().UTC().Add(4 * time.Hour)
	if _, err := service.Extend(context.Background(), created.TokenID, ExtendRequest{
		ActorID: "user_1", ExpiresAt: next,
	}); err != nil {
		t.Fatalf("Extend() error = %v", err)
	}
	got, _ := repo.Get(context.Background(), created.TokenID)
	if !got.ExpiresAt.Equal(next) {
		t.Fatalf("expiry = %v, want %v", got.ExpiresAt, next)
	}
}
