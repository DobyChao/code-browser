# Search API Migration Report — Frontend Guide

> Date: 2026-05-12
> PR: [#2 Embed Zoekt search and indexing](https://github.com/DobyChao/code-browser/pull/2)
> Status: **Backward compatible via `format` parameter**

---

## Overview

The search backend has been migrated from ripgrep + external zoekt-webserver to an embedded Zoekt library. API changes are **backward compatible** — the default response format remains the old flat array. Pass `format=v2` to opt into the new paginated format.

---

## 1. Content Search

`GET /api/repositories/{id}/search`

### New Parameters

| Parameter | Status | Description |
|-----------|--------|-------------|
| `q` | required | Search query |
| `format` | **new, optional** | `v1` or omit = flat array (default), `v2` = paginated object |
| `branch` | new, optional | Scope search to a specific branch |
| `file` | new, optional | Filter results by file path pattern |
| `page` | new, optional | Page number (default `1`) — only effective with `format=v2` |
| `page_size` | new, optional | Results per page (default `50`) — only effective with `format=v2` |
| `engine` | **deprecated** | Omit it. `engine=ripgrep` returns `400` |

### Response Format

**Default (no `format` or `format=v1`)** — flat array (unchanged):
```json
[
  {
    "repo_name": "code-browser",
    "path": "internal/search/handler.go",
    "lineNum": 42,
    "lineText": "func (h *Handlers) SearchContent(...)",
    "fragments": [{ "offset": 6, "length": 13 }]
  }
]
```

**`format=v2`** — paginated object:
```json
{
  "results": [
    {
      "repo_name": "code-browser",
      "path": "internal/search/handler.go",
      "lineNum": 42,
      "lineText": "func (h *Handlers) SearchContent(...)",
      "fragments": [{ "offset": 6, "length": 13 }]
    }
  ],
  "total": 128,
  "page": 1,
  "page_size": 50
}
```

---

## 2. File Search

`GET /api/repositories/{id}/search-files`

### New Parameters

| Parameter | Status | Description |
|-----------|--------|-------------|
| `q` | **required** | File name query (was optional, now required) |
| `format` | **new, optional** | `v1` or omit = flat array (default), `v2` = paginated object |
| `branch` | new, optional | Scope search to a specific branch |
| `page` | new, optional | Page number (default `1`) — only effective with `format=v2` |
| `page_size` | new, optional | Results per page (default `50`) — only effective with `format=v2` |
| `engine` | **deprecated** | Omit it. `engine=ripgrep` returns `400` |

### Response Format

**Default (no `format` or `format=v1`)** — flat array (unchanged):
```json
["internal/search/handler.go", "internal/search/service.go"]
```

**`format=v2`** — paginated object:
```json
{
  "files": ["internal/search/handler.go", "internal/search/service.go"],
  "total": 15,
  "page": 1,
  "page_size": 50
}
```

---

## 3. Migration Path for Frontend

### No Immediate Action Required

The default response format is unchanged. Existing frontend code will continue to work without modification.

### Recommended Changes

1. **Remove `engine` query parameter** from API calls — deprecated, will be removed in a future version.

2. **Remove engine selector UI** — there is only one engine now (Zoekt).

3. **When ready, adopt `format=v2`** to get pagination support:
   - Add `format=v2` to API calls
   - Update response parsing: `response` → `response.results` (content), `response.files` (files)
   - Implement pagination UI using `total`, `page`, `page_size` fields

### New Capabilities (available with any format)

4. **Branch filter** — pass `branch=main` to scope searches
5. **File filter** — content search accepts `file=pattern` to filter by file path
