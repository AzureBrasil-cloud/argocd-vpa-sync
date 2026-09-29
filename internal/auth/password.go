// Package auth implements the single local admin account that guards the
// dashboard and its API, modelled on Argo CD's: the password is only ever
// stored as a hash (bcrypt or argon2id, e.g. taken from a Kubernetes
// Secret), and a successful login is exchanged for a stateless, HMAC-signed
// session token.
package auth

import (
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/bcrypt"
)

// PasswordHash is a parsed, validated password hash that plaintext
// passwords can be checked against.
type PasswordHash interface {
	Matches(password string) bool
}

// ParsePasswordHash validates hash and returns a PasswordHash for it. The
// format is detected from its prefix:
//
//   - bcrypt: $2a$, $2b$ or $2y$ (e.g. `htpasswd -nbBC 10 "" pw` or
//     `argocd account bcrypt --password pw`)
//   - argon2id, PHC string format: $argon2id$v=19$m=<KiB>,t=<iterations>,p=<threads>$<salt>$<hash>
//     (e.g. `echo -n pw | argon2 <salt> -id -e`)
//
// Anything else is rejected, so a misconfigured hash fails at startup
// instead of silently locking everyone out.
func ParsePasswordHash(hash string) (PasswordHash, error) {
	hash = strings.TrimSpace(hash)
	switch {
	case hash == "":
		return nil, errors.New("password hash is empty")
	case strings.HasPrefix(hash, "$2a$"), strings.HasPrefix(hash, "$2b$"), strings.HasPrefix(hash, "$2y$"):
		if _, err := bcrypt.Cost([]byte(hash)); err != nil {
			return nil, fmt.Errorf("invalid bcrypt hash: %w", err)
		}
		return bcryptHash(hash), nil
	case strings.HasPrefix(hash, "$argon2id$"):
		return parseArgon2id(hash)
	default:
		return nil, errors.New("unsupported password hash format: expected bcrypt ($2a$/$2b$/$2y$) or argon2id ($argon2id$)")
	}
}

type bcryptHash string

func (h bcryptHash) Matches(password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(h), []byte(password)) == nil
}

type argon2idHash struct {
	memory  uint32
	time    uint32
	threads uint8
	salt    []byte
	key     []byte
}

func (h argon2idHash) Matches(password string) bool {
	key := argon2.IDKey([]byte(password), h.salt, h.time, h.memory, h.threads, uint32(len(h.key)))
	return subtle.ConstantTimeCompare(key, h.key) == 1
}

// maxArgon2Memory caps the m= parameter (in KiB) so a hash can't make every
// login attempt allocate an unbounded amount of memory.
const maxArgon2Memory = 1 << 20 // 1 GiB

func parseArgon2id(hash string) (PasswordHash, error) {
	// "", "argon2id", "v=19", "m=...,t=...,p=...", salt, key
	parts := strings.Split(hash, "$")
	if len(parts) != 6 {
		return nil, errors.New("invalid argon2id hash: expected $argon2id$v=<version>$m=<m>,t=<t>,p=<p>$<salt>$<hash>")
	}

	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return nil, fmt.Errorf("invalid argon2id hash version %q: %w", parts[2], err)
	}
	if version != argon2.Version {
		return nil, fmt.Errorf("unsupported argon2id version %d (want %d)", version, argon2.Version)
	}

	var h argon2idHash
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &h.memory, &h.time, &h.threads); err != nil {
		return nil, fmt.Errorf("invalid argon2id parameters %q: %w", parts[3], err)
	}
	if h.memory == 0 || h.time == 0 || h.threads == 0 {
		return nil, fmt.Errorf("invalid argon2id parameters %q: m, t and p must be positive", parts[3])
	}
	if h.memory > maxArgon2Memory {
		return nil, fmt.Errorf("argon2id memory parameter m=%d exceeds the %d KiB limit", h.memory, maxArgon2Memory)
	}

	var err error
	if h.salt, err = decodePHCBase64(parts[4]); err != nil {
		return nil, fmt.Errorf("invalid argon2id salt: %w", err)
	}
	if h.key, err = decodePHCBase64(parts[5]); err != nil {
		return nil, fmt.Errorf("invalid argon2id hash: %w", err)
	}
	if len(h.key) < 16 {
		return nil, errors.New("invalid argon2id hash: key shorter than 16 bytes")
	}
	return h, nil
}

// decodePHCBase64 decodes the unpadded standard base64 the PHC string
// format uses, tolerating padding some tools add anyway.
func decodePHCBase64(s string) ([]byte, error) {
	return base64.RawStdEncoding.DecodeString(strings.TrimRight(s, "="))
}
