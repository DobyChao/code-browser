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
