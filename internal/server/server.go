package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/coder/websocket"
	"github.com/hashicorp/yamux"
	"github.com/splenwilz/devtunnel/internal/auth"
	"github.com/splenwilz/devtunnel/internal/protocol"
	"github.com/splenwilz/devtunnel/pkg/config"
)

// TunnelServer handles WebSocket tunnel connections and HTTP request proxying.
type TunnelServer struct {
	config      *config.ServerConfig
	registry    *Registry
	auth        *auth.Authenticator
	rateLimiter *RateLimiter
	logger      *slog.Logger
}

// New creates a new TunnelServer.
func New(cfg *config.ServerConfig, logger *slog.Logger) *TunnelServer {
	return &TunnelServer{
		config:      cfg,
		registry:    NewRegistry(cfg.Domain),
		auth:        auth.NewAuthenticator(cfg.AuthTokens),
		rateLimiter: NewRateLimiter(cfg.RateLimit.TunnelCreationPerMin, cfg.RateLimit.ConnectionsPerMin),
		logger:      logger,
	}
}

// ServeHTTP routes requests to either the tunnel control path or the proxy.
func (s *TunnelServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/_tunnel/connect":
		s.handleTunnelConnect(w, r)
	case "/_tunnel/admin/tunnels":
		s.handleAdminTunnels(w, r)
	default:
		s.handleProxyRequest(w, r)
	}
}

// handleTunnelConnect upgrades to WebSocket, establishes yamux, and registers the tunnel.
func (s *TunnelServer) handleTunnelConnect(w http.ResponseWriter, r *http.Request) {
	// Rate limit connection attempts
	clientIP := extractIP(r.RemoteAddr)
	if !s.rateLimiter.AllowConnection(clientIP) {
		http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
		return
	}

	wsConn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true,
	})
	if err != nil {
		s.logger.Error("websocket accept failed", "error", err)
		return
	}

	ctx := r.Context()
	netConn := websocket.NetConn(ctx, wsConn, websocket.MessageBinary)

	// Create yamux server session (client is yamux Client, we are Server)
	yamuxCfg := yamux.DefaultConfig()
	yamuxCfg.EnableKeepAlive = true
	yamuxCfg.KeepAliveInterval = s.config.HeartbeatInterval
	yamuxCfg.ConnectionWriteTimeout = 10 * time.Second
	yamuxCfg.StreamCloseTimeout = 30 * time.Second
	yamuxCfg.LogOutput = io.Discard

	session, err := yamux.Server(netConn, yamuxCfg)
	if err != nil {
		s.logger.Error("yamux server creation failed", "error", err)
		wsConn.Close(websocket.StatusInternalError, "yamux setup failed")
		return
	}

	// Accept the control stream (stream 0, opened by the client)
	ctrlStream, err := session.Accept()
	if err != nil {
		s.logger.Error("failed to accept control stream", "error", err)
		session.Close()
		return
	}

	// Read registration message
	env, err := protocol.ReadMessage(ctrlStream)
	if err != nil {
		s.logger.Error("failed to read registration", "error", err)
		session.Close()
		return
	}

	if env.Type != protocol.MsgRegister {
		s.logger.Error("expected register message", "got", env.Type)
		protocol.WriteMessage(ctrlStream, protocol.MsgRegisterErr, protocol.RegisterErrPayload{
			Code:    "protocol_error",
			Message: "expected register message",
		})
		session.Close()
		return
	}

	reg, err := protocol.DecodePayload[protocol.RegisterPayload](env)
	if err != nil {
		s.logger.Error("failed to decode registration", "error", err)
		session.Close()
		return
	}

	// Authenticate
	tokenInfo, err := s.auth.Validate(reg.AuthToken)
	if err != nil {
		protocol.WriteMessage(ctrlStream, protocol.MsgRegisterErr, protocol.RegisterErrPayload{
			Code:    "auth_failed",
			Message: err.Error(),
		})
		session.Close()
		return
	}

	// Rate limit tunnel creation
	if !s.rateLimiter.AllowTunnelCreation(clientIP) {
		protocol.WriteMessage(ctrlStream, protocol.MsgRegisterErr, protocol.RegisterErrPayload{
			Code:    "rate_limited",
			Message: "too many tunnel creation requests",
		})
		session.Close()
		return
	}

	// Check per-token tunnel limit
	if tokenInfo.MaxTunnels > 0 && s.registry.CountByToken(tokenInfo.Name) >= tokenInfo.MaxTunnels {
		protocol.WriteMessage(ctrlStream, protocol.MsgRegisterErr, protocol.RegisterErrPayload{
			Code:    "limit_exceeded",
			Message: fmt.Sprintf("max tunnels (%d) reached for this token", tokenInfo.MaxTunnels),
		})
		session.Close()
		return
	}

	// Validate subdomain
	if err := ValidateSubdomain(reg.Subdomain); err != nil {
		protocol.WriteMessage(ctrlStream, protocol.MsgRegisterErr, protocol.RegisterErrPayload{
			Code:    "invalid_subdomain",
			Message: err.Error(),
		})
		session.Close()
		return
	}

	// Register the tunnel
	tunnel := &Tunnel{
		Subdomain:     reg.Subdomain,
		Session:       session,
		ControlStream: ctrlStream,
		AuthTokenName: tokenInfo.Name,
	}

	if err := s.registry.Register(tunnel); err != nil {
		protocol.WriteMessage(ctrlStream, protocol.MsgRegisterErr, protocol.RegisterErrPayload{
			Code:    "subdomain_taken",
			Message: err.Error(),
		})
		session.Close()
		return
	}

	tunnelURL := s.config.TunnelURL(reg.Subdomain)

	// Send registration acknowledgment
	if err := protocol.WriteMessage(ctrlStream, protocol.MsgRegisterAck, protocol.RegisterAckPayload{
		URL:       tunnelURL,
		Subdomain: reg.Subdomain,
	}); err != nil {
		s.logger.Error("failed to send register ack", "error", err)
		s.registry.Unregister(reg.Subdomain)
		session.Close()
		return
	}

	s.logger.Info("tunnel registered",
		"subdomain", reg.Subdomain,
		"url", tunnelURL,
		"remote", r.RemoteAddr,
	)

	// Block the handler — coder/websocket requires the handler to stay alive
	// while the WebSocket is in use (it doesn't hijack the connection).
	// Each WebSocket client gets its own HTTP handler goroutine, so blocking is fine.
	s.manageTunnel(context.Background(), tunnel)
}

// manageTunnel handles heartbeats and detects disconnections for a tunnel.
func (s *TunnelServer) manageTunnel(ctx context.Context, t *Tunnel) {
	defer func() {
		s.registry.Unregister(t.Subdomain)
		t.Session.Close()
		s.logger.Info("tunnel unregistered", "subdomain", t.Subdomain)
	}()

	ticker := time.NewTicker(s.config.HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// Send heartbeat
			ts := time.Now().UnixMilli()
			if err := protocol.WriteMessage(t.ControlStream, protocol.MsgHeartbeat, protocol.HeartbeatPayload{
				Timestamp: ts,
			}); err != nil {
				s.logger.Debug("heartbeat send failed", "subdomain", t.Subdomain, "error", err)
				return
			}

			// Read heartbeat ack with timeout
			t.ControlStream.(net.Conn).SetReadDeadline(time.Now().Add(s.config.HeartbeatTimeout))
			env, err := protocol.ReadMessage(t.ControlStream)
			t.ControlStream.(net.Conn).SetReadDeadline(time.Time{}) // Clear deadline

			if err != nil {
				s.logger.Debug("heartbeat ack timeout", "subdomain", t.Subdomain, "error", err)
				return
			}

			if env.Type == protocol.MsgDisconnect {
				s.logger.Info("client disconnected gracefully", "subdomain", t.Subdomain)
				return
			}

			if env.Type == protocol.MsgHeartbeatAck {
				s.registry.UpdateLastPing(t.Subdomain)
			}

		case <-ctx.Done():
			return

		case <-s.sessionDone(t.Session):
			return
		}
	}
}

// sessionDone returns a channel that closes when the yamux session is closed.
func (s *TunnelServer) sessionDone(session *yamux.Session) <-chan struct{} {
	ch := make(chan struct{})
	go func() {
		<-session.CloseChan()
		close(ch)
	}()
	return ch
}

// handleAdminTunnels serves the admin API for listing and managing tunnels.
func (s *TunnelServer) handleAdminTunnels(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		tunnels := s.registry.List()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(tunnels)

	case http.MethodDelete:
		subdomain := r.URL.Query().Get("subdomain")
		if subdomain == "" {
			http.Error(w, "subdomain parameter required", http.StatusBadRequest)
			return
		}
		tunnel, ok := s.registry.Lookup(subdomain)
		if !ok {
			http.Error(w, "tunnel not found", http.StatusNotFound)
			return
		}
		tunnel.Session.Close()
		s.registry.Unregister(subdomain)
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// extractIP extracts the IP address from a remote address string (host:port).
func extractIP(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return remoteAddr
	}
	return host
}
