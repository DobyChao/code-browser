# Code Browser

A lightweight code browsing service with a CLI to manage repositories and an HTTP server. Uses Zoekt for fast indexing/search; optional SCIP index enables precise jump-to-definition.

**Quick Start**

```bash
./build.sh
./start.sh    # starts repo-server and zoekt-webserver
# Add a repository
./repo-cli -command add -id 1 -name "my-repo" -path "/abs/path/to/my-repo" -data-dir .data
# Trigger Zoekt index
./repo-cli -command index -id 1 -data-dir .data
# Register SCIP index
./repo-cli -command register-scip -id 1 -scip-path /path/to/index.scip
```

**Basic Usage**

- Web UI: open `http://localhost:8088/`
- API: see detailed docs below
- Stop services: `./stop.sh`

**Command-line tools**

- `./repo-cli` — CLI for managing repositories. Use it to add, delete, and trigger indexing of repositories. The implementation is in `cmd/cli/main.go` (refer to that file for exact behavior). The `repo-cli` uses command-line flags (not subcommands). Important flags:

  - `-command` : required; one of `add`, `delete`, or `index`.
  - `-data-dir` : optional; global data directory (default: `./.data`).
  - `-id` : required for `add`, `delete`, and `index`; a numeric uint32 identifier for the repository.
  - `-name` : required for `add`; the display name for the repository.
  - `-path` : required for `add`; absolute path to the repository source on disk.

  Examples (use the exact flag names shown above):

  ```bash
  # Add a repo
  ./repo-cli -command add -id 1 -name "my-repo" -path "/abs/path/to/my-repo" -data-dir ".data"

  # Delete a repo
  ./repo-cli -command delete -id 1 -data-dir ".data"

  # Trigger Zoekt indexing for a repo (see note below)
  ./repo-cli -command index -id 1 -data-dir ".data"
  ```

  Notes:
  - Requires `zoekt-git-index` and `zoekt-webserver` in `PATH`.
  - Ensure repo path is a valid Git repository before indexing.

  Register SCIP index:

  ```bash
  ./repo-cli -command register-scip -id <repoId> -scip-path </path/to/index.scip>
  ```
  Copies the provided `.scip` file into `<data-dir>/repos/<id>/scip/index.scip` without modification.

- `./repo-server` — starts the HTTP service that serves repository information and search results. See `cmd/server/main.go` for flags and configuration options. Note: at present you must start the Zoekt webserver manually (see below).

  Example:

  ```bash
  ./repo-server --port 8080 --data-dir ".data"
  ```

**Dependencies**
- Go >= 1.25.1
- Zoekt tools:
  ```bash
  go install github.com/sourcegraph/zoekt/cmd/zoekt-git-index@latest
  go install github.com/sourcegraph/zoekt/cmd/zoekt-webserver@latest
  export PATH="$PATH:$HOME/go/bin"
  ```

**Start/Stop**

- `start.sh` — convenience script to start the service and any local helpers. Run:

```bash
./start.sh
```

- `stop.sh` — stops services started by `start.sh`:

```bash
./stop.sh
```

Note: `start.sh` automatically starts `zoekt-webserver`.

**Detailed Docs**

## Detailed Documentation
- [API Interface](./docs/api.md) 
- [Configuration Guide](./docs/configuration.md) 
- [Development Manual](./docs/development.md) 

**Documentation Versioning**
- Docs follow repository changes; update `docs/` alongside code.
- For releases, tag the repo and consider freezing a `docs/` snapshot per tag.

**License & Contributing**
- License: MIT (add `LICENSE` file if missing)
- Contributions welcome: please run `go test ./...` and follow existing module patterns.

*(moved to Command-line tools section)*
