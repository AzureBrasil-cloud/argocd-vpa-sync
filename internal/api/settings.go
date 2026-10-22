package api

import (
	"net/http"
)

// SettingsResponse is the body of GET /api/v1/settings: the deployment's
// runtime configuration, read-only (it comes from the Helm values / env vars,
// see internal/config). Theme is also returned by the unauthenticated
// userinfo endpoint, so the login page is themed too.
type SettingsResponse struct {
	Theme string `json:"theme"`

	Version   string `json:"version"`
	GitCommit string `json:"gitCommit"`
	BuildDate string `json:"buildDate"`

	AuthEnabled       bool   `json:"authEnabled"`
	Username          string `json:"username,omitempty"`
	SessionTTLSeconds int64  `json:"sessionTtlSeconds,omitempty"`

	ArgoCDNamespace              string `json:"argocdNamespace"`
	StateSecretNamespace         string `json:"stateSecretNamespace"`
	StateSecretName              string `json:"stateSecretName"`
	WriteBackPollIntervalSeconds int64  `json:"writeBackPollIntervalSeconds"`
}

// defaultTheme is reported when no WithSettings option set one.
const defaultTheme = "default"

// WithSettings sets what GET /api/v1/settings reports, including the
// dashboard theme.
func WithSettings(settings SettingsResponse) Option {
	return func(s *Server) {
		s.settings = settings
	}
}

func (s *Server) theme() string {
	if s.settings.Theme == "" {
		return defaultTheme
	}
	return s.settings.Theme
}

func (s *Server) handleSettings(w http.ResponseWriter, _ *http.Request) {
	settings := s.settings
	settings.Theme = s.theme()
	settings.AuthEnabled = s.auth != nil
	if !settings.AuthEnabled {
		settings.Username = ""
		settings.SessionTTLSeconds = 0
	}
	writeJSON(w, http.StatusOK, settings)
}
