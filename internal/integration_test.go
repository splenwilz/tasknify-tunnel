package internal

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/splenwilz/devtunnel/internal/client"
	"github.com/splenwilz/devtunnel/internal/server"
	"github.com/splenwilz/devtunnel/pkg/config"
)

// startTestServer creates a tunnel server on a random port and returns its URL.
func startTestServer(t *testing.T) (*httptest.Server, *config.ServerConfig) {
	t.Helper()

	cfg := &config.ServerConfig{
		Domain:            "tasknify.com",
		HeartbeatInterval: 60 * time.Second, // Long interval to avoid heartbeat noise in tests
		HeartbeatTimeout:  5 * time.Second,
		MaxTunnelsPerToken: 10,
		RateLimit: config.RateLimitConfig{
			TunnelCreationPerMin: 100,
			RequestsPerSec:       1000,
			ConnectionsPerMin:    100,
		},
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := server.New(cfg, logger)

	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	return ts, cfg
}

// wsURL converts an httptest.Server URL to a WebSocket URL.
func wsURL(ts *httptest.Server) string {
	return "ws" + strings.TrimPrefix(ts.URL, "http") + "/_tunnel/connect"
}

func TestIntegrationHappyPath(t *testing.T) {
	// Start a local "application" server
	localApp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Custom", "test-value")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "Hello from local app! Path: %s", r.URL.Path)
	}))
	defer localApp.Close()

	// Start the tunnel server
	ts, _ := startTestServer(t)

	// Extract local app port
	localAddr := strings.TrimPrefix(localApp.URL, "http://")

	// Create and connect tunnel client
	cfg := &config.ClientConfig{
		ServerURL: wsURL(ts),
		LocalAddr: localAddr,
		Subdomain: "myapp",
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	tc := client.NewTunnelClient(cfg, logger)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	url, err := tc.Connect(ctx)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer tc.Close()

	if url != "https://myapp.tasknify.com" {
		t.Errorf("expected URL https://myapp.tasknify.com, got %s", url)
	}

	// Start the client's accept loop in background
	go tc.Run()

	// Give the accept loop a moment to start
	time.Sleep(50 * time.Millisecond)

	// Send an HTTP request through the tunnel
	req, _ := http.NewRequest("GET", ts.URL+"/api/hello", nil)
	req.Host = "myapp.tasknify.com"

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("proxy request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "Hello from local app!") {
		t.Errorf("unexpected body: %s", body)
	}
	if !strings.Contains(string(body), "/api/hello") {
		t.Errorf("expected path /api/hello in body, got: %s", body)
	}
	if resp.Header.Get("X-Custom") != "test-value" {
		t.Errorf("expected X-Custom header, got %q", resp.Header.Get("X-Custom"))
	}
}

func TestIntegrationSubdomainCollision(t *testing.T) {
	ts, _ := startTestServer(t)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// First client registers "myapp"
	cfg1 := &config.ClientConfig{
		ServerURL: wsURL(ts),
		LocalAddr: "localhost:9999",
		Subdomain: "myapp",
	}
	tc1 := client.NewTunnelClient(cfg1, logger)
	_, err := tc1.Connect(ctx)
	if err != nil {
		t.Fatalf("Connect client 1: %v", err)
	}
	defer tc1.Close()

	// Second client tries to register "myapp"
	cfg2 := &config.ClientConfig{
		ServerURL: wsURL(ts),
		LocalAddr: "localhost:9998",
		Subdomain: "myapp",
	}
	tc2 := client.NewTunnelClient(cfg2, logger)
	_, err = tc2.Connect(ctx)
	if err == nil {
		tc2.Close()
		t.Fatal("expected collision error, got nil")
	}

	if !strings.Contains(err.Error(), "subdomain_taken") && !strings.Contains(err.Error(), "already in use") {
		t.Errorf("expected subdomain collision error, got: %v", err)
	}
}

func TestIntegrationTunnelNotFound(t *testing.T) {
	ts, _ := startTestServer(t)

	// Request to a subdomain with no active tunnel
	req, _ := http.NewRequest("GET", ts.URL+"/", nil)
	req.Host = "nonexistent.tasknify.com"

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

func TestIntegrationConcurrentRequests(t *testing.T) {
	// Local app that echoes the request path
	localApp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "path=%s", r.URL.Path)
	}))
	defer localApp.Close()

	ts, _ := startTestServer(t)
	localAddr := strings.TrimPrefix(localApp.URL, "http://")

	cfg := &config.ClientConfig{
		ServerURL: wsURL(ts),
		LocalAddr: localAddr,
		Subdomain: "loadtest",
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	tc := client.NewTunnelClient(cfg, logger)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := tc.Connect(ctx)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer tc.Close()

	go tc.Run()
	time.Sleep(50 * time.Millisecond)

	// Send 20 concurrent requests
	var wg sync.WaitGroup
	errors := make(chan error, 20)

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()

			path := fmt.Sprintf("/request/%d", i)
			req, _ := http.NewRequest("GET", ts.URL+path, nil)
			req.Host = "loadtest.tasknify.com"

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				errors <- fmt.Errorf("request %d: %w", i, err)
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode != 200 {
				errors <- fmt.Errorf("request %d: status %d", i, resp.StatusCode)
				return
			}

			body, _ := io.ReadAll(resp.Body)
			if !strings.Contains(string(body), path) {
				errors <- fmt.Errorf("request %d: expected path %s in body %q", i, path, body)
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Error(err)
	}
}

func TestIntegrationMultipleTunnels(t *testing.T) {
	ts, _ := startTestServer(t)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Create 3 local apps
	apps := make([]*httptest.Server, 3)
	clients := make([]*client.TunnelClient, 3)
	subdomains := []string{"app-one", "app-two", "app-three"}

	for i := 0; i < 3; i++ {
		idx := i
		apps[i] = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintf(w, "app-%d", idx)
		}))
		defer apps[i].Close()

		cfg := &config.ClientConfig{
			ServerURL: wsURL(ts),
			LocalAddr: strings.TrimPrefix(apps[i].URL, "http://"),
			Subdomain: subdomains[i],
		}
		tc := client.NewTunnelClient(cfg, logger)
		_, err := tc.Connect(ctx)
		if err != nil {
			t.Fatalf("Connect app %d: %v", i, err)
		}
		defer tc.Close()
		go tc.Run()
		clients[i] = tc
	}

	time.Sleep(50 * time.Millisecond)

	// Verify each tunnel routes to the correct app
	for i, sub := range subdomains {
		req, _ := http.NewRequest("GET", ts.URL+"/", nil)
		req.Host = sub + ".tasknify.com"

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Errorf("request to %s: %v", sub, err)
			continue
		}

		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		expected := fmt.Sprintf("app-%d", i)
		if string(body) != expected {
			t.Errorf("tunnel %s: expected %q, got %q", sub, expected, body)
		}
	}
}

func TestIntegrationDisconnectAndReregister(t *testing.T) {
	ts, _ := startTestServer(t)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Connect first client
	cfg := &config.ClientConfig{
		ServerURL: wsURL(ts),
		LocalAddr: "localhost:9999",
		Subdomain: "reuse-me",
	}
	tc1 := client.NewTunnelClient(cfg, logger)
	_, err := tc1.Connect(ctx)
	if err != nil {
		t.Fatalf("Connect 1: %v", err)
	}

	// Disconnect
	tc1.Close()
	time.Sleep(100 * time.Millisecond)

	// Re-register same subdomain with a new client
	tc2 := client.NewTunnelClient(cfg, logger)
	_, err = tc2.Connect(ctx)
	if err != nil {
		t.Fatalf("Re-register: %v", err)
	}
	tc2.Close()
}
