package client

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"time"
)

// handleStream reads an HTTP request from a yamux stream, forwards it to the
// local service, and writes the response back.
func (c *TunnelClient) handleStream(stream net.Conn, transport *http.Transport) {
	defer stream.Close()

	start := time.Now()

	// Read the HTTP request from the stream
	req, err := http.ReadRequest(bufio.NewReader(stream))
	if err != nil {
		c.logger.Debug("failed to read request from stream", "error", err)
		writeErrorResponse(stream, http.StatusBadGateway, "Failed to read request")
		return
	}

	// Rewrite the request to point to the local service
	req.URL.Scheme = "http"
	req.URL.Host = c.config.LocalAddr
	req.RequestURI = "" // Must be cleared for http.Client

	// Forward to local service
	resp, err := transport.RoundTrip(req)
	if err != nil {
		c.logger.Warn("local service unavailable",
			"addr", c.config.LocalAddr,
			"error", err,
		)
		writeErrorResponse(stream, http.StatusBadGateway, fmt.Sprintf("Local service at %s is unavailable", c.config.LocalAddr))
		return
	}
	defer resp.Body.Close()

	// Write response back through the tunnel
	if err := resp.Write(stream); err != nil {
		c.logger.Debug("failed to write response to stream", "error", err)
		return
	}

	duration := time.Since(start)
	c.logger.Info("request proxied",
		"method", req.Method,
		"path", req.URL.Path,
		"status", resp.StatusCode,
		"duration", duration.Round(time.Millisecond),
	)
}

// writeErrorResponse writes a minimal HTTP error response to a stream.
func writeErrorResponse(conn net.Conn, statusCode int, message string) {
	resp := &http.Response{
		StatusCode: statusCode,
		Status:     fmt.Sprintf("%d %s", statusCode, http.StatusText(statusCode)),
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
		Header:     make(http.Header),
	}
	resp.Header.Set("Content-Type", "text/plain")
	resp.Body = http.NoBody
	resp.Write(conn)
}
