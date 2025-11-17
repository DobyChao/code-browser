# Code Browser

This repository provides a small code browsing service with a CLI for managing repositories and a server component. The project uses Go and Sourcegraph's Zoekt for indexing and searching repositories.

**Dependencies**
- **Go**: version >= 1.25.1.
- **Zoekt tools**: install the following binaries into your Go bin directory:

  ```bash
  go install github.com/sourcegraph/zoekt/cmd/zoekt-git-index@latest
  go install github.com/sourcegraph/zoekt/cmd/zoekt-webserver@latest
  ```

- Ensure the installed binaries are available in your shell `PATH`. A common configuration is:

  ```bash
  export PATH="$PATH:$HOME/go/bin"
  ```

  Verify with `which zoekt-git-index` and `which zoekt-webserver`.

**Build**

The repository includes a simple build helper. From the project root run:

```bash
./build.sh
```

This script builds the components (see `cmd/` subpackages). You can also use plain `go build` within each `cmd/*` directory if you prefer.

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

  Note: `repo-cli`'s `index` action calls the internal `IndexRepositoryZoekt` implementation which:

  - Verifies the repository at the configured `-path` is a Git repository.
  - Updates the repository's `.git/config` with `zoekt.name` and `zoekt.repoid` entries.
  - Calls the `zoekt-git-index` binary (must be in `PATH`) with arguments similar to:

    ```bash
    zoekt-git-index -index <data-dir>/zoekt-index /abs/path/to/my-repo
    ```

    Where `<data-dir>` is the `-data-dir` value (default `./.data`) and the command will write index files into `<data-dir>/zoekt-index`.

- `./repo-server` — starts the HTTP service that serves repository information and search results. See `cmd/server/main.go` for flags and configuration options. Note: at present you must start the Zoekt webserver manually (see below).

  Example:

  ```bash
  ./repo-server --port 8080 --data-dir ".data"
  ```

**Zoekt integration**

This project uses Zoekt for fast repository indexing/search. Two tools are required:

- `zoekt-git-index` — creates Zoekt index files for a Git repository.
- `zoekt-webserver` — serves Zoekt indexes over HTTP for queries.

Ensure the `zoekt-webserver` process is running and reachable by the `repo-server` configuration.

**Start/Stop helper (lazy mode)**

- `start.sh` — convenience script to start the service and any local helpers. Run:

```bash
./start.sh
```

- `stop.sh` — stops services started by `start.sh`:

```bash
./stop.sh
```

Note: `start.sh` automatically starts `zoekt-webserver`.