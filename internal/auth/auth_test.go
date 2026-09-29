package auth

import (
	"encoding/base64"
	"fmt"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/bcrypt"
)

const testKey = "0123456789abcdef0123456789abcdef"

func bcryptOf(t *testing.T, password string) string {
	t.Helper()
	h, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	return string(h)
}

func argon2idOf(password string) string {
	salt := []byte("somesaltsomesalt")
	key := argon2.IDKey([]byte(password), salt, 2, 64*1024, 1, 32)
	return fmt.Sprintf("$argon2id$v=%d$m=65536,t=2,p=1$%s$%s", argon2.Version,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key))
}

func TestParsePasswordHash_Formats(t *testing.T) {
	bc := bcryptOf(t, "s3cret")
	cases := map[string]string{
		"bcrypt $2a$":            bc,
		"bcrypt $2y$ (htpasswd)": "$2y$" + strings.TrimPrefix(bc, "$2a$"),
		"argon2id":               argon2idOf("s3cret"),
		"argon2id padded base64": argon2idOf("s3cret") + "=",
		"surrounding whitespace": "  " + bc + "\n",
	}
	for name, hash := range cases {
		t.Run(name, func(t *testing.T) {
			h, err := ParsePasswordHash(hash)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !h.Matches("s3cret") {
				t.Fatalf("expected the right password to match")
			}
			if h.Matches("wrong") {
				t.Fatalf("expected a wrong password not to match")
			}
		})
	}
}

func TestParsePasswordHash_Rejects(t *testing.T) {
	for name, hash := range map[string]string{
		"empty":            "",
		"plaintext":        "s3cret",
		"sha512 crypt":     "$6$salt$abc",
		"truncated bcrypt": "$2a$10$abc",
		"argon2i":          strings.Replace(argon2idOf("x"), "argon2id", "argon2i", 1),
		"argon2id no salt": "$argon2id$v=19$m=65536,t=2,p=1$",
		"argon2id bad ver": strings.Replace(argon2idOf("x"), "v=19", "v=16", 1),
		"argon2id zero t":  strings.Replace(argon2idOf("x"), "t=2", "t=0", 1),
		"argon2id huge m":  strings.Replace(argon2idOf("x"), "m=65536", "m=99999999", 1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ParsePasswordHash(hash); err == nil {
				t.Fatalf("expected %q to be rejected", hash)
			}
		})
	}
}

func newTestAuthenticator(t *testing.T, hash string) *Authenticator {
	t.Helper()
	a, err := New(Config{Username: "admin", PasswordHash: hash, SigningKey: []byte(testKey), SessionTTL: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func TestNew_Validates(t *testing.T) {
	hash := bcryptOf(t, "pw")
	for name, cfg := range map[string]Config{
		"no username": {PasswordHash: hash, SigningKey: []byte(testKey), SessionTTL: time.Hour},
		"bad hash":    {Username: "admin", PasswordHash: "pw", SigningKey: []byte(testKey), SessionTTL: time.Hour},
		"short key":   {Username: "admin", PasswordHash: hash, SigningKey: []byte("short"), SessionTTL: time.Hour},
		"no ttl":      {Username: "admin", PasswordHash: hash, SigningKey: []byte(testKey)},
	} {
		if _, err := New(cfg); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestLoginAndValidate(t *testing.T) {
	a := newTestAuthenticator(t, bcryptOf(t, "pw"))

	if _, _, err := a.Login("admin", "wrong"); err != ErrInvalidCredentials {
		t.Fatalf("expected ErrInvalidCredentials for a wrong password, got %v", err)
	}
	if _, _, err := a.Login("root", "pw"); err != ErrInvalidCredentials {
		t.Fatalf("expected ErrInvalidCredentials for a wrong username, got %v", err)
	}

	token, expires, err := a.Login("admin", "pw")
	if err != nil {
		t.Fatalf("unexpected login error: %v", err)
	}
	if time.Until(expires) <= 0 {
		t.Fatalf("expected a future expiry, got %v", expires)
	}
	claims, err := a.Validate(token)
	if err != nil {
		t.Fatalf("expected a fresh token to validate: %v", err)
	}
	if claims.Subject != "admin" {
		t.Fatalf("expected subject admin, got %q", claims.Subject)
	}
}

func TestValidate_RejectsBadTokens(t *testing.T) {
	a := newTestAuthenticator(t, bcryptOf(t, "pw"))
	token, _, err := a.Login("admin", "pw")
	if err != nil {
		t.Fatal(err)
	}
	payload, mac, _ := strings.Cut(token, ".")

	t.Run("garbage", func(t *testing.T) {
		for _, tok := range []string{"", "abc", "a.b", payload, "." + mac} {
			if _, err := a.Validate(tok); err == nil {
				t.Errorf("expected %q to be rejected", tok)
			}
		}
	})

	t.Run("tampered payload", func(t *testing.T) {
		forged := base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"admin","exp":99999999999}`))
		if _, err := a.Validate(forged + "." + mac); err == nil {
			t.Fatal("expected a tampered payload to be rejected")
		}
	})

	t.Run("other signing key", func(t *testing.T) {
		other, err := New(Config{Username: "admin", PasswordHash: bcryptOf(t, "pw"), SigningKey: []byte(strings.Repeat("x", 32)), SessionTTL: time.Hour})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := other.Validate(token); err == nil {
			t.Fatal("expected a token signed with another key to be rejected")
		}
	})

	t.Run("expired", func(t *testing.T) {
		a.now = func() time.Time { return time.Now().Add(2 * time.Hour) }
		defer func() { a.now = time.Now }()
		if _, err := a.Validate(token); err == nil {
			t.Fatal("expected an expired token to be rejected")
		}
	})

	t.Run("password changed", func(t *testing.T) {
		changed, err := New(Config{Username: "admin", PasswordHash: argon2idOf("new"), SigningKey: []byte(testKey), SessionTTL: time.Hour})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := changed.Validate(token); err == nil {
			t.Fatal("expected a token issued under the old password to be rejected")
		}
	})
}

func TestLoginLimiter(t *testing.T) {
	now := time.Now()
	l := NewLoginLimiter()
	l.now = func() time.Time { return now }

	for i := 0; i < freeFailures-1; i++ {
		l.Failure("1.2.3.4")
	}
	if ok, _ := l.Allow("1.2.3.4"); !ok {
		t.Fatal("expected attempts below the free budget to be allowed")
	}
	l.Failure("1.2.3.4")
	if ok, wait := l.Allow("1.2.3.4"); ok || wait <= 0 {
		t.Fatalf("expected the client to be throttled, got ok=%v wait=%v", ok, wait)
	}
	if ok, _ := l.Allow("5.6.7.8"); !ok {
		t.Fatal("expected other clients to be unaffected")
	}

	now = now.Add(maxDelay)
	if ok, _ := l.Allow("1.2.3.4"); !ok {
		t.Fatal("expected the client to be allowed again after the delay")
	}

	l.Success("1.2.3.4")
	l.Failure("1.2.3.4")
	if ok, _ := l.Allow("1.2.3.4"); !ok {
		t.Fatal("expected a success to reset the failure count")
	}
}
