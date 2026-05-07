package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/jmoiron/sqlx"
)

// ErrNotFound is returned when a record cannot be located or doesn't belong to the caller.
var ErrNotFound = errors.New("not found")

// Spending represents a single spending record.
type Spending struct {
	db *sqlx.DB
}

// SpendingInfo encapsulates details about a spending entry.
type SpendingInfo struct {
	ID          int64     `db:"id"`
	UserID      int64     `db:"user_id"`
	CategoryID  int64     `db:"category_id"`
	Amount      float64   `db:"amount"`
	Currency    string    `db:"currency"`
	Description string    `db:"description"`
	Timestamp   time.Time `db:"timestamp"`
}

// SpendingDisplay joins SpendingInfo with the resolved category for display.
type SpendingDisplay struct {
	SpendingInfo
	CategoryName  sql.NullString `db:"category_name"`
	CategoryEmoji sql.NullString `db:"category_emoji"`
}

// CurrencyTotal aggregates spending by currency.
type CurrencyTotal struct {
	Currency string  `db:"currency"`
	Total    float64 `db:"total"`
}

// NewSpending initializes spending record management.
func NewSpending(db *sqlx.DB) (*Spending, error) {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS spendings (
		id INTEGER PRIMARY KEY,
		user_id INTEGER NOT NULL,
		category_id INTEGER NOT NULL,
		amount REAL NOT NULL,
		currency TEXT NOT NULL DEFAULT '',
		description TEXT NOT NULL DEFAULT '',
		timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (category_id) REFERENCES categories(id)
	)`)
	if err != nil {
		return nil, fmt.Errorf("failed to create spendings table: %w", err)
	}

	// Migrate older databases that pre-date the currency/description columns.
	if err := ensureColumn(db, "spendings", "currency", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return nil, err
	}
	if err := ensureColumn(db, "spendings", "description", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return nil, err
	}

	if _, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_spendings_user_id ON spendings(user_id)`); err != nil {
		return nil, fmt.Errorf("failed to create index on user_id: %w", err)
	}
	if _, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_spendings_user_timestamp ON spendings(user_id, timestamp)`); err != nil {
		return nil, fmt.Errorf("failed to create index on user_id, timestamp: %w", err)
	}

	return &Spending{db: db}, nil
}

// AddSpending inserts a new spending record.
func (s *Spending) AddSpending(info SpendingInfo) error {
	query := `INSERT INTO spendings (user_id, category_id, amount, currency, description, timestamp)
	          VALUES (?, ?, ?, ?, ?, ?)`
	if _, err := s.db.Exec(query, info.UserID, info.CategoryID, info.Amount, info.Currency, info.Description, info.Timestamp); err != nil {
		return fmt.Errorf("failed to insert spending record: %w", err)
	}

	log.Printf("[info] new spending added: %.2f %s for user_id=%d category_id=%d", info.Amount, info.Currency, info.UserID, info.CategoryID)
	return nil
}

// ListSpendings retrieves all spending records for a user.
func (s *Spending) ListSpendings(userID int64) ([]SpendingInfo, error) {
	var spendings []SpendingInfo
	query := "SELECT id, user_id, category_id, amount, currency, description, timestamp FROM spendings WHERE user_id = ? ORDER BY timestamp DESC"
	if err := s.db.Select(&spendings, query, userID); err != nil {
		return nil, fmt.Errorf("failed to list spending records for user_id=%d: %w", userID, err)
	}
	return spendings, nil
}

// RecentSpendings returns the latest `limit` spendings for a user, joined with their category.
func (s *Spending) RecentSpendings(userID int64, limit int) ([]SpendingDisplay, error) {
	if limit <= 0 {
		limit = 10
	}
	var rows []SpendingDisplay
	query := `
		SELECT s.id, s.user_id, s.category_id, s.amount, s.currency, s.description, s.timestamp,
		       c.name AS category_name, c.emoji AS category_emoji
		FROM spendings s
		LEFT JOIN categories c ON c.id = s.category_id
		WHERE s.user_id = ?
		ORDER BY s.timestamp DESC
		LIMIT ?`
	if err := s.db.Select(&rows, query, userID, limit); err != nil {
		return nil, fmt.Errorf("failed to load recent spendings for user_id=%d: %w", userID, err)
	}
	return rows, nil
}

// TotalSince returns the sum of spendings since `since`, broken down by currency.
func (s *Spending) TotalSince(userID int64, since time.Time) ([]CurrencyTotal, error) {
	var rows []CurrencyTotal
	query := `SELECT currency, COALESCE(SUM(amount), 0) AS total
	          FROM spendings
	          WHERE user_id = ? AND timestamp >= ?
	          GROUP BY currency
	          ORDER BY currency`
	if err := s.db.Select(&rows, query, userID, since); err != nil {
		return nil, fmt.Errorf("failed to total spendings for user_id=%d: %w", userID, err)
	}
	return rows, nil
}

// DeleteSpending removes a spending entry that belongs to the given user. Returns ErrNotFound
// if the row doesn't exist or is owned by someone else.
func (s *Spending) DeleteSpending(userID, spendingID int64) error {
	res, err := s.db.Exec("DELETE FROM spendings WHERE id = ? AND user_id = ?", spendingID, userID)
	if err != nil {
		return fmt.Errorf("failed to delete spending id=%d: %w", spendingID, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// ensureColumn adds a column to a SQLite table if it isn't already present.
// SQLite doesn't support ALTER TABLE ADD COLUMN IF NOT EXISTS, so we probe via PRAGMA.
func ensureColumn(db *sqlx.DB, table, column, decl string) error {
	rows, err := db.Queryx(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return fmt.Errorf("pragma table_info(%s): %w", table, err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			cid       int
			name      string
			ctype     string
			notnull   int
			dfltValue sql.NullString
			pk        int
		)
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dfltValue, &pk); err != nil {
			return fmt.Errorf("scan pragma: %w", err)
		}
		if name == column {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iter pragma: %w", err)
	}

	if _, err := db.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, column, decl)); err != nil {
		return fmt.Errorf("add column %s.%s: %w", table, column, err)
	}
	return nil
}
