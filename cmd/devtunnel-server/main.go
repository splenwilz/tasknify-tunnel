package main

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"github.com/splenwilz/devtunnel/internal/server"
	"github.com/splenwilz/devtunnel/pkg/config"
)

var version = "dev"

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	cfg, err := config.LoadServerConfig()
	if err != nil {
		logger.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	// Railway sets PORT env var — override listen addr if present
	if port := os.Getenv("PORT"); port != "" {
		cfg.ListenAddr = ":" + port
	}

	srv := server.New(cfg, logger)

	logger.Info("starting devtunnel server",
		"version", version,
		"addr", cfg.ListenAddr,
		"domain", cfg.Domain,
	)

	if err := http.ListenAndServe(cfg.ListenAddr, srv); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
}
