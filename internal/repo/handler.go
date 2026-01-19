package repo

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

type Handlers struct {
	Provider   *Provider
	AdminToken string
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
		ID   uint32 `json:"id"`
		Name string `json:"name"`
		Path string `json:"path"`
	}

	var infos []AdminRepoInfo
	for _, repo := range repos {
		infos = append(infos, AdminRepoInfo{
			ID:   repo.RepoID,
			Name: repo.Name,
			Path: repo.SourcePath,
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

	// Async indexing
	go func() {
		if err := h.Provider.IndexRepositoryZoekt(uint32(id)); err != nil {
			fmt.Printf("Async index error for repo %d: %v\n", id, err)
		}
	}()

	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{"status": "indexing started"})
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

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
