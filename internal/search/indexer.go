package search

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"code-browser/internal/repo"

	"github.com/sourcegraph/zoekt"
	"github.com/sourcegraph/zoekt/gitindex"
	"github.com/sourcegraph/zoekt/index"
)

func (s *ZoektService) IndexRepository(ctx context.Context, repository repo.Repository, opts IndexOptions) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	opts = s.indexOptions(opts)
	if opts.IndexDir == "" {
		return fmt.Errorf("index dir is required")
	}
	if repository.SourcePath == "" {
		return fmt.Errorf("repository source path is required")
	}
	if err := os.MkdirAll(opts.IndexDir, 0755); err != nil {
		return fmt.Errorf("create zoekt index dir: %w", err)
	}

	branches := opts.Branches
	if len(branches) == 0 && repository.DefaultBranch != "" {
		branches = []string{repository.DefaultBranch}
	}
	if len(branches) == 0 {
		branches = []string{"HEAD"}
	}

	_, err := gitindex.IndexGitRepo(gitindex.Options{
		RepoDir:     repository.SourcePath,
		Incremental: opts.Incremental,
		Branches:    branches,
		BuildOptions: index.Options{
			IndexDir: opts.IndexDir,
			IsDelta:  opts.Delta,
			RepositoryDescription: zoekt.Repository{
				ID:     repository.RepoID,
				Name:   zoektRepositoryName(repository),
				URL:    repository.RemoteURL,
				Source: repository.SourcePath,
			},
		},
	})
	if err != nil {
		return fmt.Errorf("index git repository %s: %w", repository.SourcePath, err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}

func (s *ZoektService) indexOptions(opts IndexOptions) IndexOptions {
	if opts.IndexDir == "" {
		opts.IndexDir = s.options.IndexDir
	}
	if len(opts.Branches) == 0 {
		opts.Branches = s.options.Branches
	}
	if !opts.Incremental {
		opts.Incremental = s.options.Incremental
	}
	if !opts.Delta {
		opts.Delta = s.options.Delta
	}
	return opts
}

func zoektRepositoryName(repository repo.Repository) string {
	if repository.Name != "" {
		return repository.Name
	}
	if repository.SourcePath != "" {
		return filepath.Base(filepath.Clean(repository.SourcePath))
	}
	return fmt.Sprintf("repo-%d", repository.RepoID)
}
