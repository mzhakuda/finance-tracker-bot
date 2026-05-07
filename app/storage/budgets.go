package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"log"

	"github.com/jmoiron/sqlx"
)

// CategoryIDOverall is the sentinel category_id value used to represent a budget that
// applies across all the user's categories. We use 0 instead of NULL so the (user_id,
// category_id, period) UNIQUE constraint behaves predictably under SQLite's NULL semantics.
const CategoryIDOverall int64 = 0

// BudgetPeriod enumerates supported budget windows. Currently only monthly is implemented.
const BudgetPeriodMonth = "month"

// Budget is the storage handle for the budgets table.
type Budget struct {
	db *sqlx.DB
}

// BudgetInfo describes a single budget. CategoryID == CategoryIDOverall (0) means
// "applies to all spendings of the user".
type BudgetInfo struct {
	ID         int64   `db:"id"`
	UserID     int64   `db:"user_id"`
	CategoryID int64   `db:"category_id"`
	Amount     float64 `db:"amount"`
	Currency   string  `db:"currency"`
	Period     string  `db:"period"`
}

// NewBudget creates the budgets table and returns the storage handle.
func NewBudget(db *sqlx.DB) (*Budget, error) {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS budgets (
		id INTEGER PRIMARY KEY,
		user_id INTEGER NOT NULL,
		category_id INTEGER NOT NULL DEFAULT 0,
		amount REAL NOT NULL,
		currency TEXT NOT NULL,
		period TEXT NOT NULL DEFAULT 'month',
		UNIQUE(user_id, category_id, period) ON CONFLICT REPLACE
	)`)
	if err != nil {
		return nil, fmt.Errorf("failed to create budgets table: %w", err)
	}
	if _, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_budgets_user_id ON budgets(user_id)`); err != nil {
		return nil, fmt.Errorf("failed to create budgets index: %w", err)
	}
	return &Budget{db: db}, nil
}

// SetBudget upserts a budget. amount must be positive; currency must be non-empty.
func (b *Budget) SetBudget(info BudgetInfo) error {
	if info.Amount <= 0 {
		return errors.New("amount must be positive")
	}
	if info.Currency == "" {
		return errors.New("currency is required")
	}
	if info.Period == "" {
		info.Period = BudgetPeriodMonth
	}
	query := `INSERT INTO budgets (user_id, category_id, amount, currency, period) VALUES (?, ?, ?, ?, ?)
	          ON CONFLICT(user_id, category_id, period) DO UPDATE
	            SET amount = excluded.amount, currency = excluded.currency`
	if _, err := b.db.Exec(query, info.UserID, info.CategoryID, info.Amount, info.Currency, info.Period); err != nil {
		return fmt.Errorf("failed to upsert budget: %w", err)
	}
	log.Printf("[info] budget set: user=%d category=%d amount=%.2f %s period=%s",
		info.UserID, info.CategoryID, info.Amount, info.Currency, info.Period)
	return nil
}

// ListBudgets returns every budget the user has, ordered with overall first then by category id.
func (b *Budget) ListBudgets(userID int64) ([]BudgetInfo, error) {
	var rows []BudgetInfo
	query := `SELECT id, user_id, category_id, amount, currency, period
	          FROM budgets WHERE user_id = ?
	          ORDER BY CASE WHEN category_id = 0 THEN 0 ELSE 1 END, category_id`
	if err := b.db.Select(&rows, query, userID); err != nil {
		return nil, fmt.Errorf("failed to list budgets for user_id=%d: %w", userID, err)
	}
	return rows, nil
}

// GetBudgetForCategory returns the category-specific monthly budget if one exists,
// otherwise the user's overall monthly budget, otherwise ErrNotFound.
func (b *Budget) GetBudgetForCategory(userID, categoryID int64) (*BudgetInfo, error) {
	var info BudgetInfo
	err := b.db.Get(&info,
		`SELECT id, user_id, category_id, amount, currency, period
		 FROM budgets
		 WHERE user_id = ? AND category_id = ? AND period = ?`,
		userID, categoryID, BudgetPeriodMonth,
	)
	if err == nil {
		return &info, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("get category budget: %w", err)
	}
	// Fall back to overall.
	err = b.db.Get(&info,
		`SELECT id, user_id, category_id, amount, currency, period
		 FROM budgets
		 WHERE user_id = ? AND category_id = ? AND period = ?`,
		userID, CategoryIDOverall, BudgetPeriodMonth,
	)
	if err == nil {
		return &info, nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return nil, fmt.Errorf("get overall budget: %w", err)
}

// DeleteBudget removes a budget owned by the user.
func (b *Budget) DeleteBudget(userID, budgetID int64) error {
	res, err := b.db.Exec("DELETE FROM budgets WHERE id = ? AND user_id = ?", budgetID, userID)
	if err != nil {
		return fmt.Errorf("delete budget: %w", err)
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
