package search

import (
	"fmt"
	"regexp/syntax"
	"strings"

	"code-browser/internal/repo"

	zoektquery "github.com/sourcegraph/zoekt/query"
)

func NormalizeSearchRequest(req SearchRequest) SearchRequest {
	if req.Page <= 0 {
		req.Page = 1
	}
	if req.PageSize <= 0 {
		req.PageSize = 50
	}
	if req.PageSize > 500 {
		req.PageSize = 500
	}
	req.Query = strings.TrimSpace(req.Query)
	return req
}

func NormalizeFileSearchRequest(req FileSearchRequest) FileSearchRequest {
	if req.Page <= 0 {
		req.Page = 1
	}
	if req.PageSize <= 0 {
		req.PageSize = 50
	}
	if req.PageSize > 500 {
		req.PageSize = 500
	}
	req.Query = strings.TrimSpace(req.Query)
	return req
}

func BuildZoektQuery(repos []repo.Repository, req SearchRequest) (zoektquery.Q, error) {
	req = NormalizeSearchRequest(req)
	if req.Query == "" {
		return nil, fmt.Errorf("query is required")
	}
	if len(repos) == 0 {
		return nil, fmt.Errorf("at least one repository is required")
	}

	userQuery, err := zoektquery.Parse(req.Query)
	if err != nil {
		return nil, fmt.Errorf("parse zoekt query: %w", err)
	}

	repoIDs := make([]uint32, 0, len(repos))
	for _, r := range repos {
		repoIDs = append(repoIDs, r.RepoID)
	}

	filters := []zoektquery.Q{userQuery}
	if req.Branch != "" {
		filters = append(filters, zoektquery.NewSingleBranchesRepos(req.Branch, repoIDs...))
	} else {
		filters = append(filters, zoektquery.NewRepoIDs(repoIDs...))
	}
	if req.File != "" {
		fileRe, err := syntax.Parse(req.File, syntax.Perl)
		if err != nil {
			return nil, fmt.Errorf("invalid file filter: %w", err)
		}
		filters = append(filters, &zoektquery.Regexp{Regexp: fileRe, FileName: true})
	}

	return zoektquery.NewAnd(filters...), nil
}

func BuildZoektFileQuery(repos []repo.Repository, req FileSearchRequest) (zoektquery.Q, error) {
	req = NormalizeFileSearchRequest(req)
	return BuildZoektQuery(repos, SearchRequest{
		Query:    "file:" + req.Query,
		Branch:   req.Branch,
		Page:     req.Page,
		PageSize: req.PageSize,
	})
}
