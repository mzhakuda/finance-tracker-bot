package storage

import (
	"fmt"
	"log"

	"github.com/jmoiron/sqlx"
)

// Category represents information about a user's category entry.
type Category struct {
	db *sqlx.DB
}

// CategoryInfo represents the structure of a category.
type CategoryInfo struct {
	ID     int64  `db:"id"`
	UserID int64  `db:"user_id"`
	Name   string `db:"name"`
	Emoji  string `db:"emoji"` // Optional, can be used for UI representation
}

// NewCategory creates a new Category storage handler.
func NewCategory(db *sqlx.DB) (*Category, error) {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS categories (
		id INTEGER PRIMARY KEY,
		user_id INTEGER NOT NULL,
		name TEXT NOT NULL,
		emoji TEXT,
		UNIQUE(user_id, name) ON CONFLICT REPLACE
	)`)
	if err != nil {
		return nil, fmt.Errorf("failed to create categories table: %w", err)
	}

	// Add index on user_id for faster lookup of user-specific categories
	if _, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_categories_user_id ON categories(user_id)`); err != nil {
		return nil, fmt.Errorf("failed to create index on user_id: %w", err)
	}

	return &Category{db: db}, nil
}

// AddOrUpdateCategory adds a new category or updates an existing one for a specific user.
func (c *Category) AddOrUpdateCategory(info CategoryInfo) error {
	query := `INSERT INTO categories (user_id, name, emoji) VALUES (?, ?, ?) ON CONFLICT(user_id, name) DO UPDATE SET emoji = excluded.emoji`
	if _, err := c.db.Exec(query, info.UserID, info.Name, info.Emoji); err != nil {
		return fmt.Errorf("failed to insert or update category: %w", err)
	}

	log.Printf("[info] Category '%s' updated for user_id: %d", info.Name, info.UserID)
	return nil
}

// ListCategories returns all categories for a given user ID.
func (c *Category) ListCategories(userID int64) ([]CategoryInfo, error) {
	var categories []CategoryInfo
	query := "SELECT * FROM categories WHERE user_id = ? ORDER BY name ASC"
	if err := c.db.Select(&categories, query, userID); err != nil {
		return nil, fmt.Errorf("failed to list categories for user_id: %d, %w", userID, err)
	}

	return categories, nil
}
