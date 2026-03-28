package client

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"sync"

	"github.com/coder/websocket"
	"github.com/hashicorp/yamux"
	"github.com/splenwilz/devtunnel/internal/protocol"
	"github.com/splenwilz/devtunnel/pkg/config"
)

// TunnelClient manages a single tunnel connection to the server.
type TunnelClient struct {
	config     *config.ClientConfig
	session    *yamux.Session
	wsConn     *websocket.Conn
	ctrlStream net.Conn
	logger     *slog.Logger
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
}

// NewTunnelClient creates a new tunnel client.
func NewTunnelClient(cfg *config.ClientConfig, logger *slog.Logger) *TunnelClient {
	return &TunnelClient{
		config: cfg,
		logger: logger,
	}
}

// Connect establishes a WebSocket + yamux connection and registers the tunnel.
// Returns the tunnel URL on success.
func (c *TunnelClient) Connect(ctx context.Context) (string, error) {
	c.ctx, c.cancel = context.WithCancel(ctx)

	// Dial WebSocket
	wsConn, _, err := websocket.Dial(c.ctx, c.config.ServerURL, nil)
	if err != nil {
		return "", fmt.Errorf("websocket dial: %w", err)
	}
	c.wsConn = wsConn

	// Wrap WebSocket as net.Conn for yamux
	netConn := websocket.NetConn(c.ctx, wsConn, websocket.MessageBinary)

	// Create yamux client session
	yamuxCfg := yamux.DefaultConfig()
	yamuxCfg.EnableKeepAlive = true
	yamuxCfg.KeepAliveInterval = 15 // seconds
	yamuxCfg.LogOutput = io.Discard

	session, err := yamux.Client(netConn, yamuxCfg)
	if err != nil {
		wsConn.Close(websocket.StatusInternalError, "yamux setup failed")
		return "", fmt.Errorf("yamux client: %w", err)
	}
	c.session = session

	// Open control stream (stream 0)
	ctrlStream, err := session.Open()
	if err != nil {
		session.Close()
		return "", fmt.Errorf("open control stream: %w", err)
	}
	c.ctrlStream = ctrlStream

	// Send registration
	if err := protocol.WriteMessage(ctrlStream, protocol.MsgRegister, protocol.RegisterPayload{
		Subdomain: c.config.Subdomain,
		AuthToken: c.config.AuthToken,
	}); err != nil {
		session.Close()
		return "", fmt.Errorf("send register: %w", err)
	}

	// Read response
	env, err := protocol.ReadMessage(ctrlStream)
	if err != nil {
		session.Close()
		return "", fmt.Errorf("read register response: %w", err)
	}

	switch env.Type {
	case protocol.MsgRegisterAck:
		ack, err := protocol.DecodePayload[protocol.RegisterAckPayload](env)
		if err != nil {
			session.Close()
			return "", fmt.Errorf("decode register ack: %w", err)
		}
		return ack.URL, nil

	case protocol.MsgRegisterErr:
		errPayload, err := protocol.DecodePayload[protocol.RegisterErrPayload](env)
		if err != nil {
			session.Close()
			return "", fmt.Errorf("decode register error: %w", err)
		}
		session.Close()
		return "", fmt.Errorf("registration failed: [%s] %s", errPayload.Code, errPayload.Message)

	default:
		session.Close()
		return "", fmt.Errorf("unexpected response type: %s", env.Type)
	}
}

// Run starts the accept loop and heartbeat responder. Blocks until disconnected.
func (c *TunnelClient) Run() error {
	// Start accept loop for proxied requests
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		c.acceptLoop()
	}()

	// Start heartbeat responder
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		c.heartbeatLoop()
	}()

	// Wait for yamux session to close
	<-c.session.CloseChan()
	return nil
}

// Close gracefully shuts down the tunnel.
func (c *TunnelClient) Close() {
	if c.ctrlStream != nil {
		// Send disconnect message (best effort)
		protocol.WriteMessage(c.ctrlStream, protocol.MsgDisconnect, nil)
	}
	if c.cancel != nil {
		c.cancel()
	}
	if c.session != nil {
		c.session.Close()
	}
	c.wg.Wait()
}

// acceptLoop accepts yamux streams from the server (each carrying an HTTP request).
func (c *TunnelClient) acceptLoop() {
	transport := &http.Transport{}

	for {
		stream, err := c.session.Accept()
		if err != nil {
			if c.ctx.Err() != nil {
				return // Shutting down
			}
			c.logger.Debug("accept stream error", "error", err)
			return
		}
		go c.handleStream(stream, transport)
	}
}

// heartbeatLoop responds to heartbeat messages from the server.
func (c *TunnelClient) heartbeatLoop() {
	for {
		env, err := protocol.ReadMessage(c.ctrlStream)
		if err != nil {
			if c.ctx.Err() != nil {
				return
			}
			c.logger.Debug("heartbeat read error", "error", err)
			return
		}

		switch env.Type {
		case protocol.MsgHeartbeat:
			payload, _ := protocol.DecodePayload[protocol.HeartbeatPayload](env)
			protocol.WriteMessage(c.ctrlStream, protocol.MsgHeartbeatAck, payload)
		default:
			c.logger.Debug("unexpected control message", "type", env.Type)
		}
	}
}
