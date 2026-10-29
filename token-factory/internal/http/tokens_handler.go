package http

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
)

func captureBody(req *http.Request) {
	raw, _ := io.ReadAll(req.Body)
	req.Body = io.NopCloser(bytes.NewReader(raw))
	*req = *req.WithContext(context.WithValue(req.Context(), requestBodyKey{}, raw))
}

func (r *Router) tokens(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case http.MethodGet:
		r.invoke(w, req, func() (any, error) { return r.backend.ListTokens(req.Context(), parsePage(req)) }, http.StatusOK)
	case http.MethodPost:
		if !r.requireWriter(w, req) {
			return
		}
		captureBody(req)
		var input TokenInput
		if err := decodeJSON(req, &input); err != nil {
			writeDomainError(w, req, err)
			return
		}
		data, err := r.idempotent(req, http.StatusCreated, func() (any, error) {
			return r.backend.CreateToken(req.Context(), actorFromContext(req.Context()).ID, input)
		})
		if err != nil {
			writeDomainError(w, req, err)
			return
		}
		r.logSecretSafe(req, "token create completed")
		writeJSON(w, req, http.StatusCreated, data)
	default:
		writeError(w, req, http.StatusNotFound, "resource not found")
	}
}

func (r *Router) validate(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		writeError(w, req, http.StatusNotFound, "resource not found")
		return
	}
	var input ValidateInput
	if err := decodeJSON(req, &input); err != nil {
		writeDomainError(w, req, err)
		return
	}
	r.invoke(w, req, func() (any, error) { return r.backend.ValidateToken(req.Context(), input) }, http.StatusOK)
}

func (r *Router) tokenByID(w http.ResponseWriter, req *http.Request, path string) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	tokenID := parts[0]
	if len(parts) == 1 {
		if req.Method != http.MethodGet {
			writeError(w, req, http.StatusNotFound, "resource not found")
			return
		}
		r.invoke(w, req, func() (any, error) { return r.backend.GetToken(req.Context(), tokenID) }, http.StatusOK)
		return
	}
	action := parts[1]
	if !r.requireWriter(w, req) {
		return
	}
	switch action {
	case "rotate":
		captureBody(req)
		var input RotateInput
		if err := decodeJSON(req, &input); err != nil {
			writeDomainError(w, req, err)
			return
		}
		data, err := r.idempotent(req, http.StatusOK, func() (any, error) {
			return r.backend.RotateToken(req.Context(), tokenID, actorFromContext(req.Context()).ID, input)
		})
		if err != nil {
			writeDomainError(w, req, err)
			return
		}
		r.logSecretSafe(req, "token rotate completed")
		writeJSON(w, req, http.StatusOK, data)
	case "revoke":
		var input RevokeInput
		if err := decodeJSON(req, &input); err != nil {
			writeDomainError(w, req, err)
			return
		}
		r.invoke(w, req, func() (any, error) {
			return r.backend.RevokeToken(req.Context(), tokenID, actorFromContext(req.Context()).ID, input)
		}, http.StatusOK)
	case "extend":
		var input ExtendInput
		if err := decodeJSON(req, &input); err != nil {
			writeDomainError(w, req, err)
			return
		}
		r.invoke(w, req, func() (any, error) {
			return r.backend.ExtendToken(req.Context(), tokenID, actorFromContext(req.Context()).ID, input)
		}, http.StatusOK)
	case "versions":
		if req.Method != http.MethodGet {
			writeError(w, req, http.StatusNotFound, "resource not found")
			return
		}
		r.invoke(w, req, func() (any, error) { return r.backend.ListVersions(req.Context(), tokenID, parsePage(req)) }, http.StatusOK)
	case "usage":
		if req.Method != http.MethodGet {
			writeError(w, req, http.StatusNotFound, "resource not found")
			return
		}
		r.invoke(w, req, func() (any, error) { return r.backend.GetUsage(req.Context(), tokenID, parsePage(req)) }, http.StatusOK)
	default:
		writeError(w, req, http.StatusNotFound, "resource not found")
	}
}
