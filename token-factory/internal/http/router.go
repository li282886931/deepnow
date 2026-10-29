package http

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
)

type Router struct {
	backend     Backend
	auth        Authenticator
	mu          sync.Mutex
	idempotency map[string]idempotencyEntry
}

type idempotencyEntry struct {
	fingerprint string
	body        []byte
	status      int
}

func NewRouter(cfg RouterConfig) http.Handler {
	router := &Router{backend: cfg.Backend, auth: cfg.Authenticator, idempotency: make(map[string]idempotencyEntry)}
	return router.middleware(http.HandlerFunc(router.dispatch))
}

func (r *Router) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		requestID := req.Header.Get("X-Request-ID")
		if requestID == "" {
			requestID = newRequestID()
		}
		w.Header().Set("X-Request-ID", requestID)
		req = req.WithContext(contextWithValues(req.Context(), requestID))
		defer func() {
			if recovered := recover(); recovered != nil {
				writeError(w, req, http.StatusInternalServerError, "internal server error")
			}
		}()
		if r.auth == nil {
			writeError(w, req, http.StatusUnauthorized, "authentication required")
			return
		}
		actor, ok := r.auth.Authenticate(req)
		if !ok {
			writeError(w, req, http.StatusUnauthorized, "authentication required")
			return
		}
		req = req.WithContext(context.WithValue(req.Context(), actorKey{}, actor))
		next.ServeHTTP(w, req)
	})
}

func contextWithValues(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, requestIDKey{}, requestID)
}

func (r *Router) dispatch(w http.ResponseWriter, req *http.Request) {
	if !strings.HasPrefix(req.URL.Path, "/api/v1/") {
		http.NotFound(w, req)
		return
	}
	path := strings.TrimPrefix(req.URL.Path, "/api/v1/")
	switch {
	case path == "apps":
		r.apps(w, req)
	case strings.HasPrefix(path, "apps/"):
		r.appByID(w, req, routeID(path, "apps/"))
	case path == "tokens":
		r.tokens(w, req)
	case path == "tokens/validate":
		r.validate(w, req)
	case strings.HasPrefix(path, "tokens/"):
		r.tokenByID(w, req, routeID(path, "tokens/"))
	case path == "approvals":
		r.approvals(w, req)
	case strings.HasPrefix(path, "approvals/"):
		r.approvalByID(w, req, routeID(path, "approvals/"))
	case path == "audit-logs":
		r.auditLogs(w, req)
	case path == "risk-events":
		r.riskEvents(w, req)
	case strings.HasPrefix(path, "risk-events/"):
		r.riskByID(w, req, routeID(path, "risk-events/"))
	case path == "policies":
		r.policies(w, req)
	case strings.HasPrefix(path, "policies/"):
		r.policyByID(w, req, routeID(path, "policies/"))
	default:
		writeError(w, req, http.StatusNotFound, "resource not found")
	}
}

func newRequestID() string {
	var raw [12]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "req_unknown"
	}
	return "req_" + hex.EncodeToString(raw[:])
}

func writeJSON(w http.ResponseWriter, req *http.Request, status int, data any) {
	switch value := data.(type) {
	case Page[App]:
		if value.Page == 0 {
			value.Page = 1
		}
		if value.PageSize == 0 {
			value.PageSize = 20
		}
		data = value
	case Page[TokenMetadataResponse]:
		if value.Page == 0 {
			value.Page = 1
		}
		if value.PageSize == 0 {
			value.PageSize = 20
		}
		data = value
	case Page[TokenVersionResponse]:
		if value.Page == 0 {
			value.Page = 1
		}
		if value.PageSize == 0 {
			value.PageSize = 20
		}
		data = value
	case Page[UsageResponse]:
		if value.Page == 0 {
			value.Page = 1
		}
		if value.PageSize == 0 {
			value.PageSize = 20
		}
		data = value
	case Page[ApprovalResponse]:
		if value.Page == 0 {
			value.Page = 1
		}
		if value.PageSize == 0 {
			value.PageSize = 20
		}
		data = value
	case Page[AuditResponse]:
		if value.Page == 0 {
			value.Page = 1
		}
		if value.PageSize == 0 {
			value.PageSize = 20
		}
		data = value
	case Page[RiskResponse]:
		if value.Page == 0 {
			value.Page = 1
		}
		if value.PageSize == 0 {
			value.PageSize = 20
		}
		data = value
	case Page[PolicyResponse]:
		if value.Page == 0 {
			value.Page = 1
		}
		if value.PageSize == 0 {
			value.PageSize = 20
		}
		data = value
	}
	body, err := json.Marshal(map[string]any{"request_id": requestIDFromContext(req.Context()), "success": true, "message": "", "data": data})
	if err != nil {
		writeError(w, req, http.StatusInternalServerError, "internal server error")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

func writeError(w http.ResponseWriter, req *http.Request, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"request_id": requestIDFromContext(req.Context()), "success": false, "message": message, "data": nil})
}

func decodeJSON(req *http.Request, target any) error {
	if req.Body == nil {
		return nil
	}
	decoder := json.NewDecoder(req.Body)
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("%w: invalid JSON", ErrInvalidRequest)
	}
	return nil
}

func (r *Router) requireWriter(w http.ResponseWriter, req *http.Request) bool {
	if canWrite(actorFromContext(req.Context())) {
		return true
	}
	writeError(w, req, http.StatusForbidden, "permission denied")
	return false
}

func (r *Router) invoke(w http.ResponseWriter, req *http.Request, fn func() (any, error), status int) {
	data, err := fn()
	if err != nil {
		writeDomainError(w, req, err)
		return
	}
	writeJSON(w, req, status, data)
}

func writeDomainError(w http.ResponseWriter, req *http.Request, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, ErrInvalidRequest):
		status = http.StatusBadRequest
	case errors.Is(err, ErrForbidden):
		status = http.StatusForbidden
	case errors.Is(err, ErrNotFound):
		status = http.StatusNotFound
	case errors.Is(err, ErrConflict):
		status = http.StatusConflict
	case errors.Is(err, ErrPolicyDenied):
		status = http.StatusUnprocessableEntity
	case errors.Is(err, ErrRateLimited):
		status = http.StatusTooManyRequests
	case errors.Is(err, ErrDependencyUnavailable):
		status = http.StatusServiceUnavailable
	}
	writeError(w, req, status, err.Error())
}

func (r *Router) idempotent(req *http.Request, status int, fn func() (any, error)) (any, error) {
	key := req.Header.Get("Idempotency-Key")
	if key == "" {
		return fn()
	}
	raw := bodyFromContext(req.Context())
	if raw == nil {
		raw, _ = io.ReadAll(req.Body)
		req.Body = io.NopCloser(bytes.NewReader(raw))
	}
	fingerprint := string(raw)
	r.mu.Lock()
	entry, ok := r.idempotency[key]
	r.mu.Unlock()
	if ok {
		if entry.fingerprint != fingerprint {
			return nil, ErrConflict
		}
		var data any
		if err := json.Unmarshal(entry.body, &data); err != nil {
			return nil, err
		}
		return data, nil
	}
	data, err := fn()
	if err != nil {
		return nil, err
	}
	body, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	r.mu.Lock()
	r.idempotency[key] = idempotencyEntry{fingerprint: fingerprint, body: body, status: status}
	r.mu.Unlock()
	return data, nil
}

func (r *Router) logSecretSafe(req *http.Request, message string) {
	log.Printf("request_id=%s actor=%s %s", requestIDFromContext(req.Context()), actorFromContext(req.Context()).ID, strings.ReplaceAll(message, "secret", "[REDACTED]"))
}
