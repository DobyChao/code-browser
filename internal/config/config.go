package config

import (
	"encoding/json"
	"os"
	"sync"
)

// Repo 定义了单个代码仓库的配置结构
type Repo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Path string `json:"path"`
}

var (
	loadedConfig []Repo      // 用于存储加载的配置
	configOnce   sync.Once   // 确保配置只加载一次
	configLock   sync.RWMutex  // 读写锁保护配置
)

// Load 从指定路径加载和解析 JSON 配置文件
func Load(path string) error {
	var loadErr error
	configOnce.Do(func() {
		file, err := os.ReadFile(path)
		if err != nil {
			loadErr = err
			return
		}

		var repos []Repo
		if err := json.Unmarshal(file, &repos); err != nil {
			loadErr = err
			return
		}

		configLock.Lock()
		loadedConfig = repos
		configLock.Unlock()
	})
	return loadErr
}

// GetRepos 返回所有已加载的仓库配置的副本，用于显示列表
func GetRepos() []Repo {
	configLock.RLock()
	defer configLock.RUnlock()
	// 返回副本以防止外部修改
	reposCopy := make([]Repo, len(loadedConfig))
	copy(reposCopy, loadedConfig)
	return reposCopy
}

// GetRepoPath 根据仓库 ID 返回其物理路径
// 这是给搜索等模块使用的关键函数
func GetRepoPath(id string) string {
	configLock.RLock()
	defer configLock.RUnlock()
	for _, repo := range loadedConfig {
		if repo.ID == id {
			return repo.Path
		}
	}
	return ""
}
