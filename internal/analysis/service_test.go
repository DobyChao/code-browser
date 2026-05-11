package analysis

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"code-browser/internal/core"
	"code-browser/internal/repo"
	"code-browser/internal/search"

	"github.com/patrickmn/go-cache"
	"github.com/sourcegraph/scip/bindings/go/scip"
)

func TestGetDefinitionFromSCIP_LineNumberMapping_SingleLine(t *testing.T) {
	idx := &scip.Index{Documents: []*scip.Document{}}
	doc := &scip.Document{RelativePath: "a.go"}
	doc.Occurrences = append(doc.Occurrences, &scip.Occurrence{
		Range:       []int32{0, 1, 5}, // single-line: [startLine=0, startCol=1, endCol=5]
		Symbol:      "symA",
		SymbolRoles: int32(scip.SymbolRole_Definition),
	})
	idx.Documents = append(idx.Documents, doc)

	// 构造服务并直接调用内部函数
	s := &Service{}
	defs, err := s.getDefinitionFromSCIPForTest(idx, "a.go", 0, 2, "1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(defs) == 0 {
		t.Fatalf("expected at least one definition")
	}
	d := defs[0]
	if d.Range.StartLine != 1 || d.Range.EndLine != 1 { // 1-based
		t.Fatalf("expected 1-based line, got start=%d end=%d", d.Range.StartLine, d.Range.EndLine)
	}
	if d.Range.StartColumn != 1 || d.Range.EndColumn != 5 {
		t.Fatalf("columns mismatch: %d..%d", d.Range.StartColumn, d.Range.EndColumn)
	}
}

func TestGetDefinitionFromSCIP_LineNumberMapping_MultiLine(t *testing.T) {
	idx := &scip.Index{Documents: []*scip.Document{}}
	doc := &scip.Document{RelativePath: "b.py"}
	doc.Occurrences = append(doc.Occurrences, &scip.Occurrence{
		Range:       []int32{2, 0, 4, 3}, // multi-line: [startLine=2, startCol=0, endLine=4, endCol=3]
		Symbol:      "symB",
		SymbolRoles: int32(scip.SymbolRole_Definition),
	})
	idx.Documents = append(idx.Documents, doc)

	s := &Service{}
	defs, err := s.getDefinitionFromSCIPForTest(idx, "b.py", 2, 1, "1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(defs) == 0 {
		t.Fatalf("expected at least one definition")
	}
	d := defs[0]
	if d.Range.StartLine != 3 || d.Range.EndLine != 5 { // +1
		t.Fatalf("expected 1-based lines 3..5, got %d..%d", d.Range.StartLine, d.Range.EndLine)
	}
	if d.Range.StartColumn != 0 || d.Range.EndColumn != 3 {
		t.Fatalf("columns mismatch: %d..%d", d.Range.StartColumn, d.Range.EndColumn)
	}
}

type fakeFallbackSearch struct {
	req search.SearchRequest
}

func (f *fakeFallbackSearch) SearchContent(ctx context.Context, repos []repo.Repository, req search.SearchRequest) (*search.SearchResponse, error) {
	f.req = req
	return &search.SearchResponse{
		Results: []search.SearchResult{{Path: "main.go", LineNum: 3, LineText: "func Target() {}"}},
		Total:   1,
		Page:    1, PageSize: 50,
	}, nil
}

func TestGetDefinitionFallsBackToSearchService(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git binary not available: %v", err)
	}

	provider, err := repo.NewProvider(t.TempDir())
	if err != nil {
		t.Fatalf("NewProvider failed: %v", err)
	}
	t.Cleanup(func() { _ = provider.Close() })

	repoDir := t.TempDir()
	runAnalysisGit(t, repoDir, "init")
	runAnalysisGit(t, repoDir, "checkout", "-b", "main")
	if err := os.WriteFile(filepath.Join(repoDir, "main.go"), []byte("package main\n\nfunc Target() {}\n"), 0644); err != nil {
		t.Fatalf("write main.go: %v", err)
	}
	runAnalysisGit(t, repoDir, "add", ".")
	runAnalysisGit(t, repoDir, "commit", "-m", "initial")

	if err := provider.AddRepository(1, "repo", repoDir); err != nil {
		t.Fatalf("AddRepository failed: %v", err)
	}

	fallback := &fakeFallbackSearch{}
	coreService := core.NewService(provider, cache.New(cache.NoExpiration, cache.NoExpiration))
	service := NewService(provider, fallback, coreService)

	got, err := service.GetDefinition(DefinitionRequest{
		RepoID:    "1",
		FilePath:  "main.go",
		Line:      2,
		Character: 6,
	})
	if err != nil {
		t.Fatalf("GetDefinition failed: %v", err)
	}
	if fallback.req.Query != "sym:Target" {
		t.Fatalf("expected sym query, got %q", fallback.req.Query)
	}
	if len(got) == 0 || got[0].Source != "search" {
		t.Fatalf("expected search fallback result, got %+v", got)
	}
}

func runAnalysisGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_GLOBAL=",
		"GIT_CONFIG_SYSTEM=",
		"GIT_COMMITTER_NAME=Test User",
		"GIT_COMMITTER_EMAIL=test@example.com",
		"GIT_AUTHOR_NAME=Test User",
		"GIT_AUTHOR_EMAIL=test@example.com",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, output)
	}
}
