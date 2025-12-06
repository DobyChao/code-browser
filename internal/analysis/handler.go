package analysis

import (
	"encoding/json"
	"log"
	"net/http"
)

// Handlers 封装了 Analysis 服务的所有 HTTP 处理器
type Handlers struct {
	Service *Service
}

// GetDefinitionHandler 查找定义
func (h *Handlers) GetDefinitionHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req DefinitionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// 简单的参数校验
	if req.RepoID == "" || req.FilePath == "" {
		http.Error(w, "Missing required fields: repoId, filePath", http.StatusBadRequest)
		return
	}

	definitions, err := h.Service.GetDefinition(req)
	if err != nil {
		log.Printf("获取定义失败: %v", err)
		// 区分错误类型：如果是索引不存在，返回 404；如果是解析错误，返回 500
		// 这里简化处理
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(definitions)
}

// GetReferencesHandler 查找引用
func (h *Handlers) GetReferencesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req DefinitionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.RepoID == "" || req.FilePath == "" {
		http.Error(w, "Missing required fields: repoId, filePath", http.StatusBadRequest)
		return
	}
	refs, err := h.Service.GetReferences(req)
	if err != nil {
		log.Printf("获取引用失败: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(refs)
}
