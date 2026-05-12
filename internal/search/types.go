package search

import (
	"context"

	"code-browser/internal/repo"
)

// SearchFragment identifies one highlighted match inside a result line.
type SearchFragment struct {
	Offset int `json:"offset"`
	Length int `json:"length"`
}

// SearchResult is the public search result shape returned by HTTP APIs.
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
	Results   []SearchResult `json:"results"`
	Total     int            `json:"total"`
	Page      int            `json:"page"`
	PageSize  int            `json:"page_size"`
	Truncated bool           `json:"truncated,omitempty"`
}

type FileSearchResponse struct {
	Files     []string `json:"files"`
	Total     int      `json:"total"`
	Page      int      `json:"page"`
	PageSize  int      `json:"page_size"`
	Truncated bool           `json:"truncated,omitempty"`
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
