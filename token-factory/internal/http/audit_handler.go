package http

import (
	"net/http"
	"strings"
)

func (r *Router) auditLogs(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		writeError(w, req, http.StatusNotFound, "resource not found")
		return
	}
	r.invoke(w, req, func() (any, error) {
		page, err := r.backend.ListAuditLogs(req.Context(), parsePage(req))
		for index := range page.Items {
			if strings.Contains(strings.ToLower(page.Items[index].After), "secret") ||
				strings.Contains(strings.ToLower(page.Items[index].After), "ciphertext") ||
				strings.Contains(strings.ToLower(page.Items[index].After), "raw_token") {
				page.Items[index].After = "{}"
			}
		}
		return page, err
	}, http.StatusOK)
}

func (r *Router) riskEvents(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		writeError(w, req, http.StatusNotFound, "resource not found")
		return
	}
	r.invoke(w, req, func() (any, error) { return r.backend.ListRiskEvents(req.Context(), parsePage(req)) }, http.StatusOK)
}

func (r *Router) riskByID(w http.ResponseWriter, req *http.Request, path string) {
	parts := strings.Split(path, "/")
	if len(parts) != 2 || parts[1] != "resolve" || req.Method != http.MethodPost || !r.requireWriter(w, req) {
		writeError(w, req, http.StatusNotFound, "resource not found")
		return
	}
	var input ResolveRiskInput
	if err := decodeJSON(req, &input); err != nil {
		writeDomainError(w, req, err)
		return
	}
	if err := r.backend.ResolveRisk(req.Context(), parts[0], actorFromContext(req.Context()).ID, input); err != nil {
		writeDomainError(w, req, err)
		return
	}
	writeJSON(w, req, http.StatusNoContent, nil)
}
