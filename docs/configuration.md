# Configuration Guide

## Data Directory
- Default: `./.data`
- Structure:
  - `app.db` — SQLite database
  - `repos/<id>/scip/index.scip` — SCIP index per repository
  - `zoekt-index/` — global Zoekt index directory

## Environment & Binaries
- Required binaries in `PATH`: none for search/indexing. Zoekt is embedded in `repo-server` and `repo-cli`.

## Server Options
- Run server: `./repo-server -data-dir .data`
- Port: fixed `:8088` (current build).

## CLI Usage
- Add repo:
  ```bash
  ./repo-cli -command add -id 1 -name "my-repo" -path "/abs/path" -data-dir .data
  ```
- Delete repo:
  ```bash
  ./repo-cli -command delete -id 1 -data-dir .data
  ```
- Index with Zoekt:
  ```bash
  ./repo-cli -command index -id 1 -data-dir .data
  ```
- Register SCIP index:
  ```bash
  ./repo-cli -command register-scip -id 1 -scip-path /path/to/index.scip
  ```

## Notes
- Ensure target repo path is a valid Git repository before indexing.
- Zoekt shards are stored under `<data-dir>/zoekt-index/`.
