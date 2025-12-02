package core

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"code-browser/internal/repo"
	"github.com/patrickmn/go-cache"
)

// Service 提供文件系统操作的核心逻辑，包含缓存
type Service struct {
	RepoProvider *repo.Provider
	Cache        *cache.Cache
}

// blobCacheEntry 用于缓存文件内容及其类型
type blobCacheEntry struct {
	Content     []byte
	ContentType string
}

// NewService 创建核心服务
func NewService(repoProvider *repo.Provider, cache *cache.Cache) *Service {
	return &Service{
		RepoProvider: repoProvider,
		Cache:        cache,
	}
}

// RepositoryInfo 用于 ListRepositories 返回的简化结构
type RepositoryInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// FileInfo 用于 GetTree 返回的文件信息
type FileInfo struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Type string `json:"type"`
}

// ListRepositories 获取所有仓库列表（带缓存）
func (s *Service) ListRepositories() ([]RepositoryInfo, error) {
	repos := s.RepoProvider.GetAll()
	infos := make([]RepositoryInfo, len(repos))
	for i, repo := range repos {
		infos[i] = RepositoryInfo{
			ID:   strconv.FormatUint(uint64(repo.RepoID), 10),
			Name: repo.Name,
		}
	}
	return infos, nil
}

// GetTree 获取指定仓库和路径下的文件树（带缓存）
func (s *Service) GetTree(repoID uint32, relPath string) ([]FileInfo, error) {
	cacheKey := fmt.Sprintf("tree:%d:%s", repoID, relPath)
	if data, found := s.Cache.Get(cacheKey); found {
		return data.([]FileInfo), nil
	}

	repoInfo, ok := s.RepoProvider.GetRepo(repoID)
	if !ok {
		return nil, fmt.Errorf("仓库 ID '%d' 未找到", repoID)
	}

	targetPath := filepath.Join(repoInfo.SourcePath, relPath)
	absRepoPath, _ := filepath.Abs(repoInfo.SourcePath)
	absTargetPath, err := filepath.Abs(targetPath)
	if err != nil {
		return nil, fmt.Errorf("无效的路径: %w", err)
	}
	if !strings.HasPrefix(absTargetPath, absRepoPath) {
		return nil, fmt.Errorf("禁止访问仓库外的路径")
	}

	entries, err := os.ReadDir(targetPath)
	if err != nil {
		return nil, err
	}

	var files []FileInfo
	for _, entry := range entries {
		fileType := "file"
		if entry.IsDir() {
			fileType = "directory"
		}
		entryRelativePath := filepath.Join(relPath, entry.Name())
		files = append(files, FileInfo{
			Name: entry.Name(),
			Path: filepath.ToSlash(entryRelativePath),
			Type: fileType,
		})
	}

	if files == nil {
		files = make([]FileInfo, 0)
	}

	s.Cache.Set(cacheKey, files, cache.DefaultExpiration)
	return files, nil
}

// GetFileContent 获取指定仓库和路径的文件内容（带缓存）
// 返回内容字节和推断的 Content-Type
func (s *Service) GetFileContent(repoID uint32, relPath string) ([]byte, string, error) {
	cacheKey := fmt.Sprintf("blob:%d:%s", repoID, relPath)
	if data, found := s.Cache.Get(cacheKey); found {
		log.Printf("DEBUG: 文件内容缓存命中: %s", cacheKey)
		entry := data.(blobCacheEntry)
		return entry.Content, entry.ContentType, nil
	}

	repoInfo, ok := s.RepoProvider.GetRepo(repoID)
	if !ok {
		return nil, "", fmt.Errorf("仓库 ID '%d' 未找到", repoID)
	}

	targetPath := filepath.Join(repoInfo.SourcePath, relPath)
	absRepoPath, _ := filepath.Abs(repoInfo.SourcePath)
	absTargetPath, err := filepath.Abs(targetPath)
	if err != nil {
		return nil, "", fmt.Errorf("无效的文件路径: %w", err)
	}
	if !strings.HasPrefix(absTargetPath, absRepoPath) {
		return nil, "", fmt.Errorf("禁止访问仓库外的路径")
	}
	// Prevent reading root dir as blob
	if absTargetPath == absRepoPath && relPath == "" {
		return nil, "", fmt.Errorf("禁止读取仓库根目录作为文件")
	}

	info, err := os.Stat(targetPath)
	if err != nil {
		return nil, "", err
	}
	if info.IsDir() {
		return nil, "", fmt.Errorf("路径是一个目录")
	}

	content, err := os.ReadFile(targetPath)
	if err != nil {
		return nil, "", fmt.Errorf("读取文件失败: %w", err)
	}

	// Detect Content-Type
	contentType := "text/plain; charset=utf-8" // Default
	// Here you could use http.DetectContentType(content), but for code browser,
	// text/plain is usually safer/better unless it's an image.
	// Let's stick to text/plain for code.

	entry := blobCacheEntry{
		Content:     content,
		ContentType: contentType,
	}
	s.Cache.Set(cacheKey, entry, cache.DefaultExpiration)

	return content, contentType, nil
}
