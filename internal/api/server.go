// Package api implements the HTTP API the dashboard consumes: list and
// detail views of VPA recommendations compared against the value currently
// requested by the live workload they target (see
// internal/workloadresources), and select endpoints that queue a
// recommendation for the write-back worker to commit to Git. With WithAuth,
// every /api/ route requires an admin session (see session.go).
package api

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/azurebrasil/argocd-vpa-updater/internal/auth"
	"github.com/azurebrasil/argocd-vpa-updater/web"
)

// Server is the HTTP API server.
type Server struct {
	service *Service
	logger  *slog.Logger
	mux     *http.ServeMux

	// auth is nil when authentication is disabled (see WithAuth).
	auth         *auth.Authenticator
	cookieSecure bool
	loginLimiter *auth.LoginLimiter
}

// NewServer builds a Server backed by service.
func NewServer(service *Service, logger *slog.Logger, opts ...Option) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	s := &Server{service: service, logger: logger, mux: http.NewServeMux()}
	for _, opt := range opts {
		opt(s)
	}
	s.routes()
	return s
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /healthz", s.handleHealthz)
	s.mux.HandleFunc("GET /readyz", s.handleReadyz)

	s.mux.HandleFunc("POST /api/v1/session", s.handleLogin)
	s.mux.HandleFunc("DELETE /api/v1/session", s.handleLogout)
	s.mux.HandleFunc("GET /api/v1/session/userinfo", s.handleUserInfo)

	s.mux.HandleFunc("GET /api/v1/recommendations", s.handleListRecommendations)
	s.mux.HandleFunc("POST /api/v1/recommendations/bulk-select", s.handleBulkSelectRecommendations)
	s.mux.HandleFunc("GET /api/v1/recommendations/{namespace}/{vpaName}/{containerName}", s.handleGetRecommendation)
	s.mux.HandleFunc("POST /api/v1/recommendations/{namespace}/{vpaName}/{containerName}/select", s.handleSelectRecommendation)

	// TODO(write-back phase): once real write-back is implemented, mount
	// POST /api/v1/recommendations/{namespace}/{vpaName}/{containerName}/apply
	// here, backed by internal/gitwriteback.GitWriteBackService -- select
	// (above) only queues a domain.PendingSelection; nothing is written to
	// Git yet.

	// Catch-all fallback, registered last so /api/* and the health endpoints
	// above always take precedence.
	if web.Embedded {
		s.mux.Handle("/", spaHandler(web.FS))
	} else {
		s.mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "dashboard not embedded in this build (built without -tags dist); see README", http.StatusNotFound)
		})
	}
}

// ServeHTTP implements http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	lw := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
	if s.auth != nil && requiresSession(r) {
		if _, ok := s.authenticated(r); !ok {
			writeError(lw, http.StatusUnauthorized, "authentication required")
			s.logRequest(r, lw.status, start)
			return
		}
	}
	s.mux.ServeHTTP(lw, r)
	s.logRequest(r, lw.status, start)
}

func (s *Server) logRequest(r *http.Request, status int, start time.Time) {
	s.logger.Info("http request",
		"method", r.Method,
		"path", r.URL.Path,
		"status", status,
		"duration_ms", time.Since(start).Milliseconds(),
	)
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}
