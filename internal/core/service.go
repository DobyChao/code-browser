package core

import (
	"fmt"
	"io"
	"log"
	"path/filepath"
	"strconv"
	"strings"

	"code-browser/internal/repo"

	"github.com/go-git/go-git/v5"
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
// 使用 go-git 读取 HEAD commit 中的文件树，天然支持 gitignore 且不依赖本地文件系统状态
func (s *Service) GetTree(repoID uint32, relPath string) ([]FileInfo, error) {
	cacheKey := fmt.Sprintf("tree:%d:%s", repoID, relPath)
	if data, found := s.Cache.Get(cacheKey); found {
		return data.([]FileInfo), nil
	}

	repoInfo, ok := s.RepoProvider.GetRepo(repoID)
	if !ok {
		return nil, fmt.Errorf("仓库 ID '%d' 未找到", repoID)
	}

	// 1. 打开 Git 仓库
	r, err := git.PlainOpen(repoInfo.SourcePath)
	if err != nil {
		return nil, fmt.Errorf("打开 Git 仓库失败: %w", err)
	}

	// 2. 获取 HEAD 引用
	ref, err := r.Head()
	if err != nil {
		return nil, fmt.Errorf("获取 HEAD 引用失败: %w", err)
	}

	// 3. 获取 HEAD 指向的 Commit 对象
	commit, err := r.CommitObject(ref.Hash())
	if err != nil {
		return nil, fmt.Errorf("获取 Commit 对象失败: %w", err)
	}

	// 4. 获取 Commit 对应的 Tree
	tree, err := commit.Tree()
	if err != nil {
		return nil, fmt.Errorf("获取 Tree 失败: %w", err)
	}

	// 5. 如果请求的是子目录，需要找到对应的子 Tree
	targetTree := tree
	cleanRelPath := filepath.Clean(relPath)
	if cleanRelPath != "." && cleanRelPath != "" {
		// 注意: git tree 使用 forward slash '/' 作为分隔符，即使在 Windows 上
		// relPath 应该是相对根目录的路径，不包含前导 /
		gitPath := filepath.ToSlash(cleanRelPath)
		gitPath = strings.TrimPrefix(gitPath, "/")

		entry, err := tree.FindEntry(gitPath)
		if err != nil {
			// 如果找不到路径，或者路径不是一个目录，返回错误或空列表
			// object.ErrEntryNotFound
			return nil, fmt.Errorf("路径 '%s' 在 HEAD 中未找到: %w", gitPath, err)
		}

		if entry.Mode != 16384 && entry.Mode.String() != "040000" {
			return nil, fmt.Errorf("路径 '%s' 不是一个目录", gitPath)
		}

		targetTree, err = tree.Tree(gitPath)
		if err != nil {
			return nil, fmt.Errorf("获取子 Tree 失败: %w", err)
		}
	}

	// 6. 遍历目标 Tree 的直接子节点 (Entries)
	var files []FileInfo
	for _, entry := range targetTree.Entries {
		fileType := "file"
		// Git Mode 检查: 16384 (040000) 是目录, 33188 (0100644) 是文件
		if entry.Mode == 16384 || entry.Mode.String() == "040000" || !entry.Mode.IsFile() {
			fileType = "directory"
		}

		// 构建相对路径用于前端导航
		entryPath := filepath.Join(relPath, entry.Name)

		files = append(files, FileInfo{
			Name: entry.Name,
			Path: filepath.ToSlash(entryPath),
			Type: fileType,
		})
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

	// 1. 打开 Git 仓库
	r, err := git.PlainOpen(repoInfo.SourcePath)
	if err != nil {
		return nil, "", fmt.Errorf("打开 Git 仓库失败: %w", err)
	}

	// 2. 获取 HEAD 引用
	ref, err := r.Head()
	if err != nil {
		return nil, "", fmt.Errorf("获取 HEAD 引用失败: %w", err)
	}

	// 3. 获取 HEAD 指向的 Commit 对象
	commit, err := r.CommitObject(ref.Hash())
	if err != nil {
		return nil, "", fmt.Errorf("获取 Commit 对象失败: %w", err)
	}

	// 4. 获取对应的 Tree
	tree, err := commit.Tree()
	if err != nil {
		return nil, "", fmt.Errorf("获取 Tree 失败: %w", err)
	}

	// 5. 查找文件 Entry
	cleanRelPath := filepath.Clean(relPath)
	gitPath := filepath.ToSlash(cleanRelPath)
	gitPath = strings.TrimPrefix(gitPath, "/")

	entry, err := tree.FindEntry(gitPath)
	if err != nil {
		return nil, "", fmt.Errorf("文件 '%s' 未找到: %w", gitPath, err)
	}

	if !entry.Mode.IsFile() {
		return nil, "", fmt.Errorf("路径 '%s' 不是一个文件", gitPath)
	}

	// 6. 获取 Blob 对象并读取内容
	blob, err := tree.TreeEntryFile(entry)
	if err != nil {
		// 某些特殊对象（如 submodule）可能无法作为 blob 读取，这里简单处理
		// 尝试直接通过 Hash 获取 Blob
		blob, err = commit.File(gitPath)
		if err != nil {
			return nil, "", fmt.Errorf("获取文件 Blob 失败: %w", err)
		}
	}

	reader, err := blob.Reader()
	if err != nil {
		return nil, "", fmt.Errorf("创建 Blob Reader 失败: %w", err)
	}
	defer reader.Close()

	content, err := io.ReadAll(reader)
	if err != nil {
		return nil, "", fmt.Errorf("读取 Blob 内容失败: %w", err)
	}

	// Detect Content-Type
	contentType := "text/plain; charset=utf-8" // Default
	// Here you could use http.DetectContentType(content), but for code browser,
	// text/plain is usually safer/better unless it's an image.
	// Let's stick to text/plain for code.

	entryCache := blobCacheEntry{
		Content:     content,
		ContentType: contentType,
	}
	s.Cache.Set(cacheKey, entryCache, cache.DefaultExpiration)

	return content, contentType, nil
}
