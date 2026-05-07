package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"log"

	"github.com/jmoiron/sqlx"
)

// Category provides CRUD over the categories table.
type Category struct {
	db *sqlx.DB
}

// CategoryInfo represents a single category.
type CategoryInfo struct {
	ID     int64  `db:"id"`
	UserID int64  `db:"user_id"`
	Name   string `db:"name"`
	Emoji  string `db:"emoji"`
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

	if _, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_categories_user_id ON categories(user_id)`); err != nil {
		return nil, fmt.Errorf("failed to create index on user_id: %w", err)
	}

	return &Category{db: db}, nil
}

// AddOrUpdateCategory adds a new category or updates an existing one for a specific user.
func (c *Category) AddOrUpdateCategory(info CategoryInfo) error {
	query := `INSERT INTO categories (user_id, name, emoji) VALUES (?, ?, ?)
	          ON CONFLICT(user_id, name) DO UPDATE SET emoji = excluded.emoji`
	if _, err := c.db.Exec(query, info.UserID, info.Name, info.Emoji); err != nil {
		return fmt.Errorf("failed to insert or update category: %w", err)
	}

	log.Printf("[info] category %q upserted for user_id=%d", info.Name, info.UserID)
	return nil
}

// ErrCategoryNameTaken is returned by UpdateCategoryByID when the new name collides with
// another category owned by the same user.
var ErrCategoryNameTaken = errors.New("category name already in use")

// UpdateCategoryByID renames and re-emojis a specific category owned by the user. It does
// not auto-merge with an existing category that already has the new name — caller-friendly
// error returned instead.
func (c *Category) UpdateCategoryByID(userID, categoryID int64, newName, newEmoji string) error {
	tx, err := c.db.Beginx()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var existingID int64
	err = tx.Get(&existingID, "SELECT id FROM categories WHERE user_id = ? AND name = ? AND id != ?", userID, newName, categoryID)
	switch {
	case err == nil:
		return ErrCategoryNameTaken
	case errors.Is(err, sql.ErrNoRows):
		// no collision, proceed
	default:
		return fmt.Errorf("check name collision: %w", err)
	}

	res, err := tx.Exec("UPDATE categories SET name = ?, emoji = ? WHERE id = ? AND user_id = ?", newName, newEmoji, categoryID, userID)
	if err != nil {
		return fmt.Errorf("update category: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	log.Printf("[info] category id=%d updated for user_id=%d", categoryID, userID)
	return nil
}

// ListCategories returns all categories for a given user ID.
func (c *Category) ListCategories(userID int64) ([]CategoryInfo, error) {
	var categories []CategoryInfo
	query := "SELECT id, user_id, name, emoji FROM categories WHERE user_id = ? ORDER BY name ASC"
	if err := c.db.Select(&categories, query, userID); err != nil {
		return nil, fmt.Errorf("failed to list categories for user_id=%d: %w", userID, err)
	}
	return categories, nil
}

// GetCategoryForUser returns the category if it exists and belongs to the supplied user.
// Returns ErrNotFound otherwise.
func (c *Category) GetCategoryForUser(userID, categoryID int64) (*CategoryInfo, error) {
	var info CategoryInfo
	err := c.db.Get(&info, "SELECT id, user_id, name, emoji FROM categories WHERE id = ? AND user_id = ?", categoryID, userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get category id=%d user_id=%d: %w", categoryID, userID, err)
	}
	return &info, nil
}

// DeleteCategory removes a category owned by the user. Returns ErrNotFound if missing.
func (c *Category) DeleteCategory(userID, categoryID int64) error {
	res, err := c.db.Exec("DELETE FROM categories WHERE id = ? AND user_id = ?", categoryID, userID)
	if err != nil {
		return fmt.Errorf("failed to delete category id=%d: %w", categoryID, err)
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
