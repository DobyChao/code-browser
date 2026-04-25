package search

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"code-browser/internal/repo"

	zoektsearch "github.com/sourcegraph/zoekt/search"
)

func TestZoektServiceIndexRepositoryCreatesShard(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git binary not available: %v", err)
	}

	dir := t.TempDir()
	repoDir := filepath.Join(dir, "repo")
	runGit(t, dir, "init", "repo")
	runGit(t, repoDir, "checkout", "-b", "main")

	if err := os.WriteFile(filepath.Join(repoDir, "hello.go"), []byte("package hello\n\nconst Message = \"zoekt shard test\"\n"), 0644); err != nil {
		t.Fatalf("write test file: %v", err)
	}
	runGit(t, repoDir, "add", ".")
	runGit(t, repoDir, "commit", "-m", "initial commit")

	indexDir := filepath.Join(dir, "index")
	svc := NewZoektServiceFromSearcher(nil, IndexOptions{IndexDir: indexDir})
	err := svc.IndexRepository(context.Background(), repo.Repository{
		RepoID:        42,
		Name:          "test-repo",
		SourcePath:    repoDir,
		DefaultBranch: "main",
	}, IndexOptions{IndexDir: indexDir, Branches: []string{"main"}})
	if err != nil {
		t.Fatalf("IndexRepository failed: %v", err)
	}

	shards, err := filepath.Glob(filepath.Join(indexDir, "*.zoekt"))
	if err != nil {
		t.Fatalf("glob shards: %v", err)
	}
	if len(shards) == 0 {
		entries, _ := os.ReadDir(indexDir)
		names := make([]string, 0, len(entries))
		for _, entry := range entries {
			names = append(names, entry.Name())
		}
		t.Fatalf("expected at least one .zoekt shard in %s, found %v", indexDir, names)
	}

	searcher, err := zoektsearch.NewDirectorySearcher(indexDir)
	if err != nil {
		t.Fatalf("open zoekt searcher: %v", err)
	}
	defer searcher.Close()

	query, err := BuildZoektQuery([]repo.Repository{{RepoID: 42}}, SearchRequest{Query: "Message", Branch: "main"})
	if err != nil {
		t.Fatalf("BuildZoektQuery failed: %v", err)
	}
	result, err := searcher.Search(context.Background(), query, searchOptionsForPage(1, 10))
	if err != nil {
		t.Fatalf("search indexed shard: %v", err)
	}
	if len(result.Files) == 0 {
		t.Fatalf("expected indexed shard to match by repo id and branch")
	}
}

func runGit(t *testing.T, dir string, args ...string) {
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
