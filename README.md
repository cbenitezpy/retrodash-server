# RetroDash Bridge Server

Go-based MJPEG streaming server using ChromeDP for headless browser automation. Renders web dashboards (Grafana, Home Assistant, etc.) and streams them as MJPEG to mobile clients.

## Features

- Headless Chrome/Chromium rendering via ChromeDP
- MJPEG streaming with quality selection (high/low)
- Multi-source switching (dashboards, SSH terminals, local commands)
- Touch event relay (tap, drag)
- Health check endpoint
- Auto-recovery from browser crashes
- Origin management API (CRUD)
- Docker support with multi-arch builds (AMD64/ARM64)

## Quick Start

### Using Docker (recommended)

```bash
docker run --rm \
  --shm-size=256m \
  -p 8080:8080 \
  -e DASHBOARD_URL=http://host.docker.internal:3000/d/my-dashboard \
  ghcr.io/cbenitezpy-ueno/retrodash-server:latest
```

### Using Docker Compose

```bash
cp .env.example .env
# Edit .env with your dashboard URL
docker compose up
```

### From Source

```bash
# Install dependencies
make deps

# Run tests
make test

# Build binary
make build

# Run server
DASHBOARD_URL=http://localhost:3000/d/my-dashboard ./bridge
```

## Configuration

All configuration is via environment variables:

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DASHBOARD_URL` | Yes | - | URL of dashboard to render |
| `PORT` | No | 8080 | HTTP server port |
| `FPS` | No | 15 | Target frames per second (1-30) |
| `JPEG_QUALITY_HIGH` | No | 85 | JPEG quality for high quality stream (1-100) |
| `JPEG_QUALITY_LOW` | No | 50 | JPEG quality for low quality stream (1-100) |
| `VIEWPORT_WIDTH` | No | 1920 | Browser viewport width |
| `VIEWPORT_HEIGHT` | No | 1080 | Browser viewport height |
| `CHROME_PATH` | No | auto-detect | Path to Chrome/Chromium binary |
| `AUTH_COOKIES` | No | - | Cookies for authenticated dashboards |
| `ORIGINS_FILE` | No | `./data/origins.json` | Path to origins config file |

## API Endpoints

### GET /health

Health check endpoint.

```json
{
  "status": "ok",
  "version": "1.0.0",
  "uptime": 3600,
  "browserStatus": "ready",
  "activeClients": 2
}
```

### GET /stream

MJPEG video stream. Query parameters:
- `quality`: `high` (default) or `low`

```bash
# View in browser
open http://localhost:8080/stream

# View in VLC
vlc http://localhost:8080/stream
```

### POST /touch

Send touch events to the rendered dashboard.

```bash
# Tap at center (coordinates are normalized 0-1)
curl -X POST http://localhost:8080/touch \
  -H "Content-Type: application/json" \
  -d '{"x": 0.5, "y": 0.5, "type": "start"}'
```

Touch types: `start`, `move`, `end`

### Origins API

Manage dashboard sources (origins):

```bash
# List all origins
curl http://localhost:8080/api/origins

# Create an origin
curl -X POST http://localhost:8080/api/origins \
  -H "Content-Type: application/json" \
  -d '{"name": "my-dashboard", "type": "grafana", "config": {"url": "http://grafana:3000/d/abc"}}'

# Switch active origin
curl -X POST http://localhost:8080/api/origins/switch \
  -H "Content-Type: application/json" \
  -d '{"originId": "<uuid>"}'
```

## Project Structure

```
retrodash-server/
├── cmd/bridge/          # Main application entry point
├── internal/
│   ├── api/             # HTTP handlers and middleware
│   ├── browser/         # ChromeDP browser management
│   ├── capture/         # Screen capture interface
│   ├── config/          # Configuration loading
│   ├── health/          # Health check logic
│   ├── origins/         # Origin management (CRUD)
│   ├── stream/          # MJPEG streaming
│   ├── switching/       # Source switching logic
│   └── terminal/        # Terminal/SSH rendering
├── pkg/types/           # Shared types
├── data/                # Runtime data (origins config)
├── Dockerfile           # Multi-stage Docker build
├── docker-compose.yml   # Docker Compose config
└── Makefile             # Build automation
```

## Development

### Prerequisites

- Go 1.24+
- Chrome or Chromium browser
- golangci-lint (for linting)

### Make Targets

```bash
make build              # Build binary
make test               # Run tests with coverage
make test-coverage      # Generate HTML coverage report
make lint               # Run golangci-lint
make clean              # Remove build artifacts
make deps               # Download and tidy dependencies
make docker-build       # Build Docker image
make docker-run         # Run Docker container (requires DASHBOARD_URL)
```

### Running Tests

```bash
# All tests
make test

# With coverage report
make test-coverage

# Specific package
go test -v ./internal/browser/...
```

## Deployment

### Raspberry Pi / Low-power devices

1. Lower FPS (10-15) for less CPU usage
2. Use low quality stream for bandwidth savings
3. Smaller viewport (1280x720) reduces memory
4. Docker with 256MB shm_size is recommended

### Docker Compose example

```yaml
services:
  bridge:
    image: ghcr.io/cbenitezpy-ueno/retrodash-server:latest
    ports:
      - "8080:8080"
    environment:
      - DASHBOARD_URL=http://grafana:3000/d/my-dashboard
      - FPS=15
    shm_size: '256mb'
    restart: unless-stopped
```

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

## License

This project is licensed under the GNU General Public License v3.0 - see the [LICENSE](LICENSE) file for details.
