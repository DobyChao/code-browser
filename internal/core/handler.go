package core

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv" // Needed for parsing uint32 repoID
	"strings"

	"code-browser/internal/repo"
	"github.com/patrickmn/go-cache"
)

// Handlers 封装了所有与核心浏览功能相关的 HTTP 处理器
type Handlers struct {
	RepoProvider *repo.Provider
	Cache        *cache.Cache
}

// ListRepositories 返回所有已配置的仓库列表
func (h *Handlers) ListRepositories(w http.ResponseWriter, r *http.Request) {
	repos := h.RepoProvider.GetAll()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(repos); err != nil {
		log.Printf("序列化仓库列表失败: %v", err)
		http.Error(w, "无法序列化仓库列表", http.StatusInternalServerError)
	}
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

// GetTree 返回指定仓库和路径下的文件/目录列表
func (h *Handlers) GetTree(w http.ResponseWriter, r *http.Request) {
	// 打印请求：
	log.Printf("请求: %s %s", r.Method, r.URL.Path)
	repoID, err := parseRepoIDHelper(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	relativePath := r.URL.Query().Get("path")
	log.Printf("请求: relativePath: %s", relativePath)

	// 尝试从缓存中获取结果
	cacheKey := fmt.Sprintf("tree:%d:%s", repoID, relativePath)
	if data, found := h.Cache.Get(cacheKey); found {
		log.Printf("DEBUG: 缓存命中 (tree): %s", cacheKey)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(data)
		return
	}

	repoInfo, ok := h.RepoProvider.GetRepo(repoID)
	if !ok {
		http.Error(w, fmt.Sprintf("仓库 ID '%d' 未找到", repoID), http.StatusNotFound)
		return
	}

	targetPath := filepath.Join(repoInfo.SourcePath, relativePath)

	absRepoPath, err := filepath.Abs(repoInfo.SourcePath)
	if err != nil {
		log.Printf("错误: 无法获取仓库 '%d' 的绝对路径 '%s': %v", repoID, repoInfo.SourcePath, err)
		http.Error(w, "服务器内部错误 (获取仓库路径失败)", http.StatusInternalServerError)
		return
	}
	absTargetPath, err := filepath.Abs(targetPath)
	if err != nil {
		log.Printf("警告: 无法规范化目标路径 '%s': %v", targetPath, err)
		http.Error(w, "无效的目标路径", http.StatusBadRequest)
		return
	}
	if !strings.HasPrefix(absTargetPath, absRepoPath) {
		http.Error(w, "禁止访问", http.StatusForbidden)
		return
	}

	entries, err := os.ReadDir(targetPath)
	if err != nil {
		log.Printf("读取目录失败 %s: %v", targetPath, err)
		if os.IsNotExist(err) {
			http.Error(w, "目录不存在", http.StatusNotFound)
		} else if os.IsPermission(err) {
			http.Error(w, "无权访问目录", http.StatusForbidden)
		} else {
			http.Error(w, "无法读取目录", http.StatusInternalServerError)
		}
		return
	}

	type FileInfo struct {
		Name string `json:"name"`
		Path string `json:"path"`
		Type string `json:"type"`
	}

	var files []FileInfo
	for _, entry := range entries {
		fileType := "file"
		if entry.IsDir() {
			fileType = "directory"
		}
		entryRelativePath := filepath.Join(relativePath, entry.Name())
		files = append(files, FileInfo{
			Name: entry.Name(),
			Path: filepath.ToSlash(entryRelativePath),
			Type: fileType,
		})
	}

	if files == nil {
		files = make([]FileInfo, 0)
	}

	// 缓存结果
	h.Cache.Set(cacheKey, files, cache.DefaultExpiration)

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(files); err != nil {
		log.Printf("序列化文件列表失败: %v", err)
	}
}

// blobCacheEntry 定义了用于缓存文件内容的结构
type blobCacheEntry struct {
	Content     []byte
	ContentType string
}

// GetBlob 返回指定文件的原始内容
func (h *Handlers) GetBlob(w http.ResponseWriter, r *http.Request) {
	// 打印请求：
	log.Printf("请求: %s %s", r.Method, r.URL.Path)
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

	// 尝试从缓存中获取结果
	cacheKey := fmt.Sprintf("blob:%d:%s", repoID, relativePath)
	if data, found := h.Cache.Get(cacheKey); found {
		log.Printf("DEBUG: 缓存命中 (blob): %s", cacheKey)
		entry := data.(blobCacheEntry) // 从接口类型断言
		w.Header().Set("Content-Type", entry.ContentType)
		w.Write(entry.Content)
		return
	}

	repoInfo, ok := h.RepoProvider.GetRepo(repoID)
	if !ok {
		http.Error(w, fmt.Sprintf("仓库 ID '%d' 未找到", repoID), http.StatusNotFound)
		return
	}

	targetPath := filepath.Join(repoInfo.SourcePath, relativePath)

	absRepoPath, _ := filepath.Abs(repoInfo.SourcePath)
	absTargetPath, err := filepath.Abs(targetPath)
	if err != nil {
		log.Printf("警告: 无法规范化文件路径 '%s': %v", targetPath, err)
		http.Error(w, "无效的文件路径", http.StatusBadRequest)
		return
	}
	if !strings.HasPrefix(absTargetPath, absRepoPath) {
		http.Error(w, "禁止访问", http.StatusForbidden)
		return
	}
	if absTargetPath == absRepoPath { // Disallow reading root as blob
		http.Error(w, "禁止读取仓库根目录作为文件", http.StatusForbidden)
		return
	}

	fileInfo, err := os.Stat(targetPath)
	if err != nil {
		log.Printf("检查文件/目录 '%s' 失败: %v", targetPath, err)
		if os.IsNotExist(err) {
			http.Error(w, "文件不存在", http.StatusNotFound)
		} else if os.IsPermission(err) {
			http.Error(w, "无权访问文件", http.StatusForbidden)
		} else {
			http.Error(w, "无法访问文件", http.StatusInternalServerError)
		}
		return
	}
	if fileInfo.IsDir() {
		http.Error(w, "路径是一个目录，无法读取内容", http.StatusBadRequest)
		return
	}

	content, err := os.ReadFile(targetPath)
	if err != nil {
		log.Printf("读取文件失败 %s: %v", targetPath, err)
		http.Error(w, "无法读取文件", http.StatusInternalServerError)
		return
	}

	contentType := http.DetectContentType(content)
	if strings.HasPrefix(contentType, "text/") || contentType == "application/octet-stream" {
		contentType = "text/plain; charset=utf-8" // Default to text/plain for code files
	}

	// 将文件内容和类型存入缓存
	entry := blobCacheEntry{Content: content, ContentType: contentType}
	h.Cache.Set(cacheKey, entry, cache.DefaultExpiration)

	w.Header().Set("Content-Type", contentType)
	w.Write(content)
}

