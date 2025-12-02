package core

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"

	"code-browser/internal/repo"
)

// Handlers 封装了所有与核心浏览功能相关的 HTTP 处理器
type Handlers struct {
	RepoProvider *repo.Provider
	Service      *Service // 依赖 Service
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

// ListRepositories 返回所有已配置的仓库列表
func (h *Handlers) ListRepositories(w http.ResponseWriter, r *http.Request) {
	repos, err := h.Service.ListRepositories()
	if err != nil {
		log.Printf("获取仓库列表失败: %v", err)
		http.Error(w, "无法获取仓库列表", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(repos); err != nil {
		log.Printf("序列化仓库列表失败: %v", err)
	}
}

// GetTree 返回指定仓库和路径下的文件/目录列表
func (h *Handlers) GetTree(w http.ResponseWriter, r *http.Request) {
	repoID, err := parseRepoIDHelper(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	relativePath := r.URL.Query().Get("path")

	files, err := h.Service.GetTree(repoID, relativePath)
	if err != nil {
		log.Printf("获取目录树失败 (repo=%d, path=%s): %v", repoID, relativePath, err)
		// 简单区分一下错误类型，实际项目中可以定义明确的 Error 类型
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(files); err != nil {
		log.Printf("序列化文件列表失败: %v", err)
	}
}

// GetBlob 返回指定文件的原始内容
func (h *Handlers) GetBlob(w http.ResponseWriter, r *http.Request) {
	repoID, err := parseRepoIDHelper(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	relativePath := r.URL.Query().Get("path")
	if relativePath == "" {
		http.Error(w, "Query parameter 'path' is required", http.StatusBadRequest)
		return
	}

	content, contentType, err := h.Service.GetFileContent(repoID, relativePath)
	if err != nil {
		log.Printf("获取文件内容失败: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", contentType)
	w.Write(content)
}