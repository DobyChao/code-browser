package search

import (
	"context"
	"errors"
	"strings"
	"testing"

	"code-browser/internal/repo"

	"github.com/sourcegraph/zoekt"
	zoektquery "github.com/sourcegraph/zoekt/query"
)

type fakeZoektSearcher struct {
	searchResult *zoekt.SearchResult
	searchErr    error
	listErr      error

	lastQuery   zoektquery.Q
	lastOptions *zoekt.SearchOptions
	listCalled  bool
	closed      bool
}

func (f *fakeZoektSearcher) Search(_ context.Context, q zoektquery.Q, opts *zoekt.SearchOptions) (*zoekt.SearchResult, error) {
	f.lastQuery = q
	f.lastOptions = opts
	return f.searchResult, f.searchErr
}

func (f *fakeZoektSearcher) List(context.Context, zoektquery.Q, *zoekt.ListOptions) (*zoekt.RepoList, error) {
	f.listCalled = true
	return &zoekt.RepoList{}, f.listErr
}

func (f *fakeZoektSearcher) Close() {
	f.closed = true
}

func (f *fakeZoektSearcher) String() string {
	return "fake"
}

func TestZoektServiceSearchContentBuildsQueryAndConvertsResults(t *testing.T) {
	searcher := &fakeZoektSearcher{
		searchResult: &zoekt.SearchResult{Files: []zoekt.FileMatch{{
			FileName:   "internal/main.go",
			Repository: "repo",
			LineMatches: []zoekt.LineMatch{{
				Line:       []byte("func main() {}"),
				LineNumber: 3,
				LineFragments: []zoekt.LineFragmentMatch{{
					LineOffset:  5,
					MatchLength: 4,
				}},
			}},
		}}},
	}
	svc := NewZoektServiceFromSearcher(searcher, IndexOptions{IndexDir: t.TempDir()})

	resp, err := svc.SearchContent(context.Background(), []repo.Repository{{RepoID: 7, Name: "repo"}}, SearchRequest{
		Query:    "main",
		File:     `internal/.*\.go`,
		Page:     1,
		PageSize: 10,
	})
	if err != nil {
		t.Fatalf("SearchContent failed: %v", err)
	}

	if searcher.lastQuery == nil {
		t.Fatal("expected query to be passed to zoekt")
	}
	if got := searcher.lastQuery.String(); !strings.Contains(got, "main") || !strings.Contains(got, "repoid=") || !strings.Contains(got, "file_regex") {
		t.Fatalf("expected query to include user text, repo, and file filters, got %s", got)
	}
	if resp.Total != 1 || len(resp.Results) != 1 {
		t.Fatalf("expected one result, got %+v", resp)
	}
	got := resp.Results[0]
	if got.Path != "internal/main.go" || got.LineNum != 3 || got.LineText != "func main() {}" {
		t.Fatalf("unexpected converted result %+v", got)
	}
	if len(got.Fragments) != 1 || got.Fragments[0] != (SearchFragment{Offset: 5, Length: 4}) {
		t.Fatalf("unexpected fragments %+v", got.Fragments)
	}
}

func TestZoektServiceSearchFilesBuildsFileQueryAndConvertsResults(t *testing.T) {
	searcher := &fakeZoektSearcher{
		searchResult: &zoekt.SearchResult{Files: []zoekt.FileMatch{
			{FileName: "cmd/server/main.go"},
			{FileName: "cmd/server/main.go"},
			{FileName: "internal/search/service.go"},
		}},
	}
	svc := NewZoektServiceFromSearcher(searcher, IndexOptions{IndexDir: t.TempDir()})

	resp, err := svc.SearchFiles(context.Background(), []repo.Repository{{RepoID: 7, Name: "repo"}}, FileSearchRequest{
		Query:    "server",
		Branch:   "main",
		Page:     1,
		PageSize: 5,
	})
	if err != nil {
		t.Fatalf("SearchFiles failed: %v", err)
	}

	if searcher.lastQuery == nil {
		t.Fatal("expected query to be passed to zoekt")
	}
	if got := searcher.lastQuery.String(); !strings.Contains(got, "server") || !strings.Contains(got, "main") {
		t.Fatalf("expected file query to include filename and branch filters, got %s", got)
	}
	if resp.Total != 2 || len(resp.Files) != 2 {
		t.Fatalf("expected two unique files, got %+v", resp)
	}
	if resp.Files[0] != "cmd/server/main.go" || resp.Files[1] != "internal/search/service.go" {
		t.Fatalf("unexpected files %+v", resp.Files)
	}
}

func TestZoektServiceSearchOptionsAreSizedForRequestedPage(t *testing.T) {
	searcher := &fakeZoektSearcher{searchResult: &zoekt.SearchResult{}}
	svc := NewZoektServiceFromSearcher(searcher, IndexOptions{IndexDir: t.TempDir()})

	_, err := svc.SearchContent(context.Background(), []repo.Repository{{RepoID: 7, Name: "repo"}}, SearchRequest{
		Query:    "main",
		Page:     3,
		PageSize: 25,
	})
	if err != nil {
		t.Fatalf("SearchContent failed: %v", err)
	}
	if searcher.lastOptions == nil {
		t.Fatal("expected search options")
	}
	if searcher.lastOptions.ShardMaxMatchCount != 75 {
		t.Fatalf("expected shard max match count for requested page, got %d", searcher.lastOptions.ShardMaxMatchCount)
	}
	if searcher.lastOptions.TotalMaxMatchCount != 75 {
		t.Fatalf("expected total max match count for requested page, got %d", searcher.lastOptions.TotalMaxMatchCount)
	}
	if searcher.lastOptions.MaxMatchDisplayCount != 75 {
		t.Fatalf("expected max match display count for requested page, got %d", searcher.lastOptions.MaxMatchDisplayCount)
	}
}

func TestZoektServiceSearchPropagatesErrors(t *testing.T) {
	searchErr := errors.New("search failed")
	searcher := &fakeZoektSearcher{searchErr: searchErr}
	svc := NewZoektServiceFromSearcher(searcher, IndexOptions{IndexDir: t.TempDir()})

	_, err := svc.SearchContent(context.Background(), []repo.Repository{{RepoID: 7, Name: "repo"}}, SearchRequest{Query: "main"})
	if !errors.Is(err, searchErr) {
		t.Fatalf("expected search error, got %v", err)
	}
}

func TestZoektServiceWrapsInvalidRequests(t *testing.T) {
	searcher := &fakeZoektSearcher{}
	svc := NewZoektServiceFromSearcher(searcher, IndexOptions{IndexDir: t.TempDir()})

	_, err := svc.SearchContent(context.Background(), []repo.Repository{{RepoID: 7, Name: "repo"}}, SearchRequest{})
	if !IsInvalidRequestError(err) {
		t.Fatalf("expected invalid request error, got %v", err)
	}
}

func TestZoektServiceHealthCallsListAndPropagatesErrors(t *testing.T) {
	listErr := errors.New("list failed")
	searcher := &fakeZoektSearcher{listErr: listErr}
	svc := NewZoektServiceFromSearcher(searcher, IndexOptions{IndexDir: t.TempDir()})

	err := svc.Health(context.Background())
	if !errors.Is(err, listErr) {
		t.Fatalf("expected list error, got %v", err)
	}
	if !searcher.listCalled {
		t.Fatal("expected Health to call List")
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
