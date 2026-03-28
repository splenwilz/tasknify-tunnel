package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/viper"
)

// ServerConfig holds tunnel server configuration.
type ServerConfig struct {
	ListenAddr        string        `mapstructure:"listen_addr"`
	Domain            string        `mapstructure:"domain"`
	HeartbeatInterval time.Duration `mapstructure:"heartbeat_interval"`
	HeartbeatTimeout  time.Duration `mapstructure:"heartbeat_timeout"`
	MaxTunnelsPerToken int          `mapstructure:"max_tunnels_per_token"`
	AuthTokens        []AuthToken   `mapstructure:"auth_tokens"`
	RateLimit         RateLimitConfig `mapstructure:"rate_limit"`
}

// AuthToken represents a hashed auth token with limits.
type AuthToken struct {
	Hash       string `mapstructure:"hash"`
	Name       string `mapstructure:"name"`
	MaxTunnels int    `mapstructure:"max_tunnels"`
}

// RateLimitConfig holds rate limiting settings.
type RateLimitConfig struct {
	TunnelCreationPerMin int `mapstructure:"tunnel_creation_per_min"`
	RequestsPerSec       int `mapstructure:"requests_per_sec"`
	ConnectionsPerMin    int `mapstructure:"connections_per_min"`
}

// ClientConfig holds CLI client configuration.
type ClientConfig struct {
	ServerURL string `mapstructure:"server_url"`
	AuthToken string `mapstructure:"auth_token"`
	LocalAddr string
	Subdomain string
}

// DefaultServerConfig returns server config with sensible defaults.
func DefaultServerConfig() *ServerConfig {
	return &ServerConfig{
		ListenAddr:        ":8001",
		Domain:            "tasknify.com",
		HeartbeatInterval: 15 * time.Second,
		HeartbeatTimeout:  5 * time.Second,
		MaxTunnelsPerToken: 10,
		RateLimit: RateLimitConfig{
			TunnelCreationPerMin: 5,
			RequestsPerSec:       100,
			ConnectionsPerMin:    10,
		},
	}
}

// DefaultClientConfig returns client config with sensible defaults.
func DefaultClientConfig() *ClientConfig {
	return &ClientConfig{
		ServerURL: "wss://tasknify.com/_tunnel/connect",
	}
}

// LoadServerConfig loads server configuration from file, env vars, and defaults.
func LoadServerConfig() (*ServerConfig, error) {
	v := viper.New()
	v.SetConfigName("server")
	v.SetConfigType("yaml")
	v.AddConfigPath("/etc/devtunnel")
	v.AddConfigPath(".")

	v.SetEnvPrefix("DEVTUNNEL")
	v.AutomaticEnv()

	defaults := DefaultServerConfig()
	v.SetDefault("listen_addr", defaults.ListenAddr)
	v.SetDefault("domain", defaults.Domain)
	v.SetDefault("heartbeat_interval", defaults.HeartbeatInterval)
	v.SetDefault("heartbeat_timeout", defaults.HeartbeatTimeout)
	v.SetDefault("max_tunnels_per_token", defaults.MaxTunnelsPerToken)
	v.SetDefault("rate_limit.tunnel_creation_per_min", defaults.RateLimit.TunnelCreationPerMin)
	v.SetDefault("rate_limit.requests_per_sec", defaults.RateLimit.RequestsPerSec)
	v.SetDefault("rate_limit.connections_per_min", defaults.RateLimit.ConnectionsPerMin)

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("read server config: %w", err)
		}
	}

	cfg := defaults
	if err := v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("unmarshal server config: %w", err)
	}

	return cfg, nil
}

// LoadClientConfig loads client configuration from file, env vars, and defaults.
func LoadClientConfig() (*ClientConfig, error) {
	v := viper.New()
	v.SetConfigName(".devtunnel")
	v.SetConfigType("yaml")

	home, err := os.UserHomeDir()
	if err == nil {
		v.AddConfigPath(home)
	}
	v.AddConfigPath(".")

	v.SetEnvPrefix("DEVTUNNEL")
	v.AutomaticEnv()

	defaults := DefaultClientConfig()
	v.SetDefault("server_url", defaults.ServerURL)

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("read client config: %w", err)
		}
	}

	cfg := defaults
	if err := v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("unmarshal client config: %w", err)
	}

	return cfg, nil
}

// TunnelURL returns the full HTTPS URL for a given subdomain.
func (c *ServerConfig) TunnelURL(subdomain string) string {
	return fmt.Sprintf("https://%s.%s", subdomain, c.Domain)
}

// ClientConfigPath returns the path to the client config file.
func ClientConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".devtunnel.yaml"
	}
	return filepath.Join(home, ".devtunnel.yaml")
}
