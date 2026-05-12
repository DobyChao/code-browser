<!-- Moved from root: API.md -->
# Backend API Reference

## Basics
- Base URL: `http://localhost:8088`
- Static assets: `GET /` (serves the `web/` directory)
- API prefix: `/api`
- CORS: `*` allowed, methods `GET, POST, OPTIONS`
- Port: `:8088`

## Coordinate & Encoding Conventions
- Line numbers:
  - Search results `lineNum` are 1-based.
  - Jump-to-definition and references return `Location.startLine/endLine` as 1-based.
- Column numbers: follow SCIP semantics (unchanged for now).
- Base fields:
  - Responses include `lineBase` and `columnBase` (`1` and `0` respectively).
- Content type: JSON responses use `application/json`; `GetBlob` returns plain text.

## Repositories
### GET `/api/repositories`
- Description: List all repositories.
- Response: `[{ id: string, name: string }]`

## File Browsing
### GET `/api/repositories/{id}/tree?path=<relativePath>`
- Description: List files and directories under the given path.
- Path params: `id` (uint32, provided as string).
- Query params: `path` (relative path; empty string means repo root).
- Response: `[{ name: string, path: string, type: 'file'|'directory' }]`

### GET `/api/repositories/{id}/blob?path=<relativePath>`
- Description: Return the raw content of a file (text).
- Query params: `path` (required).
- Response: text (default `text/plain; charset=utf-8`).

## Search
### GET `/api/repositories/{id}/search?q=<query>`
- Description: Content search, returning match positions and line fragments.
- Query params: `q` (required), `branch`, `file`, `page`, `page_size`, and `format` are optional. `engine` is deprecated; `engine=ripgrep` returns `400 Bad Request`.
- `format` parameter controls response shape:
  - Omit or `format=v1` (default): returns a flat array (backward compatible).
  - `format=v2`: returns a paginated object with `results`, `total`, `page`, `page_size`, and optionally `truncated`.
- `truncated` field (only in `format=v2`): `true` when total matches exceed the internal limit (10000). Indicates the `total` is a lower bound and results may be incomplete. Advise users to narrow the search query.
- Response (default / `format=v1`):
  ```json
  [
    {
      "repo_name": "string",
      "path": "string",
      "lineNum": 123,
      "lineText": "string",
      "fragments": [{ "offset": 0, "length": 5 }]
    }
  ]
  ```
- Response (`format=v2`):
  ```json
  {
    "results": [
      {
        "repo_name": "string",
        "path": "string",
        "lineNum": 123,
        "lineText": "string",
        "fragments": [{ "offset": 0, "length": 5 }]
      }
    ],
    "total": 1,
    "page": 1,
    "page_size": 50,
    "truncated": false
  }
  ```

### GET `/api/repositories/{id}/search-files?q=<query>`
- Description: File name search, returning matched file paths.
- Query params: `q` (required), `branch`, `page`, `page_size`, and `format` are optional. `engine=ripgrep` returns `400 Bad Request`.
- `format` parameter controls response shape:
  - Omit or `format=v1` (default): returns a flat array (backward compatible).
  - `format=v2`: returns a paginated object with `files`, `total`, `page`, `page_size`.
- Response (default / `format=v1`):
  ```json
  ["path/to/file"]
  ```
- Response (`format=v2`):
  ```json
  {
    "files": ["path/to/file"],
    "total": 1,
    "page": 1,
    "page_size": 50,
    "truncated": false
  }
  ```

## Intelligence (Definitions & References)
### POST `/api/intelligence/definitions`
- Description: Jump to symbol definition; prefers SCIP index and falls back to search.
- Request body:
  ```json
  { "repoId": "string", "filePath": "string", "line": 0, "character": 0 }
  ```
- Response:
  ```json
  [
    {
      "kind": "definition",
      "repoId": "string",
      "filePath": "string",
      "range": {
        "startLine": 1,
        "startColumn": 0,
        "endLine": 1,
        "endColumn": 0,
        "lineBase": 1,
        "columnBase": 0
      },
      "source": "scip" | "search"
    }
  ]
  ```
- Notes:
- SCIP index file location: `<dataDir>/repos/<id>/scip/index.scip`.
- Falls back to content search when no definition is found via SCIP.

Notes:
- Kinds are restricted to `definition` and `reference`. There is no `search-result` kind anymore; when falling back to text/engine search, results are still returned as `kind: "definition"` with `source: "search"`.

### POST `/api/intelligence/references`
- Description: Find symbol references; prefers SCIP index and falls back to text search.
- Request body:
  ```json
  { "repoId": "string", "filePath": "string", "line": 0, "character": 0 }
  ```
- Response:
  ```json
  [
    {
      "kind": "reference",
      "repoId": "string",
      "filePath": "string",
      "range": {
        "startLine": 1,
        "startColumn": 0,
        "endLine": 1,
        "endColumn": 0,
        "lineBase": 1,
        "columnBase": 0
      },
      "source": "scip" | "search"
    }
  ]
  ```

## Errors & Status Codes
- `400`: Parameter validation errors (e.g., invalid repo ID, missing `path`).
- `404`: Repository not found.
- `500`: Internal errors (read failures, index parsing errors, etc.).

## Caching
- File tree and blob caches (keys: `tree:<repo>:<path>`, `blob:<repo>:<path>`).
- Search result caches (prefixes: `search:content:*`, `search:files:*`).
- SCIP index object cache to avoid repeated deserialization.

## Examples
- List repositories: `GET /api/repositories`
- Root tree: `GET /api/repositories/1/tree?path=`
- File content: `GET /api/repositories/1/blob?path=cmd/server/main.go`
- Content search: `GET /api/repositories/1/search?q=sym:Provider`
- File search: `GET /api/repositories/1/search-files?q=main.go`
- Jump to definition: `POST /api/intelligence/definitions` with `{"repoId":"1","filePath":"internal/repo/provider.go","line":182,"character":10}`
- Find references: `POST /api/intelligence/references` with the same body
