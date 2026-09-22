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

	return cfg, nil
}
