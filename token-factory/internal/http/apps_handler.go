package http

import (
	"net/http"
)

func (r *Router) apps(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case http.MethodGet:
		r.invoke(w, req, func() (any, error) { return r.backend.ListApps(req.Context(), parsePage(req)) }, http.StatusOK)
	case http.MethodPost:
		if !r.requireWriter(w, req) {
			return
		}
		var input AppInput
		if err := decodeJSON(req, &input); err != nil {
			writeDomainError(w, req, err)
			return
		}
		r.invoke(w, req, func() (any, error) { return r.backend.CreateApp(req.Context(), input) }, http.StatusCreated)
	default:
		writeError(w, req, http.StatusNotFound, "resource not found")
	}
}

func (r *Router) appByID(w http.ResponseWriter, req *http.Request, appID string) {
	switch req.Method {
	case http.MethodGet:
		r.invoke(w, req, func() (any, error) { return r.backend.GetApp(req.Context(), appID) }, http.StatusOK)
	case http.MethodPatch:
		if !r.requireWriter(w, req) {
			return
		}
		var input AppInput
		if err := decodeJSON(req, &input); err != nil {
			writeDomainError(w, req, err)
			return
		}
		r.invoke(w, req, func() (any, error) { return r.backend.UpdateApp(req.Context(), appID, input) }, http.StatusOK)
	default:
		writeError(w, req, http.StatusNotFound, "resource not found")
	}
}
