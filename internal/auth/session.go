package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// MinSigningKeyLength is the minimum session signing key length, in bytes.
const MinSigningKeyLength = 32

// ErrInvalidCredentials is returned by Authenticator.Login for a wrong
// username or password -- deliberately without saying which.
var ErrInvalidCredentials = errors.New("invalid username or password")

// ErrInvalidToken is returned by Authenticator.Validate for a token that is
// malformed, tampered with, expired, or was issued for a different password.
var ErrInvalidToken = errors.New("invalid or expired session")

// Config configures an Authenticator.
type Config struct {
	Username     string
	PasswordHash string
	SigningKey   []byte
	SessionTTL   time.Duration
}

// Claims is the payload of a session token.
type Claims struct {
	Subject  string `json:"sub"`
	IssuedAt int64  `json:"iat"`
	Expires  int64  `json:"exp"`
	// Password is a fingerprint of the password hash the token was issued
	// under, so changing the admin password invalidates every existing
	// session (like Argo CD's admin.passwordMtime) without any server-side
	// session store.
	Password string `json:"pwd"`
}

// Authenticator checks admin credentials and issues/validates session
// tokens.
type Authenticator struct {
	username    string
	hash        PasswordHash
	fingerprint string
	key         []byte
	ttl         time.Duration
	now         func() time.Time
}

// New validates cfg and returns an Authenticator.
func New(cfg Config) (*Authenticator, error) {
	if cfg.Username == "" {
		return nil, errors.New("auth: admin username is empty")
	}
	hash, err := ParsePasswordHash(cfg.PasswordHash)
	if err != nil {
		return nil, fmt.Errorf("auth: admin password hash: %w", err)
	}
	if len(cfg.SigningKey) < MinSigningKeyLength {
		return nil, fmt.Errorf("auth: session signing key must be at least %d bytes, got %d", MinSigningKeyLength, len(cfg.SigningKey))
	}
	if cfg.SessionTTL <= 0 {
		return nil, errors.New("auth: session TTL must be positive")
	}
	sum := sha256.Sum256([]byte(strings.TrimSpace(cfg.PasswordHash)))
	return &Authenticator{
		username:    cfg.Username,
		hash:        hash,
		fingerprint: hex.EncodeToString(sum[:8]),
		key:         cfg.SigningKey,
		ttl:         cfg.SessionTTL,
		now:         time.Now,
	}, nil
}

// Username returns the admin account's username.
func (a *Authenticator) Username() string { return a.username }

// Login checks username/password and, if they match, returns a signed
// session token and its expiry.
func (a *Authenticator) Login(username, password string) (string, time.Time, error) {
	// Always run the (deliberately slow) hash comparison, even for a wrong
	// username, so response timing doesn't reveal which one was wrong.
	passwordOK := a.hash.Matches(password)
	usernameOK := subtle.ConstantTimeCompare([]byte(username), []byte(a.username)) == 1
	if !passwordOK || !usernameOK {
		return "", time.Time{}, ErrInvalidCredentials
	}

	now := a.now()
	expires := now.Add(a.ttl)
	token, err := a.sign(Claims{
		Subject:  a.username,
		IssuedAt: now.Unix(),
		Expires:  expires.Unix(),
		Password: a.fingerprint,
	})
	if err != nil {
		return "", time.Time{}, err
	}
	return token, expires, nil
}

// Validate verifies token's signature, expiry and password fingerprint.
func (a *Authenticator) Validate(token string) (Claims, error) {
	payload, mac, ok := strings.Cut(token, ".")
	if !ok {
		return Claims{}, ErrInvalidToken
	}
	gotMAC, err := base64.RawURLEncoding.DecodeString(mac)
	if err != nil || !hmac.Equal(gotMAC, a.mac(payload)) {
		return Claims{}, ErrInvalidToken
	}
	raw, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return Claims{}, ErrInvalidToken
	}
	var claims Claims
	if err := json.Unmarshal(raw, &claims); err != nil {
		return Claims{}, ErrInvalidToken
	}
	if claims.Subject != a.username || claims.Password != a.fingerprint || a.now().Unix() >= claims.Expires {
		return Claims{}, ErrInvalidToken
	}
	return claims, nil
}

func (a *Authenticator) sign(claims Claims) (string, error) {
	raw, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("auth: encode session: %w", err)
	}
	payload := base64.RawURLEncoding.EncodeToString(raw)
	return payload + "." + base64.RawURLEncoding.EncodeToString(a.mac(payload)), nil
}

func (a *Authenticator) mac(payload string) []byte {
	m := hmac.New(sha256.New, a.key)
	m.Write([]byte(payload))
	return m.Sum(nil)
}
