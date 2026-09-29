package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/azurebrasil/argocd-vpa-updater/internal/auth"
)

// SessionCookieName is the cookie carrying the session token.
const SessionCookieName = "argocd-vpa-session"

// maxLoginBody caps the size of a login request body.
const maxLoginBody = 4 << 10

// Option customizes a Server.
type Option func(*Server)

// WithAuth requires a valid admin session (cookie or "Authorization:
// Bearer" token) on every /api/ route except login and userinfo. Without
// it, the API is open -- only meant for local development.
func WithAuth(a *auth.Authenticator, cookieSecure bool) Option {
	return func(s *Server) {
		s.auth = a
		s.cookieSecure = cookieSecure
		s.loginLimiter = auth.NewLoginLimiter()
	}
}

// LoginRequest is the body of POST /api/v1/session.
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// LoginResponse is returned by a successful POST /api/v1/session. Token is
// also set as the session cookie; API clients can send it as "Authorization:
// Bearer <token>" instead.
type LoginResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// UserInfoResponse is returned by GET /api/v1/session/userinfo.
type UserInfoResponse struct {
	LoggedIn    bool   `json:"loggedIn"`
	Username    string `json:"username,omitempty"`
	AuthEnabled bool   `json:"authEnabled"`
}

// requiresSession reports whether r must carry a valid session.
func requiresSession(r *http.Request) bool {
	if !strings.HasPrefix(r.URL.Path, "/api/") {
		// Health probes and the SPA's static assets, which hold no data
		// (the SPA itself shows the login page).
		return false
	}
	switch r.URL.Path {
	case "/api/v1/session", "/api/v1/session/userinfo":
		return false
	}
	return true
}

// authenticated returns the request's valid session claims, if any.
func (s *Server) authenticated(r *http.Request) (auth.Claims, bool) {
	token := ""
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		token = strings.TrimPrefix(h, "Bearer ")
	} else if c, err := r.Cookie(SessionCookieName); err == nil {
		token = c.Value
	}
	if token == "" {
		return auth.Claims{}, false
	}
	claims, err := s.auth.Validate(token)
	return claims, err == nil
}

// handleLogin serves POST /api/v1/session.
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if s.auth == nil {
		writeError(w, http.StatusNotFound, "authentication is disabled")
		return
	}

	client := clientIP(r)
	if ok, wait := s.loginLimiter.Allow(client); !ok {
		w.Header().Set("Retry-After", fmt.Sprint(int(math.Ceil(wait.Seconds()))))
		writeError(w, http.StatusTooManyRequests, "too many failed login attempts, try again later")
		return
	}

	var req LoginRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, maxLoginBody)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid login request body")
		return
	}

	token, expires, err := s.auth.Login(req.Username, req.Password)
	if err != nil {
		if errors.Is(err, auth.ErrInvalidCredentials) {
			s.loginLimiter.Failure(client)
			s.logger.Warn("failed login", "username", req.Username, "client", client)
			writeError(w, http.StatusUnauthorized, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.loginLimiter.Success(client)
	s.logger.Info("successful login", "username", req.Username, "client", client)

	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    token,
		Path:     "/",
		Expires:  expires,
		HttpOnly: true,
		Secure:   s.cookieSecure,
		SameSite: http.SameSiteStrictMode,
	})
	writeJSON(w, http.StatusOK, LoginResponse{Token: token, ExpiresAt: expires})
}

// handleLogout serves DELETE /api/v1/session. Sessions are stateless, so
// this only clears the cookie; a copied token stays valid until it expires
// or the admin password changes.
func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   s.cookieSecure,
		SameSite: http.SameSiteStrictMode,
	})
	w.WriteHeader(http.StatusNoContent)
}

// handleUserInfo serves GET /api/v1/session/userinfo.
func (s *Server) handleUserInfo(w http.ResponseWriter, r *http.Request) {
	if s.auth == nil {
		writeJSON(w, http.StatusOK, UserInfoResponse{LoggedIn: true, AuthEnabled: false})
		return
	}
	claims, ok := s.authenticated(r)
	writeJSON(w, http.StatusOK, UserInfoResponse{LoggedIn: ok, Username: claims.Subject, AuthEnabled: true})
}

// clientIP identifies the caller for login throttling. It deliberately uses
// the connection's address rather than X-Forwarded-For, which the caller
// controls.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
