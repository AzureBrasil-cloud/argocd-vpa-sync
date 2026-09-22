package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/azurebrasil/argocd-vpa-updater/internal/domain"
)

func TestServer_HealthzAndRecommendationsRoutes(t *testing.T) {
	svc := newTestService(t, []domain.NormalizedVPA{sampleVPA()})
	srv := NewServer(svc, nil)

	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 from /healthz, got %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/recommendations", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 from /api/v1/recommendations, got %d: %s", rec.Code, rec.Body.String())
	}
	var body ListRecommendationsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unexpected error decoding response: %v", err)
	}
	if len(body.Items) != 1 {
		t.Fatalf("expected 1 recommendation, got %d", len(body.Items))
	}

	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/recommendations/payments/checkout-api-vpa/app", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 from the detail route, got %d: %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/recommendations/payments/checkout-api-vpa/does-not-exist", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for an unknown container, got %d", rec.Code)
	}
}

func TestServer_SelectRecommendationRoute(t *testing.T) {
	svc := newTestService(t, []domain.NormalizedVPA{sampleVPA()})
	srv := NewServer(svc, nil)

	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/recommendations/payments/checkout-api-vpa/app/select", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for an eligible selection, got %d: %s", rec.Code, rec.Body.String())
	}
	var sel domain.PendingSelection
	if err := json.Unmarshal(rec.Body.Bytes(), &sel); err != nil {
		t.Fatalf("unexpected error decoding response: %v", err)
	}
	if sel.IdempotencyKey == "" {
		t.Fatalf("expected a non-empty idempotency key in the response")
	}

	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/recommendations/payments/checkout-api-vpa/does-not-exist/select", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for an unknown container, got %d", rec.Code)
	}
}

func TestServer_SelectRecommendationRoute_NotEligible(t *testing.T) {
	vpa := sampleVPA()
	vpa.Containers[0].Target.CPU = qptr("101m") // 1%: below the 10% threshold
	vpa.Containers[0].Target.Memory = qptr("256Mi")

	svc := newTestService(t, []domain.NormalizedVPA{vpa})
	srv := NewServer(svc, nil)

	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/recommendations/payments/checkout-api-vpa/app/select", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for an ineligible selection, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestServer_SelectRecommendationRoute_ExplicitResourceFlags(t *testing.T) {
	// CPU moves well past the 10% threshold (100m -> 250m); memory barely
	// moves (256Mi -> 260Mi, ~1.5%) and stays below it -- so CPU-only must
	// succeed while memory-only must be rejected, even though the combined
	// (whole-container) verdict would be eligible.
	vpa := sampleVPA()
	vpa.Containers[0].Target.Memory = qptr("260Mi")

	svc := newTestService(t, []domain.NormalizedVPA{vpa})
	srv := NewServer(svc, nil)

	body, _ := json.Marshal(SelectRequest{ApplyCPU: true, ApplyMemory: false})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/recommendations/payments/checkout-api-vpa/app/select", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for a CPU-only selection, got %d: %s", rec.Code, rec.Body.String())
	}

	body, _ = json.Marshal(SelectRequest{ApplyCPU: false, ApplyMemory: true})
	req = httptest.NewRequest(http.MethodPost, "/api/v1/recommendations/payments/checkout-api-vpa/app/select", bytes.NewReader(body))
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 when the only requested resource isn't individually eligible, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestServer_BulkSelectRoute(t *testing.T) {
	svc := newTestService(t, []domain.NormalizedVPA{sampleVPA(), cpuOnlyIneligibleVPA()})
	srv := NewServer(svc, nil)

	body, _ := json.Marshal(SelectRequest{ApplyCPU: true, ApplyMemory: true})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/recommendations/bulk-select", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result BulkSelectResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("unexpected error decoding response: %v", err)
	}
	if len(result.Selected) != 1 || len(result.Skipped) != 1 {
		t.Fatalf("expected 1 selected and 1 skipped, got selected=%d skipped=%d (%+v)", len(result.Selected), len(result.Skipped), result)
	}
}
