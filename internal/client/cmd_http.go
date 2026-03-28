package client

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/splenwilz/devtunnel/pkg/config"
)

// NewHTTPCmd creates the `devtunnel http` command.
func NewHTTPCmd() *cobra.Command {
	var serverURL string
	var authToken string

	cmd := &cobra.Command{
		Use:   "http <port> [subdomain]",
		Short: "Create an HTTP tunnel to a local port",
		Long:  "Exposes a local HTTP server to the internet via a custom subdomain.",
		Example: `  devtunnel http 3000 myapp
  devtunnel http 8080 api --server ws://localhost:8001
  devtunnel http 3000 myapp --token my-secret-token`,
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Parse port
			port, err := strconv.Atoi(args[0])
			if err != nil || port < 1 || port > 65535 {
				return fmt.Errorf("invalid port: %s (must be 1-65535)", args[0])
			}

			// Check if the local port is accessible
			conn, err := net.Dial("tcp", fmt.Sprintf("localhost:%d", port))
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: nothing is listening on localhost:%d\n", port)
			} else {
				conn.Close()
			}

			// Load client config
			cfg, err := config.LoadClientConfig()
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}

			cfg.LocalAddr = fmt.Sprintf("localhost:%d", port)

			// Subdomain: argument > random not implemented yet, so require it
			if len(args) >= 2 {
				cfg.Subdomain = args[1]
			} else {
				return fmt.Errorf("subdomain is required (e.g., devtunnel http 3000 myapp)")
			}

			// Override from flags
			if serverURL != "" {
				cfg.ServerURL = ensureTunnelPath(serverURL)
			}
			if authToken != "" {
				cfg.AuthToken = authToken
			}

			// Setup logger
			logLevel := slog.LevelInfo
			if os.Getenv("DEVTUNNEL_DEBUG") == "1" {
				logLevel = slog.LevelDebug
			}
			logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
				Level: logLevel,
			}))

			// Setup graceful shutdown
			ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()

			fmt.Println()
			fmt.Println("DevTunnel")
			fmt.Println()

			// Connect with auto-reconnect
			return ConnectWithRetry(ctx, cfg, logger, func(url string) {
				fmt.Printf("  Tunnel URL:    %s\n", url)
				fmt.Printf("  Local:         http://%s\n", cfg.LocalAddr)
				fmt.Printf("  Status:        Connected\n")
				fmt.Println()
				fmt.Println("  Press Ctrl+C to close the tunnel")
				fmt.Println()
			})
		},
	}

	cmd.Flags().StringVar(&serverURL, "server", "", "Tunnel server URL (e.g., ws://localhost:8001)")
	cmd.Flags().StringVar(&authToken, "token", "", "Auth token for tunnel creation")

	return cmd
}

const tunnelPath = "/_tunnel/connect"

// ensureTunnelPath appends /_tunnel/connect if not already present.
func ensureTunnelPath(serverURL string) string {
	if len(serverURL) >= len(tunnelPath) && serverURL[len(serverURL)-len(tunnelPath):] == tunnelPath {
		return serverURL
	}
	// Strip trailing slash
	if len(serverURL) > 0 && serverURL[len(serverURL)-1] == '/' {
		serverURL = serverURL[:len(serverURL)-1]
	}
	return serverURL + tunnelPath
}
