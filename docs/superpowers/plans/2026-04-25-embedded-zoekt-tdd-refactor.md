# Embedded Zoekt TDD Refactor Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the current Zoekt HTTP + Ripgrep search stack with embedded Zoekt search and indexing, while adding TDD coverage around the modules touched by the refactor.

**Architecture:** `internal/search` becomes the only Zoekt integration boundary: it owns query construction, result conversion, `search.NewDirectorySearcher`, and `gitindex.IndexGitRepo`. `internal/repo` keeps repository metadata and IndexJob state, and calls an injected index runner instead of shelling out to `zoekt-git-index`. HTTP handlers, analysis fallback, server wiring, and CLI wiring depend on the new `search.Service` interface.

**Tech Stack:** Go 1.25.1, SQLite, go-git, `github.com/sourcegraph/zoekt`, `github.com/sourcegraph/zoekt/gitindex`, `github.com/sourcegraph/zoekt/search`, `github.com/sourcegraph/zoekt/query`, standard `testing`/`httptest`.

---

## File Structure

- Create `internal/search/types.go`: public search DTOs, request structs, service interface, index options.
- Create `internal/search/query.go`: converts API requests and repo IDs into `zoekt/query.Q`.
- Create `internal/search/query_test.go`: RED tests for repo filtering, branch filtering, file filtering, pagination defaults, and invalid requests.
- Create `internal/search/convert.go`: converts `*zoekt.SearchResult` into API response structs.
- Create `internal/search/convert_test.go`: RED tests for line fragments, repo name, empty results, and file searches.
- Create `internal/search/service.go`: embedded Zoekt service backed by `zoekt.Searcher`.
- Create `internal/search/service_test.go`: tests with a fake Zoekt searcher.
- Create `internal/search/indexer.go`: embedded `gitindex.IndexGitRepo` adapter.
- Create `internal/search/indexer_test.go`: integration test with a temporary Git repository and temporary index directory.
- Modify `internal/search/handler.go`: depend on `search.Service`; reject `engine=ripgrep`; remove engine map.
- Create `internal/search/handler_test.go`: handler tests with a fake service.
- Delete `internal/search/engine.go`: remove Zoekt HTTP and Ripgrep implementations after replacement tests are green.
- Modify `internal/repo/provider.go`: remove `os/exec` indexing, add injected `IndexRunner`, keep IndexJob state flow.
- Modify `internal/repo/index_job_test.go`: add async job tests using a fake index runner without sleeps.
- Modify `internal/analysis/service.go`: replace `search.Engine` dependency with a small fallback search interface compatible with `search.Service`.
- Modify `internal/analysis/service_test.go`: add/adjust fallback search tests.
- Modify `cmd/server/main.go`: construct embedded Zoekt service, wire handlers, close service on exit, remove Ripgrep.
- Modify `cmd/cli/main.go`: construct embedded Zoekt service for `index` command or call the new indexer directly.
- Modify `start.sh`, `stop.sh`, `package.sh`, `README.md`, `docs/api.md`, `docs/configuration.md`, `docs/development.md`: remove runtime `zoekt-webserver`, `zoekt-git-index`, and `rg` dependencies where no longer needed.

---

## Task 1: Search Contracts and Query Builder

**Files:**
- Create: `internal/search/types.go`
- Create: `internal/search/query.go`
- Create: `internal/search/query_test.go`

- [ ] **Step 1: Write failing query builder tests**

Create `internal/search/query_test.go`:

```go
package search

import (
	"strings"
	"testing"

	"code-browser/internal/repo"
)

func TestBuildZoektQueryRequiresQuery(t *testing.T) {
	_, err := BuildZoektQuery([]repo.Repository{{RepoID: 1, Name: "repo"}}, SearchRequest{})
	if err == nil {
		t.Fatal("expected empty query to fail")
	}
	if !strings.Contains(err.Error(), "query is required") {
		t.Fatalf("expected query required error, got %v", err)
	}
}

func TestBuildZoektQueryFiltersReposByID(t *testing.T) {
	q, err := BuildZoektQuery([]repo.Repository{{RepoID: 7, Name: "repo"}}, SearchRequest{Query: "Provider"})
	if err != nil {
		t.Fatalf("BuildZoektQuery failed: %v", err)
	}
	got := q.String()
	if !strings.Contains(got, "repoid=") || !strings.Contains(got, "7") {
		t.Fatalf("expected repo id filter in query, got %s", got)
	}
	if !strings.Contains(got, "Provider") {
		t.Fatalf("expected user query in query string, got %s", got)
	}
}

func TestBuildZoektQueryAddsBranchFilter(t *testing.T) {
	q, err := BuildZoektQuery([]repo.Repository{{RepoID: 7, Name: "repo"}}, SearchRequest{Query: "Provider", Branch: "main"})
	if err != nil {
		t.Fatalf("BuildZoektQuery failed: %v", err)
	}
	got := q.String()
	if !strings.Contains(got, "main") || !strings.Contains(got, "7") {
		t.Fatalf("expected branch+repo filter in query, got %s", got)
	}
}

func TestBuildZoektQueryAddsFileFilter(t *testing.T) {
	q, err := BuildZoektQuery([]repo.Repository{{RepoID: 7, Name: "repo"}}, SearchRequest{Query: "Provider", File: "internal/.*\\.go"})
	if err != nil {
		t.Fatalf("BuildZoektQuery failed: %v", err)
	}
	got := q.String()
	if !strings.Contains(got, "file_regex") || !strings.Contains(got, "internal") {
		t.Fatalf("expected file filter in query, got %s", got)
	}
}

func TestNormalizeSearchRequestDefaults(t *testing.T) {
	req := NormalizeSearchRequest(SearchRequest{Query: "Provider"})
	if req.Page != 1 {
		t.Fatalf("expected default page 1, got %d", req.Page)
	}
	if req.PageSize != 50 {
		t.Fatalf("expected default page size 50, got %d", req.PageSize)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
go test ./internal/search -run 'TestBuildZoektQuery|TestNormalizeSearchRequest' -count=1
```

Expected: FAIL with undefined `BuildZoektQuery`, `SearchRequest`, and `NormalizeSearchRequest`.

- [ ] **Step 3: Add minimal types**

Create `internal/search/types.go`:

```go
package search

import (
	"context"

	"code-browser/internal/repo"
)

type SearchFragment struct {
	Offset int `json:"offset"`
	Length int `json:"length"`
}

type SearchResult struct {
	RepoName  string           `json:"repo_name,omitempty"`
	Path      string           `json:"path"`
	LineNum   int              `json:"lineNum"`
	LineText  string           `json:"lineText"`
	Fragments []SearchFragment `json:"fragments"`
}

type SearchRequest struct {
	Query    string
	Branch   string
	File     string
	Page     int
	PageSize int
}

type FileSearchRequest struct {
	Query    string
	Branch   string
	Page     int
	PageSize int
}

type SearchResponse struct {
	Results  []SearchResult `json:"results"`
	Total    int            `json:"total"`
	Page     int            `json:"page"`
	PageSize int            `json:"page_size"`
}

type FileSearchResponse struct {
	Files    []string `json:"files"`
	Total    int      `json:"total"`
	Page     int      `json:"page"`
	PageSize int      `json:"page_size"`
}

type IndexOptions struct {
	IndexDir    string
	Branches    []string
	Incremental bool
	Delta       bool
}

type Service interface {
	SearchContent(ctx context.Context, repos []repo.Repository, req SearchRequest) (*SearchResponse, error)
	SearchFiles(ctx context.Context, repos []repo.Repository, req FileSearchRequest) (*FileSearchResponse, error)
	IndexRepository(ctx context.Context, repository repo.Repository, opts IndexOptions) error
	Health(ctx context.Context) error
	Close() error
}
```

- [ ] **Step 4: Add query builder**

Create `internal/search/query.go`:

```go
package search

import (
	"fmt"
	"regexp/syntax"
	"strings"

	"code-browser/internal/repo"

	zoektquery "github.com/sourcegraph/zoekt/query"
)

func NormalizeSearchRequest(req SearchRequest) SearchRequest {
	if req.Page <= 0 {
		req.Page = 1
	}
	if req.PageSize <= 0 {
		req.PageSize = 50
	}
	if req.PageSize > 500 {
		req.PageSize = 500
	}
	req.Query = strings.TrimSpace(req.Query)
	return req
}

func NormalizeFileSearchRequest(req FileSearchRequest) FileSearchRequest {
	if req.Page <= 0 {
		req.Page = 1
	}
	if req.PageSize <= 0 {
		req.PageSize = 50
	}
	if req.PageSize > 500 {
		req.PageSize = 500
	}
	req.Query = strings.TrimSpace(req.Query)
	return req
}

func BuildZoektQuery(repos []repo.Repository, req SearchRequest) (zoektquery.Q, error) {
	req = NormalizeSearchRequest(req)
	if req.Query == "" {
		return nil, fmt.Errorf("query is required")
	}
	if len(repos) == 0 {
		return nil, fmt.Errorf("at least one repository is required")
	}

	userQuery, err := zoektquery.Parse(req.Query)
	if err != nil {
		return nil, fmt.Errorf("parse zoekt query: %w", err)
	}

	repoIDs := make([]uint32, 0, len(repos))
	for _, r := range repos {
		repoIDs = append(repoIDs, r.RepoID)
	}

	var filters []zoektquery.Q
	if req.Branch != "" {
		filters = append(filters, zoektquery.NewSingleBranchesRepos(req.Branch, repoIDs...))
	} else {
		filters = append(filters, zoektquery.NewRepoIDs(repoIDs...))
	}
	if req.File != "" {
		fileRe, err := syntax.Parse(req.File, syntax.Perl)
		if err != nil {
			return nil, fmt.Errorf("invalid file filter: %w", err)
		}
		filters = append(filters, &zoektquery.Regexp{Regexp: fileRe, FileName: true})
	}

	parts := append([]zoektquery.Q{userQuery}, filters...)
	return zoektquery.NewAnd(parts...), nil
}

func BuildZoektFileQuery(repos []repo.Repository, req FileSearchRequest) (zoektquery.Q, error) {
	req = NormalizeFileSearchRequest(req)
	if req.Query == "" {
		return nil, fmt.Errorf("query is required")
	}
	return BuildZoektQuery(repos, SearchRequest{
		Query:    "file:" + req.Query,
		Branch:   req.Branch,
		Page:     req.Page,
		PageSize: req.PageSize,
	})
}
```

- [ ] **Step 5: Run tests to verify green**

Run:

```bash
go test ./internal/search -run 'TestBuildZoektQuery|TestNormalizeSearchRequest' -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/search/types.go internal/search/query.go internal/search/query_test.go
git commit -m "feat(search): add zoekt query contract"
```

---

## Task 2: Zoekt Result Conversion

**Files:**
- Create: `internal/search/convert.go`
- Create: `internal/search/convert_test.go`

- [ ] **Step 1: Write failing conversion tests**

Create `internal/search/convert_test.go`:

```go
package search

import (
	"testing"

	"github.com/sourcegraph/zoekt"
)

func TestConvertSearchResultPreservesLinesAndFragments(t *testing.T) {
	src := &zoekt.SearchResult{
		Files: []zoekt.FileMatch{{
			FileName:     "internal/repo/provider.go",
			Repository:   "0000000007_repo",
			RepositoryID: 7,
			LineMatches: []zoekt.LineMatch{{
				Line:       []byte("func Provider() {}"),
				LineNumber: 12,
				LineFragments: []zoekt.LineFragmentMatch{{
					LineOffset: 5,
					MatchLength: 8,
				}},
			}},
		}},
	}

	got := ConvertSearchResult(src, SearchRequest{Page: 1, PageSize: 50})
	if got.Total != 1 {
		t.Fatalf("expected total 1, got %d", got.Total)
	}
	if got.Results[0].RepoName != "0000000007_repo" {
		t.Fatalf("expected repo name, got %q", got.Results[0].RepoName)
	}
	if got.Results[0].Path != "internal/repo/provider.go" {
		t.Fatalf("unexpected path %q", got.Results[0].Path)
	}
	if got.Results[0].LineNum != 12 {
		t.Fatalf("unexpected line number %d", got.Results[0].LineNum)
	}
	if got.Results[0].LineText != "func Provider() {}" {
		t.Fatalf("unexpected line text %q", got.Results[0].LineText)
	}
	if got.Results[0].Fragments[0] != (SearchFragment{Offset: 5, Length: 8}) {
		t.Fatalf("unexpected fragment %+v", got.Results[0].Fragments[0])
	}
}

func TestConvertSearchResultHandlesEmpty(t *testing.T) {
	got := ConvertSearchResult(&zoekt.SearchResult{}, SearchRequest{Page: 2, PageSize: 10})
	if got.Total != 0 || len(got.Results) != 0 {
		t.Fatalf("expected empty response, got %+v", got)
	}
	if got.Page != 2 || got.PageSize != 10 {
		t.Fatalf("expected pagination to be preserved, got page=%d size=%d", got.Page, got.PageSize)
	}
}

func TestConvertFileSearchResultReturnsUniqueFiles(t *testing.T) {
	src := &zoekt.SearchResult{
		Files: []zoekt.FileMatch{
			{FileName: "a.go"},
			{FileName: "a.go"},
			{FileName: "b.go"},
		},
	}
	got := ConvertFileSearchResult(src, FileSearchRequest{Page: 1, PageSize: 50})
	if got.Total != 2 {
		t.Fatalf("expected total 2, got %d", got.Total)
	}
	if got.Files[0] != "a.go" || got.Files[1] != "b.go" {
		t.Fatalf("unexpected files %+v", got.Files)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
go test ./internal/search -run 'TestConvert' -count=1
```

Expected: FAIL with undefined `ConvertSearchResult` and `ConvertFileSearchResult`.

- [ ] **Step 3: Implement converters**

Create `internal/search/convert.go`:

```go
package search

import "github.com/sourcegraph/zoekt"

func ConvertSearchResult(src *zoekt.SearchResult, req SearchRequest) *SearchResponse {
	req = NormalizeSearchRequest(req)
	resp := &SearchResponse{
		Page:     req.Page,
		PageSize: req.PageSize,
	}
	if src == nil {
		return resp
	}

	for _, file := range src.Files {
		for _, line := range file.LineMatches {
			fragments := make([]SearchFragment, 0, len(line.LineFragments))
			for _, fragment := range line.LineFragments {
				fragments = append(fragments, SearchFragment{
					Offset: fragment.LineOffset,
					Length: fragment.MatchLength,
				})
			}
			resp.Results = append(resp.Results, SearchResult{
				RepoName:  file.Repository,
				Path:      file.FileName,
				LineNum:   line.LineNumber,
				LineText:  string(line.Line),
				Fragments: fragments,
			})
		}
	}
	resp.Total = len(resp.Results)
	return resp
}

func ConvertFileSearchResult(src *zoekt.SearchResult, req FileSearchRequest) *FileSearchResponse {
	req = NormalizeFileSearchRequest(req)
	resp := &FileSearchResponse{
		Page:     req.Page,
		PageSize: req.PageSize,
	}
	if src == nil {
		return resp
	}

	seen := map[string]struct{}{}
	for _, file := range src.Files {
		if _, ok := seen[file.FileName]; ok {
			continue
		}
		seen[file.FileName] = struct{}{}
		resp.Files = append(resp.Files, file.FileName)
	}
	resp.Total = len(resp.Files)
	return resp
}
```

- [ ] **Step 4: Run tests to verify green**

Run:

```bash
go test ./internal/search -run 'TestConvert' -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/search/convert.go internal/search/convert_test.go
git commit -m "feat(search): convert zoekt results"
```

---

## Task 3: HTTP Handlers Use Search Service

**Files:**
- Modify: `internal/search/handler.go`
- Create: `internal/search/handler_test.go`

- [ ] **Step 1: Write failing handler tests**

Create `internal/search/handler_test.go`:

```go
package search

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"code-browser/internal/repo"

	"github.com/patrickmn/go-cache"
)

type fakeSearchService struct {
	contentReq SearchRequest
	filesReq   FileSearchRequest
}

func (f *fakeSearchService) SearchContent(_ context.Context, repos []repo.Repository, req SearchRequest) (*SearchResponse, error) {
	f.contentReq = req
	return &SearchResponse{
		Results: []SearchResult{{Path: "main.go", LineNum: 1, LineText: "package main"}},
		Total: 1, Page: req.Page, PageSize: req.PageSize,
	}, nil
}

func (f *fakeSearchService) SearchFiles(_ context.Context, repos []repo.Repository, req FileSearchRequest) (*FileSearchResponse, error) {
	f.filesReq = req
	return &FileSearchResponse{Files: []string{"main.go"}, Total: 1, Page: req.Page, PageSize: req.PageSize}, nil
}

func (f *fakeSearchService) IndexRepository(context.Context, repo.Repository, IndexOptions) error { return nil }
func (f *fakeSearchService) Health(context.Context) error { return nil }
func (f *fakeSearchService) Close() error { return nil }

func newSearchHandlerTestProvider(t *testing.T) *repo.Provider {
	t.Helper()
	p, err := repo.NewProvider(t.TempDir())
	if err != nil {
		t.Fatalf("NewProvider failed: %v", err)
	}
	t.Cleanup(func() { _ = p.Close() })
	sourceDir := t.TempDir()
	if err := p.AddRepository(1, "repo", sourceDir); err != nil {
		t.Fatalf("AddRepository failed: %v", err)
	}
	return p
}

func TestSearchContentRejectsRipgrepEngine(t *testing.T) {
	h := &Handlers{
		RepoProvider: newSearchHandlerTestProvider(t),
		Service:      &fakeSearchService{},
		Cache:        cache.New(cache.NoExpiration, cache.NoExpiration),
	}
	req := httptest.NewRequest(http.MethodGet, "/api/repositories/1/search?q=main&engine=ripgrep", nil)
	req.SetPathValue("id", "1")
	rec := httptest.NewRecorder()

	h.SearchContent(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Ripgrep has been removed") {
		t.Fatalf("expected ripgrep removal message, got %s", rec.Body.String())
	}
}

func TestSearchContentUsesServiceAndDefaultsPagination(t *testing.T) {
	svc := &fakeSearchService{}
	h := &Handlers{
		RepoProvider: newSearchHandlerTestProvider(t),
		Service:      svc,
		Cache:        cache.New(cache.NoExpiration, cache.NoExpiration),
	}
	req := httptest.NewRequest(http.MethodGet, "/api/repositories/1/search?q=main", nil)
	req.SetPathValue("id", "1")
	rec := httptest.NewRecorder()

	h.SearchContent(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if svc.contentReq.Page != 1 || svc.contentReq.PageSize != 50 {
		t.Fatalf("expected default pagination, got %+v", svc.contentReq)
	}
	var body SearchResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Results[0].Path != "main.go" {
		t.Fatalf("unexpected body %+v", body)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
go test ./internal/search -run 'TestSearchContent' -count=1
```

Expected: FAIL because `Handlers` has no `Service` field and still uses engine map.

- [ ] **Step 3: Refactor handler**

Modify `internal/search/handler.go` so `Handlers` becomes:

```go
type Handlers struct {
	Service      Service
	RepoProvider *repo.Provider
	Cache        *cache.Cache
}
```

Replace engine lookup in `SearchContent` with:

```go
if r.URL.Query().Get("engine") == "ripgrep" {
	http.Error(w, "Ripgrep has been removed; embedded Zoekt is the only search engine", http.StatusBadRequest)
	return
}

req := NormalizeSearchRequest(SearchRequest{
	Query:    r.URL.Query().Get("q"),
	Branch:   r.URL.Query().Get("branch"),
	File:     r.URL.Query().Get("file"),
	Page:     parsePositiveIntDefault(r.URL.Query().Get("page"), 1),
	PageSize: parsePositiveIntDefault(r.URL.Query().Get("page_size"), 50),
})
if req.Query == "" {
	http.Error(w, "Query parameter 'q' is required", http.StatusBadRequest)
	return
}

cacheKey := fmt.Sprintf("search:content:zoekt:%d:%s:%s:%s:%d:%d", repoID, req.Query, req.Branch, req.File, req.Page, req.PageSize)
if data, found := h.Cache.Get(cacheKey); found {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
	return
}

repoInfo, ok := h.RepoProvider.GetRepo(repoID)
if !ok {
	http.Error(w, fmt.Sprintf("仓库 ID '%d' 未找到", repoID), http.StatusNotFound)
	return
}

results, err := h.Service.SearchContent(r.Context(), []repo.Repository{repoInfo}, req)
if err != nil {
	http.Error(w, fmt.Sprintf("Search failed: %v", err), http.StatusInternalServerError)
	return
}
h.Cache.Set(cacheKey, results, cache.DefaultExpiration)
w.Header().Set("Content-Type", "application/json")
json.NewEncoder(w).Encode(results)
```

Add helper in `handler.go`:

```go
func parsePositiveIntDefault(raw string, fallback int) int {
	if raw == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}
```

Apply the same pattern in `SearchFiles`, using `FileSearchRequest`, `Service.SearchFiles`, cache key `search:files:zoekt:%d:%s:%s:%d:%d`, and response type `FileSearchResponse`.

Remove `getMapKeys` after no callers remain.

- [ ] **Step 4: Run tests to verify green**

Run:

```bash
go test ./internal/search -run 'TestSearchContent|TestSearchFiles' -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/search/handler.go internal/search/handler_test.go
git commit -m "refactor(search): route handlers through service"
```

---

## Task 4: Embedded Zoekt Search Service

**Files:**
- Create: `internal/search/service.go`
- Create: `internal/search/service_test.go`

- [ ] **Step 1: Write failing service tests with fake searcher**

Create `internal/search/service_test.go`:

```go
package search

import (
	"context"
	"testing"

	"code-browser/internal/repo"

	"github.com/sourcegraph/zoekt"
	zoektquery "github.com/sourcegraph/zoekt/query"
)

type fakeZoektSearcher struct {
	lastQuery zoektquery.Q
	result    *zoekt.SearchResult
	closed    bool
}

func (f *fakeZoektSearcher) Search(_ context.Context, q zoektquery.Q, _ *zoekt.SearchOptions) (*zoekt.SearchResult, error) {
	f.lastQuery = q
	return f.result, nil
}

func (f *fakeZoektSearcher) List(context.Context, zoektquery.Q, *zoekt.ListOptions) (*zoekt.RepoList, error) {
	return &zoekt.RepoList{}, nil
}

func (f *fakeZoektSearcher) Close() { f.closed = true }
func (f *fakeZoektSearcher) String() string { return "fake" }

func TestZoektServiceSearchContent(t *testing.T) {
	searcher := &fakeZoektSearcher{
		result: &zoekt.SearchResult{Files: []zoekt.FileMatch{{
			FileName: "main.go",
			LineMatches: []zoekt.LineMatch{{Line: []byte("package main"), LineNumber: 1}},
		}}},
	}
	svc := NewZoektServiceFromSearcher(searcher, IndexOptions{IndexDir: t.TempDir()})

	resp, err := svc.SearchContent(context.Background(), []repo.Repository{{RepoID: 1, Name: "repo"}}, SearchRequest{Query: "main"})
	if err != nil {
		t.Fatalf("SearchContent failed: %v", err)
	}
	if resp.Total != 1 || resp.Results[0].Path != "main.go" {
		t.Fatalf("unexpected response %+v", resp)
	}
	if searcher.lastQuery == nil {
		t.Fatal("expected query to be passed to zoekt")
	}
}

func TestZoektServiceCloseClosesSearcher(t *testing.T) {
	searcher := &fakeZoektSearcher{}
	svc := NewZoektServiceFromSearcher(searcher, IndexOptions{IndexDir: t.TempDir()})
	if err := svc.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
	if !searcher.closed {
		t.Fatal("expected underlying searcher to be closed")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
go test ./internal/search -run 'TestZoektService' -count=1
```

Expected: FAIL with undefined `NewZoektServiceFromSearcher`.

- [ ] **Step 3: Implement service**

Create `internal/search/service.go`:

```go
package search

import (
	"context"
	"fmt"
	"os"

	"code-browser/internal/repo"

	"github.com/sourcegraph/zoekt"
	zoektsearch "github.com/sourcegraph/zoekt/search"
)

type ZoektService struct {
	searcher zoekt.Searcher
	options  IndexOptions
}

func NewZoektService(options IndexOptions) (*ZoektService, error) {
	if options.IndexDir == "" {
		return nil, fmt.Errorf("index dir is required")
	}
	if err := os.MkdirAll(options.IndexDir, 0755); err != nil {
		return nil, fmt.Errorf("create zoekt index dir: %w", err)
	}
	searcher, err := zoektsearch.NewDirectorySearcher(options.IndexDir)
	if err != nil {
		return nil, fmt.Errorf("create zoekt directory searcher: %w", err)
	}
	return &ZoektService{searcher: searcher, options: options}, nil
}

func NewZoektServiceFromSearcher(searcher zoekt.Searcher, options IndexOptions) *ZoektService {
	return &ZoektService{searcher: searcher, options: options}
}

func (s *ZoektService) SearchContent(ctx context.Context, repos []repo.Repository, req SearchRequest) (*SearchResponse, error) {
	if s.searcher == nil {
		return nil, fmt.Errorf("zoekt searcher is not initialized")
	}
	req = NormalizeSearchRequest(req)
	q, err := BuildZoektQuery(repos, req)
	if err != nil {
		return nil, err
	}
	result, err := s.searcher.Search(ctx, q, &zoekt.SearchOptions{
		ShardMaxMatchCount:   req.PageSize,
		MaxMatchDisplayCount: req.PageSize,
		TotalMaxMatchCount:   req.Page * req.PageSize,
	})
	if err != nil {
		return nil, err
	}
	return ConvertSearchResult(result, req), nil
}

func (s *ZoektService) SearchFiles(ctx context.Context, repos []repo.Repository, req FileSearchRequest) (*FileSearchResponse, error) {
	if s.searcher == nil {
		return nil, fmt.Errorf("zoekt searcher is not initialized")
	}
	req = NormalizeFileSearchRequest(req)
	q, err := BuildZoektFileQuery(repos, req)
	if err != nil {
		return nil, err
	}
	result, err := s.searcher.Search(ctx, q, &zoekt.SearchOptions{
		ShardMaxMatchCount:   req.PageSize,
		MaxMatchDisplayCount: req.PageSize,
		TotalMaxMatchCount:   req.Page * req.PageSize,
	})
	if err != nil {
		return nil, err
	}
	return ConvertFileSearchResult(result, req), nil
}

func (s *ZoektService) Health(ctx context.Context) error {
	if s.searcher == nil {
		return fmt.Errorf("zoekt searcher is not initialized")
	}
	_, err := s.searcher.List(ctx, nil, &zoekt.ListOptions{})
	return err
}

func (s *ZoektService) Close() error {
	if s.searcher != nil {
		s.searcher.Close()
	}
	return nil
}
```

- [ ] **Step 4: Run tests to verify green**

Run:

```bash
go test ./internal/search -run 'TestZoektService' -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/search/service.go internal/search/service_test.go
git commit -m "feat(search): add embedded zoekt service"
```

---

## Task 5: Embedded Zoekt Indexer

**Files:**
- Create: `internal/search/indexer.go`
- Create: `internal/search/indexer_test.go`

- [ ] **Step 1: Write failing indexer test**

Create `internal/search/indexer_test.go`:

```go
package search

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"code-browser/internal/repo"
)

func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary is required for zoekt index integration test")
	}
}

func createTempGitRepo(t *testing.T) string {
	t.Helper()
	requireGit(t)
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, out)
		}
	}
	run("init")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "Test User")
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc main() {}\n"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	run("add", "main.go")
	run("commit", "-m", "initial")
	return dir
}

func TestZoektServiceIndexRepositoryCreatesShard(t *testing.T) {
	repoDir := createTempGitRepo(t)
	indexDir := t.TempDir()
	svc := NewZoektServiceFromSearcher(&fakeZoektSearcher{}, IndexOptions{IndexDir: indexDir, Branches: []string{"HEAD"}, Incremental: true})

	err := svc.IndexRepository(context.Background(), repo.Repository{
		RepoID:     42,
		Name:       "test/repo",
		SourcePath: repoDir,
	}, IndexOptions{IndexDir: indexDir, Branches: []string{"HEAD"}, Incremental: true})
	if err != nil {
		t.Fatalf("IndexRepository failed: %v", err)
	}

	matches, err := filepath.Glob(filepath.Join(indexDir, "*.zoekt"))
	if err != nil {
		t.Fatalf("glob shards: %v", err)
	}
	if len(matches) == 0 {
		t.Fatalf("expected at least one zoekt shard in %s", indexDir)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:

```bash
go test ./internal/search -run TestZoektServiceIndexRepositoryCreatesShard -count=1
```

Expected: FAIL with missing `IndexRepository` implementation.

- [ ] **Step 3: Implement indexer**

Create `internal/search/indexer.go`:

```go
package search

import (
	"context"
	"fmt"
	"os"
	"regexp"

	"code-browser/internal/repo"

	"github.com/sourcegraph/zoekt"
	"github.com/sourcegraph/zoekt/gitindex"
	"github.com/sourcegraph/zoekt/index"
)

func (s *ZoektService) IndexRepository(ctx context.Context, repository repo.Repository, opts IndexOptions) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if opts.IndexDir == "" {
		opts.IndexDir = s.options.IndexDir
	}
	if len(opts.Branches) == 0 {
		opts.Branches = s.options.Branches
	}
	if len(opts.Branches) == 0 {
		opts.Branches = []string{"HEAD"}
	}
	if err := os.MkdirAll(opts.IndexDir, 0755); err != nil {
		return fmt.Errorf("create zoekt index dir: %w", err)
	}

	updated, err := gitindex.IndexGitRepo(gitindex.Options{
		RepoDir:            repository.SourcePath,
		Incremental:        opts.Incremental,
		AllowMissingBranch: true,
		BranchPrefix:       "refs/heads/",
		Branches:           opts.Branches,
		BuildOptions: index.Options{
			IndexDir: opts.IndexDir,
			IsDelta:  opts.Delta,
			RepositoryDescription: zoekt.Repository{
				ID:     repository.RepoID,
				Name:   zoektRepositoryName(repository),
				Source: repository.SourcePath,
			},
		},
	})
	if err != nil {
		return fmt.Errorf("index repository %d: %w", repository.RepoID, err)
	}
	_ = updated
	return nil
}

func zoektRepositoryName(repository repo.Repository) string {
	re := regexp.MustCompile("[^a-zA-Z0-9]+")
	sanitizedName := re.ReplaceAllString(repository.Name, "_")
	return fmt.Sprintf("%010d_%s", repository.RepoID, sanitizedName)
}
```

- [ ] **Step 4: Run test to verify green**

Run:

```bash
go test ./internal/search -run TestZoektServiceIndexRepositoryCreatesShard -count=1
```

Expected: PASS.

- [ ] **Step 5: Run search package tests**

Run:

```bash
go test ./internal/search -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/search/indexer.go internal/search/indexer_test.go
git commit -m "feat(search): index repositories with zoekt library"
```

---

## Task 6: Repo IndexJob Uses Injected Index Runner

**Files:**
- Modify: `internal/repo/provider.go`
- Modify: `internal/repo/index_job_test.go`

- [ ] **Step 1: Write failing async job tests**

Append to `internal/repo/index_job_test.go`:

```go
type fakeIndexRunner struct {
	err    error
	called chan Repository
}

func (f *fakeIndexRunner) IndexRepository(ctx context.Context, repository Repository) error {
	f.called <- repository
	return f.err
}

func TestRunIndexJobAsyncUsesInjectedRunner(t *testing.T) {
	p := setupTestProvider(t)
	addTestRepo(t, p, 1, "test-repo")
	runner := &fakeIndexRunner{called: make(chan Repository, 1)}
	p.SetIndexRunner(runner)

	jobID, err := p.RunIndexJobAsync(1, "zoekt", "manual")
	if err != nil {
		t.Fatalf("RunIndexJobAsync failed: %v", err)
	}

	select {
	case got := <-runner.called:
		if got.RepoID != 1 {
			t.Fatalf("expected repo 1, got %d", got.RepoID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("index runner was not called")
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		job, err := p.GetIndexJob(jobID)
		if err != nil {
			t.Fatalf("GetIndexJob failed: %v", err)
		}
		if job.Status == JobStatusCompleted {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("job did not complete")
}

func TestRunIndexJobAsyncRecordsRunnerFailure(t *testing.T) {
	p := setupTestProvider(t)
	addTestRepo(t, p, 1, "test-repo")
	runner := &fakeIndexRunner{err: errors.New("index failed"), called: make(chan Repository, 1)}
	p.SetIndexRunner(runner)

	jobID, err := p.RunIndexJobAsync(1, "zoekt", "manual")
	if err != nil {
		t.Fatalf("RunIndexJobAsync failed: %v", err)
	}
	<-runner.called

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		job, err := p.GetIndexJob(jobID)
		if err != nil {
			t.Fatalf("GetIndexJob failed: %v", err)
		}
		if job.Status == JobStatusFailed {
			if job.Error != "index failed" {
				t.Fatalf("expected error to be recorded, got %q", job.Error)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("job did not fail")
}
```

Also add imports to `internal/repo/index_job_test.go`:

```go
import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)
```

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
go test ./internal/repo -run 'TestRunIndexJobAsync' -count=1
```

Expected: FAIL with undefined `SetIndexRunner`.

- [ ] **Step 3: Add injected runner to provider**

Modify `internal/repo/provider.go`:

```go
type IndexRunner interface {
	IndexRepository(ctx context.Context, repository Repository) error
}

type IndexRunnerFunc func(ctx context.Context, repository Repository) error

func (f IndexRunnerFunc) IndexRepository(ctx context.Context, repository Repository) error {
	return f(ctx, repository)
}

type Provider struct {
	db           *sql.DB
	DataDir      string
	repositories []Repository
	repoMap      map[uint32]Repository
	mu           sync.RWMutex
	indexRunner  IndexRunner
}

func (p *Provider) SetIndexRunner(runner IndexRunner) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.indexRunner = runner
}

func (p *Provider) getIndexRunner() IndexRunner {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.indexRunner
}
```

Add `context` to imports and remove `os/exec` after `IndexRepositoryZoekt` is removed.

- [ ] **Step 4: Replace async indexing path**

Modify `RunIndexJobAsync`:

```go
func (p *Provider) RunIndexJobAsync(repoID uint32, jobType, triggerType string) (uint32, error) {
	repoInfo, ok := p.GetRepo(repoID)
	if !ok {
		return 0, fmt.Errorf("仓库 ID '%d' 未找到", repoID)
	}
	if jobType != "zoekt" {
		return 0, fmt.Errorf("未知的索引类型: %s", jobType)
	}
	runner := p.getIndexRunner()
	if runner == nil {
		return 0, fmt.Errorf("索引服务未初始化")
	}

	jobID, err := p.CreateIndexJob(repoID, jobType, triggerType)
	if err != nil {
		return 0, err
	}
	go func() {
		_ = p.UpdateJobStatus(jobID, JobStatusRunning, "")
		if err := runner.IndexRepository(context.Background(), repoInfo); err != nil {
			_ = p.UpdateJobStatus(jobID, JobStatusFailed, err.Error())
			return
		}
		_ = p.UpdateJobStatus(jobID, JobStatusCompleted, "")
	}()
	return jobID, nil
}
```

Delete `IndexRepositoryZoekt` from `provider.go` after search indexer is wired into CLI/server. If the CLI still needs a transition point during this task, keep a temporary deprecated method:

```go
func (p *Provider) IndexRepositoryZoekt(id uint32) error {
	repoInfo, ok := p.GetRepo(id)
	if !ok {
		return fmt.Errorf("仓库 ID '%d' 未找到", id)
	}
	runner := p.getIndexRunner()
	if runner == nil {
		return fmt.Errorf("索引服务未初始化")
	}
	return runner.IndexRepository(context.Background(), repoInfo)
}
```

- [ ] **Step 5: Run tests to verify green**

Run:

```bash
go test ./internal/repo -run 'TestRunIndexJobAsync|TestCreateIndexJob|TestUpdateJobStatus|TestListIndexJobs' -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/repo/provider.go internal/repo/index_job_test.go
git commit -m "refactor(repo): inject zoekt index runner"
```

---

## Task 7: Analysis Fallback Uses Search Service

**Files:**
- Modify: `internal/analysis/service.go`
- Modify: `internal/analysis/service_test.go`

- [ ] **Step 1: Write or update failing analysis fallback test**

Add a fake search service to `internal/analysis/service_test.go`:

```go
type fakeFallbackSearch struct {
	req search.SearchRequest
}

func (f *fakeFallbackSearch) SearchContent(ctx context.Context, repos []repo.Repository, req search.SearchRequest) (*search.SearchResponse, error) {
	f.req = req
	return &search.SearchResponse{
		Results: []search.SearchResult{{Path: "main.go", LineNum: 3, LineText: "func Target() {}"}},
		Total: 1, Page: 1, PageSize: 50,
	}, nil
}
```

Add a test that creates a temp Git repo with `main.go`, registers it in `repo.Provider`, constructs `core.Service`, constructs `analysis.Service` with `fakeFallbackSearch`, calls `GetDefinition`, and asserts:

```go
if fake.req.Query != "sym:Target" {
	t.Fatalf("expected sym query, got %q", fake.req.Query)
}
if got[0].Source != "search" {
	t.Fatalf("expected search fallback source, got %q", got[0].Source)
}
```

- [ ] **Step 2: Run analysis test to verify it fails**

Run:

```bash
go test ./internal/analysis -run TestGetDefinitionFallsBackToSearchService -count=1
```

Expected: FAIL because `analysis.NewService` still accepts `search.Engine`.

- [ ] **Step 3: Refactor analysis service dependency**

Modify `internal/analysis/service.go`:

```go
type SearchFallback interface {
	SearchContent(ctx context.Context, repos []repo.Repository, req search.SearchRequest) (*search.SearchResponse, error)
}

type Service struct {
	RepoProvider *repo.Provider
	Search       SearchFallback
	CoreService  *core.Service
	ScipCache    *cache.Cache
}

func NewService(repoProvider *repo.Provider, searchService SearchFallback, coreService *core.Service) *Service {
	scipCache := cache.New(cache.NoExpiration, cache.NoExpiration)
	return &Service{
		RepoProvider: repoProvider,
		Search:       searchService,
		CoreService:  coreService,
		ScipCache:    scipCache,
	}
}
```

Replace fallback search logic:

```go
query := fmt.Sprintf("sym:%s", symbol)
searchResp, err := s.Search.SearchContent(context.Background(), []repo.Repository{repoInfo}, search.SearchRequest{
	Query:    query,
	Page:     1,
	PageSize: 50,
})
if err != nil || len(searchResp.Results) == 0 {
	query = fmt.Sprintf("\\b%s\\b", symbol)
	searchResp, err = s.Search.SearchContent(context.Background(), []repo.Repository{repoInfo}, search.SearchRequest{
		Query:    query,
		Page:     1,
		PageSize: 50,
	})
}
```

Then iterate over `searchResp.Results`.

- [ ] **Step 4: Run tests to verify green**

Run:

```bash
go test ./internal/analysis -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/analysis/service.go internal/analysis/service_test.go
git commit -m "refactor(analysis): use search service fallback"
```

---

## Task 8: Server, CLI, Scripts, and Documentation Wiring

**Files:**
- Modify: `cmd/server/main.go`
- Modify: `cmd/cli/main.go`
- Modify: `start.sh`
- Modify: `stop.sh`
- Modify: `package.sh`
- Modify: `README.md`
- Modify: `docs/api.md`
- Modify: `docs/configuration.md`
- Modify: `docs/development.md`

- [ ] **Step 1: Write compile-level expectation**

Run before edits:

```bash
go test ./...
```

Expected: FAIL after previous tasks until wiring is updated, with references to removed `search.ZoektEngine`, `search.RipgrepEngine`, or `search.Engine`.

- [ ] **Step 2: Wire server to embedded Zoekt**

Modify `cmd/server/main.go`:

```go
zoektIndexDir := filepath.Join(*dataDir, "zoekt-index")
zoektService, err := search.NewZoektService(search.IndexOptions{
	IndexDir:    zoektIndexDir,
	Branches:    []string{"HEAD"},
	Incremental: true,
})
if err != nil {
	log.Fatalf("错误: 无法初始化 Zoekt 服务: %v", err)
}
defer func() {
	if err := zoektService.Close(); err != nil {
		log.Printf("关闭 Zoekt 服务时出错: %v", err)
	}
}()
repoProvider.SetIndexRunner(repo.IndexRunnerFunc(func(ctx context.Context, repository repo.Repository) error {
	return zoektService.IndexRepository(ctx, repository, search.IndexOptions{
		IndexDir:    zoektIndexDir,
		Branches:    []string{"HEAD"},
		Incremental: true,
	})
}))

searchHandlers := &search.Handlers{
	RepoProvider: repoProvider,
	Service:      zoektService,
	Cache:        appCache,
}
analysisService := analysis.NewService(repoProvider, zoektService, coreService)
```

Add `context` and `path/filepath` imports. Remove `zoektEngine`, `ripgrepEngine`, and `Engines` map.

- [ ] **Step 3: Wire CLI index command**

Modify `cmd/cli/main.go` after provider initialization:

```go
zoektService, err := search.NewZoektService(search.IndexOptions{
	IndexDir:    filepath.Join(*dataDir, "zoekt-index"),
	Branches:    []string{"HEAD"},
	Incremental: true,
})
if err != nil {
	log.Fatalf("错误: 无法初始化 Zoekt 服务: %v", err)
}
defer zoektService.Close()
repoProvider.SetIndexRunner(repo.IndexRunnerFunc(func(ctx context.Context, repository repo.Repository) error {
	return zoektService.IndexRepository(ctx, repository, search.IndexOptions{
		IndexDir:    filepath.Join(*dataDir, "zoekt-index"),
		Branches:    []string{"HEAD"},
		Incremental: true,
	})
}))
```

Import `context` and `code-browser/internal/search`. Keep `case "index"` using `repoProvider.IndexRepositoryZoekt` only if the temporary compatibility method remains; otherwise call:

```go
repoInfo, ok := repoProvider.GetRepo(uint32(*repoID))
if !ok {
	log.Fatalf("仓库 %d 未找到", *repoID)
}
if err := zoektService.IndexRepository(context.Background(), repoInfo, search.IndexOptions{
	IndexDir:    filepath.Join(*dataDir, "zoekt-index"),
	Branches:    []string{"HEAD"},
	Incremental: true,
}); err != nil {
	log.Fatalf("错误: 索引仓库失败: %v", err)
}
```

Add `context` import if calling the service directly.

- [ ] **Step 4: Remove obsolete search engine file**

Delete `internal/search/engine.go` after all references are gone:

```bash
git rm internal/search/engine.go
```

Expected: no remaining references to `RipgrepEngine`, `ZoektEngine`, or `search.Engine`.

- [ ] **Step 5: Update scripts**

Modify `start.sh` to remove `zoekt-webserver` startup and only start `repo-server`.

Modify `stop.sh` to keep repo-server stop. If old Zoekt cleanup remains for compatibility, label it:

```bash
# Compatibility cleanup for older releases that started zoekt-webserver.
pkill -f "zoekt-webserver.*\\.data/zoekt-index" 2>/dev/null && echo "   killed old zoekt-webserver" || true
```

Modify `package.sh` so runtime dependencies no longer include `zoekt-webserver`, `zoekt-git-index`, or `rg`. Keep `universal-ctags` only if symbol indexing is required by the package.

- [ ] **Step 6: Update docs**

Update docs with these concrete statements:

```markdown
Code Browser embeds Zoekt directly in `repo-server`. You do not need to start `zoekt-webserver`, and indexing no longer shells out to `zoekt-git-index`.
```

In `docs/api.md`, change:

```markdown
GET /api/repositories/{id}/search?q=<query>
```

and document:

```markdown
The `engine` query parameter is deprecated. `engine=ripgrep` returns `400 Bad Request`.
```

- [ ] **Step 7: Run full verification**

Run:

```bash
go test ./...
```

Expected: PASS.

Run:

```bash
rg -n "RipgrepEngine|ZoektEngine|search.Engine|zoekt-webserver|zoekt-git-index|engine=<zoekt\\|ripgrep>" internal cmd README.md docs start.sh stop.sh package.sh
```

Expected: no matches except documentation that explicitly says old `engine=ripgrep` returns 400 or compatibility cleanup for old `zoekt-webserver` processes.

- [ ] **Step 8: Commit**

```bash
git add cmd/server/main.go cmd/cli/main.go start.sh stop.sh package.sh README.md docs/api.md docs/configuration.md docs/development.md internal/search internal/repo internal/analysis
git commit -m "refactor(search): embed zoekt and remove ripgrep"
```

---

## Final Verification

- [ ] Run all tests:

```bash
go test ./...
```

Expected: PASS.

- [ ] Build binaries:

```bash
./build.sh
```

Expected: `repo-server` and `repo-cli` build successfully.

- [ ] Manual smoke test with an existing registered repository:

```bash
./repo-server --data-dir .data
```

In another shell:

```bash
curl -s "http://localhost:8088/api/repositories"
curl -s "http://localhost:8088/api/repositories/1/search?q=package"
curl -i "http://localhost:8088/api/repositories/1/search?q=package&engine=ripgrep"
```

Expected:
- Repository list returns JSON.
- Search returns a `SearchResponse` JSON object.
- `engine=ripgrep` returns `400 Bad Request`.

---

## Self-Review

**Spec coverage:** This plan covers F0 through characterization/unit tests, F6 through Zoekt-only handlers and Ripgrep removal, and F7 through embedded `search.NewDirectorySearcher` plus `gitindex.IndexGitRepo`. F8, F1, Repo Group, scheduler, Git history, Code Map, and Annotations remain separate follow-up batches as specified.

**Placeholder scan:** No implementation step relies on TBD/TODO placeholders. The plan names concrete files, functions, tests, commands, and expected outcomes.

**Type consistency:** `search.Service`, `SearchRequest`, `FileSearchRequest`, `SearchResponse`, `FileSearchResponse`, and `IndexOptions` are introduced before later tasks use them. `repo.IndexRunner` depends only on `repo.Repository`, avoiding an import cycle from `repo` to `search`.
