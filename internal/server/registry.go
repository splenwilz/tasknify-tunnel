package server

import (
	"fmt"
	"net"
	"regexp"
	"sync"
	"time"

	"github.com/hashicorp/yamux"
)

var (
	// subdomainRegex validates subdomain format: lowercase alphanumeric and hyphens.
	subdomainRegex = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)

	// reservedSubdomains cannot be registered by clients.
	reservedSubdomains = map[string]bool{
		"api":       true,
		"www":       true,
		"admin":     true,
		"app":       true,
		"dashboard": true,
		"status":    true,
		"tunnel":    true,
		"_tunnel":   true,
		"mail":      true,
		"ftp":       true,
		"ns1":       true,
		"ns2":       true,
		"dev":       true,
		"staging":   true,
		"cdn":       true,
		"smtp":      true,
		"imap":      true,
		"pop":       true,
	}
)

// Tunnel represents an active tunnel connection.
type Tunnel struct {
	Subdomain     string
	Session       *yamux.Session
	ControlStream net.Conn
	CreatedAt     time.Time
	LastPing      time.Time
	AuthTokenName string
}

// TunnelInfo is a read-only view of a tunnel for the admin API.
type TunnelInfo struct {
	Subdomain     string    `json:"subdomain"`
	URL           string    `json:"url"`
	CreatedAt     time.Time `json:"created_at"`
	LastPing      time.Time `json:"last_ping"`
	AuthTokenName string    `json:"auth_token_name"`
}

// Registry manages the mapping of subdomains to active tunnels.
type Registry struct {
	mu      sync.RWMutex
	tunnels map[string]*Tunnel
	domain  string
}

// NewRegistry creates a new subdomain registry.
func NewRegistry(domain string) *Registry {
	return &Registry{
		tunnels: make(map[string]*Tunnel),
		domain:  domain,
	}
}

// ValidateSubdomain checks if a subdomain name is valid for registration.
func ValidateSubdomain(subdomain string) error {
	if len(subdomain) < 3 {
		return fmt.Errorf("subdomain must be at least 3 characters")
	}
	if len(subdomain) > 63 {
		return fmt.Errorf("subdomain must be at most 63 characters")
	}
	if !subdomainRegex.MatchString(subdomain) {
		return fmt.Errorf("subdomain must contain only lowercase letters, numbers, and hyphens, and must start and end with a letter or number")
	}
	if reservedSubdomains[subdomain] {
		return fmt.Errorf("subdomain %q is reserved", subdomain)
	}
	return nil
}

// Register adds a tunnel to the registry. Returns an error if the subdomain
// is invalid or already taken.
func (r *Registry) Register(t *Tunnel) error {
	if err := ValidateSubdomain(t.Subdomain); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.tunnels[t.Subdomain]; exists {
		return fmt.Errorf("subdomain %q is already in use", t.Subdomain)
	}

	t.CreatedAt = time.Now()
	t.LastPing = time.Now()
	r.tunnels[t.Subdomain] = t
	return nil
}

// Unregister removes a tunnel from the registry.
func (r *Registry) Unregister(subdomain string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.tunnels, subdomain)
}

// Lookup returns the tunnel for a given subdomain.
func (r *Registry) Lookup(subdomain string) (*Tunnel, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tunnels[subdomain]
	return t, ok
}

// UpdateLastPing updates the last ping time for a tunnel.
func (r *Registry) UpdateLastPing(subdomain string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if t, ok := r.tunnels[subdomain]; ok {
		t.LastPing = time.Now()
	}
}

// List returns info about all active tunnels.
func (r *Registry) List() []TunnelInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	infos := make([]TunnelInfo, 0, len(r.tunnels))
	for _, t := range r.tunnels {
		infos = append(infos, TunnelInfo{
			Subdomain:     t.Subdomain,
			URL:           fmt.Sprintf("https://%s.%s", t.Subdomain, r.domain),
			CreatedAt:     t.CreatedAt,
			LastPing:      t.LastPing,
			AuthTokenName: t.AuthTokenName,
		})
	}
	return infos
}

// Count returns the number of active tunnels.
func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.tunnels)
}

// CountByToken returns the number of tunnels for a given auth token name.
func (r *Registry) CountByToken(tokenName string) int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	count := 0
	for _, t := range r.tunnels {
		if t.AuthTokenName == tokenName {
			count++
		}
	}
	return count
}
