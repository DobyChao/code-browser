package feedback

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

type Handler struct {
	Service    *Service
	AdminToken string
}

func NewHandler(s *Service, adminToken string) *Handler {
	return &Handler{
		Service:    s,
		AdminToken: adminToken,
	}
}

// AuthMiddleware checks for the correct admin token
func (h *Handler) AuthMiddleware(next http.HandlerFunc) http.HandlerFunc {
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

func (h *Handler) HandleSubmit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var f Feedback
	if err := json.NewDecoder(r.Body).Decode(&f); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Invalid request body"})
		return
	}

	if f.Title == "" || f.Description == "" || f.Type == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Title, description and type are required"})
		return
	}

	if err := h.Service.SaveFeedback(&f); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Internal server error"})
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Feedback received",
	})
}

// HandleList handles GET /api/admin/feedbacks
func (h *Handler) HandleList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	feedbacks, err := h.Service.ListFeedbacks()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to list feedbacks: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(feedbacks)
}

// HandleUpdateStatus handles PATCH /api/admin/feedbacks/{id}
func (h *Handler) HandleUpdateStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	var req struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Status == "" {
		http.Error(w, "Status is required", http.StatusBadRequest)
		return
	}

	if err := h.Service.UpdateFeedbackStatus(id, req.Status); err != nil {
		if err.Error() == "feedback not found" {
			http.Error(w, "Feedback not found", http.StatusNotFound)
		} else {
			http.Error(w, fmt.Sprintf("Failed to update status: %v", err), http.StatusInternalServerError)
		}
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// HandleDelete handles DELETE /api/admin/feedbacks/{id}
func (h *Handler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	if err := h.Service.DeleteFeedback(id); err != nil {
		if err.Error() == "feedback not found" {
			http.Error(w, "Feedback not found", http.StatusNotFound)
		} else {
			http.Error(w, fmt.Sprintf("Failed to delete feedback: %v", err), http.StatusInternalServerError)
		}
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
