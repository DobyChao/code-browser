# Code Browser - 索引与搜索架构设计规格

> 日期: 2026-03-31
> 状态: Draft
> 目标: 在不引入 AI 的前提下，将 Code Browser 打造为高质量的 Git 仓库索引与搜索平台
> 2026-04-25 修订: 搜索与索引方向调整为 Zoekt embedded first，Ripgrep 退出主线，并将 TDD 补齐列为本轮重构目标

---

## 1. 项目愿景

对标 Sourcegraph 和 DeepWiki（非 AI 部分），构建一个**单机部署、多仓库管理、高性能搜索**的代码浏览器后端。

核心定位优先级: **代码搜索 = 仓库管理 > 代码智能**

关键目标:
- 支持仓库集合（Repo Group），集合内跨仓库搜索
- 混合索引触发（手动 + 定时轮询 + Webhook）
- 将 Zoekt 作为单进程内嵌搜索和索引核心，去掉 `zoekt-webserver` 与 `zoekt-git-index` 运行时命令依赖
- 删除 Ripgrep 搜索引擎主线，避免维护双引擎抽象
- 完善的 Git 版本管理（历史、分支、Blame、Diff）
- 补齐现有模块的 TDD 覆盖，先用 characterization tests 固定当前行为，再重构
- 从单机架构出发，预留分布式演进能力

---

## 2. 当前状态评估

### 2.1 已有能力

| 模块 | 完成度 | 说明 |
|------|--------|------|
| `internal/repo` | 95% | 仓库 CRUD、SQLite 持久化、内存缓存、IndexJob 状态管理已基本落地；Zoekt 索引仍需从命令调用迁移到 library |
| `internal/core` | 90% | go-git 文件树和内容读取、缓存、Content-Type 推断 |
| `internal/search` | 70% | 当前仍是 Zoekt HTTP + Ripgrep 双引擎；需要收敛为内嵌 Zoekt 单引擎并补齐测试 |
| `internal/analysis` | 70% | SCIP 定义跳转/引用查找、搜索引擎回退 |
| `internal/feedback` | 80% | 基础反馈收集与管理 |

### 2.2 能力缺口

| 缺口 | 影响 | 优先级 |
|------|------|--------|
| 无配置文件系统 | 端口、URL、参数全部硬编码 | P0 |
| 无跨仓库搜索 | 无法搜仓库集合 | P0 |
| 无仓库集合概念 | 无法管理关联仓库组 | P0 |
| Zoekt 仍依赖外部进程 | 部署复杂，需要同时管理 `zoekt-webserver` 与 `zoekt-git-index` | P0 |
| 双搜索引擎抽象过早 | Ripgrep 后续不再作为主线，接口和 API 复杂度偏高 | P0 |
| 模块测试覆盖不足 | 重构风险高，现有行为缺少 characterization tests | P0 |
| 索引状态管理需收尾 | IndexJob 已落地，但需要与内嵌 Zoekt 索引服务解耦 | P0 |
| 无 Git 版本管理 | 只能看 HEAD，无历史/分支/Blame | P1 |
| 无索引调度器 | 无自动更新能力 | P1 |
| Zoekt 深度能力未启用 | 多分支、增量、符号搜索未完整启用 | P1 |
| 无 Webhook | 无法响应外部事件 | P2 |
| 无健康检查 | 无法监控服务状态 | P2 |

---

## 3. 架构设计

### 3.1 目标模块架构

```
cmd/
├── server/main.go          # HTTP 服务入口（修改：加载配置）
└── cli/main.go             # CLI 工具（修改：增加 group/status 命令）

internal/
├── config/                  # [新增] YAML 配置管理
│   └── config.go
├── repo/                    # [修改] 仓库元数据、IndexJob 状态、远程仓库信息
│   ├── provider.go
│   ├── index_job.go
│   └── handler.go
├── group/                   # [新增] 仓库集合管理
│   ├── service.go
│   └── handler.go
├── search/                  # [重构] Zoekt-only 搜索与索引集成
│   ├── service.go           # 内嵌 Zoekt 搜索服务
│   ├── indexer.go           # gitindex.IndexGitRepo 封装
│   ├── query.go             # API 参数到 Zoekt query 的转换
│   ├── types.go
│   └── handler.go
├── core/                    # [修改] Git 版本管理能力
│   └── service.go
├── scheduler/               # [新增] 索引调度器
│   ├── scheduler.go
│   └── webhook.go
├── codemap/                 # [新增] Code Map（实验性）
│   ├── service.go
│   └── handler.go
├── annotation/              # [新增] Code Annotations（实验性）
│   ├── service.go
│   └── handler.go
├── analysis/                # [修改] 保持现状，后续增强
│   └── service.go
└── feedback/                # 不变
    └── ...
```

### 3.2 数据流

```
                    ┌─────────────────────────────────┐
                    │           Frontend               │
                    └───────────────┬─────────────────┘
                                    │ HTTP API
                    ┌───────────────▼─────────────────┐
                    │         cmd/server               │
                    │    (路由注册 + 中间件)             │
                    └──┬──────┬──────┬──────┬──────┬───┘
                       │      │      │      │      │
              ┌────────▼┐ ┌───▼───┐ ┌▼────┐ ┌▼────┐ ┌▼────────┐
              │  group/  │ │ core/ │ │repo/│ │search│ │scheduler│
              │ 仓库集合  │ │文件浏览│ │仓库  │ │搜索  │ │索引调度  │
              └────┬─────┘ └───┬───┘ └──┬──┘ └──┬──┘ └────┬────┘
                   │           │        │       │         │
                   │           │        │       │         ▼
                   │           │        │       │    ┌───────────────┐
                   │           │        │       │    │ IndexJob Queue │
                   │           │        │       │    └───────┬───────┘
                   │           │        │       │            │
                   │           │        │       ▼            ▼
                   │           │        │   ┌──────────────────────────┐
                   │           │        │   │ embedded Zoekt            │
                   │           │        │   │ - gitindex.IndexGitRepo   │
                   │           │        │   │ - search.NewDirectory...  │
                   │           │        │   └────────────┬─────────────┘
                   │           ▼        ▼                │
                   │      ┌─────────────────┐            │
                   └─────►│    SQLite DB    │◄───────────┘
                          │  (仓库/集合/    │
                          │   索引状态)     │
                          └─────────────────┘
```

### 3.3 关键接口契约

以下接口定义了模块间的边界，每个 Feature 可独立实现，只要遵守契约即可。

#### 3.3.1 Zoekt 搜索服务接口（替代多引擎接口）

```go
// internal/search/service.go
type Service interface {
    SearchContent(ctx context.Context, repos []repo.Repository, req SearchRequest) (*SearchResponse, error)
    SearchFiles(ctx context.Context, repos []repo.Repository, req FileSearchRequest) (*FileSearchResponse, error)
    IndexRepository(ctx context.Context, repo repo.Repository, opts IndexOptions) error
    Health(ctx context.Context) error
    Close() error
}

type SearchRequest struct {
    Query    string
    Branch   string
    File     string
    Page     int
    PageSize int
}

type IndexOptions struct {
    IndexDir    string
    Branches    []string
    Incremental bool
    Delta       bool
}
```

说明:
- `search.Service` 是唯一搜索入口，不再暴露 `engine=zoekt|ripgrep` 的多引擎分发。
- 查询侧通过 `github.com/sourcegraph/zoekt/search.NewDirectorySearcher` 直接读取 shard 目录。
- 索引侧通过 `github.com/sourcegraph/zoekt/gitindex.IndexGitRepo` 生成或更新 shard。
- `repo.Provider` 只负责仓库元数据和 IndexJob 状态，具体索引执行委托给 `search.Service` 或其 indexer。

#### 3.3.2 仓库集合接口

```go
// internal/group/service.go
type Service struct {
    db           *sql.DB
    repoProvider *repo.Provider
}

// 集合操作
func (s *Service) CreateGroup(name, description string) (*Group, error)
func (s *Service) DeleteGroup(id uint32) error
func (s *Service) ListGroups() ([]Group, error)
func (s *Service) GetGroup(id uint32) (*Group, error)

// 集合成员管理
func (s *Service) AddRepoToGroup(groupID, repoID uint32) error
func (s *Service) RemoveRepoFromGroup(groupID, repoID) error
func (s *Service) GetGroupRepos(groupID uint32) ([]repo.Repository, error)

// 集合搜索（委托给 search.Service，传入集合成员 repos）
func (s *Service) SearchContent(groupID uint32, query string) ([]search.SearchResult, error)
func (s *Service) SearchFiles(groupID uint32, query string) ([]string, error)
```

#### 3.3.3 索引调度器接口

```go
// internal/scheduler/scheduler.go
type Scheduler struct {
    repoProvider *repo.Provider
    config       *config.Config
}

// 索引任务模型
type IndexJob struct {
    ID          uint32    `json:"id"`
    RepoID      uint32    `json:"repo_id"`
    Type        string    `json:"type"`        // "zoekt" | "scip"
    Status      string    `json:"status"`      // "pending" | "running" | "completed" | "failed"
    Trigger     string    `json:"trigger"`     // "manual" | "poll" | "webhook"
    Error       string    `json:"error,omitempty"`
    StartedAt   *time.Time `json:"started_at,omitempty"`
    CompletedAt *time.Time `json:"completed_at,omitempty"`
    CreatedAt   time.Time  `json:"created_at"`
}

// 调度器操作
func (s *Scheduler) Start()                           // 启动定时轮询
func (s *Scheduler) Stop()                            // 停止调度
func (s *Scheduler) TriggerIndex(repoID uint32) error // 手动触发
func (s *Scheduler) HandleWebhook(payload []byte) error
func (s *Scheduler) GetJobStatus(jobID uint32) (*IndexJob, error)
func (s *Scheduler) ListJobs(repoID uint32) ([]IndexJob, error)
```

#### 3.3.4 配置接口

```go
// internal/config/config.go
type Config struct {
    Server   ServerConfig   `yaml:"server"`
    Zoekt    ZoektConfig    `yaml:"zoekt"`
    Storage  StorageConfig  `yaml:"storage"`
    Indexer  IndexerConfig  `yaml:"indexer"`
    Webhook  WebhookConfig  `yaml:"webhook"`
}

type ServerConfig struct {
    Port         int           `yaml:"port"`
    ReadTimeout  time.Duration `yaml:"read_timeout"`
    WriteTimeout time.Duration `yaml:"write_timeout"`
    AdminToken   string        `yaml:"admin_token"`
}

type ZoektConfig struct {
    IndexDir        string   `yaml:"index_dir"`
    Branches        []string `yaml:"branches"`       // 索引哪些分支
    ShardMaxMatches int      `yaml:"shard_max_matches"`
    Incremental     bool     `yaml:"incremental"`
    Delta           bool     `yaml:"delta"`
}

type StorageConfig struct {
    DataDir string `yaml:"data_dir"`
}

type IndexerConfig struct {
    PollInterval    time.Duration `yaml:"poll_interval"`     // 0 = 禁用轮询
    MaxConcurrent   int           `yaml:"max_concurrent"`    // 最大并发索引数
}

type WebhookConfig struct {
    Enabled  bool   `yaml:"enabled"`
    Secret   string `yaml:"secret"`
    Endpoint string `yaml:"endpoint"`
}
```

---

## 4. Feature 规格清单

### F0: TDD 补齐与重构护栏

**模块**: `internal/repo/`, `internal/core/`, `internal/search/`, `internal/analysis/`, `cmd/server/`

**目标**: 在 Zoekt-only 重构前先固定当前关键行为，避免把外部进程迁移、接口删除和模块拆分混在没有测试保护的代码里。

**测试策略**:
- `internal/repo`: 覆盖 IndexJob 创建、状态流、失败记录、repository index metadata 更新。
- `internal/core`: 覆盖 `ListRepositories`、`GetTree`、`GetFileContent` 的正常路径和不存在仓库/路径错误。
- `internal/search`: 覆盖 API 参数解析、query 构造、Zoekt 结果转换、Ripgrep 已移除的错误响应。
- `internal/analysis`: 覆盖 SCIP 缺失时调用搜索服务回退的行为。
- `cmd/server`: 覆盖服务装配不再创建 Ripgrep engine，不再依赖 Zoekt HTTP URL。

**执行规则**:
- 每个行为变更先写失败测试，确认 RED 后再改实现。
- 先补 characterization tests，再拆文件和删除 Ripgrep。
- 单元测试优先使用 fake searcher/fake repo provider；只有 Zoekt library 集成路径使用临时 Git 仓库和临时 index dir。
- 不把 UI 大改纳入本批，前端只做必要的 `engine` 参数移除或错误提示适配。

---

### F1: YAML 配置系统

**模块**: `internal/config/`

**目标**: 消除所有硬编码配置，支持 YAML 配置文件 + 环境变量覆盖。

**改动**:
- 新建 `internal/config/config.go`，定义 `Config` 结构体并实现 YAML 加载
- 修改 `cmd/server/main.go`，从配置文件初始化而非硬编码
- 修改 `cmd/cli/main.go`，支持 `--config` 参数

**默认配置文件** (`config.yaml`):
```yaml
server:
  port: 8088
  read_timeout: 10s
  write_timeout: 30s    # 搜索可能耗时，从 10s 提高
  admin_token: ""       # 空则不开启鉴权

zoekt:
  index_dir: "./.data/zoekt-index"
  branches: ["main", "master"]  # 默认索引的分支
  shard_max_matches: 500
  incremental: true
  delta: false

storage:
  data_dir: "./.data"

indexer:
  poll_interval: 0       # 0 = 禁用自动轮询
  max_concurrent: 2

webhook:
  enabled: false
  secret: ""
  endpoint: "/api/webhook"
```

**兼容性**: 保留命令行参数作为覆盖项（`--data-dir`, `--admin-token`），命令行优先级高于配置文件。

---

### F2: Git 版本管理

**模块**: `internal/core/`

**目标**: 从"只能看 HEAD"进化到"完整的 Git 版本浏览"。

**新增 API 端点**:

```
GET /api/repositories/{id}/commits
  ?path=<file-path>    # 可选，过滤特定文件的提交
  ?ref=<branch|tag>    # 可选，默认 HEAD
  ?limit=20            # 可选，默认 20
  &offset=0            # 可选

Response:
{
  "commits": [
    {
      "hash": "abc1234",
      "message": "feat: add search",
      "author": { "name": "dev", "email": "dev@example.com" },
      "date": "2026-03-31T10:00:00Z",
      "parents": ["def5678"]
    }
  ],
  "total": 150
}

GET /api/repositories/{id}/branches
Response:
{
  "branches": [
    { "name": "main", "hash": "abc1234", "is_default": true },
    { "name": "dev", "hash": "def5678", "is_default": false }
  ]
}

GET /api/repositories/{id}/tags
Response:
{
  "tags": [
    { "name": "v1.0.0", "hash": "abc1234" }
  ]
}

GET /api/repositories/{id}/blame
  ?path=<file-path>     # 必需
  &ref=<branch|tag>     # 可选，默认 HEAD

Response:
{
  "lines": [
    {
      "line": 1,
      "text": "package main",
      "hash": "abc1234",
      "author": "dev",
      "date": "2026-03-01T10:00:00Z"
    }
  ]
}

GET /api/repositories/{id}/diff
  ?from=<hash>          # 必需
  &to=<hash>            # 必需
  &path=<file-path>     # 可选，只看特定文件

Response:
{
  "files": [
    {
      "path": "internal/core/service.go",
      "status": "modified",
      "additions": 10,
      "deletions": 3,
      "hunks": [
        {
          "header": "@@ -50,6 +50,10 @@",
          "changes": [
            { "type": "add", "new_line": 51, "content": "..." },
            { "type": "delete", "old_line": 51, "content": "..." },
            { "type": "context", "old_line": 52, "new_line": 53, "content": "..." }
          ]
        }
      ]
    }
  ],
  "stats": { "additions": 10, "deletions": 3, "files_changed": 1 }
}
```

**修改现有 API**:

```
GET /api/repositories/{id}/tree
  ?path=<path>          # 已有
  &ref=<branch|tag|hash> # [新增] 默认 HEAD

GET /api/repositories/{id}/blob
  ?path=<path>          # 已有
  &ref=<branch|tag|hash> # [新增] 默认 HEAD
```

**缓存策略调整**:
- 缓存键加入 ref: `tree:{repoID}:{ref}:{path}`, `blob:{repoID}:{ref}:{path}`
- HEAD 的结果可以额外缓存一份无 ref 的副本，方便快速访问

**实现依赖**: go-git v5（已有依赖，无需新增）

---

### F3: 索引状态管理

**模块**: `internal/repo/`

**目标**: 将索引操作从"一次性命令"升级为"有状态的任务"。

**数据库 Schema 扩展**:

```sql
CREATE TABLE IF NOT EXISTS index_jobs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    repo_id INTEGER NOT NULL,
    type TEXT NOT NULL DEFAULT 'zoekt',    -- 'zoekt' | 'scip'
    status TEXT NOT NULL DEFAULT 'pending', -- 'pending' | 'running' | 'completed' | 'failed'
    trigger_type TEXT NOT NULL DEFAULT 'manual', -- 'manual' | 'poll' | 'webhook'
    error TEXT,
    started_at DATETIME,
    completed_at DATETIME,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (repo_id) REFERENCES repositories(repo_id)
);

CREATE INDEX idx_index_jobs_repo ON index_jobs(repo_id);
CREATE INDEX idx_index_jobs_status ON index_jobs(status);
```

**仓库表扩展**:

```sql
-- 添加索引状态相关字段
ALTER TABLE repositories ADD COLUMN last_indexed_at DATETIME;
ALTER TABLE repositories ADD COLUMN index_status TEXT DEFAULT 'none'; -- 'none' | 'indexing' | 'indexed' | 'failed'
ALTER TABLE repositories ADD COLUMN remote_url TEXT;       -- Git 远程 URL（用于轮询）
ALTER TABLE repositories ADD COLUMN default_branch TEXT;   -- 默认分支
```

**新增 API 端点**:

```
GET /api/admin/repositories/{id}/index-status
Response:
{
  "repo_id": 1,
  "status": "indexed",           // 'none' | 'indexing' | 'indexed' | 'failed'
  "last_indexed_at": "2026-03-31T10:00:00Z",
  "jobs": [
    {
      "id": 42,
      "type": "zoekt",
      "status": "completed",
      "trigger": "manual",
      "started_at": "...",
      "completed_at": "..."
    }
  ]
}
```

**行为变更**:
- `POST /api/repositories/{id}/index` 不再同步执行索引，而是创建一个 `IndexJob` 记录，立即返回 job ID
- 后台 goroutine 消费 job 队列，执行实际索引
- 客户端通过 `GET /api/admin/repositories/{id}/index-status` 轮询状态

---

### F4: 索引调度器

**模块**: `internal/scheduler/`

**目标**: 支持三种索引触发方式。

**4a. 定时轮询**:
- 每个仓库配置 `poll_interval`（继承全局默认值，可覆盖）
- 轮询逻辑: `git ls-remote` 检查 HEAD 是否变化 → 变化则创建 IndexJob
- 首次轮询时记录 remote HEAD hash，后续对比

**4b. Webhook 接收**:
- 新增 `POST /api/webhook` 端点（可选启用）
- 支持 GitHub/GitLab push event 格式
- 验证 webhook secret（HMAC-SHA256）
- 解析 push event → 提取 repo → 创建 IndexJob

**4c. 手动触发**:
- 复用现有 `POST /api/repositories/{id}/index`
- 行为变更为创建 IndexJob（见 F3）

**新增 API**:

```
POST /api/webhook
  (接收 GitHub/GitLab push event)
  Headers: X-Hub-Signature-256: sha256=...
  Body: GitHub/GitLab push event JSON

GET /api/admin/index-jobs
  ?status=running       # 可选过滤
  ?repo_id=1            # 可选过滤
  ?limit=20

Response:
{
  "jobs": [...],
  "total": 42
}
```

**依赖**: F3（IndexJob 模型）

---

### F5: 仓库集合（Repo Group）

**模块**: `internal/group/`

**目标**: 支持将多个关联仓库归为一个集合，集合内跨仓库搜索。

**数据库 Schema**:

```sql
CREATE TABLE IF NOT EXISTS repo_groups (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    group_id INTEGER UNIQUE NOT NULL,     -- 用户指定的集合 ID
    name TEXT NOT NULL,
    description TEXT DEFAULT '',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS repo_group_members (
    group_id INTEGER NOT NULL,
    repo_id INTEGER NOT NULL,
    PRIMARY KEY (group_id, repo_id),
    FOREIGN KEY (group_id) REFERENCES repo_groups(group_id) ON DELETE CASCADE,
    FOREIGN KEY (repo_id) REFERENCES repositories(repo_id) ON DELETE CASCADE
);

CREATE TRIGGER IF NOT EXISTS update_group_updated_at
AFTER UPDATE ON repo_groups FOR EACH ROW
BEGIN
    UPDATE repo_groups SET updated_at = CURRENT_TIMESTAMP WHERE id = OLD.id;
END;
```

**新增 API 端点**:

```
POST /api/admin/groups
  Body: { "id": 1, "name": "Chromium", "description": "Chromium 项目相关仓库" }

DELETE /api/admin/groups/{id}

GET /api/groups
Response:
{
  "groups": [
    {
      "id": "1",
      "name": "Chromium",
      "description": "...",
      "repo_count": 5
    }
  ]
}

GET /api/groups/{id}
Response:
{
  "id": "1",
  "name": "Chromium",
  "description": "...",
  "repos": [
    { "id": "101", "name": "chromium/src" },
    { "id": "102", "name": "chromium/v8" }
  ]
}

POST /api/admin/groups/{id}/repos
  Body: { "repo_id": 101 }

DELETE /api/admin/groups/{id}/repos/{repo_id}

GET /api/groups/{id}/search?q=<query>
Response: (与单仓库搜索格式相同，每条结果增加 repo_name 字段)
{
  "results": [
    {
      "repo_name": "chromium/src",   // [新增] 来源仓库
      "path": "base/logging.cc",
      "line_num": 42,
      "line_text": "...",
      "fragments": [...]
    }
  ]
}

GET /api/groups/{id}/search-files?q=<query>
Response:
{
  "files": [
    { "repo_name": "chromium/src", "path": "base/logging.h" },
    { "repo_name": "chromium/v8", "path": "src/log.h" }
  ]
}
```

**SearchResult 结构扩展**:

```go
type SearchResult struct {
    RepoName string           `json:"repo_name,omitempty"` // [新增] 跨仓库搜索时填充
    Path     string           `json:"path"`
    LineNum  int              `json:"line_num"`
    LineText string           `json:"line_text"`
    Fragments []SearchFragment `json:"fragments"`
}
```

---

### F6: Zoekt-only 搜索重构

**模块**: `internal/search/`

**目标**: 将搜索系统从 "Zoekt HTTP + Ripgrep 双引擎" 收敛为内嵌 Zoekt 单引擎，删除 Ripgrep 主线和 `engine` 参数依赖。

**内嵌查询实现**:
- 服务启动时用 `search.NewDirectorySearcher(indexDir)` 加载 `.zoekt` shard，并持有 returned searcher。
- 搜索请求通过 `github.com/sourcegraph/zoekt/query` 构造查询树，使用 `Repo` 条件限定仓库 ID 或仓库名。
- 文件搜索不再调用 `rg --files`，而是构造 Zoekt 文件查询。
- 搜索结果从 `zoekt.SearchResult` 转换为现有 API 的 `SearchResult`，保留 `path`、`lineNum`、`lineText`、`fragments` 字段。
- 结果结构新增可选 `repo_name`，为未来集合搜索和全局搜索准备。

```go
func (s *ZoektService) SearchContent(ctx context.Context, repos []repo.Repository, req SearchRequest) (*SearchResponse, error) {
    q, err := BuildZoektQuery(repos, req)
    if err != nil {
        return nil, err
    }
    result, err := s.searcher.Search(ctx, q, &zoekt.SearchOptions{
        ShardMaxMatchCount: req.PageSize,
    })
    if err != nil {
        return nil, err
    }
    return ConvertSearchResult(result), nil
}
```

**Ripgrep 删除策略**:
- 删除 `RipgrepEngine` 实现和 `rg` 运行时依赖。
- `GET /api/repositories/{id}/search` 和 `/search-files` 保留 URL，但 `engine` 参数标记为 deprecated。
- 若请求显式传入 `engine=ripgrep`，返回 `400 Bad Request`，错误信息说明 Ripgrep 已移除。
- 文档、启动脚本、打包脚本移除 `rg` 与 `zoekt-webserver` 依赖。

**统一搜索入口**:
- `GET /api/repositories/{id}/search` — 单仓库搜索（已有）
- `GET /api/groups/{id}/search` — 集合搜索（F5 新增）
- `GET /api/search?q=<query>` — 全局搜索（未来，可选）

**依赖**: 无。单仓库搜索先落地，集合搜索在 F5 后复用同一个 `repos []repo.Repository` 参数。

---

### F7: Zoekt 内嵌索引与深度能力

**模块**: `internal/search/`, `internal/repo/`

**目标**: 用 Zoekt Go library 完成索引和搜索的单进程集成，并启用多分支、增量、符号搜索等能力。

**7a. 内嵌索引**:
- 将 `repo.Provider.IndexRepositoryZoekt` 中的 `exec.Command("zoekt-git-index", ...)` 替换为 `gitindex.IndexGitRepo`。
- `repo.Provider` 不直接依赖 Zoekt 细节；IndexJob 执行路径调用 `search.Service.IndexRepository`。
- 继续写入稳定的 Zoekt repository metadata：`Name` 使用当前 `0000000001_repo_name` 规则，`ID` 使用 `repo_id`。
- 索引成功后由现有 `IndexJob` 流程更新 `index_status` 和 `last_indexed_at`。

**7b. 多分支索引**:
- `gitindex.Options.Branches` 从配置读取。
- 默认索引 `main` 和 `master` 分支（通过配置指定）
- 搜索时支持 `branch:<name>` 过滤

**7c. 增量索引**:
- 使用 `gitindex.Options.Incremental` 替代命令行 `-incremental=true`
- 使用 `index.Options.IsDelta` 支持 delta 模式，可在配置中启用
- 调度器轮询检测到变更时使用增量模式

**7d. 符号搜索增强**:
- 确保 Zoekt builder 启用 ctags（需要系统安装 `universal-ctags`）
- 在搜索 API 中支持 `sym:<query>` 查询语法
- 利用 Zoekt 的符号信息改进 `analysis` 模块的回退搜索

**7e. 搜索选项扩展**:
- 支持 `file:<pattern>` 文件过滤
- 支持 `lang:<language>` 语言过滤
- 支持正则表达式模式（Zoekt 原生支持）

**新增搜索 API 参数**:

```
GET /api/repositories/{id}/search
  ?q=<query>              # 搜索查询（支持 Zoekt 语法）
  &branch=<name>          # [新增] 限定分支
  &file=<pattern>         # [新增] 文件路径过滤
  &page=1                 # [新增] 分页
  &page_size=50           # [新增] 每页数量

Response:
{
  "results": [...],
  "total": 230,
  "page": 1,
  "page_size": 50
}
```

**服务生命周期**:
- `cmd/server` 创建一个 `search.ZoektService` 并在 server shutdown 时 `Close()`。
- `start.sh` 只启动 `repo-server`，不再启动 `zoekt-webserver`。
- `stop.sh` 只停止 `repo-server`，保留清理旧 `zoekt-webserver` pid 的兼容逻辑一个版本后删除。

---

### F8: 健康检查与监控

**模块**: `cmd/server/` 路由层

**新增 API**:

```
GET /health
Response:
{
  "status": "ok",
  "components": {
    "database": "ok",
    "zoekt": "ok",          // 检查 Zoekt 连通性
    "repositories": 15      // 已注册仓库数
  }
}
```

**实现**:
- 检查 SQLite 连接: `db.Ping()`
- 检查内嵌 Zoekt: 确认 shard searcher 已初始化，并执行轻量 `List` 或空查询
- 返回组件状态

---

### F9: Code Map（实验性）

**模块**: `internal/codemap/`

**目标**: 让用户手动圈定跨文件的代码区域，将它们关联到一个"功能"、"特性"或"调用链"上。在中大型项目中帮助理解代码流程。

**核心概念**:

- **Map**: 一个命名的代码地图，代表一个特性/功能/调用链
  - 例: "HTTP 请求处理流程"、"认证中间件"、"数据持久化链路"
- **Region**: 地图中的一个代码区域，指向某个文件的一段行范围
  - 例: `repo=1, file=internal/handler.go, lines=42-58`
- **Stop**: 地图中的一个"站点"，包含一个或多个 Region + 说明文字
  - 例: "Step 1: 接收请求" → handler.go:42-58, middleware.go:10-25
- 地图中的 Stop 按顺序排列，形成一条可阅读的"代码导游路线"

**数据库 Schema**:

```sql
CREATE TABLE IF NOT EXISTS code_maps (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    map_id INTEGER UNIQUE NOT NULL,
    repo_id INTEGER NOT NULL,            -- 所属仓库（单仓库地图）
    group_id INTEGER,                    -- 所属集合（跨仓库地图，可选）
    name TEXT NOT NULL,
    description TEXT DEFAULT '',
    color TEXT DEFAULT '',               -- 显示颜色（前端提示，如 '#4A90D9'）
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (repo_id) REFERENCES repositories(repo_id) ON DELETE CASCADE,
    FOREIGN KEY (group_id) REFERENCES repo_groups(group_id) ON DELETE SET NULL
);

CREATE TABLE IF NOT EXISTS code_map_stops (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    map_id INTEGER NOT NULL,
    order_index INTEGER NOT NULL,        -- 站点顺序
    title TEXT NOT NULL,                 -- 站点标题，如 "Step 1: 解析请求"
    description TEXT DEFAULT '',         -- 站点说明
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (map_id) REFERENCES code_maps(map_id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS code_map_regions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    stop_id INTEGER NOT NULL,
    repo_id INTEGER NOT NULL,
    file_path TEXT NOT NULL,             -- 文件路径
    start_line INTEGER NOT NULL,         -- 起始行（1-based）
    end_line INTEGER NOT NULL,           -- 结束行（含）
    label TEXT DEFAULT '',               -- 区域内标注（如 "入口"、"核心逻辑"）
    FOREIGN KEY (stop_id) REFERENCES code_map_stops(id) ON DELETE CASCADE,
    FOREIGN KEY (repo_id) REFERENCES repositories(repo_id) ON DELETE CASCADE
);
```

**API 端点**:

```
-- 地图 CRUD
POST   /api/admin/repos/{id}/maps                    # 创建地图
GET    /api/repos/{id}/maps                           # 列出仓库的地图
GET    /api/repos/{id}/maps/{map_id}                  # 获取地图详情（含所有 stops 和 regions）
PUT    /api/admin/repos/{id}/maps/{map_id}            # 更新地图元信息
DELETE /api/admin/repos/{id}/maps/{map_id}            # 删除地图

-- 站点管理
POST   /api/admin/maps/{map_id}/stops                 # 添加站点
PUT    /api/admin/maps/{map_id}/stops/{stop_id}       # 更新站点
DELETE /api/admin/maps/{map_id}/stops/{stop_id}       # 删除站点
POST   /api/admin/maps/{map_id}/stops/reorder         # 重排站点顺序
  Body: { "stop_ids": [3, 1, 5, 2] }

-- 区域管理
POST   /api/admin/stops/{stop_id}/regions             # 添加区域
PUT    /api/admin/stops/{stop_id}/regions/{region_id} # 更新区域
DELETE /api/admin/stops/{stop_id}/regions/{region_id} # 删除区域

-- 查询：获取文件涉及的所有地图区域（前端用于高亮）
GET /api/repos/{id}/maps/regions?file=<path>
Response:
{
  "regions": [
    {
      "map_id": 1,
      "map_name": "HTTP 请求流程",
      "stop_title": "Step 1: 路由匹配",
      "start_line": 42,
      "end_line": 58,
      "label": "路由入口",
      "color": "#4A90D9"
    },
    {
      "map_id": 1,
      "map_name": "HTTP 请求流程",
      "stop_title": "Step 3: 响应写入",
      "start_line": 120,
      "end_line": 145,
      "label": "",
      "color": "#4A90D9"
    }
  ]
}

-- 集合级地图（跨仓库）
POST   /api/admin/groups/{id}/maps                    # 创建集合级地图
GET    /api/groups/{id}/maps                           # 列出集合的地图
GET    /api/groups/{id}/maps/{map_id}                  # 获取详情
```

**典型使用场景**:
1. 开发者创建一个 "用户认证流程" 地图
2. 添加 Stop 1: "路由拦截" → `middleware/auth.go:15-30`
3. 添加 Stop 2: "Token 校验" → `service/token.go:42-65`
4. 添加 Stop 3: "用户查询" → `repository/user.go:10-25`
5. 前端在文件浏览时，通过 `/maps/regions?file=...` 获取当前文件被哪些地图引用，高亮显示
6. 前端提供地图浏览视图，按 Stop 顺序展示代码片段，形成"代码导游"

**依赖**: F1（配置）、F5（集合级地图需要 Group 概念，但单仓库地图不依赖）

---

### F10: Code Annotations（实验性）

**模块**: `internal/annotation/`

**目标**: 让用户给代码文件或代码片段附加文档注释，形成一层"知识层"叠加在代码之上。

**核心概念**:

- **Annotation**: 一条注释，绑定到某个仓库的某个文件（可选绑定行范围）
- 可用于: 架构说明、注意事项、TODO 标记、设计决策记录、onboarding 指引
- 区别于代码内注释: Annotation 不修改源代码，是外部叠加的文档层

**数据库 Schema**:

```sql
CREATE TABLE IF NOT EXISTS annotations (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    repo_id INTEGER NOT NULL,
    file_path TEXT NOT NULL,             -- 文件路径
    start_line INTEGER,                  -- 起始行（NULL = 整个文件的注释）
    end_line INTEGER,                    -- 结束行（NULL = 整个文件的注释）
    title TEXT NOT NULL,                 -- 注释标题
    content TEXT NOT NULL,               -- 注释内容（支持 Markdown）
    category TEXT DEFAULT 'note',        -- 分类: 'note' | 'todo' | 'decision' | 'warning' | 'question'
    author TEXT DEFAULT '',              -- 作者标识
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (repo_id) REFERENCES repositories(repo_id) ON DELETE CASCADE
);

CREATE INDEX idx_annotations_repo_file ON annotations(repo_id, file_path);
```

**API 端点**:

```
-- 注释 CRUD
POST   /api/admin/repos/{id}/annotations
  Body: {
    "file_path": "internal/handler.go",
    "start_line": 42,           // 可选，NULL = 整个文件
    "end_line": 58,             // 可选
    "title": "请求处理核心逻辑",
    "content": "这段代码负责...\n\n## 注意事项\n- 需要处理超时",
    "category": "note",         // note | todo | decision | warning | question
    "author": "dev@example.com"
  }

GET /api/repos/{id}/annotations?file=<path>
  # 获取指定文件的所有注释
Response:
{
  "annotations": [
    {
      "id": 1,
      "file_path": "internal/handler.go",
      "start_line": 42,
      "end_line": 58,
      "title": "请求处理核心逻辑",
      "content": "这段代码负责...",
      "category": "note",
      "author": "dev@example.com",
      "created_at": "2026-04-01T10:00:00Z",
      "updated_at": "2026-04-01T10:00:00Z"
    },
    {
      "id": 2,
      "file_path": "internal/handler.go",
      "start_line": null,
      "end_line": null,
      "title": "本文件架构说明",
      "content": "handler.go 是...",
      "category": "note",
      "author": "",
      "created_at": "...",
      "updated_at": "..."
    }
  ]
}

PUT    /api/admin/repos/{id}/annotations/{annotation_id}
DELETE /api/admin/repos/{id}/annotations/{annotation_id}

-- 查询: 列出仓库所有有注释的文件（用于文件树标记）
GET /api/repos/{id}/annotations/files
Response:
{
  "files": [
    { "path": "internal/handler.go", "count": 3 },
    { "path": "internal/service.go", "count": 1 }
  ]
}

-- 查询: 按分类过滤
GET /api/repos/{id}/annotations?category=todo
GET /api/repos/{id}/annotations?category=decision
```

**与 Code Map 的关系**:
- Code Map 的 Stop description 可以引用 Annotation
- 两者都是"叠加在代码之上的知识层"，但定位不同:
  - **Code Map**: 关注"流程"和"关联"，把分散的代码片段串成故事
  - **Annotation**: 关注"解释"和"记录"，给代码附加上下文知识
- 未来可以考虑让 Map Stop 关联 Annotation，形成更丰富的代码理解体验

**依赖**: F1（配置），独立模块，不依赖其他 Feature

---

## 5. 数据库 Schema 演进汇总

### 现有表

```sql
-- 仓库表（已有，需扩展字段）
repositories (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    repo_id INTEGER UNIQUE NOT NULL,
    name TEXT NOT NULL,
    source_path TEXT NOT NULL,
    data_path TEXT NOT NULL,
    created_at DATETIME,
    updated_at DATETIME
    -- [F3 新增] ↓
    last_indexed_at DATETIME,
    index_status TEXT DEFAULT 'none',
    remote_url TEXT,
    default_branch TEXT
);
```

### 新增表

```sql
-- [F3] 索引任务表
index_jobs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    repo_id INTEGER NOT NULL,
    type TEXT NOT NULL DEFAULT 'zoekt',
    status TEXT NOT NULL DEFAULT 'pending',
    trigger_type TEXT NOT NULL DEFAULT 'manual',
    error TEXT,
    started_at DATETIME,
    completed_at DATETIME,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (repo_id) REFERENCES repositories(repo_id)
);

-- [F5] 仓库集合表
repo_groups (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    group_id INTEGER UNIQUE NOT NULL,
    name TEXT NOT NULL,
    description TEXT DEFAULT '',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- [F5] 集合成员关联表
repo_group_members (
    group_id INTEGER NOT NULL,
    repo_id INTEGER NOT NULL,
    PRIMARY KEY (group_id, repo_id),
    FOREIGN KEY (group_id) REFERENCES repo_groups(group_id) ON DELETE CASCADE,
    FOREIGN KEY (repo_id) REFERENCES repositories(repo_id) ON DELETE CASCADE
);

-- [F9] Code Map 地图表
code_maps (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    map_id INTEGER UNIQUE NOT NULL,
    repo_id INTEGER NOT NULL,
    group_id INTEGER,
    name TEXT NOT NULL,
    description TEXT DEFAULT '',
    color TEXT DEFAULT '',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (repo_id) REFERENCES repositories(repo_id) ON DELETE CASCADE,
    FOREIGN KEY (group_id) REFERENCES repo_groups(group_id) ON DELETE SET NULL
);

-- [F9] Code Map 站点表
code_map_stops (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    map_id INTEGER NOT NULL,
    order_index INTEGER NOT NULL,
    title TEXT NOT NULL,
    description TEXT DEFAULT '',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (map_id) REFERENCES code_maps(map_id) ON DELETE CASCADE
);

-- [F9] Code Map 区域表
code_map_regions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    stop_id INTEGER NOT NULL,
    repo_id INTEGER NOT NULL,
    file_path TEXT NOT NULL,
    start_line INTEGER NOT NULL,
    end_line INTEGER NOT NULL,
    label TEXT DEFAULT '',
    FOREIGN KEY (stop_id) REFERENCES code_map_stops(id) ON DELETE CASCADE,
    FOREIGN KEY (repo_id) REFERENCES repositories(repo_id) ON DELETE CASCADE
);

-- [F10] Code Annotations 注释表
annotations (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    repo_id INTEGER NOT NULL,
    file_path TEXT NOT NULL,
    start_line INTEGER,
    end_line INTEGER,
    title TEXT NOT NULL,
    content TEXT NOT NULL,
    category TEXT DEFAULT 'note',
    author TEXT DEFAULT '',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (repo_id) REFERENCES repositories(repo_id) ON DELETE CASCADE
);
-- annotations 索引
CREATE INDEX idx_annotations_repo_file ON annotations(repo_id, file_path);
```

### 迁移策略
- 使用 `ALTER TABLE ADD COLUMN` 添加新字段（SQLite 支持）
- 新增表使用 `CREATE TABLE IF NOT EXISTS`（已有模式）
- 在 `initSchema()` 中执行所有迁移

---

## 6. 演化路线图

### Phase 1: 单机基础强化（当前）

```
┌─────────────────────────────────────────┐
│  repo-server (HTTP :8088)               │
│  ├── config.yaml (新增)                 │
│  ├── SQLite (仓库 + 集合 + 索引状态)    │
│  ├── embedded Zoekt searcher            │
│  ├── embedded Zoekt git indexer         │
│  └── 后台索引调度器                      │
│                                         │
│  外部: universal-ctags（符号索引用）     │
└─────────────────────────────────────────┘
```

**设计约束（为 Phase 2 预留）**:
1. **存储抽象**: 数据库操作集中在 `repo.Provider` 和 `group.Service` 中，不散落 SQL
2. **Zoekt 服务边界**: 通过 `search.Service` 封装 Zoekt library，避免 Zoekt 类型扩散到 handler 和 repo 层
3. **无状态 API**: 不在内存中存储会话状态，缓存仅做性能优化
4. **任务模型化**: 索引操作抽象为 `IndexJob`，可被不同触发器驱动
5. **配置外部化**: 所有可变参数通过 `config.yaml` 管理

### Phase 2: 单机优化

```
┌─────────────────────────────────────────┐
│  repo-server (单进程)                    │
│  ├── config.yaml                        │
│  ├── SQLite WAL + 优化连接池            │
│  ├── 内嵌 Zoekt Library (消除外部依赖)  │
│  ├── 后台调度器 + Webhook 监听          │
│  └── 指标采集 (Prometheus)              │
└─────────────────────────────────────────┘
```

**变更**:
- 添加 Prometheus metrics
- 优化缓存策略（LRU、内存限制）
- 支持 HTTPS / 反向代理配置

### Phase 3: 分布式集群（未来）

```
┌──────────────┐
│ API Gateway  │ (无状态，可水平扩展)
└──────┬───────┘
       │
┌──────▼───────┐     ┌──────────────┐
│ Index Worker │ × N │ Search Node  │ × N
│ (索引任务)    │     │ (Zoekt Shard)│
└──────┬───────┘     └──────┬───────┘
       │                    │
┌──────▼────────────────────▼───────┐
│     PostgreSQL + 共享存储          │
└───────────────────────────────────┘
```

**关键变更**:
- SQLite → PostgreSQL
- Zoekt shard 分布式存储
- Index Worker 和 Search Node 独立扩缩容
- 引入消息队列（Redis Streams / NATS）分发 IndexJob

---

## 7. Feature 拆分与并行策略

### 依赖关系图

```
F0 (TDD 补齐 + Zoekt-only 重构)
├── F6 (Zoekt-only 搜索重构)
│   └── F7 (Zoekt 内嵌索引与深度能力)
├── F8 (健康检查)         ← 可并行（简单）
├── F1 (配置系统)         ← 可并行，但不是内嵌 Zoekt 的前置
├── F2 (Git 版本管理)     ← 后续独立批次
├── F3 (索引状态管理)     ← 已基本落地，需适配 search.Service
│   └── F4 (索引调度器)   ← F3/F7 完成后
├── F5 (仓库集合)
│   └── 集合搜索复用 F6 的 repos[] 查询入口
├── F9 (Code Map)         ← 实验性，依赖 F5 做集合级地图
└── F10 (Annotations)     ← 实验性，无依赖
```

### 执行批次

| 批次 | Features | 说明 |
|------|----------|------|
| **Batch 1** | F0, F6, F7 | 本轮优先：补 characterization tests，删除 Ripgrep，内嵌 Zoekt 搜索和索引 |
| **Batch 2** | F8, F1 | 健康检查和配置外部化，巩固单进程部署 |
| **Batch 3** | F5, 集合搜索 | 仓库集合和基于 Zoekt repos[] 的跨仓库搜索 |
| **Batch 4** | F4 | 索引调度器和 Webhook，复用 F3/F7 的任务模型 |
| **Batch 5** | F2, F9, F10 | Git 版本管理和知识层能力，按产品优先级拆独立计划 |

### Agent 分配建议

每个 Feature 可分配给一个独立的 agent，接口契约已在 3.3 节定义。Agent 需要:

1. **遵守接口契约**: 不修改已有接口签名（只扩展），新增类型在自己的包内定义
2. **自包含**: 每个 Feature 的代码在自己包内完成；涉及 Zoekt 的逻辑集中在 `internal/search`
3. **测试**: 每个 Feature 必须先写 failing test，再写实现；重构前先补 characterization tests
4. **数据库迁移**: 在 `repo/initSchema()` 中添加新表，通过注释标记所属 Feature

---

## 8. 风险与缓解

| 风险 | 影响 | 缓解 |
|------|------|------|
| Zoekt library API 变更 | 搜索或索引编译失败 | 锁定 Zoekt 版本，所有 Zoekt 调用集中在 `internal/search` |
| 内嵌 shard watcher 生命周期 | server shutdown 或重新索引后 shard 状态不一致 | `search.Service` 提供 `Close()` 和健康检查；测试覆盖初始化、关闭、重新索引后查询 |
| SQLite 并发瓶颈 | 大量索引任务时写锁争用 | 使用 WAL 模式，索引任务串行执行 |
| 大仓库索引耗时 | Chromium 级别仓库索引可能超时 | 后台异步执行 + 状态轮询，分片索引 |
| 内存占用 | SCIP 索引 + 缓存可能占用大量内存 | 配置缓存上限，LRU 淘汰策略 |
| go-git 大仓库性能 | 大型仓库 commit 历史遍历慢 | 分页查询，限制遍历深度 |
| 测试补齐范围失控 | 重构被测试工程拖慢 | 第一批只覆盖重构触达模块和关键现有行为，低风险 UI 细节不纳入本批 |

---

## 9. 验收标准

### F0 验收（TDD 补齐 + Zoekt-only 重构）
- [ ] `internal/repo` 的 IndexJob 状态流有 characterization tests 覆盖
- [ ] `internal/core` 的仓库列表、tree、blob 行为有 characterization tests 覆盖
- [ ] `internal/search` 的 query 构造、结果转换、handler 错误路径有单元测试覆盖
- [ ] `internal/analysis` 对搜索服务的回退调用有测试覆盖
- [ ] 每个重构任务遵循 RED → GREEN → REFACTOR，计划中记录失败测试命令和预期失败
- [ ] `go test ./...` 通过

### F1 验收
- [ ] `config.yaml` 支持所有配置项
- [ ] 命令行参数可覆盖配置文件
- [ ] 不指定配置文件时使用合理默认值

### F2 验收
- [ ] 可查看任意 commit 的文件树和内容
- [ ] 可列出所有分支和标签
- [ ] 可查看 commit 历史（支持分页）
- [ ] 可查看文件的 blame 信息
- [ ] 可比较两个 commit 的 diff
- [ ] 缓存键包含 ref 信息

### F3 验收
- [ ] 索引操作创建 IndexJob 记录
- [ ] 可查询索引状态和历史
- [ ] 索引失败时记录错误信息
- [ ] 仓库表包含 remote_url 和 last_indexed_at

### F4 验收
- [ ] 定时轮询检测到远程变更时自动创建 IndexJob
- [ ] Webhook 端点可接收 GitHub push event
- [ ] 手动触发仍然可用

### F5 验收
- [ ] 可创建/删除仓库集合
- [ ] 可添加/移除集合成员
- [ ] 数据库包含 group 相关表

### F6 验收
- [ ] `internal/search` 不再包含 Ripgrep 实现
- [ ] API 不再需要 `engine` 参数，默认使用内嵌 Zoekt
- [ ] 显式传入 `engine=ripgrep` 返回 400，并说明 Ripgrep 已移除
- [ ] 搜索结果保持现有字段兼容，并支持可选 `repo_name`
- [ ] 搜索文件和搜索内容都通过 Zoekt library 查询 shard

### F7 验收
- [ ] 索引不再调用 `zoekt-git-index` 命令
- [ ] 搜索不再调用 `zoekt-webserver` HTTP API
- [ ] `start.sh` 不再启动 `zoekt-webserver`
- [ ] 可按分支搜索
- [ ] 可按文件路径过滤
- [ ] 搜索结果支持分页
- [ ] 增量索引正确工作

### F8 验收
- [ ] `/health` 端点返回各组件状态
- [ ] 内嵌 Zoekt searcher 未初始化或 shard 加载失败时正确报告

### F9 验收（实验性）
- [ ] 可创建/删除 Code Map
- [ ] 可向 Map 添加/删除/重排 Stop
- [ ] 可向 Stop 添加/删除 Region（指定文件和行范围）
- [ ] `/maps/regions?file=...` 返回文件涉及的所有区域
- [ ] 支持单仓库和集合级地图

### F10 验收（实验性）
- [ ] 可给文件/代码段创建/编辑/删除 Annotation
- [ ] 可按文件路径查询所有 Annotation
- [ ] 可按分类（note/todo/decision/warning/question）过滤
- [ ] `/annotations/files` 返回有注释的文件列表及数量
- [ ] 文件级注释（无行范围）和行范围注释都支持
