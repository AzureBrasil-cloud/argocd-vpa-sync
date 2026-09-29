package config

import (
	"testing"
	"time"
)

func TestFromEnv_AuthIsOptional(t *testing.T) {
	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Auth.Enabled() {
		t.Fatal("expected auth to be disabled without ADMIN_PASSWORD_HASH")
	}

	t.Setenv("ADMIN_PASSWORD_HASH", "$2a$10$whatever")
	t.Setenv("ADMIN_USERNAME", "ops")
	t.Setenv("SESSION_TTL", "2h")
	t.Setenv("COOKIE_SECURE", "false")
	cfg, err = FromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.Auth.Enabled() || cfg.Auth.Username != "ops" || cfg.Auth.SessionTTL != 2*time.Hour || cfg.Auth.CookieSecure {
		t.Fatalf("unexpected auth config: %+v", cfg.Auth)
	}
}

func TestFromEnv_RejectsInvalidAuthSettings(t *testing.T) {
	for name, env := range map[string][2]string{
		"ttl":    {"SESSION_TTL", "1day"},
		"zero":   {"SESSION_TTL", "0s"},
		"cookie": {"COOKIE_SECURE", "maybe"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Setenv(env[0], env[1])
			if _, err := FromEnv(); err == nil {
				t.Fatalf("expected %s=%q to be rejected", env[0], env[1])
			}
		})
	}
}
