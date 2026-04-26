# Admin Index Scheduling UI Design

> Date: 2026-04-26
> Status: Approved for planning
> Scope: Wire the existing indexing job APIs into `web/admin.html` without adding a new page.

## Context

The embedded Zoekt worktree already exposes the backend pieces needed for manual indexing visibility:

- `POST /api/repositories/{id}/index` creates an asynchronous Zoekt index job and returns `202 Accepted` with `job_id`.
- `GET /api/admin/repositories/{id}/index-status` returns a repository's latest indexing status and recent jobs.
- `GET /api/admin/index-jobs?limit=20` returns recent indexing jobs across repositories.
- `GET /api/admin/repositories` includes repository `index_status`.

The admin page should make this existing scheduling flow visible. This spec intentionally avoids building scheduler polling, webhook configuration, retry controls, or a separate index operations page.

## Goals

- Show each repository's current `index_status` in the existing repository table.
- Provide a manual "index" action per repository that calls the existing job creation API.
- Show a global recent index jobs table on `admin.html`.
- Refresh repository status and job history after creating an index job.
- Keep the implementation small and compatible with the current static admin page.

## Non-Goals

- No new backend endpoints.
- No dedicated index jobs page.
- No job retry, cancellation, or deletion.
- No scheduler/webhook controls.
- No live websocket or server-sent event updates.
- No large frontend framework migration.

## UI Design

Use the lightweight admin-page integration selected during brainstorming:

- Keep the existing "Add Repository" card.
- Extend the repository table with an index status column.
- Add an "Index" button in each repository row.
- Add a new "Index Jobs" card below the repository list.
- Keep the existing feedback card below the index jobs card.

The index jobs table displays:

- Job ID
- Repository label
- Type
- Trigger
- Status
- Created time
- Error

Repository labels should prefer `repo.name (repo_id)` by matching `job.repo_id` against the current repository list. If the repository is not loaded or has been deleted, display the numeric `repo_id`.

Errors should be truncated visually in the table and exposed through the cell `title` attribute so the full message remains inspectable without adding a modal.

## Data Flow

On dashboard load:

1. Fetch `/api/admin/repositories`.
2. Render the repository table.
3. Fetch `/api/admin/index-jobs?limit=20`.
4. Render the recent job table.
5. Fetch feedback as before.

On manual index trigger:

1. Confirm the user wants to create an index job.
2. Call `POST /api/repositories/{id}/index`.
3. Show a toast including the returned `job_id`.
4. Refresh repositories so `index_status` can update.
5. Refresh recent index jobs.

The page should not poll automatically in this first version. Manual refresh buttons are enough for the current scope.

## Error Handling

- Preserve the existing admin token behavior and `fetchAPI` error handling.
- If `/api/admin/index-jobs` fails, show the existing toast error and leave the previous table state in place.
- If no jobs are returned, render a single empty-state row.
- If `created_at` is missing or not parseable, render the original value or an empty string.

## Testing

Use the current static HTML test style for this narrow integration:

- Verify `admin.html` contains the index jobs table body.
- Verify it calls `/api/admin/index-jobs?limit=20`.
- Verify it has an `actions.triggerIndex` path using `/repositories/${id}/index`.
- Verify the page includes visible "Index Jobs" copy.

This is intentionally a characterization-style guard. If admin JavaScript continues growing, a later refactor should extract the script into a testable JS module.

## Implementation Notes

- Keep changes scoped to `web/admin.html` and `web/admin_html_test.go`.
- Do not change backend APIs for this UI integration.
- Do not include visual companion artifacts under `.superpowers/` in commits.
