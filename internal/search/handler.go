package search

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv" // Needed for parsing uint32 repoID

	"code-browser/internal/repo"
)

// Handlers 封装了所有与搜索相关的 HTTP 处理器
type Handlers struct {
	Engines      map[string]Engine // 搜索引擎实例映射
	RepoProvider *repo.Provider    // 仓库服务实例，用于获取仓库信息
}

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
	query := r.URL.Query().Get("q")
	engineName := r.URL.Query().Get("engine")

	if query == "" {
		http.Error(w, "Query parameter 'q' is required", http.StatusBadRequest)
		return
	}
	engine, ok := h.Engines[engineName]
	if !ok {
		http.Error(w, fmt.Sprintf("Invalid search engine: %s. Available: %v", engineName, getMapKeys(h.Engines)), http.StatusBadRequest)
		return
	}

	repoInfo, ok := h.RepoProvider.GetRepo(repoID)
	if !ok {
		http.Error(w, fmt.Sprintf("仓库 ID '%d' 未找到", repoID), http.StatusNotFound)
		return
	}

	results, err := engine.SearchContent(repoInfo, query)
	if err != nil {
		log.Printf("内容搜索失败 (engine: %s, repo: %d): %v", engineName, repoID, err)
		http.Error(w, fmt.Sprintf("Search failed: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(results); err != nil {
		log.Printf("序列化搜索结果失败: %v", err)
	}
}

// SearchFiles 处理文件名搜索请求
func (h *Handlers) SearchFiles(w http.ResponseWriter, r *http.Request) {
	repoID, err := parseRepoIDHelper(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	query := r.URL.Query().Get("q")
	engineName := r.URL.Query().Get("engine")

	if engineName == "" {
		engineName = "ripgrep" // Default to ripgrep for file search
	}

	engine, ok := h.Engines[engineName]
	if !ok {
		http.Error(w, fmt.Sprintf("Invalid search engine: %s. Available: %v", engineName, getMapKeys(h.Engines)), http.StatusBadRequest)
		return
	}

	repoInfo, ok := h.RepoProvider.GetRepo(repoID)
	if !ok {
		http.Error(w, fmt.Sprintf("仓库 ID '%d' 未找到", repoID), http.StatusNotFound)
		return
	}

	results, err := engine.SearchFiles(repoInfo, query)
	if err != nil {
		log.Printf("文件名搜索失败 (engine: %s, repo: %d): %v", engineName, repoID, err)
		http.Error(w, fmt.Sprintf("File search failed: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(results); err != nil {
		log.Printf("序列化文件结果失败: %v", err)
	}
}

// getMapKeys 辅助函数，获取 map 的键
func getMapKeys(m map[string]Engine) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

