package repo

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func setupTestProvider(t *testing.T) *Provider {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "code-browser-test-*")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(tmpDir) })
	p, err := NewProvider(tmpDir)
	if err != nil {
		t.Fatalf("创建 Provider 失败: %v", err)
	}
	t.Cleanup(func() { p.Close() })
	return p
}

// addTestRepo is a helper that adds a repository using the provider's DataDir
// as the source path (a valid existing directory).
func addTestRepo(t *testing.T, p *Provider, id uint32, name string) {
	t.Helper()
	reposDir := filepath.Join(p.DataDir, reposSubDir)
	if err := p.AddRepository(id, name, reposDir); err != nil {
		t.Fatalf("添加测试仓库失败: %v", err)
	}
}

func TestCreateIndexJob(t *testing.T) {
	p := setupTestProvider(t)
	addTestRepo(t, p, 1, "test-repo")

	jobID, err := p.CreateIndexJob(1, "zoekt", "manual")
	if err != nil {
		t.Fatalf("CreateIndexJob 失败: %v", err)
	}
	if jobID == 0 {
		t.Fatal("期望 jobID > 0, 实际得到 0")
	}

	// 验证任务确实存在且状态正确
	job, err := p.GetIndexJob(jobID)
	if err != nil {
		t.Fatalf("GetIndexJob 失败: %v", err)
	}
	if job == nil {
		t.Fatal("期望任务不为 nil")
	}
	if job.RepoID != 1 {
		t.Errorf("期望 RepoID=1, 实际=%d", job.RepoID)
	}
	if job.Type != "zoekt" {
		t.Errorf("期望 Type='zoekt', 实际='%s'", job.Type)
	}
	if job.Status != JobStatusPending {
		t.Errorf("期望 Status='pending', 实际='%s'", job.Status)
	}
	if job.TriggerType != "manual" {
		t.Errorf("期望 TriggerType='manual', 实际='%s'", job.TriggerType)
	}
	if job.StartedAt != nil {
		t.Error("期望 StartedAt 为 nil (pending 状态)")
	}
	if job.CompletedAt != nil {
		t.Error("期望 CompletedAt 为 nil (pending 状态)")
	}
}

func TestUpdateJobStatus_Running(t *testing.T) {
	p := setupTestProvider(t)
	addTestRepo(t, p, 1, "test-repo")

	jobID, err := p.CreateIndexJob(1, "zoekt", "manual")
	if err != nil {
		t.Fatalf("CreateIndexJob 失败: %v", err)
	}

	if err := p.UpdateJobStatus(jobID, JobStatusRunning, ""); err != nil {
		t.Fatalf("UpdateJobStatus(running) 失败: %v", err)
	}

	job, err := p.GetIndexJob(jobID)
	if err != nil {
		t.Fatalf("GetIndexJob 失败: %v", err)
	}
	if job.Status != JobStatusRunning {
		t.Errorf("期望 Status='running', 实际='%s'", job.Status)
	}
	if job.StartedAt == nil {
		t.Fatal("期望 StartedAt 不为 nil (running 状态)")
	}
}

func TestUpdateJobStatus_Completed(t *testing.T) {
	p := setupTestProvider(t)
	addTestRepo(t, p, 1, "test-repo")

	jobID, err := p.CreateIndexJob(1, "zoekt", "manual")
	if err != nil {
		t.Fatalf("CreateIndexJob 失败: %v", err)
	}

	// pending -> running -> completed
	if err := p.UpdateJobStatus(jobID, JobStatusRunning, ""); err != nil {
		t.Fatalf("UpdateJobStatus(running) 失败: %v", err)
	}
	if err := p.UpdateJobStatus(jobID, JobStatusCompleted, ""); err != nil {
		t.Fatalf("UpdateJobStatus(completed) 失败: %v", err)
	}

	job, err := p.GetIndexJob(jobID)
	if err != nil {
		t.Fatalf("GetIndexJob 失败: %v", err)
	}
	if job.Status != JobStatusCompleted {
		t.Errorf("期望 Status='completed', 实际='%s'", job.Status)
	}
	if job.CompletedAt == nil {
		t.Fatal("期望 CompletedAt 不为 nil (completed 状态)")
	}

	// 验证仓库 index_status 更新为 "indexed"
	status, err := p.GetLatestIndexStatus(1)
	if err != nil {
		t.Fatalf("GetLatestIndexStatus 失败: %v", err)
	}
	if status != "indexed" {
		t.Errorf("期望仓库 index_status='indexed', 实际='%s'", status)
	}
}

func TestUpdateJobStatus_Failed(t *testing.T) {
	p := setupTestProvider(t)
	addTestRepo(t, p, 1, "test-repo")

	jobID, err := p.CreateIndexJob(1, "zoekt", "manual")
	if err != nil {
		t.Fatalf("CreateIndexJob 失败: %v", err)
	}

	// pending -> running -> failed
	if err := p.UpdateJobStatus(jobID, JobStatusRunning, ""); err != nil {
		t.Fatalf("UpdateJobStatus(running) 失败: %v", err)
	}
	if err := p.UpdateJobStatus(jobID, JobStatusFailed, "something went wrong"); err != nil {
		t.Fatalf("UpdateJobStatus(failed) 失败: %v", err)
	}

	job, err := p.GetIndexJob(jobID)
	if err != nil {
		t.Fatalf("GetIndexJob 失败: %v", err)
	}
	if job.Status != JobStatusFailed {
		t.Errorf("期望 Status='failed', 实际='%s'", job.Status)
	}
	if job.Error != "something went wrong" {
		t.Errorf("期望 Error='something went wrong', 实际='%s'", job.Error)
	}

	// 验证仓库 index_status 更新为 "failed"
	status, err := p.GetLatestIndexStatus(1)
	if err != nil {
		t.Fatalf("GetLatestIndexStatus 失败: %v", err)
	}
	if status != "failed" {
		t.Errorf("期望仓库 index_status='failed', 实际='%s'", status)
	}
}

func TestListIndexJobs(t *testing.T) {
	p := setupTestProvider(t)
	addTestRepo(t, p, 1, "test-repo")

	// 创建 3 个任务, 每次间隔 1s 以确保 created_at 不同
	id1, _ := p.CreateIndexJob(1, "zoekt", "manual")
	time.Sleep(1 * time.Second)
	id2, _ := p.CreateIndexJob(1, "scip", "api")
	time.Sleep(1 * time.Second)
	id3, _ := p.CreateIndexJob(1, "zoekt", "cron")

	// 不使用 limit 限制
	jobs, err := p.ListIndexJobs(1, 100)
	if err != nil {
		t.Fatalf("ListIndexJobs 失败: %v", err)
	}
	if len(jobs) != 3 {
		t.Fatalf("期望 3 个任务, 实际=%d", len(jobs))
	}

	// 验证排序: 最新的在前 (created_at DESC)
	if jobs[0].ID != id3 {
		t.Errorf("期望第一个任务 ID=%d, 实际=%d", id3, jobs[0].ID)
	}
	if jobs[1].ID != id2 {
		t.Errorf("期望第二个任务 ID=%d, 实际=%d", id2, jobs[1].ID)
	}
	if jobs[2].ID != id1 {
		t.Errorf("期望第三个任务 ID=%d, 实际=%d", id1, jobs[2].ID)
	}

	// 测试 limit
	limited, err := p.ListIndexJobs(1, 2)
	if err != nil {
		t.Fatalf("ListIndexJobs(limit=2) 失败: %v", err)
	}
	if len(limited) != 2 {
		t.Fatalf("期望 2 个任务 (limit), 实际=%d", len(limited))
	}
	if limited[0].ID != id3 || limited[1].ID != id2 {
		t.Errorf("limit 结果排序不正确: [%d, %d], 期望 [%d, %d]",
			limited[0].ID, limited[1].ID, id3, id2)
	}
}

type fakeIndexRunner struct {
	err    error
	called chan Repository
}

func (f *fakeIndexRunner) IndexRepository(ctx context.Context, repository Repository) error {
	f.called <- repository
	return f.err
}

func TestRunIndexJobAsyncUsesInjectedRunner(t *testing.T) {
	p := setupTestProvider(t)
	addTestRepo(t, p, 1, "test-repo")
	runner := &fakeIndexRunner{called: make(chan Repository, 1)}
	p.SetIndexRunner(runner)

	jobID, err := p.RunIndexJobAsync(1, "zoekt", "manual")
	if err != nil {
		t.Fatalf("RunIndexJobAsync failed: %v", err)
	}

	select {
	case got := <-runner.called:
		if got.RepoID != 1 {
			t.Fatalf("expected repo 1, got %d", got.RepoID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("index runner was not called")
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		job, err := p.GetIndexJob(jobID)
		if err != nil {
			t.Fatalf("GetIndexJob failed: %v", err)
		}
		if job.Status == JobStatusCompleted {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("job did not complete")
}

func TestRunIndexJobAsyncRecordsRunnerFailure(t *testing.T) {
	p := setupTestProvider(t)
	addTestRepo(t, p, 1, "test-repo")
	runner := &fakeIndexRunner{err: errors.New("index failed"), called: make(chan Repository, 1)}
	p.SetIndexRunner(runner)

	jobID, err := p.RunIndexJobAsync(1, "zoekt", "manual")
	if err != nil {
		t.Fatalf("RunIndexJobAsync failed: %v", err)
	}
	<-runner.called

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		job, err := p.GetIndexJob(jobID)
		if err != nil {
			t.Fatalf("GetIndexJob failed: %v", err)
		}
		if job.Status == JobStatusFailed {
			if job.Error != "index failed" {
				t.Fatalf("expected error to be recorded, got %q", job.Error)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("job did not fail")
}

func TestGetIndexJob_NotFound(t *testing.T) {
	p := setupTestProvider(t)

	job, err := p.GetIndexJob(99999)
	if err != nil {
		t.Fatalf("GetIndexJob 对不存在的 ID 不应返回错误: %v", err)
	}
	if job != nil {
		t.Fatal("期望 job 为 nil (不存在的 ID)")
	}
}

func TestRepositoryMigration_NewFields(t *testing.T) {
	p := setupTestProvider(t)
	addTestRepo(t, p, 1, "test-repo")

	repo, ok := p.GetRepo(1)
	if !ok {
		t.Fatal("期望找到仓库 ID=1")
	}

	if repo.IndexStatus != "none" {
		t.Errorf("期望默认 IndexStatus='none', 实际='%s'", repo.IndexStatus)
	}
	if repo.RemoteURL != "" {
		t.Errorf("期望默认 RemoteURL='', 实际='%s'", repo.RemoteURL)
	}
	if repo.DefaultBranch != "" {
		t.Errorf("期望默认 DefaultBranch='', 实际='%s'", repo.DefaultBranch)
	}
	if repo.LastIndexedAt != nil {
		t.Error("期望默认 LastIndexedAt 为 nil")
	}
}
