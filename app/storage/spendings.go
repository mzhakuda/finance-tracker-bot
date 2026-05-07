package storage

import (
	"fmt"
	"log"

	"github.com/jmoiron/sqlx"
	"time"
)

// Spending represents a single spending record.
type Spending struct {
	db *sqlx.DB
}

// SpendingInfo encapsulates details about a spending entry.
type SpendingInfo struct {
	ID          int64     `db:"id"`
	UserID      int64     `db:"user_id"`
	CategoryID  int64     `db:"category_id"` // Assuming category is recorded in the user_states.
	Amount      float64   `db:"amount"`
	Description string    `db:"description"` // Optional: More details about the spending
	Timestamp   time.Time `db:"timestamp"`
}

// NewSpending initializes spending record management.
func NewSpending(db *sqlx.DB) (*Spending, error) {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS spendings (
		id INTEGER PRIMARY KEY,
		user_id INTEGER NOT NULL,
		category_id INTEGER NOT NULL,
		amount REAL NOT NULL,
		description TEXT,
		timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (category_id) REFERENCES categories(id)
	)`)
	if err != nil {
		return nil, fmt.Errorf("failed to create spendings table: %w", err)
	}

	if _, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_spendings_user_id ON spendings(user_id)`); err != nil {
		return nil, fmt.Errorf("failed to create index on user_id: %w", err)
	}
	if _, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_spendings_user_timestamp ON spendings(user_id, timestamp)`); err != nil {
		return nil, fmt.Errorf("failed to create index on user_id, timestamp: %w", err)
	}

	return &Spending{db: db}, nil
}

// AddSpending adds a new spending record.
func (s *Spending) AddSpending(info SpendingInfo) error {
	query := `INSERT INTO spendings (user_id, category_id, amount, description, timestamp) VALUES (?, ?, ?, ?, ?)`
	if _, err := s.db.Exec(query, info.UserID, info.CategoryID, info.Amount, info.Description, info.Timestamp); err != nil {
		return fmt.Errorf("failed to insert spending record: %w", err)
	}

	log.Printf("[info] New spending record added: %f for user_id: %d, category_id: %d", info.Amount, info.UserID, info.CategoryID)
	return nil
}

// ListSpendings retrieves spending records for a given user.
func (s *Spending) ListSpendings(userID int64) ([]SpendingInfo, error) {
	var spendings []SpendingInfo
	query := "SELECT * FROM spendings WHERE user_id = ? ORDER BY timestamp DESC"
	if err := s.db.Select(&spendings, query, userID); err != nil {
		return nil, fmt.Errorf("failed to list spending records for user_id: %d: %w", userID, err)
	}

	return spendings, nil
}
