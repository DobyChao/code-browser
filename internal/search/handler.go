package search

import (
	"encoding/json"
	"log"
	"net/http"

	"code-browser/internal/config"
)

// Handlers 是一个包含 search 服务所有 HTTP 处理器方法的结构体
type Handlers struct{}

// engines 是一个存储了所有可用搜索引擎实例的映射
var engines = map[string]Engine{
	"zoekt":   &ZoektEngine{ApiUrl: "http://localhost:6070/api"},
	"ripgrep": &RipgrepEngine{},
}

// getEngineFromRequest 从 HTTP 请求中获取 'engine' 参数并返回对应的搜索引擎实例
func getEngineFromRequest(r *http.Request) (Engine, string) {
	engineName := r.URL.Query().Get("engine")
	if engine, ok := engines[engineName]; ok {
		return engine, engineName
	}
	// 默认返回 ripgrep
	return engines["ripgrep"], "ripgrep"
}

// SearchContent 处理代码内容的搜索请求
func (h *Handlers) SearchContent(w http.ResponseWriter, r *http.Request) {
	repoId := r.PathValue("repoId")
	query := r.URL.Query().Get("q")
	if query == "" {
		http.Error(w, "查询参数 'q' 不能为空", http.StatusBadRequest)
		return
	}

	repoPath := config.GetRepoPath(repoId)
	if repoPath == "" {
		http.Error(w, "未找到指定的仓库", http.StatusNotFound)
		return
	}

	engine, engineName := getEngineFromRequest(r)
	results, err := engine.SearchContent(repoId, repoPath, query)

	if err != nil {
		log.Printf("内容搜索失败 (engine: %s): %v", engineName, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(results); err != nil {
		log.Printf("序列化 JSON 失败: %v", err)
	}
}

// SearchFiles 处理文件名的搜索请求
func (h *Handlers) SearchFiles(w http.ResponseWriter, r *http.Request) {
	repoId := r.PathValue("repoId")
	query := r.URL.Query().Get("q")
	if query == "" {
		http.Error(w, "查询参数 'q' 不能为空", http.StatusBadRequest)
		return
	}

	repoPath := config.GetRepoPath(repoId)
	if repoPath == "" {
		http.Error(w, "未找到指定的仓库", http.StatusNotFound)
		return
	}

	engine, engineName := getEngineFromRequest(r)
	results, err := engine.SearchFiles(repoId, repoPath, query)

	if err != nil {
		log.Printf("文件名搜索失败 (engine: %s): %v", engineName, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(results); err != nil {
		log.Printf("序列化 JSON 失败: %v", err)
	}
}

