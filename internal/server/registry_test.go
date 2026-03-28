package server

import (
	"sync"
	"testing"

	"github.com/hashicorp/yamux"
)

func TestValidateSubdomain(t *testing.T) {
	tests := []struct {
		name      string
		subdomain string
		wantErr   bool
	}{
		{"valid simple", "myapp", false},
		{"valid with numbers", "app123", false},
		{"valid with hyphens", "my-app", false},
		{"valid min length", "abc", false},
		{"too short", "ab", true},
		{"too long", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", true}, // 64 chars
		{"starts with hyphen", "-app", true},
		{"ends with hyphen", "app-", true},
		{"uppercase", "MyApp", true},
		{"special chars", "my_app", true},
		{"dots", "my.app", true},
		{"reserved api", "api", true},
		{"reserved www", "www", true},
		{"reserved admin", "admin", true},
		{"reserved tunnel", "tunnel", true},
		{"empty", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSubdomain(tt.subdomain)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateSubdomain(%q) error = %v, wantErr %v", tt.subdomain, err, tt.wantErr)
			}
		})
	}
}

func TestRegistryRegisterAndLookup(t *testing.T) {
	r := NewRegistry("tasknify.com")

	tunnel := &Tunnel{
		Subdomain: "myapp",
		Session:   nil, // nil is fine for registry tests
	}

	if err := r.Register(tunnel); err != nil {
		t.Fatalf("Register: %v", err)
	}

	// Lookup should find it
	found, ok := r.Lookup("myapp")
	if !ok {
		t.Fatal("expected to find tunnel")
	}
	if found.Subdomain != "myapp" {
		t.Errorf("expected subdomain %q, got %q", "myapp", found.Subdomain)
	}

	// Lookup should not find non-existent
	_, ok = r.Lookup("other")
	if ok {
		t.Error("did not expect to find non-existent tunnel")
	}
}

func TestRegistrySubdomainCollision(t *testing.T) {
	r := NewRegistry("tasknify.com")

	t1 := &Tunnel{Subdomain: "myapp"}
	t2 := &Tunnel{Subdomain: "myapp"}

	if err := r.Register(t1); err != nil {
		t.Fatalf("Register t1: %v", err)
	}

	if err := r.Register(t2); err == nil {
		t.Fatal("expected collision error")
	}
}

func TestRegistryUnregister(t *testing.T) {
	r := NewRegistry("tasknify.com")

	tunnel := &Tunnel{Subdomain: "myapp"}
	r.Register(tunnel)

	r.Unregister("myapp")

	_, ok := r.Lookup("myapp")
	if ok {
		t.Error("expected tunnel to be unregistered")
	}

	// Should be able to re-register after unregister
	t2 := &Tunnel{Subdomain: "myapp"}
	if err := r.Register(t2); err != nil {
		t.Fatalf("Re-register: %v", err)
	}
}

func TestRegistryList(t *testing.T) {
	r := NewRegistry("tasknify.com")

	r.Register(&Tunnel{Subdomain: "app1"})
	r.Register(&Tunnel{Subdomain: "app2"})
	r.Register(&Tunnel{Subdomain: "app3"})

	list := r.List()
	if len(list) != 3 {
		t.Errorf("expected 3 tunnels, got %d", len(list))
	}
}

func TestRegistryCount(t *testing.T) {
	r := NewRegistry("tasknify.com")

	if r.Count() != 0 {
		t.Errorf("expected 0, got %d", r.Count())
	}

	r.Register(&Tunnel{Subdomain: "app1"})
	r.Register(&Tunnel{Subdomain: "app2"})

	if r.Count() != 2 {
		t.Errorf("expected 2, got %d", r.Count())
	}
}

func TestRegistryCountByToken(t *testing.T) {
	r := NewRegistry("tasknify.com")

	r.Register(&Tunnel{Subdomain: "app1", AuthTokenName: "team-a"})
	r.Register(&Tunnel{Subdomain: "app2", AuthTokenName: "team-a"})
	r.Register(&Tunnel{Subdomain: "app3", AuthTokenName: "team-b"})

	if got := r.CountByToken("team-a"); got != 2 {
		t.Errorf("expected 2 for team-a, got %d", got)
	}
	if got := r.CountByToken("team-b"); got != 1 {
		t.Errorf("expected 1 for team-b, got %d", got)
	}
	if got := r.CountByToken("team-c"); got != 0 {
		t.Errorf("expected 0 for team-c, got %d", got)
	}
}

func TestRegistryConcurrentAccess(t *testing.T) {
	r := NewRegistry("tasknify.com")
	_ = yamux.DefaultConfig() // ensure import is used

	var wg sync.WaitGroup
	// 50 goroutines registering different subdomains
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			subdomain := "app" + string(rune('a'+i%26)) + string(rune('a'+i/26))
			tunnel := &Tunnel{Subdomain: subdomain}
			r.Register(tunnel)
			r.Lookup(subdomain)
			r.List()
			r.Count()
		}(i)
	}
	wg.Wait()
}
