// Package initialadmin generates the admin password when the chart exposes
// the dashboard (e.g. through an Ingress) without one configured, in the
// style of Argo CD's argocd-initial-admin-secret: on first start the pod
// creates a Secret holding a random password, its bcrypt hash and a session
// signing key, and every later start reuses it. Generating it here rather
// than in the Helm chart keeps it stable under `helm template` renders (as
// Argo CD does), where lookup() returns nothing and a templated random
// value would change on every sync.
package initialadmin

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"golang.org/x/crypto/bcrypt"
)

// Secret data keys. PasswordHashKey and SigningKeyKey match the keys of
// auth.existingSecret, so the Secret can later be reused as one.
const (
	// PasswordKey holds the plaintext password, for the operator to read.
	// It may be removed once saved elsewhere: only the hash is needed.
	PasswordKey     = "password"
	PasswordHashKey = "admin.password"
	SigningKeyKey   = "server.secretkey"
)

const (
	passwordLength = 24
	bcryptCost     = 10
	signingKeySize = 32 // bytes, hex-encoded into the Secret
)

const passwordAlphabet = "abcdefghijkmnopqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789"

// Credentials are what the authenticator needs from the Secret.
type Credentials struct {
	PasswordHash string
	SigningKey   string
}

// Ensure returns the credentials stored in Secret namespace/name, creating
// it with a freshly generated password first if it doesn't exist. created
// reports whether this call generated it.
func Ensure(ctx context.Context, c client.Client, namespace, name string) (creds Credentials, created bool, err error) {
	key := client.ObjectKey{Namespace: namespace, Name: name}
	var secret corev1.Secret
	err = c.Get(ctx, key, &secret)
	if err == nil {
		creds, err = fromSecret(&secret)
		return creds, false, err
	}
	if !apierrors.IsNotFound(err) {
		return Credentials{}, false, fmt.Errorf("initialadmin: get secret %s/%s: %w", namespace, name, err)
	}

	password, err := randomPassword()
	if err != nil {
		return Credentials{}, false, err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return Credentials{}, false, fmt.Errorf("initialadmin: hash password: %w", err)
	}
	signingKey := make([]byte, signingKeySize)
	if _, err := rand.Read(signingKey); err != nil {
		return Credentials{}, false, fmt.Errorf("initialadmin: generate signing key: %w", err)
	}

	secret = corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Type:       corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			PasswordKey:     []byte(password),
			PasswordHashKey: hash,
			SigningKeyKey:   []byte(hex.EncodeToString(signingKey)),
		},
	}
	if err := c.Create(ctx, &secret); err != nil {
		if !apierrors.IsAlreadyExists(err) {
			return Credentials{}, false, fmt.Errorf("initialadmin: create secret %s/%s: %w", namespace, name, err)
		}
		// Lost a race with another start: use the one that won.
		if err := c.Get(ctx, key, &secret); err != nil {
			return Credentials{}, false, fmt.Errorf("initialadmin: get secret %s/%s: %w", namespace, name, err)
		}
		creds, err = fromSecret(&secret)
		return creds, false, err
	}
	creds, err = fromSecret(&secret)
	return creds, true, err
}

func fromSecret(secret *corev1.Secret) (Credentials, error) {
	creds := Credentials{
		PasswordHash: string(secret.Data[PasswordHashKey]),
		SigningKey:   string(secret.Data[SigningKeyKey]),
	}
	if creds.PasswordHash == "" {
		return Credentials{}, fmt.Errorf("initialadmin: secret %s/%s has no %q key; delete it to generate a new password", secret.Namespace, secret.Name, PasswordHashKey)
	}
	return creds, nil
}

// randomPassword draws from an alphabet without look-alike characters
// (0/O, 1/l/I), so it can be read off a terminal and typed.
func randomPassword() (string, error) {
	buf := make([]byte, passwordLength)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("initialadmin: generate password: %w", err)
	}
	for i, b := range buf {
		// 256 % 57 leaves a slight bias, negligible at this length.
		buf[i] = passwordAlphabet[int(b)%len(passwordAlphabet)]
	}
	return string(buf), nil
}
