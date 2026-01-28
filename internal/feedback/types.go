package feedback

import "time"

type FeedbackContext struct {
	RepoID string `json:"repoId,omitempty"`
	Path   string `json:"path,omitempty"`
	URL    string `json:"url,omitempty"`
}

type Feedback struct {
	ID          int64           `json:"id,omitempty"` // Database ID
	Type        string          `json:"type"`
	Title       string          `json:"title"`
	Description string          `json:"description"`
	Email       string          `json:"email,omitempty"`
	Status      string          `json:"status"` // open, closed, in_progress
	Context     FeedbackContext `json:"context,omitempty"`
	CreatedAt   time.Time       `json:"created_at,omitempty"`
	UpdatedAt   time.Time       `json:"updated_at,omitempty"`
}
