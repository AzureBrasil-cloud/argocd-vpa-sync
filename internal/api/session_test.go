package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/azurebrasil/argocd-vpa-updater/internal/auth"
	"github.com/azurebrasil/argocd-vpa-updater/internal/domain"
)

func newAuthServer(t *testing.T) *Server {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte("pw"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	a, err := auth.New(auth.Config{
		Username:     "admin",
		PasswordHash: string(hash),
		SigningKey:   []byte("0123456789abcdef0123456789abcdef"),
		SessionTTL:   time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	return NewServer(newTestService(t, []domain.NormalizedVPA{sampleVPA()}), nil, WithAuth(a, true))
}

func do(srv *Server, method, path, body string, mutate ...func(*http.Request)) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	for _, m := range mutate {
		m(req)
	}
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	return rec
}

func login(t *testing.T, srv *Server) (*http.Cookie, LoginResponse) {
	t.Helper()
	rec := do(srv, http.MethodPost, "/api/v1/session", `{"username":"admin","password":"pw"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 from login, got %d: %s", rec.Code, rec.Body.String())
	}
	var body LoginResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	for _, c := range rec.Result().Cookies() {
		if c.Name == SessionCookieName {
			if !c.HttpOnly || !c.Secure || c.SameSite != http.SameSiteStrictMode {
				t.Fatalf("expected an HttpOnly, Secure, SameSite=Strict cookie, got %+v", c)
			}
			return c, body
		}
	}
	t.Fatal("expected a session cookie")
	return nil, body
}

func TestAuth_UnauthenticatedRequests(t *testing.T) {
	srv := newAuthServer(t)

	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/api/v1/recommendations"},
		{http.MethodGet, "/api/v1/recommendations/payments/checkout-api-vpa/app"},
		{http.MethodPost, "/api/v1/recommendations/payments/checkout-api-vpa/app/select"},
		{http.MethodPost, "/api/v1/recommendations/bulk-select"},
		{http.MethodGet, "/api/v1/does-not-exist"},
	} {
		if rec := do(srv, tc.method, tc.path, ""); rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s: expected 401, got %d", tc.method, tc.path, rec.Code)
		}
	}

	for _, path := range []string{"/healthz", "/readyz"} {
		if rec := do(srv, http.MethodGet, path, ""); rec.Code != http.StatusOK {
			t.Errorf("%s: expected 200 without a session, got %d", path, rec.Code)
		}
	}

	rec := do(srv, http.MethodGet, "/api/v1/session/userinfo", "")
	var info UserInfoResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &info); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK || info.LoggedIn || !info.AuthEnabled {
		t.Fatalf("expected userinfo to report logged out, got %d %+v", rec.Code, info)
	}

	withBadCookie := func(r *http.Request) { r.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "forged.token"}) }
	if rec := do(srv, http.MethodGet, "/api/v1/recommendations", "", withBadCookie); rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for a forged cookie, got %d", rec.Code)
	}
}

func TestAuth_LoginFlow(t *testing.T) {
	srv := newAuthServer(t)

	if rec := do(srv, http.MethodPost, "/api/v1/session", `{"username":"admin","password":"nope"}`); rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for a wrong password, got %d", rec.Code)
	}
	if rec := do(srv, http.MethodPost, "/api/v1/session", `not json`); rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for a malformed body, got %d", rec.Code)
	}

	cookie, body := login(t, srv)

	withCookie := func(r *http.Request) { r.AddCookie(cookie) }
	if rec := do(srv, http.MethodGet, "/api/v1/recommendations", "", withCookie); rec.Code != http.StatusOK {
		t.Fatalf("expected 200 with the session cookie, got %d: %s", rec.Code, rec.Body.String())
	}
	withBearer := func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+body.Token) }
	if rec := do(srv, http.MethodPost, "/api/v1/recommendations/payments/checkout-api-vpa/app/select", "", withBearer); rec.Code != http.StatusOK {
		t.Fatalf("expected 200 with a bearer token, got %d: %s", rec.Code, rec.Body.String())
	}

	rec := do(srv, http.MethodGet, "/api/v1/session/userinfo", "", withCookie)
	var info UserInfoResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &info); err != nil {
		t.Fatal(err)
	}
	if !info.LoggedIn || info.Username != "admin" {
		t.Fatalf("expected userinfo to report admin logged in, got %+v", info)
	}

	rec = do(srv, http.MethodDelete, "/api/v1/session", "", withCookie)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204 from logout, got %d", rec.Code)
	}
	cleared := rec.Result().Cookies()
	if len(cleared) != 1 || cleared[0].MaxAge >= 0 {
		t.Fatalf("expected logout to expire the session cookie, got %+v", cleared)
	}
}

func TestAuth_LoginThrottling(t *testing.T) {
	srv := newAuthServer(t)
	var rec *httptest.ResponseRecorder
	for i := 0; i < 10; i++ {
		rec = do(srv, http.MethodPost, "/api/v1/session", `{"username":"admin","password":"nope"}`)
		if rec.Code == http.StatusTooManyRequests {
			break
		}
	}
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") == "" {
		t.Fatalf("expected repeated failures to be throttled with Retry-After, got %d", rec.Code)
	}
	if rec := do(srv, http.MethodPost, "/api/v1/session", `{"username":"admin","password":"pw"}`); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected even the right password to wait out the throttle, got %d", rec.Code)
	}
}

func TestAuth_Disabled(t *testing.T) {
	srv := NewServer(newTestService(t, []domain.NormalizedVPA{sampleVPA()}), nil)
	rec := do(srv, http.MethodGet, "/api/v1/session/userinfo", "")
	var info UserInfoResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &info); err != nil {
		t.Fatal(err)
	}
	if !info.LoggedIn || info.AuthEnabled {
		t.Fatalf("expected userinfo to report auth disabled, got %+v", info)
	}
	if rec := do(srv, http.MethodPost, "/api/v1/session", `{}`); rec.Code != http.StatusNotFound {
		t.Fatalf("expected login to 404 with auth disabled, got %d", rec.Code)
	}
}
