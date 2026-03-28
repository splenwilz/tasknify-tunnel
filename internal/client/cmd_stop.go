package client

import (
	"fmt"
	"net/http"

	"github.com/spf13/cobra"
	"github.com/splenwilz/devtunnel/pkg/config"
)

// NewStopCmd creates the `devtunnel stop` command.
func NewStopCmd() *cobra.Command {
	var serverURL string

	cmd := &cobra.Command{
		Use:   "stop <subdomain>",
		Short: "Stop an active tunnel",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			subdomain := args[0]

			cfg, err := config.LoadClientConfig()
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}

			adminURL := serverURL
			if adminURL == "" {
				adminURL = cfg.ServerURL
			}

			adminURL = wsToHTTP(adminURL)
			adminURL = stripSuffix(adminURL, "/_tunnel/connect") + "/_tunnel/admin/tunnels"

			req, err := http.NewRequest(http.MethodDelete, adminURL+"?subdomain="+subdomain, nil)
			if err != nil {
				return fmt.Errorf("create request: %w", err)
			}

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return fmt.Errorf("failed to reach server: %w", err)
			}
			defer resp.Body.Close()

			switch resp.StatusCode {
			case http.StatusNoContent:
				fmt.Printf("Tunnel %q stopped.\n", subdomain)
			case http.StatusNotFound:
				fmt.Printf("Tunnel %q not found.\n", subdomain)
			default:
				return fmt.Errorf("unexpected status: %s", resp.Status)
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&serverURL, "server", "", "Tunnel server URL")

	return cmd
}
