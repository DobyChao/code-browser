package core

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"code-browser/internal/config"
)

// Handlers 是一个包含 core 服务所有 HTTP 处理器方法的结构体
type Handlers struct{}

// FileInfo 定义了返回给前端的文件/目录信息的结构
type FileInfo struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Type string `json:"type"` // "file" or "directory"
}

// ListRepositories 处理获取所有已配置仓库列表的请求
func (h *Handlers) ListRepositories(w http.ResponseWriter, r *http.Request) {
	repos := config.GetRepos()
	
	// 为了安全，我们只返回 id 和 name 给前端
	type repoInfo struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	
	repoInfos := make([]repoInfo, len(repos))
	for i, repo := range repos {
		repoInfos[i] = repoInfo{ID: repo.ID, Name: repo.Name}
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(repoInfos); err != nil {
		log.Printf("序列化仓库列表为 JSON 失败: %v", err)
	}
}

// GetTree 处理获取指定仓库和路径下文件/目录列表的请求
func (h *Handlers) GetTree(w http.ResponseWriter, r *http.Request) {
	repoId := r.PathValue("repoId")
	relativePath := r.URL.Query().Get("path")
	
	basePath := config.GetRepoPath(repoId)
	if basePath == "" {
		http.Error(w, "未找到指定的仓库", http.StatusNotFound)
		return
	}

	// 安全性检查：防止目录遍历攻击
	targetPath := filepath.Join(basePath, relativePath)
	if !strings.HasPrefix(filepath.Clean(targetPath), filepath.Clean(basePath)) {
		http.Error(w, "禁止访问的路径", http.StatusForbidden)
		return
	}

	entries, err := os.ReadDir(targetPath)
	if err != nil {
		log.Printf("读取目录失败 %s: %v", targetPath, err)
		http.Error(w, "无法读取目录", http.StatusInternalServerError)
		return
	}

	var files []FileInfo
	for _, entry := range entries {
		fileType := "file"
		if entry.IsDir() {
			fileType = "directory"
		}
		
		// 构造相对于仓库根目录的路径
		entryRelativePath := filepath.Join(relativePath, entry.Name())

		files = append(files, FileInfo{
			Name: entry.Name(),
			Path: filepath.ToSlash(entryRelativePath), // 统一使用 / 作为路径分隔符
			Type: fileType,
		})
	}
	
	// 保证即使目录为空也返回一个空数组 `[]` 而不是 `null`
	if files == nil {
		files = make([]FileInfo, 0)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(files)
}

// GetBlob 处理获取指定文件内容的请求
func (h *Handlers) GetBlob(w http.ResponseWriter, r *http.Request) {
	repoId := r.PathValue("repoId")
	relativePath := r.URL.Query().Get("path")
	
	basePath := config.GetRepoPath(repoId)
	if basePath == "" {
		http.Error(w, "未找到指定的仓库", http.StatusNotFound)
		return
	}

	targetPath := filepath.Join(basePath, relativePath)
	if !strings.HasPrefix(filepath.Clean(targetPath), filepath.Clean(basePath)) {
		http.Error(w, "禁止访问的路径", http.StatusForbidden)
		return
	}

	content, err := os.ReadFile(targetPath)
	if err != nil {
		log.Printf("读取文件失败 %s: %v", targetPath, err)
		http.Error(w, "无法读取文件", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write(content)
}
