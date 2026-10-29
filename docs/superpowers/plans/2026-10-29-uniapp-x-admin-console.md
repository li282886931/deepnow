# DeepNow uni-app x Admin Console Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the minimal static DeepNow admin page with a tested uni-app x H5 management console while extending the Go API with the P0/P1 management capabilities defined in the approved specification.

**Architecture:** Keep the Go gateway and SQLite databases as the source of truth. Add a separate `frontend/` uni-app x project whose pages use typed domain services over one request client; serve the H5 build independently first, then add Go embedding only after H5 acceptance. Extend existing routes compatibly, add idempotent SQLite migrations, and never expose stored upstream secrets through admin responses.

**Tech Stack:** Go 1.21, `net/http`, `modernc.org/sqlite`, Go `httptest`, uni-app x, UTS/UVue, TypeScript-compatible frontend service types, Vitest for pure frontend logic, Playwright for H5 workflows.

---

## Scope and file map

The implementation must preserve the existing uncommitted Go fixes and tests. Do not rewrite `src/api_gateway.go` or gateway behavior except where a shared usage record is required for the new Key statistics.

Backend files:

- Modify: `src/main.go` to register new admin routes and later serve the built H5 directory.
- Modify: `src/api_admin.go` to add logout/session, response IDs, model sanitization, model test, document CRUD, diagnostics, and audit writes.
- Modify: `src/database.go` to add idempotent schema migrations and typed database helpers.
- Modify: `src/api_gateway.go` to record request/token/error usage through the usage helper without changing OpenAI response behavior.
- Create: `src/admin_contract_test.go` for HTTP contract, masking, and admin feature tests.
- Create: `src/database_migration_test.go` for fresh and upgrade schema tests.
- Create: `src/test_helpers.go` for shared in-memory database setup, authenticated request construction, and JSON decoding helpers used by the new backend tests.

Frontend files:

- Create: `frontend/package.json`, `frontend/manifest.json`, `frontend/pages.json`, `frontend/App.uvue`, `frontend/main.uts`, `frontend/uni.scss`.
- Create: `frontend/pages/login/login.uvue`.
- Create: `frontend/pages/overview/overview.uvue`.
- Create: `frontend/pages/models/models.uvue`, `frontend/pages/models/edit.uvue`.
- Create: `frontend/pages/knowledge/knowledge.uvue`, `frontend/pages/knowledge/test.uvue`.
- Create: `frontend/pages/access-keys/access-keys.uvue`, `frontend/pages/access-keys/usage.uvue`.
- Create: `frontend/pages/policies/policies.uvue`, `frontend/pages/system/system.uvue`.
- Create: `frontend/pages/audit/audit.uvue`, `frontend/pages/diagnostics/diagnostics.uvue`.
- Create: `frontend/layouts/AppShell.uvue`, `frontend/layouts/DesktopSidebar.uvue`, `frontend/layouts/MobileTabbar.uvue`, `frontend/layouts/TopBar.uvue`.
- Create: `frontend/components/StatusBadge.uvue`, `frontend/components/MetricStrip.uvue`, `frontend/components/DataList.uvue`, `frontend/components/EmptyState.uvue`, `frontend/components/ErrorState.uvue`, `frontend/components/ConfigForm.uvue`, `frontend/components/ConfirmDialog.uvue`, `frontend/components/OperationResult.uvue`.
- Create: `frontend/services/http.uts`, `frontend/services/auth.uts`, `frontend/services/system.uts`, `frontend/services/models.uts`, `frontend/services/keys.uts`, `frontend/services/knowledge.uts`, `frontend/services/policies.uts`, `frontend/services/audit.uts`.
- Create: `frontend/stores/session.uts`, `frontend/stores/console.uts`.
- Create: `frontend/types/api.uts`, `frontend/types/domain.uts`.
- Create: `frontend/utils/errors.uts`, `frontend/utils/format.uts`, `frontend/utils/mask.uts`, `frontend/utils/breakpoints.uts`.
- Create: `frontend/tests/unit/http.test.ts`, `frontend/tests/unit/mask.test.ts`, `frontend/tests/unit/format.test.ts`, `frontend/tests/unit/stores.test.ts`.
- Create: `frontend/tests/e2e/admin-console.spec.ts`.

## Task 1: Lock down the backend response and security contract

**Files:**
- Create: `src/admin_contract_test.go`
- Modify: `src/api_admin.go`
- Modify: `src/main.go`

- [ ] **Step 1: Write failing contract tests**

Add tests that prove the intended public behavior:

```go
func TestSystemStatusNeverReturnsUpstreamAPIKey(t *testing.T) {
    setupAdminFeatureDB(t)
    seedPrimaryModel(t, "openai", "sk-live-secret", "gpt-test")
    request := authenticatedRequest(http.MethodGet, "/api/system/status", nil)
    response := serveAdminRequest(request)

    if response.Code != http.StatusOK {
        t.Fatalf("status = %d, want 200", response.Code)
    }
    if strings.Contains(response.Body.String(), "sk-live-secret") {
        t.Fatal("system status leaked upstream API key")
    }
}

func TestLogoutExpiresSessionAndSessionEndpointReflectsState(t *testing.T) {
    setupAdminFeatureDB(t)
    sessionToken = "session-under-test"
    logout := authenticatedRequest(http.MethodPost, "/api/logout", nil)
    response := serveAdminRequest(logout)

    if response.Code != http.StatusOK {
        t.Fatalf("logout status = %d, want 200", response.Code)
    }
    cookie := response.Result().Cookies()[0]
    if cookie.Name != "session" || cookie.MaxAge >= 0 {
        t.Fatalf("logout cookie = %#v, want expired session cookie", cookie)
    }
}
```

Add the shared test helpers in `src/test_helpers.go` before running the tests:

```go
func setupAdminFeatureDB(t *testing.T)
func authenticatedRequest(method, path string, body io.Reader) *http.Request
func authenticatedJSONRequest(method, path string, body io.Reader) *http.Request
func serveAdminRequest(r *http.Request) *httptest.ResponseRecorder
func decodeJSON(t *testing.T, response *httptest.ResponseRecorder, target any)
```

The helpers must use in-memory SQLite databases, set `sessionToken = "session-under-test"`, and register the same `/api/*` handlers as `main.go` without starting a network listener.

- [ ] **Step 2: Run the focused tests and verify they fail**

Run:

```bash
CGO_ENABLED=0 go test ./src -run 'TestSystemStatusNeverReturnsUpstreamAPIKey|TestLogoutExpiresSessionAndSessionEndpointReflectsState' -v
```

Expected: FAIL because the current status response serializes `ModelConfig` directly and logout/session handlers do not exist.

- [ ] **Step 3: Implement the shared response and session helpers**

Add a `requestID` response field, a `sanitizedModelConfig` DTO, `handleAdminLogout`, and `handleAdminSession`. Make `handleGetSystemStatus` and `handleGetModelConfigs` return only sanitized DTOs. Register:

```go
muxWeb.HandleFunc("/api/logout", handleAdminLogout)
muxWeb.HandleFunc("/api/session", requireAdmin(handleAdminSession))
```

The logout handler must issue an expired `HttpOnly`, `SameSite=Lax` cookie and clear the in-memory session token. The status code for an unauthenticated protected request remains `401`.

- [ ] **Step 4: Run the focused tests and the existing admin tests**

Run:

```bash
CGO_ENABLED=0 go test ./src -run 'TestSystemStatusNeverReturnsUpstreamAPIKey|TestLogoutExpiresSessionAndSessionEndpointReflectsState|TestAdmin' -v
```

Expected: PASS.

- [ ] **Step 5: Commit the backend contract change**

```bash
git add src/api_admin.go src/main.go src/admin_contract_test.go
git commit -m "feat: harden admin response and session contract"
```

## Task 2: Add idempotent SQLite migrations and management data

**Files:**
- Create: `src/database_migration_test.go`
- Modify: `src/database.go`
- Modify: `src/api_admin.go`
- Modify: `src/api_gateway.go`
- Modify: `src/admin_contract_test.go`
- Modify: `src/gateway_test.go`

- [ ] **Step 1: Write migration and usage tests**

Cover a fresh database, an existing database with the current schema, and a second migration run:

```go
func TestMigrateManagementSchemaIsIdempotent(t *testing.T) {
    db := openTestDB(t)
    createCurrentSchema(t, db)
    if err := migrateManagementSchema(db); err != nil {
        t.Fatal(err)
    }
    if err := migrateManagementSchema(db); err != nil {
        t.Fatal(err)
    }

    assertColumnExists(t, db, "access_keys", "name")
    assertColumnExists(t, db, "access_keys", "status")
    assertTableExists(t, db, "key_usage_daily")
    assertTableExists(t, db, "knowledge_documents")
    assertTableExists(t, db, "audit_events")
}

func TestRecordKeyUsageUpdatesDailyCounters(t *testing.T) {
    setupGatewayFeatureDB(t)
    if err := recordKeyUsage("sk-test", 12, 8, false); err != nil {
        t.Fatal(err)
    }
    var requests, input, output, errors int
    err := keyDB.QueryRow(`SELECT request_count, input_tokens, output_tokens, error_count
        FROM key_usage_daily WHERE api_key=?`, "sk-test").
        Scan(&requests, &input, &output, &errors)
    if err != nil {
        t.Fatal(err)
    }
    if requests != 1 || input != 12 || output != 8 || errors != 0 {
        t.Fatalf("usage = %d/%d/%d/%d", requests, input, output, errors)
    }
}
```

- [ ] **Step 2: Run the tests and verify the migration API is missing**

Run:

```bash
CGO_ENABLED=0 go test ./src -run 'TestMigrateManagementSchemaIsIdempotent|TestRecordKeyUsageUpdatesDailyCounters' -v
```

Expected: FAIL because the migration and usage helpers do not exist.

- [ ] **Step 3: Implement the schema migration**

Add idempotent migration helpers in `src/database.go`:

```go
func migrateManagementSchema(db *sql.DB) error
func recordKeyUsage(apiKey string, inputTokens, outputTokens int, failed bool) error
func writeAuditEvent(actor, action, resourceType, resourceID, result string) error
```

Use `CREATE TABLE IF NOT EXISTS` for `key_usage_daily`, `knowledge_documents`, and `audit_events`. Add missing `access_keys` columns with an inspect-before-`ALTER TABLE` helper so a second run does not depend on SQLite error-string matching. Store dates as UTC `YYYY-MM-DD`.

Call migrations from `initDatabases` after base tables exist. Update `incrementKeyUsage` to update `last_used_at` and call `recordKeyUsage` with zero token values; later token extraction may populate the counters without changing this API.

- [ ] **Step 4: Connect document and audit records to existing handlers**

When `handleRAGUpload` starts, insert a `knowledge_documents` row with `status='processing'`; update it to `ready` with file size and chunk count or `error` with a bounded error string. Make delete operate on the document record and its indexed chunks. Add audit writes to login success/failure, config save, model save/activate, Key mutations, upload, and document delete.

- [ ] **Step 5: Run all Go tests**

Run:

```bash
CGO_ENABLED=0 go test ./src -race -v
```

Expected: PASS, including the existing atomic usage test.

- [ ] **Step 6: Commit the migration**

```bash
git add src/database.go src/api_admin.go src/api_gateway.go src/admin_contract_test.go src/database_migration_test.go src/gateway_test.go
git commit -m "feat: add admin management schema and usage records"
```

## Task 3: Complete the backend P0/P1 admin API

**Files:**
- Modify: `src/api_admin.go`
- Modify: `src/main.go`
- Modify: `src/admin_contract_test.go`

- [ ] **Step 1: Write endpoint contract tests**

Cover the new endpoints with authenticated requests:

```go
func TestGenerateKeysReturnsPlaintextOnlyInCreationResponse(t *testing.T) {
    setupAdminFeatureDB(t)
    response := serveAdminRequest(authenticatedRequest(http.MethodPost, "/api/keys/generate", nil))
    var body struct {
        Success bool     `json:"success"`
        Data    []string `json:"data"`
    }
    decodeJSON(t, response, &body)
    if !body.Success || len(body.Data) != 5 {
        t.Fatalf("unexpected generation response: %#v", body)
    }
    list := serveAdminRequest(authenticatedRequest(http.MethodGet, "/api/keys/list", nil))
    if strings.Contains(list.Body.String(), body.Data[0]) {
        t.Fatal("key list returned plaintext generated key")
    }
}

func TestRAGDocumentsAndDiagnosticsReturnStableData(t *testing.T) {
    setupAdminFeatureDB(t)
    documents := serveAdminRequest(authenticatedRequest(http.MethodGet, "/api/rag/documents", nil))
    diagnostics := serveAdminRequest(authenticatedRequest(http.MethodGet, "/api/diagnostics", nil))
    if documents.Code != http.StatusOK || diagnostics.Code != http.StatusOK {
        t.Fatalf("documents=%d diagnostics=%d", documents.Code, diagnostics.Code)
    }
}
```

- [ ] **Step 2: Run the endpoint tests and verify missing endpoints fail**

Run:

```bash
CGO_ENABLED=0 go test ./src -run 'TestGenerateKeysReturnsPlaintextOnlyInCreationResponse|TestRAGDocumentsAndDiagnosticsReturnStableData' -v
```

Expected: FAIL until the routes and handlers are implemented.

- [ ] **Step 3: Implement the compatible endpoint handlers**

Implement and register:

```text
GET  /api/rag/documents
DELETE /api/rag/documents/{id}
POST /api/rag/test
POST /api/models/test
GET  /api/diagnostics
GET  /api/audit/events
```

Use query parameters or path parsing already supported by `net/http`; reject malformed IDs with `400`. Model test must execute a bounded upstream request without changing active model rules. RAG test must report the best match and score without invoking the language model. Diagnostics must report database ping state, active model presence, Embedding presence, and build information.

- [ ] **Step 4: Normalize validation and errors**

Validate required provider, API type, model name, and endpoint fields before saving. Ensure malformed JSON and missing multipart files return `400`; upstream test failures return `502`; database failures return `500`. Every response must contain `success`, `message`, `data`, and `request_id`.

- [ ] **Step 5: Run the complete backend suite**

Run:

```bash
CGO_ENABLED=0 go test ./src -race -v
```

Expected: PASS.

- [ ] **Step 6: Commit the admin API**

```bash
git add src/api_admin.go src/main.go src/admin_contract_test.go
git commit -m "feat: complete admin management endpoints"
```

## Task 4: Scaffold the uni-app x H5 project and typed request layer

**Files:**
- Create: `frontend/package.json`
- Create: `frontend/manifest.json`
- Create: `frontend/pages.json`
- Create: `frontend/App.uvue`
- Create: `frontend/main.uts`
- Create: `frontend/uni.scss`
- Create: `frontend/types/api.uts`
- Create: `frontend/types/domain.uts`
- Create: `frontend/services/http.uts`
- Create: `frontend/utils/errors.uts`
- Create: `frontend/utils/mask.uts`
- Create: `frontend/tests/unit/http.test.ts`
- Create: `frontend/tests/unit/mask.test.ts`

- [ ] **Step 1: Create the H5 project manifest and scripts**

Use the installed official uni-app x project generator or HBuilderX template to create the project under `frontend/`, then make the generated scripts explicit:

```json
{
  "scripts": {
    "dev:h5": "uni -p h5",
    "build:h5": "uni build -p h5",
    "test:unit": "vitest run",
    "test:unit:watch": "vitest"
  }
}
```

Set the H5 API base URL through `VITE_API_BASE_URL`, defaulting to the same origin. Do not commit generated `node_modules` or build output.

- [ ] **Step 2: Write request-layer tests**

Test that the request client:

```ts
it("adds credentials and converts non-success API envelopes to ConsoleError", async () => {
  const response = await request("/api/system/status", { method: "GET" })
  expect(response.data).toBeDefined()
  expect(fetchMock).toHaveBeenCalledWith(
    "/api/system/status",
    expect.objectContaining({ credentials: "include" })
  )
})

it("maps 401 to session_expired", async () => {
  await expect(request("/api/session")).rejects.toMatchObject({
    code: "session_expired"
  })
})
```

`mask.test.ts` must prove that `maskSecret("sk-live-secret")` never returns the full secret and that empty values remain empty.

- [ ] **Step 3: Run frontend unit tests and verify the initial failure**

Run:

```bash
cd frontend
npm install
npm run test:unit -- --run
```

Expected: the tests initially fail until `http.uts` and `mask.uts` are implemented.

- [ ] **Step 4: Implement typed API primitives**

Define:

```ts
type ApiEnvelope<T> = {
  success: boolean
  message: string
  data: T
  request_id: string
}

type ConsoleErrorCode =
  | "session_expired"
  | "validation"
  | "conflict"
  | "upstream"
  | "server"
  | "network"
```

Implement `request<T>(path, options): Promise<ApiEnvelope<T>>`, `upload<T>(path, file): Promise<ApiEnvelope<T>>`, a single `401` callback, and safe JSON parsing. The request layer must use `credentials: "include"` and must not log request bodies or secrets.

- [ ] **Step 5: Run unit tests and H5 compilation**

Run:

```bash
cd frontend
npm run test:unit -- --run
npm run build:h5
```

Expected: PASS and a successful H5 build.

- [ ] **Step 6: Commit the frontend foundation**

```bash
git add frontend
git commit -m "feat: scaffold uniapp x admin console"
```

## Task 5: Implement authentication, shell, navigation, and responsive primitives

**Files:**
- Create: `frontend/stores/session.uts`
- Create: `frontend/stores/console.uts`
- Create: `frontend/layouts/AppShell.uvue`
- Create: `frontend/layouts/DesktopSidebar.uvue`
- Create: `frontend/layouts/MobileTabbar.uvue`
- Create: `frontend/layouts/TopBar.uvue`
- Create: `frontend/pages/login/login.uvue`
- Create: `frontend/components/StatusBadge.uvue`
- Create: `frontend/components/EmptyState.uvue`
- Create: `frontend/components/ErrorState.uvue`
- Create: `frontend/components/OperationResult.uvue`
- Modify: `frontend/pages.json`
- Create: `frontend/tests/unit/stores.test.ts`

- [ ] **Step 1: Write session and breakpoint tests**

Cover `restoreSession`, `setSession`, `clearSession`, and the breakpoint mapping:

```ts
it("clears session and redirects on session_expired", async () => {
  const session = createSessionStore()
  session.setAuthenticated(true)
  session.handleRequestError({ code: "session_expired" })
  expect(session.authenticated).toBe(false)
  expect(navigateTo).toHaveBeenCalledWith("/pages/login/login")
})
```

- [ ] **Step 2: Implement the auth flow**

`login.uvue` submits username and password to `auth.login`, displays an inline error, and redirects to `/pages/overview/overview` on success. `AppShell` calls `/api/session` before rendering protected pages. `TopBar` calls `/api/logout` and clears the session. No password or API Key value may be stored in persistent frontend storage.

- [ ] **Step 3: Implement responsive navigation**

Use a fixed `224px` desktop sidebar at widths `>= 1200px`, a compact sidebar between `768px` and `1199px`, and a bottom tabbar below `768px`. Register only the confirmed pages. Put P2 navigation behind disabled/hidden capability checks rather than fake screens.

- [ ] **Step 4: Run unit tests and compile**

Run:

```bash
cd frontend
npm run test:unit -- --run
npm run build:h5
```

Expected: PASS and successful H5 compilation.

- [ ] **Step 5: Commit the shell**

```bash
git add frontend/pages frontend/layouts frontend/components frontend/stores frontend/pages.json
git commit -m "feat: add admin console shell and authentication"
```

## Task 6: Implement overview and model management

**Files:**
- Create: `frontend/services/system.uts`
- Create: `frontend/services/models.uts`
- Create: `frontend/pages/overview/overview.uvue`
- Create: `frontend/pages/models/models.uvue`
- Create: `frontend/pages/models/edit.uvue`
- Create: `frontend/components/MetricStrip.uvue`
- Create: `frontend/components/DataList.uvue`
- Create: `frontend/components/ConfigForm.uvue`
- Create: `frontend/utils/format.uts`
- Create: `frontend/tests/unit/format.test.ts`

- [ ] **Step 1: Write formatter tests**

```ts
it("formats unknown health without claiming success", () => {
  expect(formatHealth("unknown")).toEqual({
    label: "未检测",
    tone: "neutral"
  })
})

it("formats zero usage as a stable value", () => {
  expect(formatCount(0)).toBe("0")
})
```

- [ ] **Step 2: Implement system and model services**

Expose typed functions:

```ts
getSystemStatus(): Promise<SystemStatus>
getBuildInfo(): Promise<string>
listModels(): Promise<ModelConfigView[]>
saveModel(input: ModelConfigInput): Promise<void>
activateModels(input: ActivationInput): Promise<void>
testModel(input: ModelTestInput): Promise<ModelTestResult>
```

- [ ] **Step 3: Implement the overview page**

Load system status, build info, active model health, Key summary, RAG summary, and recent diagnostics. Render loading, empty, stale, error, and retry states. Do not render raw JSON as the primary UI.

- [ ] **Step 4: Implement model list and edit flow**

Show provider, type, model name, masked Key, endpoint, active role, and health. The edit form validates required fields before calling the API. The test action is separate from save and activate. After save or activation, reload both model list and overview state.

- [ ] **Step 5: Run tests and build**

Run:

```bash
cd frontend
npm run test:unit -- --run
npm run build:h5
```

Expected: PASS and successful build.

- [ ] **Step 6: Commit model and overview pages**

```bash
git add frontend/pages/overview frontend/pages/models frontend/services/system.uts frontend/services/models.uts frontend/components frontend/utils/format.uts frontend/tests/unit
git commit -m "feat: add overview and model management pages"
```

## Task 7: Implement API Key management and usage

**Files:**
- Create: `frontend/services/keys.uts`
- Create: `frontend/pages/access-keys/access-keys.uvue`
- Create: `frontend/pages/access-keys/usage.uvue`
- Create: `frontend/components/ConfirmDialog.uvue`
- Modify: `src/api_admin.go` if Key metadata endpoints need dedicated query fields
- Modify: `src/admin_contract_test.go`

- [ ] **Step 1: Write backend and frontend behavior tests**

The backend test must assert that a generated plaintext Key is absent from later list responses. The frontend test must assert that the one-time reveal component clears the plaintext after acknowledgement:

```ts
it("does not retain the one-time generated key after acknowledgement", () => {
  const reveal = createOneTimeSecret("sk-aibase-new")
  reveal.acknowledge()
  expect(reveal.value).toBe("")
})
```

- [ ] **Step 2: Implement Key services**

Expose:

```ts
listKeys(): Promise<ApiKeyView[]>
generateKeys(): Promise<string[]>
importKeys(keys: string[]): Promise<void>
deleteKeys(keys: string[]): Promise<void>
getKeyUsage(keyID: string, range: UsageRange): Promise<KeyUsage>
```

- [ ] **Step 3: Implement the Key list**

Support selection, batch delete, import, and generate. Generated plaintext values appear in a one-time result panel with copy action and explicit acknowledgement; list rows display only masked values. Disable destructive actions until a selection exists.

- [ ] **Step 4: Implement usage view**

Display request count, input/output token counts, errors, created time, last used time, and daily usage. If the backend has no token data, show `0` or “暂无数据”, never invented values.

- [ ] **Step 5: Run tests and build**

Run:

```bash
CGO_ENABLED=0 go test ./src -race -run 'TestGenerateKeysReturnsPlaintextOnlyInCreationResponse|TestRecordKeyUsageUpdatesDailyCounters' -v
cd frontend
npm run test:unit -- --run
npm run build:h5
```

Expected: PASS.

- [ ] **Step 6: Commit Key management**

```bash
git add src/api_admin.go src/admin_contract_test.go frontend/services/keys.uts frontend/pages/access-keys frontend/components/ConfirmDialog.uvue
git commit -m "feat: add api key management console"
```

## Task 8: Implement RAG document management and retrieval test

**Files:**
- Create: `frontend/services/knowledge.uts`
- Create: `frontend/pages/knowledge/knowledge.uvue`
- Create: `frontend/pages/knowledge/test.uvue`
- Modify: `src/api_admin.go`
- Modify: `src/admin_contract_test.go`

- [ ] **Step 1: Write document lifecycle tests**

Cover upload start, ready state, failure state, list, delete, and retrieval test. The backend must reject an upload without a file with `400`; the frontend must keep the selected file and error message after a failed upload.

- [ ] **Step 2: Implement document services**

Expose:

```ts
listDocuments(): Promise<KnowledgeDocument[]>
uploadDocument(file: UniFile): Promise<KnowledgeDocument>
deleteDocument(id: number): Promise<void>
testRetrieval(query: string): Promise<RetrievalTestResult>
getRagStatus(): Promise<RagStatus>
```

- [ ] **Step 3: Implement the document page**

Show filename, size, chunk count, status, error, and created time. Allow upload, refresh, and delete. Refresh after upload completion and provide a clear retry action for `error` documents. Do not show raw file contents by default.

- [ ] **Step 4: Implement retrieval test**

Submit a query to `/api/rag/test`, render the matched source, score, and no-match state. The page must never call the language model and must clearly distinguish “no match” from a network failure.

- [ ] **Step 5: Run tests and build**

Run:

```bash
CGO_ENABLED=0 go test ./src -race -run 'TestRAGDocumentsAndDiagnosticsReturnStableData' -v
cd frontend
npm run test:unit -- --run
npm run build:h5
```

Expected: PASS.

- [ ] **Step 6: Commit RAG management**

```bash
git add src/api_admin.go src/admin_contract_test.go frontend/services/knowledge.uts frontend/pages/knowledge
git commit -m "feat: add rag document management"
```

## Task 9: Implement policies, system configuration, diagnostics, and audit

**Files:**
- Create: `frontend/services/policies.uts`
- Create: `frontend/services/audit.uts`
- Create: `frontend/pages/policies/policies.uvue`
- Create: `frontend/pages/system/system.uvue`
- Create: `frontend/pages/diagnostics/diagnostics.uvue`
- Create: `frontend/pages/audit/audit.uvue`
- Modify: `frontend/pages.json`
- Modify: `src/api_admin.go`
- Modify: `src/admin_contract_test.go`

- [ ] **Step 1: Write validation and diagnostics tests**

```go
func TestSaveConfigRejectsInvalidPort(t *testing.T) {
    setupAdminFeatureDB(t)
    body := strings.NewReader(`{"endpoint_port":"not-a-port"}`)
    response := serveAdminRequest(authenticatedJSONRequest(http.MethodPost, "/api/config/save", body))
    if response.Code != http.StatusBadRequest {
        t.Fatalf("status = %d, want 400", response.Code)
    }
}
```

```ts
it("renders a failed diagnostic as warning rather than healthy", () => {
  expect(diagnosticTone({ status: "error" })).toBe("danger")
})
```

- [ ] **Step 2: Implement policy and system services**

Expose typed `getRules`, `saveRules`, `getConfig`, `saveConfig`, `getDiagnostics`, and `listAuditEvents`. Validate ports as numeric `1..65535`, reject empty required model fields, and preserve the previous form state when saving fails.

- [ ] **Step 3: Implement the pages**

Policies: RAG mode, primary/secondary routing, concurrency and timeout settings.  
System: network, TLS, domain, and administrator settings.  
Diagnostics: database, active model, Embedding, build, and recent errors.  
Audit: paginated, read-only operation list with actor, action, resource, result, and time.

- [ ] **Step 4: Run tests and build**

Run:

```bash
CGO_ENABLED=0 go test ./src -race -run 'TestSaveConfigRejectsInvalidPort|TestRAGDocumentsAndDiagnosticsReturnStableData' -v
cd frontend
npm run test:unit -- --run
npm run build:h5
```

Expected: PASS.

- [ ] **Step 5: Commit operations pages**

```bash
git add src/api_admin.go src/admin_contract_test.go frontend/pages/policies frontend/pages/system frontend/pages/diagnostics frontend/pages/audit frontend/services frontend/pages.json
git commit -m "feat: add policies system diagnostics and audit pages"
```

## Task 10: Verify responsive H5 workflows and build delivery

**Files:**
- Create: `frontend/tests/e2e/admin-console.spec.ts`
- Create: `frontend/playwright.config.ts`
- Modify: `frontend/package.json`
- Modify: `src/main.go` only if static H5 serving is enabled in this task
- Create: `frontend/README.md`

- [ ] **Step 1: Add deterministic H5 test data**

Add a Go test-only fixture mode or a Playwright request mock that returns deterministic login, status, model, Key, and RAG responses. Do not add fake production data or hard-code secrets in the application.

- [ ] **Step 2: Write the end-to-end workflows**

```ts
test("admin can login and view overview", async ({ page }) => {
  await page.goto("/pages/login/login")
  await page.getByLabel("用户名").fill("admin")
  await page.getByLabel("密码").fill("12345")
  await page.getByRole("button", { name: "登录" }).click()
  await expect(page.getByText("运行概览")).toBeVisible()
})

test("admin can open key and rag management", async ({ page }) => {
  await loginAsAdmin(page)
  await page.getByRole("link", { name: "访问控制" }).click()
  await expect(page.getByText("API Key")).toBeVisible()
  await page.getByRole("link", { name: "知识库" }).click()
  await expect(page.getByText("文档列表")).toBeVisible()
})
```

- [ ] **Step 3: Run desktop and mobile browser checks**

Start the Go API and H5 dev server using the project scripts, then run:

```bash
cd frontend
npx playwright test --project=chromium
npx playwright test --project=mobile
```

Expected: login, model configuration, Key operations, and RAG upload workflows pass at `1440x900`, `1024x768`, and `390x844`; no horizontal overflow or overlapping text is present.

- [ ] **Step 4: Build the production H5 output**

Run:

```bash
cd frontend
npm run test:unit -- --run
npm run build:h5
```

Expected: all unit tests pass and the H5 output is generated without external runtime dependencies beyond the configured API.

- [ ] **Step 5: Document local development and delivery**

`frontend/README.md` must contain the exact commands for installing dependencies, starting H5, running unit tests, running Playwright, and building H5. It must state that secrets are supplied by the backend and are not committed to frontend environment files.

- [ ] **Step 6: Commit verification and delivery docs**

```bash
git add frontend/tests/e2e frontend/playwright.config.ts frontend/package.json frontend/README.md
git commit -m "test: verify admin console h5 workflows"
```

## Task 11: Evaluate Go static embedding after H5 acceptance

**Files:**
- Create: `src/static_embed.go`
- Modify: `src/main.go`
- Create: `src/static_embed_test.go`
- Create: `src/webdist/index.html` as the copied H5 build entry file
- Modify: `.gitignore`
- Modify: `frontend/README.md`

- [ ] **Step 1: Write the embedding test**

```go
func TestEmbeddedFrontendContainsIndex(t *testing.T) {
    content, err := frontendFiles.ReadFile("webdist/index.html")
    if err != nil {
        t.Fatal(err)
    }
    if !bytes.Contains(content, []byte("<!DOCTYPE html>")) {
        t.Fatal("embedded H5 index is not a valid build artifact")
    }
}
```

- [ ] **Step 2: Build H5 before embedding**

Run:

```bash
cd frontend
npm run build:h5
```

Copy only the required H5 output into `src/webdist/`, the directory relative to `src/static_embed.go` that is allowed by `//go:embed`. Do not commit `node_modules`, caches, source maps containing secrets, or development output.

- [ ] **Step 3: Implement embedded serving behind the existing static handler**

Use `embed.FS` and an `http.FileServer` adapter for `webdist/**`. Keep API routes registered before the static fallback. Ensure `/` serves the built H5 `index.html` and unknown frontend routes fall back to `index.html` for client-side navigation.

- [ ] **Step 4: Run Go and H5 verification**

Run:

```bash
CGO_ENABLED=0 go test ./src -race -v
cd frontend
npm run test:unit -- --run
npm run build:h5
```

Expected: PASS and a Go binary that serves the same accepted H5 console without a separate frontend server.

- [ ] **Step 5: Commit the optional single-binary delivery**

```bash
git add src/static_embed.go src/static_embed_test.go src/webdist src/main.go .gitignore frontend/README.md
git commit -m "feat: embed accepted h5 console in gateway"
```

## Self-review checklist

- Spec P0 pages are covered by Tasks 5 through 9.
- Spec P1 model test, config validation, usage statistics, document management, diagnostics, and audit are covered by Tasks 2, 3, 6, 7, 8, and 9.
- Session handling and secret masking are covered by Tasks 1 and 5.
- Desktop and mobile H5 widths are covered by Task 10.
- Existing Go tests are rerun with `CGO_ENABLED=0 go test ./src -race -v` after backend changes.
- No task introduces Android/iOS work or a user-facing chat client.
- No production feature depends on fake data.
- The only deferred work is the optional Go embedding step, and it is explicitly gated on H5 acceptance.
