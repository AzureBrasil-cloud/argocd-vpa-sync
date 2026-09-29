// Package config defines argocd-vpa-updater's runtime configuration,
// populated from environment variables (matching the env vars set in
// deploy/manifests/Deployment).
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config is the controller's runtime configuration.
type Config struct {
	// ListenAddr is where the HTTP API (dashboard backend) listens.
	ListenAddr string

	// StateSecretNamespace/StateSecretName identify the Secret StateStore
	// persists to.
	StateSecretNamespace string
	StateSecretName      string

	// ArgoCDNamespace is where Argo CD (and its repository credential
	// Secrets) live.
	ArgoCDNamespace string

	// WriteBackPollInterval is how often the write-back worker checks
	// StateStore for newly queued selections.
	WriteBackPollInterval time.Duration

	// Auth configures the admin account guarding the dashboard and API.
	Auth AuthConfig
}

// AuthConfig configures the local admin account (see internal/auth).
type AuthConfig struct {
	// Enabled requires an admin session on every /api/ route. Disabling it
	// leaves the API -- including the endpoints that queue Git commits --
	// open to anyone who can reach it; only meant for local development.
	Enabled bool

	Username string
	// PasswordHash is a bcrypt or argon2id hash of the admin password.
	PasswordHash string
	// SigningKey signs session tokens; at least 32 bytes.
	SigningKey string
	SessionTTL time.Duration
	// CookieSecure sets the Secure flag on the session cookie. Only turn it
	// off when the dashboard is served over plain HTTP (e.g. port-forward).
	CookieSecure bool
}

// Default returns a Config with the same defaults documented in
// deploy/manifests/Deployment's env vars.
func Default() Config {
	return Config{
		ListenAddr:            ":8080",
		StateSecretNamespace:  "argocd",
		StateSecretName:       "argocd-vpa-updater-state",
		ArgoCDNamespace:       "argocd",
		WriteBackPollInterval: 20 * time.Second,
		Auth: AuthConfig{
			Enabled:      true,
			Username:     "admin",
			SessionTTL:   24 * time.Hour,
			CookieSecure: true,
		},
	}
}

// FromEnv builds a Config starting from Default() and overriding any field
// whose corresponding environment variable is set.
func FromEnv() (Config, error) {
	cfg := Default()

	if v := os.Getenv("LISTEN_ADDR"); v != "" {
		cfg.ListenAddr = v
	}
	if v := os.Getenv("STATE_SECRET_NAMESPACE"); v != "" {
		cfg.StateSecretNamespace = v
	}
	if v := os.Getenv("STATE_SECRET_NAME"); v != "" {
		cfg.StateSecretName = v
	}
	if v := os.Getenv("ARGOCD_NAMESPACE"); v != "" {
		cfg.ArgoCDNamespace = v
	}
	if v := os.Getenv("WRITEBACK_POLL_INTERVAL_SECONDS"); v != "" {
		seconds, err := strconv.Atoi(v)
		if err != nil || seconds <= 0 {
			return Config{}, fmt.Errorf("config: WRITEBACK_POLL_INTERVAL_SECONDS must be a positive integer, got %q", v)
		}
		cfg.WriteBackPollInterval = time.Duration(seconds) * time.Second
	}

	if v := os.Getenv("AUTH_ENABLED"); v != "" {
		enabled, err := strconv.ParseBool(v)
		if err != nil {
			return Config{}, fmt.Errorf("config: AUTH_ENABLED must be a boolean, got %q", v)
		}
		cfg.Auth.Enabled = enabled
	}
	if v := os.Getenv("ADMIN_USERNAME"); v != "" {
		cfg.Auth.Username = v
	}
	cfg.Auth.PasswordHash = os.Getenv("ADMIN_PASSWORD_HASH")
	cfg.Auth.SigningKey = os.Getenv("SESSION_SIGNING_KEY")
	if v := os.Getenv("SESSION_TTL"); v != "" {
		ttl, err := time.ParseDuration(v)
		if err != nil || ttl <= 0 {
			return Config{}, fmt.Errorf("config: SESSION_TTL must be a positive duration (e.g. 24h), got %q", v)
		}
		cfg.Auth.SessionTTL = ttl
	}
	if v := os.Getenv("COOKIE_SECURE"); v != "" {
		secure, err := strconv.ParseBool(v)
		if err != nil {
			return Config{}, fmt.Errorf("config: COOKIE_SECURE must be a boolean, got %q", v)
		}
		cfg.Auth.CookieSecure = secure
	}

	return cfg, nil
}
