package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/azurebrasil/argocd-vpa-updater/internal/domain"
)

func TestServer_ThemeDefaultsWithoutSettings(t *testing.T) {
	srv := NewServer(newTestService(t, []domain.NormalizedVPA{sampleVPA()}), nil)

	var info UserInfoResponse
	if err := json.NewDecoder(do(srv, http.MethodGet, "/api/v1/session/userinfo", "").Body).Decode(&info); err != nil {
		t.Fatal(err)
	}
	if info.Theme != "default" {
		t.Fatalf("expected the default theme, got %q", info.Theme)
	}
}

func TestServer_SettingsRequireSessionButThemeDoesNot(t *testing.T) {
	srv := newAuthServer(t)
	WithSettings(SettingsResponse{Theme: "calibration", ArgoCDNamespace: "argocd", WriteBackPollIntervalSeconds: 20})(srv)

	if rec := do(srv, http.MethodGet, "/api/v1/settings", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for settings without a session, got %d", rec.Code)
	}

	var info UserInfoResponse
	if err := json.NewDecoder(do(srv, http.MethodGet, "/api/v1/session/userinfo", "").Body).Decode(&info); err != nil {
		t.Fatal(err)
	}
	if info.LoggedIn || info.Theme != "calibration" {
		t.Fatalf("expected the theme without a session, got %+v", info)
	}

	cookie, _ := login(t, srv)
	rec := do(srv, http.MethodGet, "/api/v1/settings", "", func(r *http.Request) { r.AddCookie(cookie) })
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var settings SettingsResponse
	if err := json.NewDecoder(rec.Body).Decode(&settings); err != nil {
		t.Fatal(err)
	}
	if settings.Theme != "calibration" || !settings.AuthEnabled || settings.ArgoCDNamespace != "argocd" {
		t.Fatalf("unexpected settings: %+v", settings)
	}
}

func TestService_RecommendationCarriesBinding(t *testing.T) {
	srv := NewServer(newTestService(t, []domain.NormalizedVPA{sampleVPA()}), nil)

	var res ListRecommendationsResponse
	if err := json.NewDecoder(do(srv, http.MethodGet, "/api/v1/recommendations", "").Body).Decode(&res); err != nil {
		t.Fatal(err)
	}
	if len(res.Items) == 0 || res.Items[0].Binding == nil {
		t.Fatalf("expected the binding on each recommendation, got %+v", res.Items)
	}
	vpa := sampleVPA()
	if b := res.Items[0].Binding; b.RepoURL != vpa.Binding.RepoURL || b.MinChangePercent <= 0 {
		t.Fatalf("unexpected binding: %+v", b)
	}
}
