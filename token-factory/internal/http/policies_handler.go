package http

import "net/http"

func (r *Router) policies(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		writeError(w, req, http.StatusNotFound, "resource not found")
		return
	}
	r.invoke(w, req, func() (any, error) { return r.backend.ListPolicies(req.Context(), parsePage(req)) }, http.StatusOK)
}

func (r *Router) policyByID(w http.ResponseWriter, req *http.Request, policyID string) {
	if req.Method != http.MethodPut || !r.requireWriter(w, req) {
		writeError(w, req, http.StatusNotFound, "resource not found")
		return
	}
	var input PolicyInput
	if err := decodeJSON(req, &input); err != nil {
		writeDomainError(w, req, err)
		return
	}
	r.invoke(w, req, func() (any, error) { return r.backend.UpdatePolicy(req.Context(), policyID, input) }, http.StatusOK)
}
