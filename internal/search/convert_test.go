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
					LineOffset:  5,
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

func TestConvertSearchResultNormalizesPagination(t *testing.T) {
	got := ConvertSearchResult(nil, SearchRequest{})
	if got.Page != 1 || got.PageSize != 50 {
		t.Fatalf("expected default pagination, got page=%d size=%d", got.Page, got.PageSize)
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

func TestConvertFileSearchResultHandlesEmptyAndPagination(t *testing.T) {
	got := ConvertFileSearchResult(&zoekt.SearchResult{}, FileSearchRequest{Page: 3, PageSize: 25})
	if got.Total != 0 || len(got.Files) != 0 {
		t.Fatalf("expected empty response, got %+v", got)
	}
	if got.Page != 3 || got.PageSize != 25 {
		t.Fatalf("expected pagination to be preserved, got page=%d size=%d", got.Page, got.PageSize)
	}
}
