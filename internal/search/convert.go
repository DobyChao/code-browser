package search

import "github.com/sourcegraph/zoekt"

func ConvertSearchResult(src *zoekt.SearchResult, req SearchRequest) *SearchResponse {
	req = NormalizeSearchRequest(req)
	resp := &SearchResponse{
		Page:     req.Page,
		PageSize: req.PageSize,
	}
	if src == nil {
		return resp
	}

	for _, file := range src.Files {
		for _, line := range file.LineMatches {
			fragments := make([]SearchFragment, 0, len(line.LineFragments))
			for _, fragment := range line.LineFragments {
				fragments = append(fragments, SearchFragment{
					Offset: fragment.LineOffset,
					Length: fragment.MatchLength,
				})
			}
			resp.Results = append(resp.Results, SearchResult{
				RepoName:  file.Repository,
				Path:      file.FileName,
				LineNum:   line.LineNumber,
				LineText:  string(line.Line),
				Fragments: fragments,
			})
		}
	}
	resp.Total = len(resp.Results)
	return resp
}

func ConvertFileSearchResult(src *zoekt.SearchResult, req FileSearchRequest) *FileSearchResponse {
	req = NormalizeFileSearchRequest(req)
	resp := &FileSearchResponse{
		Page:     req.Page,
		PageSize: req.PageSize,
	}
	if src == nil {
		return resp
	}

	seen := map[string]struct{}{}
	for _, file := range src.Files {
		if _, ok := seen[file.FileName]; ok {
			continue
		}
		seen[file.FileName] = struct{}{}
		resp.Files = append(resp.Files, file.FileName)
	}
	resp.Total = len(resp.Files)
	return resp
}
