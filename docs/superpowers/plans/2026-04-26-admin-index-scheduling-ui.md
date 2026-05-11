# Admin Index Scheduling UI Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Wire the existing index job scheduling APIs into `web/admin.html` with visible job status, manual triggering, duplicate-submit protection, and focused characterization tests.

**Architecture:** Keep the admin UI as a static HTML page with inline JavaScript. Use the existing `fetchAPI` helper for all authenticated calls, render a recent index jobs table from `/api/admin/index-jobs?limit=20`, and use delegated click handling for repository-row index actions so refreshed table rows do not need listener rebinding.

**Tech Stack:** Go static-file tests, native browser JavaScript, existing HTML/Tailwind admin page, existing backend repo index APIs.

---

## File Structure

- Modify `web/admin.html`: add the index status column, recent index jobs card, manual refresh button, delegated index trigger handling, in-flight button state, status pills, and date formatting.
- Create or modify `web/admin_html_test.go`: static characterization tests that guard the HTML and inline JavaScript integration points.
- Do not modify backend routes. The approved design uses existing endpoints only.
- Do not commit `.superpowers/` visual companion artifacts.

## Current Worktree Note

At plan-writing time, `web/admin.html` and `web/admin_html_test.go` already have uncommitted partial changes. Treat them as user/work-in-progress edits. Do not revert them. Strengthen and adjust them to satisfy this plan.

---

### Task 1: Strengthen Admin HTML Characterization Test

**Files:**
- Create/Modify: `web/admin_html_test.go`
- Read: `docs/superpowers/specs/2026-04-26-admin-index-scheduling-ui-design.md`

- [ ] **Step 1: Write the failing test**

Replace or extend `web/admin_html_test.go` with this test file:

```go
package web

import (
	"os"
	"strings"
	"testing"
)

func TestAdminPageUsesIndexJobSchedulingAPI(t *testing.T) {
	body, err := os.ReadFile("admin.html")
	if err != nil {
		t.Fatalf("read admin.html: %v", err)
	}
	html := string(body)

	required := []string{
		`id="index-job-table-body"`,
		`id="refresh-index-jobs-btn"`,
		`fetchIndexJobs`,
		`/admin/index-jobs?limit=20`,
		`data-action="index"`,
		`data-repo-id="${repo.id}"`,
		`repoTableBody.addEventListener('click'`,
		`button.disabled = true`,
		`button.dataset.originalText`,
		`button.textContent = '创建中...'`,
		`fetchAPI(`/ + "`" + `/repositories/${id}/index` + "`" + `, { method: 'POST' })`,
		`await fetchRepos()`,
		`await fetchIndexJobs()`,
		`button.disabled = false`,
		`索引任务`,
	}
	for _, needle := range required {
		if !strings.Contains(html, needle) {
			t.Fatalf("admin.html does not contain %q", needle)
		}
	}
}

func TestAdminPageIndexSchedulingAvoidsInlineIndexHandler(t *testing.T) {
	body, err := os.ReadFile("admin.html")
	if err != nil {
		t.Fatalf("read admin.html: %v", err)
	}
	html := string(body)

	forbidden := []string{
		`onclick="actions.triggerIndex`,
		`onclick='actions.triggerIndex`,
	}
	for _, needle := range forbidden {
		if strings.Contains(html, needle) {
			t.Fatalf("admin.html should use delegated index handling, found %q", needle)
		}
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run:

```bash
go test ./web -run TestAdminPage -count=1
```

Expected: FAIL because the current partial implementation still uses inline `onclick="actions.triggerIndex(...)"` and does not disable the triggering button while the request is in flight.

- [ ] **Step 3: Commit the failing test is not allowed**

Do not commit at RED. Continue to Task 2.

---

### Task 2: Implement Delegated Index Trigger UI

**Files:**
- Modify: `web/admin.html`
- Test: `web/admin_html_test.go`

- [ ] **Step 1: Update the repository row index button**

In `renderRepos()`, replace the inline index button:

```html
<button onclick="actions.triggerIndex(${repo.id})" class="text-xs bg-green-600 hover:bg-green-700 text-white px-2 py-1 rounded">索引</button>
```

with this delegated-action button:

```html
<button
    type="button"
    data-action="index"
    data-repo-id="${repo.id}"
    class="text-xs bg-green-600 hover:bg-green-700 disabled:bg-gray-600 disabled:cursor-not-allowed text-white px-2 py-1 rounded"
>索引</button>
```

Keep the row-level `onclick="actions.viewDetails(${repo.id})"` and the operation-cell `onclick="event.stopPropagation()"` behavior unless a broader admin cleanup is explicitly requested.

- [ ] **Step 2: Add delegated click handling**

After the existing refresh button event listeners:

```js
dom.refreshBtn.addEventListener('click', fetchRepos);
dom.refreshIndexJobsBtn.addEventListener('click', fetchIndexJobs);
dom.refreshFeedbackBtn.addEventListener('click', fetchFeedbacks);
```

add this delegated handler:

```js
dom.repoTableBody.addEventListener('click', (event) => {
    const button = event.target.closest('[data-action="index"]');
    if (!button) return;
    event.preventDefault();
    event.stopPropagation();
    actions.triggerIndex(parseInt(button.dataset.repoId, 10), button);
});
```

- [ ] **Step 3: Update `actions.triggerIndex` to protect against duplicate submits**

Replace the current `actions.triggerIndex(id)` method with:

```js
async triggerIndex(id, button) {
    if (!confirm(`确定要为仓库 ID ${id} 创建索引任务吗？`)) return;
    if (button) {
        button.disabled = true;
        button.dataset.originalText = button.textContent;
        button.textContent = '创建中...';
    }
    try {
        const res = await fetchAPI(`/repositories/${id}/index`, { method: 'POST' });
        const data = await res.json();
        showToast(`索引任务已创建: #${data.job_id}`);
        await fetchRepos();
        await fetchIndexJobs();
    } catch (e) {
        // handled by fetchAPI
    } finally {
        if (button) {
            button.disabled = false;
            button.textContent = button.dataset.originalText || '索引';
            delete button.dataset.originalText;
        }
    }
},
```

This keeps the existing `fetchAPI` path, so the non-admin-prefixed job creation route still receives the `Authorization` header.

- [ ] **Step 4: Run the focused web test**

Run:

```bash
go test ./web -run TestAdminPage -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

Run:

```bash
git add web/admin.html web/admin_html_test.go
git commit -m "feat(admin): show and trigger index jobs"
```

---

### Task 3: Verify Full Backend/Frontend Compatibility

**Files:**
- Read: `web/admin.html`
- Read: `internal/repo/handler.go`
- Read: `cmd/server/main.go`

- [ ] **Step 1: Verify backend route compatibility by inspection**

Confirm these routes still exist in `cmd/server/main.go`:

```go
mux.HandleFunc("POST /api/repositories/{id}/index", repoHandlers.AuthMiddleware(repoHandlers.HandleIndex))
mux.HandleFunc("GET /api/admin/index-jobs", repoHandlers.AuthMiddleware(repoHandlers.HandleListIndexJobs))
```

Confirm `web/admin.html` uses:

```js
fetchAPI(`/repositories/${id}/index`, { method: 'POST' })
fetchAPI('/admin/index-jobs?limit=20')
```

The `fetchAPI` helper prepends `/api`, so these map to the expected backend routes and carry the admin token.

- [ ] **Step 2: Run package tests**

Run:

```bash
go test ./web ./internal/repo ./cmd/... -count=1
```

Expected: PASS.

- [ ] **Step 3: Run full test suite**

Run:

```bash
go test ./... -count=1
```

Expected: PASS.

- [ ] **Step 4: Check final diff excludes visual companion artifacts**

Run:

```bash
git status --short
```

Expected: no tracked changes except any intentionally untracked `.superpowers/` directory. Do not add `.superpowers/`.

---

## Self-Review

**Spec coverage:** This plan covers repository `index_status`, per-repo manual indexing, global recent jobs, refresh after creation, duplicate-submit protection, delegated event handling, authorization through `fetchAPI`, manual job refresh, and focused tests.

**Scope:** This plan does not add backend routes, scheduler polling, webhook controls, retry/cancel/delete actions, or a separate jobs page.

**Type consistency:** The plan uses the existing response shape `data.jobs`, repository field `repo.id`, job field `job.repo_id`, and backend route prefixes established in `cmd/server/main.go`.
