package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/splenwilz/devtunnel/internal/client"
)

var version = "dev"

func main() {
	rootCmd := &cobra.Command{
		Use:   "devtunnel",
		Short: "Expose local services to the internet via custom subdomains",
		Long:  "DevTunnel creates secure tunnels from the public internet to your local development server.",
	}

	rootCmd.AddCommand(
		client.NewHTTPCmd(),
		client.NewListCmd(),
		client.NewStopCmd(),
		newVersionCmd(),
	)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("devtunnel %s\n", version)
		},
	}
}
