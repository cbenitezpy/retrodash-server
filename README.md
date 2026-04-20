# RetroDash Bridge Server

Go-based MJPEG streaming server using ChromeDP for headless browser automation. Renders web dashboards (Grafana, Home Assistant, etc.) and streams them as MJPEG to mobile clients.

## Features

- Headless Chrome/Chromium rendering via ChromeDP
- MJPEG streaming with quality selection (high/low)
- **Embedded web UI** — open `http://server:8080/` in any browser for a zero-install viewer (react-native-web bundle baked into the binary via `//go:embed`)
- Multi-source switching (dashboards, SSH terminals, local commands)
- Touch event relay (tap, drag)
- Kubernetes-native health probes (`/healthz`, `/readyz`)
- Prometheus metrics endpoint (`/metrics`)
- Structured JSON logging with configurable log level
- Helm chart and Kustomize manifests for K8s deployment
- Health check endpoint
- Auto-recovery from browser crashes
- Origin management API (CRUD)
- Docker support with multi-arch builds (AMD64/ARM64)

## Quick Start

### Using Docker (recommended)

See the [Docker](#docker) section for `docker run`, Docker Compose, and architecture details.

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

## Docker

### Supported Architectures

| Device | Architecture | Supported |
|--------|-------------|-----------|
| PC Intel/AMD | linux/amd64 | Yes |
| Mac Intel | linux/amd64 | Yes (Docker Desktop) |
| Mac Apple Silicon (M1-M4) | linux/arm64 | Yes (Docker Desktop) |
| Raspberry Pi 3/4/5 | linux/arm64 | Yes |

Docker automatically pulls the correct image for your architecture.

### Pull

```bash
docker pull ghcr.io/cbenitezpy/retrodash-server:latest
```

### Run

```bash
docker run --rm \
  --shm-size=256m \
  -p 8080:8080 \
  -e DASHBOARD_URL=http://host.docker.internal:3000/d/my-dashboard \
  ghcr.io/cbenitezpy/retrodash-server:latest
```

### Verify it works

Open the bridge in your browser to confirm the server is rendering your dashboard:

```bash
open http://localhost:8080/          # zero-install web UI (CRT viewer + channels + settings)
# or open http://localhost:8080/stream for the raw MJPEG feed
# or: curl -I http://localhost:8080/stream
# or: vlc http://localhost:8080/stream
```

Since feature 053 (web UI), the root URL serves a react-native-web bundle embedded into the binary — you see the same retro CRT channel list + viewer as the mobile app, no install required. The raw `/stream` endpoint remains available for VLC, mpv, or the native [RetroDash app](https://github.com/cbenitezpy/retrodash/releases).

### Docker Compose

```yaml
services:
  bridge:
    image: ghcr.io/cbenitezpy/retrodash-server:latest
    ports:
      - "8080:8080"
    environment:
      - DASHBOARD_URL=http://grafana:3000/d/my-dashboard
      - FPS=15
    shm_size: '256mb'
    restart: unless-stopped
```

### Available Tags

| Tag | Description |
|-----|-------------|
| `latest` | Latest build from `main` branch |
| `1.0.0` | Specific version |
| `1.0` | Latest patch of minor version |
| `1` | Latest minor of major version |
| `abc1234` | Specific commit SHA |

### Making the Package Public (first time)

After the first successful publish, the package defaults to private. To make it public:

1. Go to https://github.com/cbenitezpy/retrodash-server/pkgs/container/retrodash-server
2. Click **Package settings**
3. Under **Danger Zone**, click **Change visibility**
4. Select **Public** and confirm

## Kubernetes

### Helm (recommended)

```bash
# Install from OCI registry
helm install retrodash-bridge \
  oci://ghcr.io/cbenitezpy/charts/retrodash-bridge \
  --set env.DASHBOARD_URL=https://grafana.example.com/d/your-dashboard

# Verify pod is ready
kubectl get pods -l app.kubernetes.io/name=retrodash-bridge -w

# Access the stream
kubectl port-forward svc/retrodash-bridge 8080:8080
open http://localhost:8080/stream
```

### Helm with custom values

```bash
helm install retrodash-bridge \
  oci://ghcr.io/cbenitezpy/charts/retrodash-bridge \
  --set env.DASHBOARD_URL=https://grafana.example.com/d/your-dashboard \
  --set resources.limits.memory=2Gi \
  --set ingress.enabled=true \
  --set ingress.hosts[0].host=retrodash.example.com \
  --set "ingress.hosts[0].paths[0].path=/" \
  --set "ingress.hosts[0].paths[0].pathType=Prefix"
```

### Kustomize

```bash
# Clone this repo
git clone https://github.com/cbenitezpy/retrodash-server.git
cd retrodash-server

# Create an overlay with your configuration
mkdir -p deploy/k8s/overlays/my-env
cat > deploy/k8s/overlays/my-env/kustomization.yaml <<EOF
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources:
  - ../../
patches:
  - target:
      kind: Deployment
      name: retrodash-bridge
    patch: |
      - op: replace
        path: /spec/template/spec/containers/0/env/0/value
        value: "https://grafana.example.com/d/your-dashboard"
EOF
kubectl apply -k deploy/k8s/overlays/my-env/
```

### Uninstall

```bash
helm uninstall retrodash-bridge
# PVC is retained to protect data. To remove:
kubectl delete pvc retrodash-bridge-data
```

### Health Probes

| Endpoint | Purpose | Used by |
|----------|---------|---------|
| `/healthz` | Liveness — is the process alive? | `livenessProbe` |
| `/readyz` | Readiness — is the browser ready? | `readinessProbe` |
| `/metrics` | Prometheus metrics | Prometheus scrape |

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
| `LOG_LEVEL` | No | `INFO` | Log level: DEBUG, INFO, WARN, ERROR |

## API Endpoints

### GET /

Serves the embedded react-native-web viewer. Returns the SPA shell
(`index.html`) with `Cache-Control: no-store`. Any client-side route
that does not match a more specific endpoint also falls through to
this shell when the `Accept` header includes `text/html` — other
Accept types get a 404 so API misses are not masked.

```bash
open http://localhost:8080/
# channel list auto-populates from /api/origins; tap → TUNE IN → live stream.
```

### GET /static/{path}

Hashed JS/CSS/image assets emitted by the webpack build. Served from
the embedded bundle with `Cache-Control: public, max-age=31536000,
immutable`. Paths that try to escape the `static/` prefix (e.g.
`/static/../secret`) return 404.

### GET /healthz

Kubernetes liveness probe. Returns `200 "ok"` if the server process is alive.

### GET /readyz

Kubernetes readiness probe. Returns `200 "ok"` when the browser is ready, `503 "not ready"` otherwise.

### GET /metrics

Prometheus metrics in text exposition format. Includes custom metrics (`retrodash_*`) and Go runtime metrics.

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

### GET /snapshot

Single JPEG frame capture. Returns the current screen as a JPEG image.
Useful for clients that cannot use MJPEG streaming (e.g., older iOS devices where WKWebView is unavailable).

Query parameters:
- `quality`: `high` (default, 85%) or `low` (50%)

```bash
# Get a single frame
curl -o frame.jpg http://localhost:8080/snapshot

# Get low quality frame
curl -o frame.jpg http://localhost:8080/snapshot?quality=low
```

Response headers:
- `Content-Type: image/jpeg`
- `Cache-Control: no-cache, no-store, must-revalidate`

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
├── deploy/
│   ├── helm/            # Helm chart for Kubernetes
│   └── k8s/             # Kustomize manifests
├── Dockerfile           # Multi-stage Docker build
├── docker-compose.yml   # Docker Compose config
└── Makefile             # Build automation
```

## Development

### Prerequisites

- Go 1.25+
- Chrome or Chromium browser
- golangci-lint (for linting)
- Node 20/22 (only if you want to rebuild the embedded web UI bundle;
  the CI pipeline does this automatically and the binary runs fine
  without the bundle present — it just logs a "web UI disabled" notice
  and still serves the full API)

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

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

## License

This project is licensed under the GNU Affero General Public License v3.0 - see the [LICENSE](LICENSE) file for details.
