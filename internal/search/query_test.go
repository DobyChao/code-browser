package search

import (
	"strings"
	"testing"

	"code-browser/internal/repo"

	zoektquery "github.com/sourcegraph/zoekt/query"
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

func TestBuildZoektFileQueryRequiresQuery(t *testing.T) {
	_, err := BuildZoektFileQuery([]repo.Repository{{RepoID: 1, Name: "repo"}}, FileSearchRequest{})
	if err == nil {
		t.Fatal("expected empty file query to fail")
	}
	if !strings.Contains(err.Error(), "query is required") {
		t.Fatalf("expected query required error, got %v", err)
	}
}

func TestBuildZoektFileQueryUsesLiteralFilenameSubstring(t *testing.T) {
	q, err := BuildZoektFileQuery([]repo.Repository{{RepoID: 7, Name: "repo"}}, FileSearchRequest{Query: "foo bar repo:other"})
	if err != nil {
		t.Fatalf("BuildZoektFileQuery failed: %v", err)
	}

	filenameQuery, ok := findSubstringQuery(q)
	if !ok {
		t.Fatalf("expected filename substring query, got %T %s", q, q.String())
	}
	if filenameQuery.Pattern != "foo bar repo:other" {
		t.Fatalf("expected literal filename query, got %q", filenameQuery.Pattern)
	}
	if !filenameQuery.FileName {
		t.Fatalf("expected filename-only query, got %s", filenameQuery.String())
	}
	if filenameQuery.Content {
		t.Fatalf("expected query not to search content, got %s", filenameQuery.String())
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
	q, err := BuildZoektQuery([]repo.Repository{{RepoID: 7, Name: "repo"}}, SearchRequest{Query: "Provider", File: `internal/.*\.go`})
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

func findSubstringQuery(q zoektquery.Q) (*zoektquery.Substring, bool) {
	switch typed := q.(type) {
	case *zoektquery.Substring:
		return typed, true
	case *zoektquery.And:
		for _, child := range typed.Children {
			if found, ok := findSubstringQuery(child); ok {
				return found, true
			}
		}
	}
	return nil, false
}
