package repo

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"os/exec" // Needed for running git and zoekt-git-index
	"path/filepath"
	"regexp"
	"strconv" // Needed for converting uint32 to string for DataPath
	"strings"
	"sync" // Mutex for safe concurrent updates to cache
	"time"

	"github.com/go-git/go-git/v5"   // ★ 新增: go-git API
	_ "github.com/mattn/go-sqlite3" // Import the SQLite driver
)

// Repository 定义了单个代码仓库的配置结构 (与数据库表对应)
type Repository struct {
	DBID       int64     `json:"-"`    // 数据库内部自增 ID
	RepoID     uint32    `json:"id"`   // 用户定义的、API 使用的唯一 uint32 ID
	Name       string    `json:"name"` // 显示给用户的名称
	SourcePath string    `json:"-"`    // 仓库在文件系统中的绝对路径
	DataPath   string    `json:"-"`    // 该仓库专属数据目录的路径
	CreatedAt  time.Time `json:"-"`    // 创建时间
	UpdatedAt  time.Time `json:"-"`    // 更新时间
}

// Provider 是仓库管理服务，负责加载和提供仓库信息
type Provider struct {
	db           *sql.DB               // SQLite 数据库连接
	DataDir      string                // 应用的全局数据目录
	repositories []Repository          // 按数据库顺序排列的仓库列表 (内存缓存)
	repoMap      map[uint32]Repository // 用于通过 uint32 RepoID 快速查找仓库 (内存缓存)
	mu           sync.RWMutex          // 用于保护内存缓存的读写锁
}

const dbFileName = "app.db"
const reposSubDir = "repos"            // 子目录，存放各仓库数据
const zoektIndexSubDir = "zoekt-index" // 子目录，存放 Zoekt 索引

// NewProvider 创建一个新的仓库服务实例
// 它会在全局数据目录 (dataDir) 下初始化 SQLite 数据库。
func NewProvider(dataDir string) (*Provider, error) {
	absDataDir, err := filepath.Abs(dataDir)
	if err != nil {
		return nil, fmt.Errorf("无法获取全局数据目录 '%s' 的绝对路径: %w", dataDir, err)
	}
	// 确保基础数据目录和仓库子目录存在
	reposPath := filepath.Join(absDataDir, reposSubDir)
	if err := os.MkdirAll(reposPath, 0755); err != nil {
		return nil, fmt.Errorf("创建仓库数据子目录 '%s' 失败: %w", reposPath, err)
	}

	dbPath := filepath.Join(absDataDir, dbFileName) + "?_foreign_keys=on&_journal_mode=WAL" // Use WAL mode for better concurrency
	log.Printf("初始化数据库: %s", filepath.Join(absDataDir, dbFileName))
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("打开数据库 '%s' 失败: %w", dbPath, err)
	}
	// Configure connection pool for potentially concurrent reads
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	p := &Provider{
		db:           db,
		DataDir:      absDataDir,
		repositories: make([]Repository, 0),
		repoMap:      make(map[uint32]Repository),
	}

	if err := p.initSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("初始化数据库 schema 失败: %w", err)
	}

	// Load initial data from DB into memory cache
	if err := p.loadReposFromDB(); err != nil {
		db.Close()
		return nil, fmt.Errorf("从数据库加载仓库失败: %w", err)
	}
	log.Printf("从数据库加载 %d 个仓库...", len(p.repositories))

	return p, nil
}

// initSchema 确保数据库表和触发器存在
func (p *Provider) initSchema() error {
	query := `
	PRAGMA foreign_keys = ON;

	CREATE TABLE IF NOT EXISTS repositories (
		id INTEGER PRIMARY KEY AUTOINCREMENT, -- 数据库内部 ID
		repo_id INTEGER UNIQUE NOT NULL,       -- 用户/API 使用的唯一 uint32 ID
		name TEXT NOT NULL,
		source_path TEXT NOT NULL,
		data_path TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TRIGGER IF NOT EXISTS update_repo_updated_at
	AFTER UPDATE ON repositories FOR EACH ROW
	BEGIN
		UPDATE repositories SET updated_at = CURRENT_TIMESTAMP WHERE id = OLD.id;
	END;
	`
	_, err := p.db.Exec(query)
	return err
}

// loadReposFromDB 从数据库加载所有仓库信息到内存缓存
func (p *Provider) loadReposFromDB() error {
	p.mu.Lock() // Acquire write lock to modify cache
	defer p.mu.Unlock()

	rows, err := p.db.Query("SELECT id, repo_id, name, source_path, data_path, created_at, updated_at FROM repositories ORDER BY name")
	if err != nil {
		return fmt.Errorf("查询数据库仓库失败: %w", err)
	}
	defer rows.Close()

	// Reset cache before loading
	p.repositories = make([]Repository, 0)
	p.repoMap = make(map[uint32]Repository)

	for rows.Next() {
		var repo Repository
		var createdAt sql.NullTime
		var updatedAt sql.NullTime
		err := rows.Scan(&repo.DBID, &repo.RepoID, &repo.Name, &repo.SourcePath, &repo.DataPath, &createdAt, &updatedAt)
		if err != nil {
			// Log individual scan errors but continue if possible
			log.Printf("警告: 扫描数据库行失败: %v", err)
			continue
		}

		if createdAt.Valid {
			repo.CreatedAt = createdAt.Time
		}
		if updatedAt.Valid {
			repo.UpdatedAt = updatedAt.Time
		}

		p.repositories = append(p.repositories, repo)
		p.repoMap[repo.RepoID] = repo
	}

	if err = rows.Err(); err != nil {
		return fmt.Errorf("数据库行迭代错误: %w", err)
	}
	return nil
}

// AddRepository 添加一个新的仓库到数据库并更新缓存
func (p *Provider) AddRepository(id uint32, name string, sourcePath string) error {
	if id == 0 {
		return fmt.Errorf("仓库 ID 不能为 0")
	}
	if name == "" {
		return fmt.Errorf("仓库名称不能为空")
	}
	if sourcePath == "" {
		return fmt.Errorf("仓库源路径不能为空")
	}

	absSourcePath, err := filepath.Abs(sourcePath)
	if err != nil {
		return fmt.Errorf("无法获取仓库 '%d' 源路径 '%s' 的绝对路径: %w", id, sourcePath, err)
	}

	// 确保源路径存在且是目录
	info, err := os.Stat(absSourcePath)
	if os.IsNotExist(err) {
		return fmt.Errorf("仓库 '%d' 的源路径 '%s' 不存在", id, absSourcePath)
	}
	if err != nil {
		return fmt.Errorf("检查仓库 '%d' 的源路径 '%s' 时出错: %w", id, absSourcePath, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("仓库 '%d' 的源路径 '%s' 不是一个目录", id, absSourcePath)
	}

	// ★ 新的数据目录结构 ★
	repoDataDirName := strconv.FormatUint(uint64(id), 10)
	repoDataPath := filepath.Join(p.DataDir, reposSubDir, repoDataDirName) // <dataDir>/repos/<id>/

	// 创建仓库专属数据目录
	if err := os.MkdirAll(repoDataPath, 0755); err != nil {
		return fmt.Errorf("为仓库 '%d' 创建数据目录 '%s' 失败: %w", id, repoDataPath, err)
	}

	// 插入数据库
	query := "INSERT INTO repositories (repo_id, name, source_path, data_path) VALUES (?, ?, ?, ?)"
	_, err = p.db.Exec(query, id, name, absSourcePath, repoDataPath)
	if err != nil {
		// Specific check for UNIQUE constraint violation
		if strings.Contains(err.Error(), "UNIQUE constraint failed: repositories.repo_id") {
			return fmt.Errorf("仓库 ID '%d' 已存在", id)
		}
		return fmt.Errorf("插入仓库 '%d' 到数据库失败: %w", id, err)
	}

	log.Printf("成功添加仓库到数据库: ID=%d, Name=%s", id, name)

	// 刷新内存缓存
	return p.loadReposFromDB()
}

// DeleteRepository 从数据库删除一个仓库并更新缓存
func (p *Provider) DeleteRepository(id uint32) error {
	// 先从缓存中获取 DataPath，以便后续删除目录
	repo, ok := p.GetRepo(id) // Use GetRepo which uses RLock
	if !ok {
		return fmt.Errorf("仓库 ID '%d' 未找到", id)
	}
	repoDataPath := repo.DataPath // Get data path before potential cache update

	// 从数据库删除
	query := "DELETE FROM repositories WHERE repo_id = ?"
	result, err := p.db.Exec(query, id)
	if err != nil {
		return fmt.Errorf("从数据库删除仓库 '%d' 失败: %w", id, err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		log.Printf("警告: 检查删除仓库 '%d' 的影响行数失败: %v", id, err)
	}
	if rowsAffected == 0 {
		// This might happen in a race condition, reload cache just in case
		_ = p.loadReposFromDB()
		return fmt.Errorf("仓库 ID '%d' 在数据库中未找到 (可能已被删除)", id)
	}

	log.Printf("成功从数据库删除仓库: ID=%d", id)

	// 删除仓库专属数据目录
	if repoDataPath != "" {
		log.Printf("正在删除仓库数据目录: %s", repoDataPath)
		if err := os.RemoveAll(repoDataPath); err != nil {
			log.Printf("警告: 删除仓库 '%d' 的数据目录 '%s' 失败: %v", id, repoDataPath, err)
			// Log the error but don't fail the whole operation
		}
	} else {
		log.Printf("警告: 无法确定仓库 '%d' 的数据目录路径，跳过删除。", id)
	}

	// 刷新内存缓存
	return p.loadReposFromDB()
}

// IndexRepositoryZoekt 为指定的 Git 仓库生成或更新 Zoekt 索引
func (p *Provider) IndexRepositoryZoekt(id uint32) error {
	repoInfo, ok := p.GetRepo(id) // Read lock
	if !ok {
		return fmt.Errorf("仓库 ID '%d' 未找到", id)
	}

	// ★ 1. 检查是否为 Git 仓库 (使用 go-git) ★
	repo, err := git.PlainOpen(repoInfo.SourcePath)
	if err != nil {
		return fmt.Errorf("仓库 '%s' (%d) 在路径 '%s' 下不是一个有效的 Git 仓库 (go-git open 失败): %w", repoInfo.Name, id, repoInfo.SourcePath, err)
	}

	// 2. 确保全局 Zoekt 索引目录存在
	zoektIndexPath := filepath.Join(p.DataDir, zoektIndexSubDir) // <dataDir>/zoekt-index/
	if err := os.MkdirAll(zoektIndexPath, 0755); err != nil {
		return fmt.Errorf("创建全局 Zoekt 索引目录 '%s' 失败: %w", zoektIndexPath, err)
	}

	// 3. 检查 zoekt-git-index 命令是否存在
	zoektCmdPath, err := exec.LookPath("zoekt-git-index")
	if err != nil {
		return fmt.Errorf("错误: 'zoekt-git-index' 命令未找到。请确保已安装并配置在系统 PATH 中。参考 README.md")
	}

	// ★ 4. 更新仓库本地 Git 配置以包含 zoekt.repoid, zoekt.name (使用 go-git) ★
	log.Printf("正在更新仓库 '%s' (%d) 的 .git/config...", repoInfo.Name, id)
	cfg, err := repo.Config() // 读取 .git/config
	if err != nil {
		return fmt.Errorf("无法读取仓库 '%s' (%d) 的 .git/config 文件: %w", repoInfo.Name, id, err)
	}

	// ★ 新的 Zoekt 索引名称格式: "id(10位补0)_reponame" ★
	// 确保仓库名对于文件名是安全的 (替换所有非字母数字字符为下划线)
	reg := regexp.MustCompile("[^a-zA-Z0-9]+")
	sanitizedName := reg.ReplaceAllString(repoInfo.Name, "_")
	// 格式化 ID 为 10 位，用 0 填充
	zoektName := fmt.Sprintf("%010d_%s", id, sanitizedName)
	cfg.Raw.SetOption("zoekt", "", "name", zoektName)

	repoIDStr := strconv.FormatUint(uint64(id), 10)
	// section="zoekt", subsection="", key="repoid", value=repoIDStr
	cfg.Raw.SetOption("zoekt", "", "repoid", repoIDStr)

	if err := repo.SetConfig(cfg); err != nil { // 写回 .git/config
		// 记录警告，但不一定是致命错误
		log.Printf("警告: 无法将 zoekt.repoid 写入仓库 '%s' (%d) 的 .git/config: %v", repoInfo.Name, id, err)
	} else {
		log.Printf("成功更新仓库 '%s' (%d) 的 Git 配置 zoekt.repoid", repoInfo.Name, id)
	}

	// ★ 5. 执行 zoekt-git-index 命令 ★
	// zoekt-git-index [-index indexDir] [-name repoName] repoDir
	args := []string{
		"-index", zoektIndexPath,
		repoInfo.SourcePath,
	}
	zoektCmd := exec.Command(zoektCmdPath, args...)
	zoektCmd.Stdout = os.Stdout // 将输出直接打印到控制台
	zoektCmd.Stderr = os.Stderr
	log.Printf("正在为仓库 '%s' (%d) 生成 Zoekt 索引...", repoInfo.Name, id)
	log.Printf("执行命令: %s %s", zoektCmdPath, strings.Join(args, " "))

	startTime := time.Now()
	if err := zoektCmd.Run(); err != nil {
		return fmt.Errorf("执行 zoekt-git-index 为仓库 '%s' (%d) 创建索引失败: %w", repoInfo.Name, id, err)
	}

	log.Printf("成功为仓库 '%s' (%d) 生成 Zoekt 索引 (名称: %s)，耗时: %v", repoInfo.Name, id, zoektName, time.Since(startTime))
	return nil
}

// GetRepo 根据 uint32 ID 查找并返回一个仓库配置 (线程安全)
func (p *Provider) GetRepo(id uint32) (Repository, bool) {
	p.mu.RLock() // Acquire read lock
	defer p.mu.RUnlock()
	repo, ok := p.repoMap[id]
	return repo, ok
}

// GetAll 返回所有已配置的仓库列表 (按名称排序, 线程安全)
func (p *Provider) GetAll() []Repository {
	p.mu.RLock() // Acquire read lock
	defer p.mu.RUnlock()
	// 返回副本以防止外部修改
	reposCopy := make([]Repository, len(p.repositories))
	copy(reposCopy, p.repositories)
	return reposCopy
}

// Count 返回已配置的仓库数量 (线程安全)
func (p *Provider) Count() int {
	p.mu.RLock() // Acquire read lock
	defer p.mu.RUnlock()
	return len(p.repositories)
}

// Close 关闭数据库连接 (应用退出时调用)
func (p *Provider) Close() error {
	if p.db != nil {
		log.Println("关闭数据库连接...")
		return p.db.Close()
	}
	return nil
}
