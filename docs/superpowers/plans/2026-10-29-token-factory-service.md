# Token Factory 独立服务 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build an independent Go Token Factory service backed by the existing MySQL/Redis containers, migrate the retained uni-app x console to Token Factory workflows, and remove only the obsolete `static/` frontend after end-to-end validation.

**Architecture:** Add a nested `token-factory/` Go module as a modular monolith with explicit domain packages for applications, tokens, policy, approval, validation, usage, audit, and risk. MySQL `token_factory` is the source of truth; Redis uses the `tf:` namespace for cache, blacklist, quotas, rate limits, and locks. The existing DeepNow gateway remains independent until a later Adapter task is enabled.

**Tech Stack:** Go 1.21+, `net/http`, MySQL 8 with `github.com/go-sql-driver/mysql`, Redis 7 with `github.com/redis/go-redis/v9`, AES-GCM, uni-app x H5, TypeScript/UTS, Vitest, Python Playwright, Docker Desktop.

---

## Execution rules

- Agents must work in the current repository without reverting unrelated existing changes.
- Each task starts with a failing test, observes the failure, then implements the minimum behavior.
- Each agent owns only the files listed in its task and creates a separate commit.
- No real password, Secret, Token, or local credential may enter Git, logs, fixtures, or docs.
- Existing MySQL container `mysql8` on `3306` and Redis container `redis` on `6379` are reused; no container removal or data reset is allowed.
- The old `static/` directory is deleted only in Task 12 after Task 11 integration checks pass.
- The current `frontend/` uni-app x project is retained and migrated in place.

## File map

Create:

- `token-factory/go.mod`
- `token-factory/cmd/token-factory/main.go`
- `token-factory/internal/app/`
- `token-factory/internal/application/`
- `token-factory/internal/token/`
- `token-factory/internal/policy/`
- `token-factory/internal/approval/`
- `token-factory/internal/validation/`
- `token-factory/internal/usage/`
- `token-factory/internal/audit/`
- `token-factory/internal/risk/`
- `token-factory/internal/notification/`
- `token-factory/internal/infrastructure/mysql/`
- `token-factory/internal/infrastructure/redis/`
- `token-factory/internal/infrastructure/crypto/`
- `token-factory/internal/http/`
- `token-factory/migrations/`
- `token-factory/api/openapi.yaml`
- `token-factory/tests/`
- `token-factory/.env.example`
- `token-factory/README.md`

Modify:

- `frontend/src/pages.json`
- `frontend/src/layouts/DesktopSidebar.uvue`
- `frontend/src/layouts/MobileTabbar.uvue`
- `frontend/src/services/http.uts`
- `frontend/src/types/domain.uts`
- `frontend/package.json`
- `frontend/README.md`

Delete only in the final task:

- `static/index.html`
- `static/`

## Task 1: Token Factory service foundation

**Owner:** Agent A  
**Depends on:** None  
**Files:**
- Create: `token-factory/go.mod`
- Create: `token-factory/cmd/token-factory/main.go`
- Create: `token-factory/internal/app/config.go`
- Create: `token-factory/internal/app/server.go`
- Create: `token-factory/internal/http/health.go`
- Create: `token-factory/internal/app/config_test.go`
- Create: `token-factory/.env.example`
- Create: `token-factory/README.md`

- [ ] **Step 1: Write failing configuration tests**

```go
func TestLoadConfigRequiresDSNAndUsesSafeDefaults(t *testing.T) {
    t.Setenv("TOKEN_FACTORY_HTTP_ADDR", ":8090")
    t.Setenv("TOKEN_FACTORY_MYSQL_DSN", "")
    _, err := LoadConfig()
    if err == nil {
        t.Fatal("expected missing MySQL DSN error")
    }
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run:

```bash
cd token-factory
GOTOOLCHAIN=local GOPROXY=off go test ./internal/app -run TestLoadConfigRequiresDSNAndUsesSafeDefaults -v
```

Expected: FAIL because the module and config loader do not exist.

- [ ] **Step 3: Implement configuration and server skeleton**

Define:

```go
type Config struct {
    HTTPAddr      string
    MySQLDSN      string
    RedisAddr     string
    RedisPassword string
    RedisPrefix   string
    SecretKey     []byte
}

func LoadConfig() (Config, error)
func NewServer(cfg Config) (*http.Server, func() error, error)
```

Require `TOKEN_FACTORY_MYSQL_DSN` and `TOKEN_FACTORY_SECRET_KEY`. Default HTTP address is `:8090`, Redis address is `127.0.0.1:6379`, and Redis prefix is `tf:`. Never print DSN, Redis password, or SecretKey.

- [ ] **Step 4: Add `/healthz`**

Return `200` only when the service process is alive; expose dependency details only through an authenticated diagnostics endpoint in a later task. The public response must be:

```json
{"status":"ok","service":"token-factory"}
```

- [ ] **Step 5: Run tests and build**

```bash
cd token-factory
GOTOOLCHAIN=local GOPROXY=off go test ./...
GOTOOLCHAIN=local GOPROXY=off CGO_ENABLED=0 go build ./cmd/token-factory
```

Expected: PASS and a CGO-free binary.

- [ ] **Step 6: Commit**

```bash
git add token-factory
git commit -m "feat: add token factory service foundation"
```

## Task 2: MySQL migrations and repositories

**Owner:** Agent B  
**Depends on:** Task 1  
**Files:**
- Create: `token-factory/migrations/001_initial.sql`
- Create: `token-factory/internal/infrastructure/mysql/client.go`
- Create: `token-factory/internal/infrastructure/mysql/migrations.go`
- Create: `token-factory/internal/application/repository.go`
- Create: `token-factory/internal/token/repository.go`
- Create: `token-factory/internal/approval/repository.go`
- Create: `token-factory/internal/audit/repository.go`
- Create: `token-factory/internal/infrastructure/mysql/*_test.go`

- [ ] **Step 1: Write migration/repository tests**

Test that `applications`, `tokens`, `token_versions`, `approval_requests`, `approval_steps`, and `audit_logs` are created, migration version is recorded, and a token metadata round trip preserves status, scopes, quota, owner, and timestamps. Use `go-sqlmock` for repository unit tests; do not require a destructive local database.

- [ ] **Step 2: Run focused tests red**

```bash
cd token-factory
GOTOOLCHAIN=local GOPROXY=off go test ./internal/infrastructure/mysql ./internal/application ./internal/token -v
```

Expected: FAIL because schema and repositories are missing.

- [ ] **Step 3: Add idempotent MySQL migration**

Create `schema_migrations` and the tables from the approved design. Use UTC `DATETIME(6)`, explicit indexes, unique constraints, JSON columns for scopes/quota/payload, and no plaintext Secret column. Running migration twice must return success without altering existing rows.

- [ ] **Step 4: Implement repository interfaces**

Define interfaces:

```go
type ApplicationRepository interface {
    Create(ctx context.Context, app Application) error
    Get(ctx context.Context, appID string) (Application, error)
    List(ctx context.Context, filter ApplicationFilter) (Page[Application], error)
}

type TokenRepository interface {
    Create(ctx context.Context, token Token) error
    Get(ctx context.Context, tokenID string) (Token, error)
    UpdateStatus(ctx context.Context, tokenID string, from, to Status) error
    AddVersion(ctx context.Context, version TokenVersion) error
    ListVersions(ctx context.Context, tokenID string) ([]TokenVersion, error)
}
```

All status updates must include the expected previous status to prevent lost transitions.

- [ ] **Step 5: Run tests and commit**

```bash
cd token-factory
GOTOOLCHAIN=local GOPROXY=off go test ./...
git add token-factory
git commit -m "feat: add token factory mysql schema and repositories"
```

## Task 3: Redis cache, blacklist, quota, and lock adapters

**Owner:** Agent C  
**Depends on:** Task 1  
**Files:**
- Create: `token-factory/internal/infrastructure/redis/client.go`
- Create: `token-factory/internal/infrastructure/redis/token_cache.go`
- Create: `token-factory/internal/infrastructure/redis/blacklist.go`
- Create: `token-factory/internal/infrastructure/redis/quota.go`
- Create: `token-factory/internal/infrastructure/redis/lock.go`
- Create: `token-factory/internal/infrastructure/redis/*_test.go`

- [ ] **Step 1: Write miniredis tests**

Cover:

```go
TestTokenCacheUsesTfPrefixAndExpires
TestBlacklistTTLMatchesTokenExpiry
TestQuotaIncrementsAtomically
TestQuotaRejectsWhenDailyLimitExceeded
TestLockCannotBeAcquiredTwice
```

- [ ] **Step 2: Run tests red**

```bash
cd token-factory
GOTOOLCHAIN=local GOPROXY=off go test ./internal/infrastructure/redis -v
```

Expected: FAIL because Redis adapters do not exist.

- [ ] **Step 3: Implement adapters**

Use `go-redis/v9`. Every key must be built through one prefix helper:

```go
func Key(prefix, kind, id string) string {
    return prefix + kind + ":" + id
}
```

Use Lua or atomic Redis commands for quota and rate counters. Lock release must verify ownership value before deleting.

- [ ] **Step 4: Verify failure policy**

Add tests that Redis errors return typed errors and never return an implicit Allow decision. The validation service will decide whether to fail closed or use a MySQL fallback.

- [ ] **Step 5: Run tests and commit**

```bash
cd token-factory
GOTOOLCHAIN=local GOPROXY=off go test ./...
git add token-factory
git commit -m "feat: add token factory redis adapters"
```

## Task 4: Token lifecycle and Secret protection

**Owner:** Agent D  
**Depends on:** Tasks 2 and 3  
**Files:**
- Create: `token-factory/internal/token/model.go`
- Create: `token-factory/internal/token/service.go`
- Create: `token-factory/internal/token/secret.go`
- Create: `token-factory/internal/token/service_test.go`
- Create: `token-factory/internal/infrastructure/crypto/aes_gcm.go`
- Create: `token-factory/internal/infrastructure/crypto/aes_gcm_test.go`

- [ ] **Step 1: Write failing lifecycle tests**

Cover:

```go
TestCreateTokenReturnsPlaintextOnlyOnce
TestCreateTokenStoresCiphertextAndHashOnly
TestRotateGracefulKeepsOldVersionUntilGraceEnd
TestRevokeUpdatesStatusAndBlacklist
TestInvalidStateTransitionReturnsConflict
TestIdempotencyKeyDoesNotCreateDuplicateToken
```

- [ ] **Step 2: Run lifecycle tests red**

```bash
cd token-factory
GOTOOLCHAIN=local GOPROXY=off go test ./internal/token -v
```

Expected: FAIL.

- [ ] **Step 3: Implement domain states and Secret service**

Use typed statuses:

```go
type Status string
const (
    Draft Status = "draft"
    PendingApproval Status = "pending_approval"
    Active Status = "active"
    Rotating Status = "rotating"
    Expired Status = "expired"
    Revoked Status = "revoked"
)
```

Generate at least 32 random bytes using `crypto/rand`. Store AES-GCM ciphertext and SHA-256 hash. Never include Secret in domain `Token` metadata JSON marshaling.

- [ ] **Step 4: Implement create/rotate/revoke/extend**

Use MySQL transactions plus Redis lock for mutations. `Idempotency-Key` must map to a stored result for the same actor and operation payload; a different payload with the same key returns `409`.

- [ ] **Step 5: Run tests and commit**

```bash
cd token-factory
GOTOOLCHAIN=local GOPROXY=off go test ./...
git add token-factory
git commit -m "feat: implement token lifecycle and secret protection"
```

## Task 5: Policy engine and approval workflow

**Owner:** Agent E  
**Depends on:** Task 4  
**Files:**
- Create: `token-factory/internal/policy/model.go`
- Create: `token-factory/internal/policy/engine.go`
- Create: `token-factory/internal/approval/service.go`
- Create: `token-factory/internal/policy/engine_test.go`
- Create: `token-factory/internal/approval/service_test.go`

- [ ] **Step 1: Write table-driven policy tests**

Cases must cover automatic approval, production approval, high-scope security approval, cross-department resource approval, and high-quota platform approval.

- [ ] **Step 2: Run policy tests red**

```bash
cd token-factory
GOTOOLCHAIN=local GOPROXY=off go test ./internal/policy ./internal/approval -v
```

Expected: FAIL.

- [ ] **Step 3: Implement risk decision and approval graph**

Define:

```go
type Decision struct {
    RiskLevel string
    AutoApprove bool
    RequiredSteps []ApprovalStep
    Reasons []string
}

func (e *Engine) Evaluate(ctx context.Context, request Request) (Decision, error)
```

Approval transitions are `pending -> approved/rejected/returned`; approve only the current step and create the next step transactionally.

- [ ] **Step 4: Connect approval to token issuance**

Automatic approval calls token issuance immediately. Manual approval creates `PendingApproval`; final approval triggers exactly one issuance operation through the idempotency key.

- [ ] **Step 5: Run tests and commit**

```bash
cd token-factory
GOTOOLCHAIN=local GOPROXY=off go test ./...
git add token-factory
git commit -m "feat: add token policy and approval workflow"
```

## Task 6: Validation, quota, and usage service

**Owner:** Agent F  
**Depends on:** Tasks 2, 3, and 4  
**Files:**
- Create: `token-factory/internal/validation/service.go`
- Create: `token-factory/internal/usage/service.go`
- Create: `token-factory/internal/validation/service_test.go`
- Create: `token-factory/internal/usage/service_test.go`

- [ ] **Step 1: Write failing validation tests**

Cover valid Token, unknown hash, expired, revoked, missing Scope, bad IP, wrong environment, quota exceeded, and Redis failure.

- [ ] **Step 2: Run tests red**

```bash
cd token-factory
GOTOOLCHAIN=local GOPROXY=off go test ./internal/validation ./internal/usage -v
```

Expected: FAIL.

- [ ] **Step 3: Implement validation**

Define:

```go
type ValidateRequest struct {
    RawToken string
    RequiredScope string
    CallerIP string
    CallerService string
    Environment string
}

type ValidateResult struct {
    Allow bool
    Reason string
    TokenID string
    AppID string
    Scopes []string
    ExpiresAt time.Time
}
```

Check blacklist before cache, then cache, then MySQL. Never return `Allow=true` when blacklist or quota state cannot be safely resolved.

- [ ] **Step 4: Implement usage aggregation**

Record accepted and denied calls with request ID, endpoint, status, latency, reason, and Token ID. Aggregate daily usage transactionally or through an explicit idempotent event key.

- [ ] **Step 5: Run tests and commit**

```bash
cd token-factory
GOTOOLCHAIN=local GOPROXY=off go test ./...
git add token-factory
git commit -m "feat: add token validation quota and usage"
```

## Task 7: Audit, risk, notification interfaces

**Owner:** Agent G  
**Depends on:** Tasks 4, 5, and 6  
**Files:**
- Create: `token-factory/internal/audit/service.go`
- Create: `token-factory/internal/risk/service.go`
- Create: `token-factory/internal/notification/service.go`
- Create: `token-factory/internal/audit/service_test.go`
- Create: `token-factory/internal/risk/service_test.go`

- [ ] **Step 1: Write failing audit/risk tests**

Assert that create, secret copy, validate, rotate, revoke, approval, quota rejection, and risk resolution each create an audit event. Assert risk score ranges:

```text
0-30 Low
31-60 Medium
61-80 High
81-100 Critical
```

- [ ] **Step 2: Run tests red**

```bash
cd token-factory
GOTOOLCHAIN=local GOPROXY=off go test ./internal/audit ./internal/risk -v
```

Expected: FAIL.

- [ ] **Step 3: Implement event and risk services**

Notification must be an interface with a no-op local implementation:

```go
type Sender interface {
    Send(ctx context.Context, message Message) error
}
```

Do not send real notifications during tests. Risk service creates events and returns recommended actions without mutating tokens unless an explicit policy action is invoked.

- [ ] **Step 4: Run tests and commit**

```bash
cd token-factory
GOTOOLCHAIN=local GOPROXY=off go test ./...
git add token-factory
git commit -m "feat: add token audit risk and notification contracts"
```

## Task 8: HTTP API and OpenAPI contract

**Owner:** Agent A + Agent D/E/F/G reviewers  
**Depends on:** Tasks 1-7  
**Files:**
- Create: `token-factory/internal/http/router.go`
- Create: `token-factory/internal/http/apps_handler.go`
- Create: `token-factory/internal/http/tokens_handler.go`
- Create: `token-factory/internal/http/approvals_handler.go`
- Create: `token-factory/internal/http/audit_handler.go`
- Create: `token-factory/internal/http/policies_handler.go`
- Create: `token-factory/internal/http/handlers_test.go`
- Create: `token-factory/api/openapi.yaml`

- [ ] **Step 1: Write handler contract tests**

Cover status codes `200/201/400/401/403/404/409/422/429/500`, pagination, request IDs, idempotency, one-time Secret, and no Secret in list/detail/audit responses.

- [ ] **Step 2: Run handler tests red**

```bash
cd token-factory
GOTOOLCHAIN=local GOPROXY=off go test ./internal/http -v
```

Expected: FAIL.

- [ ] **Step 3: Implement routes**

Register the approved `/api/v1` endpoints. Use one middleware chain for request ID, authentication, actor context, panic recovery, structured logging with Secret redaction, and JSON response envelopes.

- [ ] **Step 4: Generate/validate OpenAPI**

The checked-in OpenAPI file must describe request/response schemas, `Idempotency-Key`, authentication, error envelopes, and pagination. Validation must run without network access.

- [ ] **Step 5: Run tests and commit**

```bash
cd token-factory
GOTOOLCHAIN=local GOPROXY=off go test ./...
git add token-factory
git commit -m "feat: expose token factory api"
```

## Task 9: Migrate the retained uni-app x console

**Owner:** Agent H  
**Depends on:** Task 8 OpenAPI schemas  
**Files:**
- Modify: `frontend/src/pages.json`
- Modify: `frontend/src/layouts/DesktopSidebar.uvue`
- Modify: `frontend/src/layouts/MobileTabbar.uvue`
- Modify: `frontend/src/services/http.uts`
- Modify: `frontend/src/types/domain.uts`
- Create: `frontend/src/services/token-factory.uts`
- Create: `frontend/src/services/applications.uts`
- Create: `frontend/src/services/approvals.uts`
- Create: `frontend/src/services/risks.uts`
- Create: `frontend/src/pages/applications/*`
- Create: `frontend/src/pages/tokens/*`
- Create: `frontend/src/pages/approvals/*`
- Create: `frontend/src/pages/risks/*`
- Create: `frontend/src/pages/audit/*`
- Modify: `frontend/tests/unit/*`
- Create: `frontend/tests/e2e/token_factory_console.py`

- [ ] **Step 1: Write frontend service/page tests**

Cover one-time Secret clearing, application list normalization, Token filters, approval actions, risk status mapping, and no Secret persistence.

- [ ] **Step 2: Run frontend tests red**

```bash
cd frontend
npm run test:unit -- --run
```

Expected: new tests fail before services/pages exist; existing 27 tests remain passing.

- [ ] **Step 3: Implement API services and pages**

Use the existing request layer and type-safe services. Keep the current H5 responsive shell. Remove old model/RAG-only navigation after equivalent Token Factory navigation is available.

- [ ] **Step 4: Validate H5**

```bash
cd frontend
npm run test:unit -- --run
npm run build:h5
npm run test:e2e -- --base-url http://127.0.0.1:4173
```

Expected: desktop/mobile navigation, Token create, one-time Secret, approval, rotate, revoke, and audit flows pass with mocked API data.

- [ ] **Step 5: Commit**

```bash
git add frontend
git commit -m "feat: migrate uniapp x console to token factory"
```

## Task 10: DeepNow Gateway Adapter

**Owner:** Agent I  
**Depends on:** Task 6 and Task 8  
**Files:**
- Create: `token-factory/internal/adapter/deepnow/client.go`
- Create: `token-factory/internal/adapter/deepnow/client_test.go`
- Modify: `src/api_gateway.go` only behind an explicit configuration flag
- Modify: `src/main.go` only to load adapter configuration
- Create: `src/deepnow_adapter_test.go`

- [ ] **Step 1: Write adapter tests**

Cover valid validation, denied validation, timeout, Token Factory unavailable, usage report, and disabled adapter behavior. Existing gateway tests must pass with the flag disabled.

- [ ] **Step 2: Implement disabled-by-default adapter**

The adapter calls `POST /api/v1/tokens/validate` and optionally reports usage. The current gateway continues using existing API Key behavior until the feature flag is enabled.

- [ ] **Step 3: Run tests**

```bash
GOTOOLCHAIN=local GOPROXY=off CGO_ENABLED=0 go test ./src -race -v
cd token-factory
GOTOOLCHAIN=local GOPROXY=off go test ./...
```

- [ ] **Step 4: Commit**

```bash
git add src token-factory
git commit -m "feat: add optional deepnow token factory adapter"
```

## Task 11: MySQL/Redis integration and local run

**Owner:** Agent J  

**Depends on:** Tasks 1-10  
**Files:**
- Create: `token-factory/tests/integration_test.go`
- Create: `token-factory/scripts/check-dependencies.sh`
- Modify: `token-factory/README.md`
- Modify: `frontend/README.md`

- [ ] **Step 1: Write integration checks**

The integration test must skip with a clear message when dependencies are unavailable and run when `TOKEN_FACTORY_MYSQL_DSN` and `TOKEN_FACTORY_REDIS_ADDR` are configured. It must verify:

```text
healthz
MySQL migration
application creation
Token automatic approval
Secret one-time response
Token validation
quota rejection
graceful rotation
revoke + Redis blacklist
audit log creation
```

- [ ] **Step 2: Check the existing containers without mutating them**

Run:

```bash
docker ps --format 'table {{.Names}}\t{{.Image}}\t{{.Ports}}\t{{.Status}}'
docker exec mysql8 mysqladmin ping -uroot -p"$MYSQL_ROOT_PASSWORD"
docker exec redis redis-cli ping
```

The password must come from the shell environment; do not put it in the command history, script, or repository.

- [ ] **Step 3: Implement dependency initialization**

Create a local-only initialization command that creates the `token_factory` database and runs migrations. It must use `CREATE DATABASE IF NOT EXISTS`, never drop tables, and print only database name and migration version.

- [ ] **Step 4: Run integration and all unit tests**

```bash
cd token-factory
GOTOOLCHAIN=local GOPROXY=off go test ./...
TOKEN_FACTORY_INTEGRATION=1 GOTOOLCHAIN=local GOPROXY=off go test ./tests -v

cd ../frontend
npm run test:unit -- --run
npm run build:h5
```

- [ ] **Step 5: Commit**

```bash
git add token-factory frontend/README.md
git commit -m "test: add token factory mysql redis integration checks"
```

## Task 12: Remove the obsolete static frontend and final regression

**Owner:** Agent J with parent-agent review  
**Depends on:** Tasks 9 and 11  
**Files:**
- Delete: `static/index.html`
- Delete: `static/`
- Modify: `README.md`
- Modify: `README-eng.md`
- Modify: `.gitignore`
- Modify: `frontend/README.md`

- [ ] **Step 1: Write the cleanup regression test**

Before deletion, add a test that starts the Token Factory HTTP router and verifies `/` serves the current H5 entry while `/api/v1/...` is still routed to JSON handlers. The test must not depend on `static/`.

- [ ] **Step 2: Run the regression test red**

Run:

```bash
cd token-factory
GOTOOLCHAIN=local GOPROXY=off go test ./internal/http -run TestRootServesTokenFactoryFrontend -v
```

Expected: FAIL until the new frontend handler is registered.

- [ ] **Step 3: Delete only the old static frontend**

Run:

```bash
git rm -r static
```

Do not delete `frontend/`, `token-factory/`, current Go tests, or database files.

- [ ] **Step 4: Update documentation**

Replace old instructions that point to the embedded `static/` page with:

```text
cd token-factory
go run ./cmd/token-factory
```

Document required environment variables without values and state that MySQL/Redis containers are reused.

- [ ] **Step 5: Run final regression**

```bash
GOTOOLCHAIN=local GOPROXY=off CGO_ENABLED=0 go test ./src -race -count=1
GOTOOLCHAIN=local GOPROXY=off CGO_ENABLED=0 go build -o /tmp/deepnow ./src

cd token-factory
GOTOOLCHAIN=local GOPROXY=off go test ./...

cd ../frontend
npm run test:unit -- --run
npm run build:h5
npm run test:e2e -- --base-url http://127.0.0.1:4173
```

Expected:

- All Go tests pass with race detection.
- Token Factory tests pass.
- H5 build exits `0` with no `error TS`, `nullable`, or `Build failed`.
- Playwright passes at desktop/tablet/mobile viewports.
- No source or log contains the real database password or a plaintext test Secret.

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "feat: launch standalone token factory service"
```

## Dependency checkpoints

```text
Checkpoint 1: Task 1
  Service boots with config and health endpoint.

Checkpoint 2: Tasks 2-3
  MySQL and Redis contracts are independently tested.

Checkpoint 3: Tasks 4-7
  Domain lifecycle, approval, validation, usage, audit, and risk are tested.

Checkpoint 4: Task 8
  OpenAPI contract is stable before frontend migration.

Checkpoint 5: Task 9
  uni-app x console works against mocked Token Factory APIs.

Checkpoint 6: Task 11
  Real existing MySQL/Redis containers pass integration checks.

Checkpoint 7: Task 12
  static/ is removed only after the new service and console are verified.
```

## Plan self-review

- The approved design's independent service boundary is implemented by Tasks 1 and 8.
- MySQL DDL and migrations are covered by Task 2 and integration checks in Task 11.
- Redis key contracts, TTLs, quota, blacklist, and lock behavior are covered by Task 3 and Task 6.
- Token lifecycle and one-time Secret protection are covered by Task 4.
- Approval and risk policies are covered by Task 5 and Task 7.
- API/OpenAPI stability is established before frontend changes in Task 8.
- The retained uni-app x console is migrated in Task 9.
- DeepNow integration stays disabled by default in Task 10.
- Existing `static/` deletion is isolated to Task 12 and guarded by a regression test.
- Every task has a concrete file scope, test command, expected behavior, and commit boundary.
