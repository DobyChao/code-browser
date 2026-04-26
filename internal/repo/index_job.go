package repo

import (
	"database/sql"
	"fmt"
	"time"
)

// IndexJobStatus 表示索引任务的状态
type IndexJobStatus string

const (
	JobStatusPending   IndexJobStatus = "pending"
	JobStatusRunning   IndexJobStatus = "running"
	JobStatusCompleted IndexJobStatus = "completed"
	JobStatusFailed    IndexJobStatus = "failed"
)

// IndexJob 表示一个索引任务
type IndexJob struct {
	ID          uint32         `json:"id"`
	RepoID      uint32         `json:"repo_id"`
	Type        string         `json:"type"`
	Status      IndexJobStatus `json:"status"`
	TriggerType string         `json:"trigger_type"`
	Error       string         `json:"error,omitempty"`
	StartedAt   *time.Time     `json:"started_at,omitempty"`
	CompletedAt *time.Time     `json:"completed_at,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
}

// CreateIndexJob 创建一个新的索引任务 (状态为 pending)
func (p *Provider) CreateIndexJob(repoID uint32, jobType, triggerType string) (uint32, error) {
	result, err := p.db.Exec(
		`INSERT INTO index_jobs (repo_id, type, status, trigger_type) VALUES (?, ?, ?, ?)`,
		repoID, jobType, JobStatusPending, triggerType,
	)
	if err != nil {
		return 0, fmt.Errorf("创建索引任务失败: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("获取索引任务 ID 失败: %w", err)
	}
	return uint32(id), nil
}

// UpdateJobStatus 更新索引任务的状态
// 当状态为 completed 或 failed 时，同时更新 repositories 表的 index_status 和 last_indexed_at
func (p *Provider) UpdateJobStatus(jobID uint32, status IndexJobStatus, errMsg string) error {
	tx, err := p.db.Begin()
	if err != nil {
		return fmt.Errorf("开启事务失败: %w", err)
	}
	defer tx.Rollback()

	// 更新 index_jobs 表
	if status == JobStatusRunning {
		_, err = tx.Exec(
			`UPDATE index_jobs SET status = ?, started_at = CURRENT_TIMESTAMP WHERE id = ?`,
			status, jobID,
		)
	} else {
		_, err = tx.Exec(
			`UPDATE index_jobs SET status = ?, error = ?, completed_at = CURRENT_TIMESTAMP WHERE id = ?`,
			status, errMsg, jobID,
		)
	}
	if err != nil {
		return fmt.Errorf("更新索引任务状态失败: %w", err)
	}

	// 当任务完成或失败时，更新 repositories 表
	if status == JobStatusCompleted || status == JobStatusFailed {
		// 先获取 repo_id
		var repoID uint32
		if err := tx.QueryRow(`SELECT repo_id FROM index_jobs WHERE id = ?`, jobID).Scan(&repoID); err != nil {
			return fmt.Errorf("获取索引任务的 repo_id 失败: %w", err)
		}

		repoStatus := string(status)
		if status == JobStatusCompleted {
			repoStatus = "indexed"
		}

		_, err = tx.Exec(
			`UPDATE repositories SET index_status = ?, last_indexed_at = CURRENT_TIMESTAMP WHERE repo_id = ?`,
			repoStatus, repoID,
		)
		if err != nil {
			return fmt.Errorf("更新仓库索引状态失败: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	if status == JobStatusCompleted || status == JobStatusFailed {
		if err := p.loadReposFromDB(); err != nil {
			return fmt.Errorf("刷新仓库缓存失败: %w", err)
		}
	}
	return nil
}

// GetIndexJob 根据 ID 获取索引任务，未找到时返回 nil, nil
func (p *Provider) GetIndexJob(jobID uint32) (*IndexJob, error) {
	var job IndexJob
	var startedAt, completedAt sql.NullTime
	var errMsg sql.NullString

	err := p.db.QueryRow(
		`SELECT id, repo_id, type, status, trigger_type, error, started_at, completed_at, created_at FROM index_jobs WHERE id = ?`,
		jobID,
	).Scan(&job.ID, &job.RepoID, &job.Type, &job.Status, &job.TriggerType, &errMsg, &startedAt, &completedAt, &job.CreatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("查询索引任务失败: %w", err)
	}

	if errMsg.Valid {
		job.Error = errMsg.String
	}
	if startedAt.Valid {
		job.StartedAt = &startedAt.Time
	}
	if completedAt.Valid {
		job.CompletedAt = &completedAt.Time
	}

	return &job, nil
}

// ListIndexJobs 获取指定仓库的索引任务列表，按 created_at DESC 排序
func (p *Provider) ListIndexJobs(repoID uint32, limit int) ([]IndexJob, error) {
	rows, err := p.db.Query(
		`SELECT id, repo_id, type, status, trigger_type, error, started_at, completed_at, created_at
		 FROM index_jobs WHERE repo_id = ? ORDER BY created_at DESC LIMIT ?`,
		repoID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("查询索引任务列表失败: %w", err)
	}
	defer rows.Close()

	var jobs []IndexJob
	for rows.Next() {
		var job IndexJob
		var startedAt, completedAt sql.NullTime
		var errMsg sql.NullString

		if err := rows.Scan(&job.ID, &job.RepoID, &job.Type, &job.Status, &job.TriggerType, &errMsg, &startedAt, &completedAt, &job.CreatedAt); err != nil {
			return nil, fmt.Errorf("扫描索引任务行失败: %w", err)
		}

		if errMsg.Valid {
			job.Error = errMsg.String
		}
		if startedAt.Valid {
			job.StartedAt = &startedAt.Time
		}
		if completedAt.Valid {
			job.CompletedAt = &completedAt.Time
		}

		jobs = append(jobs, job)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("迭代索引任务行失败: %w", err)
	}

	return jobs, nil
}

// ListAllIndexJobs 获取所有仓库的索引任务列表（支持按 repo_id 可选过滤），按 created_at DESC 排序
func (p *Provider) ListAllIndexJobs(repoID *uint32, limit int) ([]IndexJob, error) {
	var rows *sql.Rows
	var err error

	if repoID != nil {
		rows, err = p.db.Query(
			`SELECT id, repo_id, type, status, trigger_type, error, started_at, completed_at, created_at
			 FROM index_jobs WHERE repo_id = ? ORDER BY created_at DESC LIMIT ?`,
			*repoID, limit,
		)
	} else {
		rows, err = p.db.Query(
			`SELECT id, repo_id, type, status, trigger_type, error, started_at, completed_at, created_at
			 FROM index_jobs ORDER BY created_at DESC LIMIT ?`,
			limit,
		)
	}
	if err != nil {
		return nil, fmt.Errorf("查询索引任务列表失败: %w", err)
	}
	defer rows.Close()

	var jobs []IndexJob
	for rows.Next() {
		var job IndexJob
		var startedAt, completedAt sql.NullTime
		var errMsg sql.NullString

		if err := rows.Scan(&job.ID, &job.RepoID, &job.Type, &job.Status, &job.TriggerType, &errMsg, &startedAt, &completedAt, &job.CreatedAt); err != nil {
			return nil, fmt.Errorf("扫描索引任务行失败: %w", err)
		}

		if errMsg.Valid {
			job.Error = errMsg.String
		}
		if startedAt.Valid {
			job.StartedAt = &startedAt.Time
		}
		if completedAt.Valid {
			job.CompletedAt = &completedAt.Time
		}

		jobs = append(jobs, job)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("迭代索引任务行失败: %w", err)
	}

	return jobs, nil
}

// GetLatestIndexStatus 返回指定仓库的最新索引状态
// 如果仓库没有 index_status 字段，返回 "none"
func (p *Provider) GetLatestIndexStatus(repoID uint32) (string, error) {
	var status sql.NullString
	err := p.db.QueryRow(
		`SELECT COALESCE(index_status, 'none') FROM repositories WHERE repo_id = ?`,
		repoID,
	).Scan(&status)
	if err != nil {
		if err == sql.ErrNoRows {
			return "none", nil
		}
		return "", fmt.Errorf("查询仓库索引状态失败: %w", err)
	}
	if !status.Valid {
		return "none", nil
	}
	return status.String, nil
}
