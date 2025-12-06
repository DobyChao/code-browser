# Development Guide

## Project Layout
- `cmd/server`: HTTP server entry
- `cmd/cli`: repository management CLI
- `internal/repo`: SQLite-backed repository provider, Zoekt indexing
- `internal/core`: file tree and blob service
- `internal/search`: Zoekt and Ripgrep engines + handlers
- `internal/analysis`: SCIP-based definition + fallback search
- `web/`: frontend (vanilla HTML/JS)

## Build & Run
```bash
./build.sh
./start.sh   # starts repo-server and zoekt-webserver
./stop.sh    # stops processes
```

## Testing
```bash
go test ./...
```
- Unit tests exist under `internal/*` (add more as needed). Example: `internal/analysis/service_test.go`.

## Coding Guidelines
- Prefer clear error wrapping (`fmt.Errorf`) and avoid leaking internals.
- Follow existing patterns for caching (`patrickmn/go-cache`).
- Avoid committing secrets; do not log sensitive values.

## API Docs
- See `./docs/api.md` for endpoints and payloads.

## Contributing
- Fork, branch per feature, open PR.
- Run `go test ./...` before submitting.
- Maintain consistent logging and error handling across modules.

## Versioning
- Documentation tracks repository versions:
  - Update `docs/` with each change that affects behavior or interfaces.
  - For tagged releases, ensure docs reflect the tagged state.
  - Consider generating a release notes file per tag summarizing API/behavior changes.
