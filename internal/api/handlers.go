package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

// handleListRecommendations serves GET /api/v1/recommendations.
func (s *Server) handleListRecommendations(w http.ResponseWriter, r *http.Request) {
	items, err := s.service.ListRecommendations(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, ListRecommendationsResponse{Items: items})
}

// handleGetRecommendation serves
// GET /api/v1/recommendations/{namespace}/{vpaName}/{containerName}.
func (s *Server) handleGetRecommendation(w http.ResponseWriter, r *http.Request) {
	namespace := r.PathValue("namespace")
	vpaName := r.PathValue("vpaName")
	containerName := r.PathValue("containerName")

	item, found, err := s.service.GetRecommendation(r.Context(), namespace, vpaName, containerName)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "recommendation not found")
		return
	}
	writeJSON(w, http.StatusOK, item)
}

// decodeSelectRequest parses the optional JSON body of a select/bulk-select
// request. A missing or empty body defaults to {applyCPU:true,
// applyMemory:true} (accept whatever the container/binding supports),
// matching the original single-button "Accept suggestion" behavior for any
// caller that doesn't send explicit flags.
func decodeSelectRequest(r *http.Request) (SelectRequest, error) {
	req := SelectRequest{ApplyCPU: true, ApplyMemory: true}
	if r.Body == nil {
		return req, nil
	}
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&req); err != nil {
		if errors.Is(err, io.EOF) {
			return SelectRequest{ApplyCPU: true, ApplyMemory: true}, nil
		}
		return SelectRequest{}, err
	}
	return req, nil
}

func writeSelectError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrRecommendationNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, ErrRecommendationNotEligible), errors.Is(err, ErrInvalidSelectRequest):
		writeError(w, http.StatusBadRequest, err.Error())
	default:
		writeError(w, http.StatusInternalServerError, err.Error())
	}
}

// handleSelectRecommendation serves
// POST /api/v1/recommendations/{namespace}/{vpaName}/{containerName}/select.
// It queues the current recommendation's requested resource(s) as a pending
// selection; it does not write anything to Git (see internal/gitwriteback
// for that seam).
func (s *Server) handleSelectRecommendation(w http.ResponseWriter, r *http.Request) {
	namespace := r.PathValue("namespace")
	vpaName := r.PathValue("vpaName")
	containerName := r.PathValue("containerName")

	req, err := decodeSelectRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	opts, err := req.options()
	if err != nil {
		writeSelectError(w, err)
		return
	}
	sel, err := s.service.SelectRecommendation(r.Context(), namespace, vpaName, containerName, opts)
	if err != nil {
		writeSelectError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, sel)
}

// handleBulkSelectRecommendations serves POST /api/v1/recommendations/bulk-select.
// It queues the requested resource(s) for every eligible, opted-in
// container, skipping (never failing the whole request over) containers
// where the requested resource(s) aren't configured or eligible.
func (s *Server) handleBulkSelectRecommendations(w http.ResponseWriter, r *http.Request) {
	req, err := decodeSelectRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	opts, err := req.options()
	if err != nil {
		writeSelectError(w, err)
		return
	}
	result, err := s.service.BulkSelectRecommendations(r.Context(), opts)
	if err != nil {
		writeSelectError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
