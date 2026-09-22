package domain

// GitAuthMethod identifies how a GitCredentials value authenticates against
// a Git remote.
type GitAuthMethod string

const (
	AuthNone  GitAuthMethod = "none"
	AuthBasic GitAuthMethod = "basic"
	AuthSSH   GitAuthMethod = "ssh"
	AuthToken GitAuthMethod = "token"
)

// redacted is what every serialization of GitCredentials shows, regardless
// of destination (log, JSON API response, error message, event). There is no
// code path in this codebase that should ever render actual secret material;
// this type makes that a property of the type itself rather than something
// every call site has to remember.
const redacted = "[REDACTED]"

// GitCredentials holds the material needed to authenticate against a Git
// remote. It is produced by a RepositoryCredentialsProvider and consumed by
// a GitWriteBackService; it must never be logged, returned by the API, or
// otherwise displayed.
type GitCredentials struct {
	AuthMethod GitAuthMethod

	// Username applies to AuthBasic and AuthToken (as the token owner, e.g.
	// for GitHub App-style auth); it is not secret by itself but is kept
	// alongside the secret material for convenience.
	Username string

	// Secret holds the password/token for AuthBasic/AuthToken.
	Secret string

	// SSHPrivateKey and SSHKnownHosts apply to AuthSSH.
	SSHPrivateKey []byte
	SSHKnownHosts []byte
}

// String, GoString and MarshalJSON all return a fixed redacted
// representation so that fmt.Println, %v/%+v formatting, structured logging
// libraries, and accidental json.Marshal calls can never leak credential
// material.
func (GitCredentials) String() string   { return redacted }
func (GitCredentials) GoString() string { return redacted }

func (c GitCredentials) MarshalJSON() ([]byte, error) {
	return []byte(`"` + redacted + `"`), nil
}
