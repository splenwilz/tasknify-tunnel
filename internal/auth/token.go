package auth

import (
	"fmt"

	"github.com/splenwilz/devtunnel/pkg/config"
	"golang.org/x/crypto/bcrypt"
)

// Authenticator validates auth tokens against stored hashes.
type Authenticator struct {
	tokens []config.AuthToken
}

// NewAuthenticator creates a new authenticator from the server config.
func NewAuthenticator(tokens []config.AuthToken) *Authenticator {
	return &Authenticator{tokens: tokens}
}

// Validate checks if the provided token matches any stored hash.
// Returns the matched token config on success.
func (a *Authenticator) Validate(token string) (*config.AuthToken, error) {
	if len(a.tokens) == 0 {
		// No auth tokens configured — allow all connections
		return &config.AuthToken{Name: "anonymous", MaxTunnels: 10}, nil
	}

	if token == "" {
		return nil, fmt.Errorf("auth token required")
	}

	for _, t := range a.tokens {
		if err := bcrypt.CompareHashAndPassword([]byte(t.Hash), []byte(token)); err == nil {
			return &t, nil
		}
	}

	return nil, fmt.Errorf("invalid auth token")
}

// HashToken generates a bcrypt hash for a token. Useful for generating
// hashes to put in the server config.
func HashToken(token string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(token), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("hash token: %w", err)
	}
	return string(hash), nil
}
