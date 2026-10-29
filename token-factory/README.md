# Token Factory

Standalone Go service foundation for issuing and managing application tokens.

## Run locally

1. Copy `.env.example` to `.env` and replace the placeholder values locally. Do not commit credentials.
2. Export the variables from `.env`.
3. Start the service:

```bash
GOTOOLCHAIN=local GOPROXY=off go run ./cmd/token-factory
```

The service listens on `:8090` by default. `GET /healthz` returns:

```json
{"status":"ok","service":"token-factory"}
```

This foundation does not connect to or modify MySQL or Redis. Later tasks add those adapters and migrations; the existing local containers remain external dependencies.

## Verify

```bash
GOTOOLCHAIN=local GOPROXY=off go test ./...
GOTOOLCHAIN=local GOPROXY=off CGO_ENABLED=0 go build ./cmd/token-factory
```
