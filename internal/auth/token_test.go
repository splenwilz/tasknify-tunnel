package auth

import (
	"testing"

	"github.com/splenwilz/devtunnel/pkg/config"
)

func TestAuthenticatorNoTokensConfigured(t *testing.T) {
	a := NewAuthenticator(nil)

	// Should allow any connection when no tokens are configured
	info, err := a.Validate("")
	if err != nil {
		t.Fatalf("expected no error with no tokens configured, got: %v", err)
	}
	if info.Name != "anonymous" {
		t.Errorf("expected anonymous token, got %q", info.Name)
	}
}

func TestAuthenticatorValidToken(t *testing.T) {
	hash, err := HashToken("my-secret-token")
	if err != nil {
		t.Fatalf("HashToken: %v", err)
	}

	a := NewAuthenticator([]config.AuthToken{
		{Hash: hash, Name: "dev-team", MaxTunnels: 5},
	})

	info, err := a.Validate("my-secret-token")
	if err != nil {
		t.Fatalf("expected valid token, got error: %v", err)
	}
	if info.Name != "dev-team" {
		t.Errorf("expected name %q, got %q", "dev-team", info.Name)
	}
	if info.MaxTunnels != 5 {
		t.Errorf("expected max_tunnels %d, got %d", 5, info.MaxTunnels)
	}
}

func TestAuthenticatorInvalidToken(t *testing.T) {
	hash, _ := HashToken("my-secret-token")

	a := NewAuthenticator([]config.AuthToken{
		{Hash: hash, Name: "dev-team", MaxTunnels: 5},
	})

	_, err := a.Validate("wrong-token")
	if err == nil {
		t.Fatal("expected error for invalid token")
	}
}

func TestAuthenticatorEmptyTokenWithConfigured(t *testing.T) {
	hash, _ := HashToken("my-secret-token")

	a := NewAuthenticator([]config.AuthToken{
		{Hash: hash, Name: "dev-team", MaxTunnels: 5},
	})

	_, err := a.Validate("")
	if err == nil {
		t.Fatal("expected error for empty token when tokens are configured")
	}
}

func TestHashToken(t *testing.T) {
	hash, err := HashToken("test-token")
	if err != nil {
		t.Fatalf("HashToken: %v", err)
	}
	if hash == "" {
		t.Fatal("expected non-empty hash")
	}
	if hash == "test-token" {
		t.Fatal("hash should not equal plaintext")
	}
}
