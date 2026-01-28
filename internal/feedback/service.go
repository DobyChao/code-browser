package feedback

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
)

type Service struct {
	db *sql.DB
}

func NewService(db *sql.DB) (*Service, error) {
	s := &Service{db: db}
	if err := s.initSchema(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Service) initSchema() error {
	// 1. Create table if not exists
	query := `
	CREATE TABLE IF NOT EXISTS feedbacks (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		type TEXT NOT NULL,
		title TEXT NOT NULL,
		description TEXT NOT NULL,
		email TEXT,
		context_json TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	`
	if _, err := s.db.Exec(query); err != nil {
		return err
	}

	// 2. Add new columns if they don't exist
	// We use a helper function to add columns safely
	if err := s.addColumnIfNotExists("status", "TEXT DEFAULT 'open'"); err != nil {
		return err
	}
	// SQLite ADD COLUMN limitation: cannot use CURRENT_TIMESTAMP as default directly in some versions/modes
	// We will allow it to be NULL initially
	if err := s.addColumnIfNotExists("updated_at", "DATETIME"); err != nil {
		return err
	}

	return nil
}

func (s *Service) addColumnIfNotExists(colName, colType string) error {
	// SQLite doesn't support IF NOT EXISTS in ADD COLUMN directly.
	// We can check PRAGMA table_info or just try to add and ignore duplicate column error.
	query := fmt.Sprintf("ALTER TABLE feedbacks ADD COLUMN %s %s", colName, colType)
	_, err := s.db.Exec(query)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate column name") {
			return nil // Column already exists, ignore
		}
		return err
	}
	return nil
}

func (s *Service) SaveFeedback(f *Feedback) error {
	contextBytes, err := json.Marshal(f.Context)
	if err != nil {
		return fmt.Errorf("failed to marshal context: %w", err)
	}

	query := `INSERT INTO feedbacks (type, title, description, email, context_json, status) VALUES (?, ?, ?, ?, ?, 'open')`
	_, err = s.db.Exec(query, f.Type, f.Title, f.Description, f.Email, string(contextBytes))
	if err != nil {
		return fmt.Errorf("failed to insert feedback: %w", err)
	}
	return nil
}

func (s *Service) ListFeedbacks() ([]Feedback, error) {
	query := `SELECT id, type, title, description, email, status, context_json, created_at, updated_at FROM feedbacks ORDER BY created_at DESC`
	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var feedbacks []Feedback
	for rows.Next() {
		var f Feedback
		var contextJSON string
		var createdAt, updatedAt sql.NullTime

		if err := rows.Scan(&f.ID, &f.Type, &f.Title, &f.Description, &f.Email, &f.Status, &contextJSON, &createdAt, &updatedAt); err != nil {
			return nil, err
		}

		if contextJSON != "" {
			_ = json.Unmarshal([]byte(contextJSON), &f.Context)
		}
		if createdAt.Valid {
			f.CreatedAt = createdAt.Time
		}
		if updatedAt.Valid {
			f.UpdatedAt = updatedAt.Time
		}
		feedbacks = append(feedbacks, f)
	}
	return feedbacks, nil
}

func (s *Service) UpdateFeedbackStatus(id int64, status string) error {
	query := `UPDATE feedbacks SET status = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`
	result, err := s.db.Exec(query, status, id)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("feedback not found")
	}
	return nil
}

func (s *Service) DeleteFeedback(id int64) error {
	query := `DELETE FROM feedbacks WHERE id = ?`
	result, err := s.db.Exec(query, id)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("feedback not found")
	}
	return nil
}
