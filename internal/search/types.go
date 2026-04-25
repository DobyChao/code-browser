package search

import (
	"context"

	"code-browser/internal/repo"
)

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
	Results  []SearchResult `json:"results"`
	Total    int            `json:"total"`
	Page     int            `json:"page"`
	PageSize int            `json:"page_size"`
}

type FileSearchResponse struct {
	Files    []string `json:"files"`
	Total    int      `json:"total"`
	Page     int      `json:"page"`
	PageSize int      `json:"page_size"`
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
