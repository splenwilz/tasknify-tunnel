# DevTunnel

A custom dev tunneling tool that exposes local development servers to the internet via custom subdomains. An ngrok alternative with full control over subdomain naming and no tunnel limits.

```
devtunnel http 3000 myapp
```
```
DevTunnel

  Tunnel URL:    https://myapp.tasknify.com
  Local:         http://localhost:3000
  Status:        Connected

  Press Ctrl+C to close the tunnel
```

## Architecture

```
Internet -> DNS (*.tasknify.com -> Server IP)
         -> Caddy (TLS termination, wildcard cert via DNS-01)
         -> Tunnel Server (Go, routes by Host header subdomain)
         -> yamux stream over WebSocket
         -> CLI Client -> localhost:PORT
```

**Stack:** Go + WebSocket + yamux + Caddy

## Quick Start

### Build

```bash
make build
```

This produces two binaries in `bin/`:
- `devtunnel` — CLI client (runs on developer machines)
- `devtunnel-server` — tunnel server (runs on your VPS)

### Run Locally (Development)

Terminal 1 — start the tunnel server:
```bash
make run-server
```

Terminal 2 — start a local HTTP server:
```bash
python3 -m http.server 3000
```

Terminal 3 — create a tunnel:
```bash
./bin/devtunnel http 3000 myapp --server ws://localhost:8001
```

Test it:
```bash
curl -H "Host: myapp.tasknify.com" http://localhost:8001/
```

## CLI Commands

```bash
devtunnel http <port> [subdomain]   # Create an HTTP tunnel
devtunnel list                       # List active tunnels
devtunnel stop <subdomain>          # Stop a tunnel
devtunnel version                    # Print version
```

### Flags

| Flag | Description |
|------|-------------|
| `--server` | Tunnel server URL (default: `wss://tasknify.com/_tunnel/connect`) |
| `--token` | Auth token for tunnel creation |

### Environment Variables

| Variable | Description |
|----------|-------------|
| `DEVTUNNEL_SERVER_URL` | Default server URL |
| `DEVTUNNEL_AUTH_TOKEN` | Default auth token |
| `DEVTUNNEL_DEBUG` | Set to `1` for debug logging |

### Config File

Client config: `~/.devtunnel.yaml`

```yaml
server_url: "wss://tasknify.com/_tunnel/connect"
auth_token: "your-token-here"
```

## Server Deployment

### Prerequisites

1. A VPS with a public IP
2. A domain with wildcard DNS configured:
   - `*.tasknify.com` -> your server IP
   - `tasknify.com` -> your server IP
3. Cloudflare account (recommended for DNS API / ACME)

### Using Docker Compose

```bash
cd deploy/

# Set your Cloudflare API token
export CLOUDFLARE_API_TOKEN=your-token

# Start server + Caddy
docker compose up -d
```

### Manual Deployment

1. Build the server binary:
   ```bash
   make build-server
   ```

2. Copy `bin/devtunnel-server` to `/usr/local/bin/`

3. Copy `deploy/server.yaml` to `/etc/devtunnel/server.yaml` and configure

4. Install the systemd service:
   ```bash
   sudo cp deploy/devtunnel-server.service /etc/systemd/system/
   sudo systemctl enable --now devtunnel-server
   ```

5. Install Caddy with Cloudflare DNS module:
   ```bash
   xcaddy build --with github.com/caddy-dns/cloudflare
   ```

6. Use `deploy/Caddyfile` as your Caddy config

### Server Configuration

`/etc/devtunnel/server.yaml`:

```yaml
listen_addr: ":8001"
domain: "tasknify.com"
heartbeat_interval: 15s
heartbeat_timeout: 5s

auth_tokens:
  - hash: "$2a$10$..."  # bcrypt hash of your token
    name: "dev-team"
    max_tunnels: 10

rate_limit:
  tunnel_creation_per_min: 5
  requests_per_sec: 100
  connections_per_min: 10
```

## Testing

```bash
make test    # Run all tests with race detector
make vet     # Run go vet
make lint    # Run golangci-lint (if installed)
```

## How It Works

1. CLI opens a WebSocket connection to the tunnel server
2. A yamux multiplexed session is created over the WebSocket
3. CLI sends a registration message with the desired subdomain
4. Server maps the subdomain to the client's yamux session
5. When an HTTP request arrives for `myapp.tasknify.com`:
   - Caddy terminates TLS and forwards to the tunnel server
   - Server extracts subdomain from the Host header
   - Server opens a new yamux stream to the client
   - The HTTP request is serialized and sent through the stream
   - Client forwards the request to `localhost:3000`
   - Response flows back through the same path

## License

MIT
