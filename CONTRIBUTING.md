# Contributing to RetroDash Bridge Server

Thank you for your interest in contributing! Here are some guidelines.

## Getting Started

1. Fork the repository
2. Clone your fork: `git clone https://github.com/<your-user>/retrodash-server.git`
3. Create a branch: `git checkout -b my-feature`
4. Make your changes
5. Run checks: `make lint test`
6. Commit and push
7. Open a Pull Request

## Development Setup

### Prerequisites

- Go 1.24+
- Chrome or Chromium
- golangci-lint

### Running locally

```bash
make deps
make test
make build
DASHBOARD_URL=http://localhost:3000 ./bridge
```

## Code Guidelines

- Run `make lint` before committing
- All tests must pass with `make test`
- New features should include tests
- Follow existing code patterns and conventions

## Reporting Issues

- Use GitHub Issues for bug reports and feature requests
- Include steps to reproduce for bugs
- Include Go version, OS, and Chrome version

## License

By contributing, you agree that your contributions will be licensed under the GPL v3 License.
