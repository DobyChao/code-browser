package search

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"code-browser/internal/repo"

	gogit "github.com/go-git/go-git/v5"
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
	if err := writeZoektGitConfig(repository); err != nil {
		return err
	}

	branches := opts.Branches
	if len(branches) == 0 && repository.DefaultBranch != "" {
		branches = []string{repository.DefaultBranch}
	}
	if len(branches) == 0 {
		branch, err := currentBranch(repository.SourcePath)
		if err != nil {
			return err
		}
		branches = []string{branch}
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

func writeZoektGitConfig(repository repo.Repository) error {
	gitRepo, err := gogit.PlainOpen(repository.SourcePath)
	if err != nil {
		return fmt.Errorf("open git repository %s: %w", repository.SourcePath, err)
	}
	cfg, err := gitRepo.Config()
	if err != nil {
		return fmt.Errorf("read git config %s: %w", repository.SourcePath, err)
	}
	cfg.Raw.SetOption("zoekt", "", "name", zoektRepositoryName(repository))
	cfg.Raw.SetOption("zoekt", "", "repoid", strconv.FormatUint(uint64(repository.RepoID), 10))
	if err := gitRepo.SetConfig(cfg); err != nil {
		return fmt.Errorf("write zoekt git config %s: %w", repository.SourcePath, err)
	}
	return nil
}

func currentBranch(repoPath string) (string, error) {
	gitRepo, err := gogit.PlainOpen(repoPath)
	if err != nil {
		return "", fmt.Errorf("open git repository %s: %w", repoPath, err)
	}
	head, err := gitRepo.Head()
	if err != nil {
		return "", fmt.Errorf("read git HEAD %s: %w", repoPath, err)
	}
	if !head.Name().IsBranch() {
		return "", fmt.Errorf("git HEAD is detached in %s; pass an explicit branch", repoPath)
	}
	return strings.TrimPrefix(head.Name().String(), "refs/heads/"), nil
}
