package search

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"code-browser/internal/repo"

	"github.com/patrickmn/go-cache"
)

type recordingSearchService struct {
	contentCalls int
	contentReq   SearchRequest
	contentRepos []repo.Repository
	contentResp  *SearchResponse
	contentErr   error

	fileCalls int
	fileReq   FileSearchRequest
	fileRepos []repo.Repository
	fileResp  *FileSearchResponse
	fileErr   error
}

func (s *recordingSearchService) SearchContent(ctx context.Context, repos []repo.Repository, req SearchRequest) (*SearchResponse, error) {
	s.contentCalls++
	s.contentReq = req
	s.contentRepos = append([]repo.Repository(nil), repos...)
	if s.contentErr != nil {
		return nil, s.contentErr
	}
	return s.contentResp, nil
}

func (s *recordingSearchService) SearchFiles(ctx context.Context, repos []repo.Repository, req FileSearchRequest) (*FileSearchResponse, error) {
	s.fileCalls++
	s.fileReq = req
	s.fileRepos = append([]repo.Repository(nil), repos...)
	if s.fileErr != nil {
		return nil, s.fileErr
	}
	return s.fileResp, nil
}

func (s *recordingSearchService) IndexRepository(ctx context.Context, repository repo.Repository, opts IndexOptions) error {
	return nil
}

func (s *recordingSearchService) Health(ctx context.Context) error {
	return nil
}

func (s *recordingSearchService) Close() error {
	return nil
}

func TestSearchContentUsesServiceRequestAndCache(t *testing.T) {
	provider := newTestRepoProvider(t)
	service := &recordingSearchService{
		contentResp: &SearchResponse{
			Results: []SearchResult{{Path: "internal/search/handler.go", LineNum: 12, LineText: "needle"}},
			Total:   1,
		},
	}
	handlers := &Handlers{
		Service:      service,
		RepoProvider: provider,
		Cache:        cache.New(time.Minute, time.Minute),
	}

	first := httptest.NewRecorder()
	handlers.SearchContent(first, searchRequest("/api/repositories/7/search?q=needle&branch=main&file=internal/.*\\.go&page=2&page_size=25", "7"))

	if first.Code != http.StatusOK {
		t.Fatalf("first response status = %d, body = %s", first.Code, first.Body.String())
	}
	if service.contentCalls != 1 {
		t.Fatalf("content service calls = %d, want 1", service.contentCalls)
	}
	if got := service.contentReq; got != (SearchRequest{Query: "needle", Branch: "main", File: `internal/.*\.go`, Page: 2, PageSize: 25}) {
		t.Fatalf("content request = %#v", got)
	}
	if len(service.contentRepos) != 1 || service.contentRepos[0].RepoID != 7 {
		t.Fatalf("content repos = %#v", service.contentRepos)
	}

	var firstResp SearchResponse
	if err := json.NewDecoder(first.Body).Decode(&firstResp); err != nil {
		t.Fatalf("decode first response: %v", err)
	}
	if firstResp.Total != 1 || len(firstResp.Results) != 1 {
		t.Fatalf("first response = %#v", firstResp)
	}

	second := httptest.NewRecorder()
	handlers.SearchContent(second, searchRequest("/api/repositories/7/search?q=needle&branch=main&file=internal/.*\\.go&page=2&page_size=25", "7"))

	if second.Code != http.StatusOK {
		t.Fatalf("second response status = %d, body = %s", second.Code, second.Body.String())
	}
	if service.contentCalls != 1 {
		t.Fatalf("content service calls after cached request = %d, want 1", service.contentCalls)
	}

	third := httptest.NewRecorder()
	handlers.SearchContent(third, searchRequest("/api/repositories/7/search?q=needle&branch=dev&file=internal/.*\\.go&page=2&page_size=25", "7"))

	if third.Code != http.StatusOK {
		t.Fatalf("third response status = %d, body = %s", third.Code, third.Body.String())
	}
	if service.contentCalls != 2 {
		t.Fatalf("content service calls after different branch = %d, want 2", service.contentCalls)
	}
}

func TestSearchContentRejectsRipgrepEngine(t *testing.T) {
	handlers := &Handlers{
		Service:      &recordingSearchService{contentResp: &SearchResponse{}},
		RepoProvider: newTestRepoProvider(t),
		Cache:        cache.New(time.Minute, time.Minute),
	}

	w := httptest.NewRecorder()
	handlers.SearchContent(w, searchRequest("/api/repositories/7/search?q=needle&engine=ripgrep", "7"))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	if !strings.Contains(w.Body.String(), "engine=ripgrep has been removed") {
		t.Fatalf("body = %q", w.Body.String())
	}
}

func TestSearchFilesUsesServiceRequestAndCache(t *testing.T) {
	provider := newTestRepoProvider(t)
	service := &recordingSearchService{
		fileResp: &FileSearchResponse{
			Files: []string{"internal/search/handler.go"},
			Total: 1,
		},
	}
	handlers := &Handlers{
		Service:      service,
		RepoProvider: provider,
		Cache:        cache.New(time.Minute, time.Minute),
	}

	first := httptest.NewRecorder()
	handlers.SearchFiles(first, searchRequest("/api/repositories/7/search-files?q=handler&branch=main&page=3&page_size=10", "7"))

	if first.Code != http.StatusOK {
		t.Fatalf("first response status = %d, body = %s", first.Code, first.Body.String())
	}
	if service.fileCalls != 1 {
		t.Fatalf("file service calls = %d, want 1", service.fileCalls)
	}
	if got := service.fileReq; got != (FileSearchRequest{Query: "handler", Branch: "main", Page: 3, PageSize: 10}) {
		t.Fatalf("file request = %#v", got)
	}
	if len(service.fileRepos) != 1 || service.fileRepos[0].RepoID != 7 {
		t.Fatalf("file repos = %#v", service.fileRepos)
	}

	var firstResp FileSearchResponse
	if err := json.NewDecoder(first.Body).Decode(&firstResp); err != nil {
		t.Fatalf("decode first response: %v", err)
	}
	if firstResp.Total != 1 || len(firstResp.Files) != 1 {
		t.Fatalf("first response = %#v", firstResp)
	}

	second := httptest.NewRecorder()
	handlers.SearchFiles(second, searchRequest("/api/repositories/7/search-files?q=handler&branch=main&page=3&page_size=10", "7"))

	if second.Code != http.StatusOK {
		t.Fatalf("second response status = %d, body = %s", second.Code, second.Body.String())
	}
	if service.fileCalls != 1 {
		t.Fatalf("file service calls after cached request = %d, want 1", service.fileCalls)
	}

	third := httptest.NewRecorder()
	handlers.SearchFiles(third, searchRequest("/api/repositories/7/search-files?q=handler&branch=main&page=4&page_size=10", "7"))

	if third.Code != http.StatusOK {
		t.Fatalf("third response status = %d, body = %s", third.Code, third.Body.String())
	}
	if service.fileCalls != 2 {
		t.Fatalf("file service calls after different page = %d, want 2", service.fileCalls)
	}
}

func TestSearchFilesRequiresQueryAndRejectsRipgrepEngine(t *testing.T) {
	handlers := &Handlers{
		Service:      &recordingSearchService{fileResp: &FileSearchResponse{}},
		RepoProvider: newTestRepoProvider(t),
		Cache:        cache.New(time.Minute, time.Minute),
	}

	missingQuery := httptest.NewRecorder()
	handlers.SearchFiles(missingQuery, searchRequest("/api/repositories/7/search-files", "7"))
	if missingQuery.Code != http.StatusBadRequest {
		t.Fatalf("missing query status = %d, want %d", missingQuery.Code, http.StatusBadRequest)
	}

	ripgrep := httptest.NewRecorder()
	handlers.SearchFiles(ripgrep, searchRequest("/api/repositories/7/search-files?q=handler&engine=ripgrep", "7"))
	if ripgrep.Code != http.StatusBadRequest {
		t.Fatalf("ripgrep status = %d, want %d", ripgrep.Code, http.StatusBadRequest)
	}
	if !strings.Contains(ripgrep.Body.String(), "engine=ripgrep has been removed") {
		t.Fatalf("body = %q", ripgrep.Body.String())
	}
}

func TestSearchContentReturnsServiceErrors(t *testing.T) {
	handlers := &Handlers{
		Service:      &recordingSearchService{contentErr: errors.New("boom")},
		RepoProvider: newTestRepoProvider(t),
		Cache:        cache.New(time.Minute, time.Minute),
	}

	w := httptest.NewRecorder()
	handlers.SearchContent(w, searchRequest("/api/repositories/7/search?q=needle", "7"))

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

func searchRequest(target string, repoID string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, target, nil)
	req.SetPathValue("id", repoID)
	return req
}

func newTestRepoProvider(t *testing.T) *repo.Provider {
	t.Helper()

	dataDir := t.TempDir()
	sourceDir := filepath.Join(t.TempDir(), "source")
	if err := os.Mkdir(sourceDir, 0755); err != nil {
		t.Fatalf("create source dir: %v", err)
	}

	provider, err := repo.NewProvider(dataDir)
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}
	t.Cleanup(func() {
		if err := provider.Close(); err != nil {
			t.Errorf("close provider: %v", err)
		}
	})

	if err := provider.AddRepository(7, "test-repo", sourceDir); err != nil {
		t.Fatalf("add repository: %v", err)
	}
	return provider
}
