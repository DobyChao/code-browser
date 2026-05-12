package search

import (
	"context"
	"errors"
	"fmt"
	"os"

	"code-browser/internal/repo"

	"github.com/sourcegraph/zoekt"
	zoektsearch "github.com/sourcegraph/zoekt/search"
)

type ZoektService struct {
	searcher zoekt.Searcher
	options  IndexOptions
}

type InvalidRequestError struct {
	Err error
}

func (e InvalidRequestError) Error() string {
	return e.Err.Error()
}

func (e InvalidRequestError) Unwrap() error {
	return e.Err
}

func IsInvalidRequestError(err error) bool {
	var invalid InvalidRequestError
	return errors.As(err, &invalid)
}

func NewZoektService(options IndexOptions) (*ZoektService, error) {
	if options.IndexDir == "" {
		return nil, fmt.Errorf("index dir is required")
	}
	if err := os.MkdirAll(options.IndexDir, 0755); err != nil {
		return nil, fmt.Errorf("create zoekt index dir: %w", err)
	}
	searcher, err := zoektsearch.NewDirectorySearcher(options.IndexDir)
	if err != nil {
		return nil, fmt.Errorf("create zoekt directory searcher: %w", err)
	}
	return NewZoektServiceFromSearcher(searcher, options), nil
}

func NewZoektServiceFromSearcher(searcher zoekt.Searcher, options IndexOptions) *ZoektService {
	return &ZoektService{searcher: searcher, options: options}
}

func (s *ZoektService) SearchContent(ctx context.Context, repos []repo.Repository, req SearchRequest) (*SearchResponse, error) {
	if s.searcher == nil {
		return nil, fmt.Errorf("zoekt searcher is not initialized")
	}
	req = NormalizeSearchRequest(req)
	q, err := BuildZoektQuery(repos, req)
	if err != nil {
		return nil, InvalidRequestError{Err: err}
	}
	result, err := s.searcher.Search(ctx, q, searchOptionsForPage(req.Page, req.PageSize))
	if err != nil {
		return nil, err
	}
	return ConvertSearchResult(result, req), nil
}

func (s *ZoektService) SearchFiles(ctx context.Context, repos []repo.Repository, req FileSearchRequest) (*FileSearchResponse, error) {
	if s.searcher == nil {
		return nil, fmt.Errorf("zoekt searcher is not initialized")
	}
	req = NormalizeFileSearchRequest(req)
	q, err := BuildZoektFileQuery(repos, req)
	if err != nil {
		return nil, InvalidRequestError{Err: err}
	}
	result, err := s.searcher.Search(ctx, q, searchOptionsForPage(req.Page, req.PageSize))
	if err != nil {
		return nil, err
	}
	return ConvertFileSearchResult(result, req), nil
}

func (s *ZoektService) Health(ctx context.Context) error {
	if s.searcher == nil {
		return fmt.Errorf("zoekt searcher is not initialized")
	}
	_, err := s.searcher.List(ctx, nil, &zoekt.ListOptions{})
	return err
}

func (s *ZoektService) Close() error {
	if s.searcher != nil {
		s.searcher.Close()
	}
	return nil
}

const maxSearchMatches = 10000

func searchOptionsForPage(page, pageSize int) *zoekt.SearchOptions {
	return &zoekt.SearchOptions{
		ShardMaxMatchCount:   maxSearchMatches,
		TotalMaxMatchCount:   maxSearchMatches,
		MaxMatchDisplayCount: maxSearchMatches,
	}
}
