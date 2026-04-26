package repo

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/patrickmn/go-cache"
)

type Handlers struct {
	Provider   *Provider
	AdminToken string
	Cache      *cache.Cache
}

// AuthMiddleware checks for the correct admin token
func (h *Handlers) AuthMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if h.AdminToken != "" {
			authHeader := r.Header.Get("Authorization")
			if !strings.HasPrefix(authHeader, "Bearer ") {
				http.Error(w, "Unauthorized: Missing or invalid token", http.StatusUnauthorized)
				return
			}
			token := strings.TrimPrefix(authHeader, "Bearer ")
			if token != h.AdminToken {
				http.Error(w, "Unauthorized: Invalid token", http.StatusUnauthorized)
				return
			}
		}
		next(w, r)
	}
}

// HandleAdd handles POST /api/repositories
func (h *Handlers) HandleAdd(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID   uint32 `json:"id"`
		Name string `json:"name"`
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := h.Provider.AddRepository(req.ID, req.Name, req.Path); err != nil {
		http.Error(w, fmt.Sprintf("Failed to add repo: %v", err), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// HandleListAdmin handles GET /api/admin/repositories
// Returns full repository details including path (Protected)
func (h *Handlers) HandleListAdmin(w http.ResponseWriter, r *http.Request) {
	repos := h.Provider.GetAll()

	type AdminRepoInfo struct {
		ID            uint32 `json:"id"`
		Name          string `json:"name"`
		Path          string `json:"path"`
		RemoteURL     string `json:"remote_url"`
		DefaultBranch string `json:"default_branch"`
		IndexStatus   string `json:"index_status"`
	}

	var infos []AdminRepoInfo
	for _, repo := range repos {
		infos = append(infos, AdminRepoInfo{
			ID:            repo.RepoID,
			Name:          repo.Name,
			Path:          repo.SourcePath,
			RemoteURL:     repo.RemoteURL,
			DefaultBranch: repo.DefaultBranch,
			IndexStatus:   repo.IndexStatus,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(infos)
}

// HandleDelete handles DELETE /api/repositories/{id}
func (h *Handlers) HandleDelete(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	if err := h.Provider.DeleteRepository(uint32(id)); err != nil {
		http.Error(w, fmt.Sprintf("Failed to delete repo: %v", err), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// HandleIndex handles POST /api/repositories/{id}/index
func (h *Handlers) HandleIndex(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	jobID, err := h.Provider.RunIndexJobAsync(uint32(id), "zoekt", "manual")
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to start indexing: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]any{
		"status": "indexing started",
		"job_id": jobID,
	})
}

// HandleRegisterScip handles POST /api/repositories/{id}/scip
func (h *Handlers) HandleRegisterScip(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	var req struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := h.Provider.RegisterScipIndex(uint32(id), req.Path); err != nil {
		http.Error(w, fmt.Sprintf("Failed to register SCIP: %v", err), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// HandleRegisterZoekt handles POST /api/repositories/{id}/zoekt-file
func (h *Handlers) HandleRegisterZoekt(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	var req struct {
		Paths []string `json:"paths"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := h.Provider.RegisterZoektIndex(uint32(id), req.Paths); err != nil {
		http.Error(w, fmt.Sprintf("Failed to register Zoekt file: %v", err), http.StatusInternalServerError)
		return
	}
	if h.Cache != nil {
		h.Cache.Flush()
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// HandleIndexStatus handles GET /api/admin/repositories/{id}/index-status
func (h *Handlers) HandleIndexStatus(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	status, err := h.Provider.GetLatestIndexStatus(uint32(id))
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get status: %v", err), http.StatusInternalServerError)
		return
	}

	jobs, err := h.Provider.ListIndexJobs(uint32(id), 10)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to list jobs: %v", err), http.StatusInternalServerError)
		return
	}

	repoInfo, ok := h.Provider.GetRepo(uint32(id))
	lastIndexedAt := ""
	if ok && repoInfo.LastIndexedAt != nil {
		lastIndexedAt = repoInfo.LastIndexedAt.UTC().Format(time.RFC3339)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"repo_id":         idStr,
		"status":          status,
		"last_indexed_at": lastIndexedAt,
		"jobs":            jobs,
	})
}

// HandleListIndexJobs handles GET /api/admin/index-jobs with optional ?repo_id=&limit= query params
func (h *Handlers) HandleListIndexJobs(w http.ResponseWriter, r *http.Request) {
	var repoID *uint32
	if repoIDStr := r.URL.Query().Get("repo_id"); repoIDStr != "" {
		parsed, err := strconv.ParseUint(repoIDStr, 10, 32)
		if err != nil {
			http.Error(w, "Invalid repo_id parameter", http.StatusBadRequest)
			return
		}
		rid := uint32(parsed)
		repoID = &rid
	}

	limit := 50
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		parsed, err := strconv.Atoi(limitStr)
		if err != nil || parsed <= 0 {
			http.Error(w, "Invalid limit parameter", http.StatusBadRequest)
			return
		}
		limit = parsed
	}

	jobs, err := h.Provider.ListAllIndexJobs(repoID, limit)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to list index jobs: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"jobs": jobs,
	})
}
