package http

import (
	"net/http"
	"strings"
)

func (r *Router) approvals(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		writeError(w, req, http.StatusNotFound, "resource not found")
		return
	}
	r.invoke(w, req, func() (any, error) { return r.backend.ListApprovals(req.Context(), parsePage(req)) }, http.StatusOK)
}

func (r *Router) approvalByID(w http.ResponseWriter, req *http.Request, path string) {
	parts := strings.Split(path, "/")
	requestID := parts[0]
	if len(parts) == 1 {
		if req.Method != http.MethodGet {
			writeError(w, req, http.StatusNotFound, "resource not found")
			return
		}
		r.invoke(w, req, func() (any, error) { return r.backend.GetApproval(req.Context(), requestID) }, http.StatusOK)
		return
	}
	if req.Method != http.MethodPost || !r.requireWriter(w, req) {
		return
	}
	var input ApprovalActionInput
	if err := decodeJSON(req, &input); err != nil {
		writeDomainError(w, req, err)
		return
	}
	action := parts[1]
	if action != "approve" && action != "reject" && action != "return" {
		writeError(w, req, http.StatusNotFound, "resource not found")
		return
	}
	if err := r.backend.ActApproval(req.Context(), requestID, action, input); err != nil {
		writeDomainError(w, req, err)
		return
	}
	writeJSON(w, req, http.StatusNoContent, nil)
}
