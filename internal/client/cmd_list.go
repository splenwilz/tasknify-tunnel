package client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"github.com/splenwilz/devtunnel/internal/server"
	"github.com/splenwilz/devtunnel/pkg/config"
)

// NewListCmd creates the `devtunnel list` command.
func NewListCmd() *cobra.Command {
	var serverURL string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List active tunnels",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.LoadClientConfig()
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}

			adminURL := serverURL
			if adminURL == "" {
				adminURL = cfg.ServerURL
			}

			// Convert ws:// to http:// for the admin API
			adminURL = wsToHTTP(adminURL)
			adminURL = adminURL + "/_tunnel/admin/tunnels"

			// Remove the /_tunnel/connect suffix if present, then add admin path
			adminURL = stripSuffix(adminURL, "/_tunnel/connect/_tunnel/admin/tunnels")
			if adminURL == "" {
				adminURL = wsToHTTP(cfg.ServerURL)
			}
			adminURL = stripSuffix(adminURL, "/_tunnel/connect") + "/_tunnel/admin/tunnels"

			resp, err := http.Get(adminURL)
			if err != nil {
				return fmt.Errorf("failed to reach server: %w", err)
			}
			defer resp.Body.Close()

			var tunnels []server.TunnelInfo
			if err := json.NewDecoder(resp.Body).Decode(&tunnels); err != nil {
				return fmt.Errorf("failed to decode response: %w", err)
			}

			if len(tunnels) == 0 {
				fmt.Println("No active tunnels.")
				return nil
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "SUBDOMAIN\tURL\tCREATED\tLAST PING")
			for _, t := range tunnels {
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
					t.Subdomain,
					t.URL,
					t.CreatedAt.Format(time.DateTime),
					t.LastPing.Format(time.DateTime),
				)
			}
			w.Flush()

			return nil
		},
	}

	cmd.Flags().StringVar(&serverURL, "server", "", "Tunnel server URL")

	return cmd
}

func wsToHTTP(url string) string {
	if len(url) >= 6 && url[:6] == "wss://" {
		return "https://" + url[6:]
	}
	if len(url) >= 5 && url[:5] == "ws://" {
		return "http://" + url[5:]
	}
	return url
}

func stripSuffix(s, suffix string) string {
	if len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix {
		return s[:len(s)-len(suffix)]
	}
	return s
}
