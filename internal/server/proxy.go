package server

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// extractSubdomain parses the subdomain from a Host header.
// For "myapp.tasknify.com:443" with domain "tasknify.com", returns "myapp".
func extractSubdomain(host, domain string) (string, error) {
	// Strip port if present
	if idx := strings.LastIndex(host, ":"); idx != -1 {
		// Ensure it's a port, not part of an IPv6 address
		if !strings.Contains(host[idx:], "]") {
			host = host[:idx]
		}
	}

	host = strings.ToLower(host)

	suffix := "." + domain
	if !strings.HasSuffix(host, suffix) {
		return "", fmt.Errorf("host %q does not match domain %q", host, domain)
	}

	subdomain := strings.TrimSuffix(host, suffix)
	if subdomain == "" {
		return "", fmt.Errorf("no subdomain in host %q", host)
	}

	// Reject null bytes and other suspicious characters
	if strings.ContainsAny(subdomain, "\x00/\\") {
		return "", fmt.Errorf("invalid characters in subdomain")
	}

	return subdomain, nil
}

// handleProxyRequest routes an incoming HTTP request to the appropriate tunnel.
func (s *TunnelServer) handleProxyRequest(w http.ResponseWriter, r *http.Request) {
	subdomain, err := extractSubdomain(r.Host, s.config.Domain)
	if err != nil {
		http.Error(w, "Invalid host", http.StatusBadRequest)
		return
	}

	tunnel, ok := s.registry.Lookup(subdomain)
	if !ok {
		http.Error(w, fmt.Sprintf("Tunnel %q not found", subdomain), http.StatusNotFound)
		return
	}

	// Open a new yamux stream to the client
	stream, err := tunnel.Session.Open()
	if err != nil {
		s.logger.Error("failed to open stream to client",
			"subdomain", subdomain,
			"error", err,
		)
		http.Error(w, "Tunnel unavailable", http.StatusBadGateway)
		// If we can't open a stream, the tunnel is likely dead
		s.registry.Unregister(subdomain)
		tunnel.Session.Close()
		return
	}
	defer stream.Close()

	// Serialize the HTTP request in wire format and send through the tunnel
	if err := r.Write(stream); err != nil {
		s.logger.Error("failed to write request to tunnel",
			"subdomain", subdomain,
			"error", err,
		)
		http.Error(w, "Failed to forward request", http.StatusBadGateway)
		return
	}

	// Read the HTTP response from the tunnel
	resp, err := http.ReadResponse(bufio.NewReader(stream), r)
	if err != nil {
		s.logger.Error("failed to read response from tunnel",
			"subdomain", subdomain,
			"error", err,
		)
		http.Error(w, "Failed to read response from tunnel", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Copy response headers
	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	// Write status code and body
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)

	s.logger.Debug("proxied request",
		"subdomain", subdomain,
		"method", r.Method,
		"path", r.URL.Path,
		"status", resp.StatusCode,
	)
}
