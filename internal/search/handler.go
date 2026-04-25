package search

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"

	"code-browser/internal/repo"
	"github.com/patrickmn/go-cache"
)

// Handlers 封装了所有与搜索相关的 HTTP 处理器
type Handlers struct {
	Service      Service        // 搜索服务实例
	RepoProvider *repo.Provider // 仓库服务实例，用于获取仓库信息
	Cache        *cache.Cache   // 缓存实例
}

const ripgrepRemovedMessage = "engine=ripgrep has been removed; only engine=zoekt is supported"

// parseRepoIDHelper 从请求路径中解析 uint32 仓库 ID (辅助函数)
func parseRepoIDHelper(r *http.Request) (uint32, error) {
	idStr := r.PathValue("id")
	idUint64, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("无效的仓库 ID 格式: '%s'", idStr)
	}
	return uint32(idUint64), nil
}

// SearchContent 处理代码内容的搜索请求
func (h *Handlers) SearchContent(w http.ResponseWriter, r *http.Request) {
	repoID, err := parseRepoIDHelper(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	engineName := r.URL.Query().Get("engine")
	if err := validateSearchEngine(engineName); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	req, err := parseSearchRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if h.Service == nil {
		http.Error(w, "Search service is not configured", http.StatusServiceUnavailable)
		return
	}

	repoInfo, ok := h.RepoProvider.GetRepo(repoID)
	if !ok {
		http.Error(w, fmt.Sprintf("仓库 ID '%d' 未找到", repoID), http.StatusNotFound)
		return
	}

	cacheKey := contentCacheKey(repoID, req)
	if data, found := h.Cache.Get(cacheKey); found {
		log.Printf("DEBUG: 缓存命中 (search-content): %s", cacheKey)
		writeJSON(w, data)
		return
	}

	results, err := h.Service.SearchContent(r.Context(), []repo.Repository{repoInfo}, req)
	if err != nil {
		log.Printf("内容搜索失败 (engine: zoekt, repo: %d): %v", repoID, err)
		http.Error(w, fmt.Sprintf("Search failed: %v", err), searchErrorStatus(err))
		return
	}

	h.Cache.Set(cacheKey, results, cache.DefaultExpiration)
	writeJSON(w, results)
}

// SearchFiles 处理文件名搜索请求
func (h *Handlers) SearchFiles(w http.ResponseWriter, r *http.Request) {
	repoID, err := parseRepoIDHelper(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	engineName := r.URL.Query().Get("engine")
	if err := validateSearchEngine(engineName); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	req, err := parseFileSearchRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if h.Service == nil {
		http.Error(w, "Search service is not configured", http.StatusServiceUnavailable)
		return
	}

	repoInfo, ok := h.RepoProvider.GetRepo(repoID)
	if !ok {
		http.Error(w, fmt.Sprintf("仓库 ID '%d' 未找到", repoID), http.StatusNotFound)
		return
	}

	cacheKey := filesCacheKey(repoID, req)
	if data, found := h.Cache.Get(cacheKey); found {
		log.Printf("DEBUG: 缓存命中 (search-files): %s", cacheKey)
		writeJSON(w, data)
		return
	}

	results, err := h.Service.SearchFiles(r.Context(), []repo.Repository{repoInfo}, req)
	if err != nil {
		log.Printf("文件名搜索失败 (engine: zoekt, repo: %d): %v", repoID, err)
		http.Error(w, fmt.Sprintf("File search failed: %v", err), searchErrorStatus(err))
		return
	}

	h.Cache.Set(cacheKey, results, cache.DefaultExpiration)
	writeJSON(w, results)
}

func validateSearchEngine(engineName string) error {
	switch engineName {
	case "", "zoekt":
		return nil
	case "ripgrep":
		return fmt.Errorf(ripgrepRemovedMessage)
	default:
		return fmt.Errorf("invalid search engine: %s; only engine=zoekt is supported", engineName)
	}
}

func parseSearchRequest(r *http.Request) (SearchRequest, error) {
	query := r.URL.Query()
	req := SearchRequest{
		Query:    query.Get("q"),
		Branch:   query.Get("branch"),
		File:     query.Get("file"),
		Page:     parsePositiveInt(query.Get("page")),
		PageSize: parsePositiveInt(query.Get("page_size")),
	}
	req = NormalizeSearchRequest(req)
	if req.Query == "" {
		return SearchRequest{}, fmt.Errorf("Query parameter 'q' is required")
	}
	return req, nil
}

func parseFileSearchRequest(r *http.Request) (FileSearchRequest, error) {
	query := r.URL.Query()
	req := FileSearchRequest{
		Query:    query.Get("q"),
		Branch:   query.Get("branch"),
		Page:     parsePositiveInt(query.Get("page")),
		PageSize: parsePositiveInt(query.Get("page_size")),
	}
	req = NormalizeFileSearchRequest(req)
	if req.Query == "" {
		return FileSearchRequest{}, fmt.Errorf("Query parameter 'q' is required")
	}
	return req, nil
}

func parsePositiveInt(raw string) int {
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0
	}
	return value
}

func contentCacheKey(repoID uint32, req SearchRequest) string {
	key, err := json.Marshal(struct {
		Kind   string        `json:"kind"`
		Engine string        `json:"engine"`
		RepoID uint32        `json:"repo_id"`
		Req    SearchRequest `json:"req"`
	}{
		Kind:   "content",
		Engine: "zoekt",
		RepoID: repoID,
		Req:    req,
	})
	if err != nil {
		return fmt.Sprintf("search:content:zoekt:%d", repoID)
	}
	return string(key)
}

func filesCacheKey(repoID uint32, req FileSearchRequest) string {
	key, err := json.Marshal(struct {
		Kind   string            `json:"kind"`
		Engine string            `json:"engine"`
		RepoID uint32            `json:"repo_id"`
		Req    FileSearchRequest `json:"req"`
	}{
		Kind:   "files",
		Engine: "zoekt",
		RepoID: repoID,
		Req:    req,
	})
	if err != nil {
		return fmt.Sprintf("search:files:zoekt:%d", repoID)
	}
	return string(key)
}

func searchErrorStatus(err error) int {
	if IsInvalidRequestError(err) {
		return http.StatusBadRequest
	}
	return http.StatusInternalServerError
}

func writeJSON(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("序列化搜索结果失败: %v", err)
	}
}
