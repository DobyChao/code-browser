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
		"fetchAPI(`/repositories/${id}/index`, { method: 'POST' })",
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
