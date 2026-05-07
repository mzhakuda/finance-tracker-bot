package storage

import (
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func newTestDB(t *testing.T) (*Category, *Spending, *UserState) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := NewSqliteDB(dbPath)
	if err != nil {
		t.Fatalf("NewSqliteDB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	cat, err := NewCategory(db)
	if err != nil {
		t.Fatalf("NewCategory: %v", err)
	}
	sp, err := NewSpending(db)
	if err != nil {
		t.Fatalf("NewSpending: %v", err)
	}
	us, err := NewUserState(db)
	if err != nil {
		t.Fatalf("NewUserState: %v", err)
	}
	return cat, sp, us
}

func TestCategory_AddAndList(t *testing.T) {
	cat, _, _ := newTestDB(t)

	if err := cat.AddOrUpdateCategory(CategoryInfo{UserID: 1, Name: "Food", Emoji: "🍔"}); err != nil {
		t.Fatalf("add food: %v", err)
	}
	if err := cat.AddOrUpdateCategory(CategoryInfo{UserID: 1, Name: "Transport", Emoji: "🚌"}); err != nil {
		t.Fatalf("add transport: %v", err)
	}
	cats, err := cat.ListCategories(1)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(cats) != 2 {
		t.Fatalf("expected 2 categories, got %d", len(cats))
	}

	if err := cat.AddOrUpdateCategory(CategoryInfo{UserID: 1, Name: "Food", Emoji: "🥗"}); err != nil {
		t.Fatalf("update food: %v", err)
	}
	cats, _ = cat.ListCategories(1)
	if len(cats) != 2 {
		t.Fatalf("expected 2 after update, got %d", len(cats))
	}

	if err := cat.AddOrUpdateCategory(CategoryInfo{UserID: 2, Name: "Food", Emoji: "🍔"}); err != nil {
		t.Fatalf("add for user 2: %v", err)
	}
	if c2, _ := cat.ListCategories(2); len(c2) != 1 {
		t.Fatalf("expected 1 category for user 2, got %d", len(c2))
	}
}

func TestCategory_GetForUserAndDelete(t *testing.T) {
	cat, _, _ := newTestDB(t)

	_ = cat.AddOrUpdateCategory(CategoryInfo{UserID: 1, Name: "Food", Emoji: "🍔"})
	_ = cat.AddOrUpdateCategory(CategoryInfo{UserID: 2, Name: "Food", Emoji: "🍕"})

	c1, _ := cat.ListCategories(1)
	c2, _ := cat.ListCategories(2)
	id1 := c1[0].ID
	id2 := c2[0].ID

	// User 1 cannot resolve user 2's category.
	if _, err := cat.GetCategoryForUser(1, id2); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound when accessing other user's category, got %v", err)
	}
	if got, err := cat.GetCategoryForUser(1, id1); err != nil || got.Name != "Food" {
		t.Fatalf("expected own category, got %+v err=%v", got, err)
	}

	// User 1 cannot delete user 2's category.
	if err := cat.DeleteCategory(1, id2); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound on cross-user delete, got %v", err)
	}
	c2After, _ := cat.ListCategories(2)
	if len(c2After) != 1 {
		t.Fatalf("user 2's category was deleted across boundary, got %d", len(c2After))
	}

	// Delete own works.
	if err := cat.DeleteCategory(1, id1); err != nil {
		t.Fatalf("delete own: %v", err)
	}
	if c1After, _ := cat.ListCategories(1); len(c1After) != 0 {
		t.Fatalf("expected user 1 to have 0 categories after delete, got %d", len(c1After))
	}
}

func TestSpending_Lifecycle(t *testing.T) {
	cat, sp, _ := newTestDB(t)

	_ = cat.AddOrUpdateCategory(CategoryInfo{UserID: 1, Name: "Food", Emoji: "🍔"})
	cats, _ := cat.ListCategories(1)
	categoryID := cats[0].ID

	for i := 0; i < 5; i++ {
		err := sp.AddSpending(SpendingInfo{
			UserID:      1,
			CategoryID:  categoryID,
			Amount:      float64(i + 1),
			Currency:    "USD",
			Description: "lunch",
			Timestamp:   time.Now(),
		})
		if err != nil {
			t.Fatalf("add spending %d: %v", i, err)
		}
	}

	got, err := sp.ListSpendings(1)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 5 {
		t.Fatalf("expected 5, got %d", len(got))
	}

	recent, err := sp.RecentSpendings(1, 3)
	if err != nil {
		t.Fatalf("recent: %v", err)
	}
	if len(recent) != 3 {
		t.Fatalf("expected 3 recent, got %d", len(recent))
	}
	if !recent[0].CategoryName.Valid || recent[0].CategoryName.String != "Food" {
		t.Fatalf("expected joined category name, got %+v", recent[0].CategoryName)
	}

	// Cross-user delete is blocked.
	if err := sp.DeleteSpending(2, recent[0].ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound on cross-user delete, got %v", err)
	}
	if err := sp.DeleteSpending(1, recent[0].ID); err != nil {
		t.Fatalf("own delete: %v", err)
	}
	got2, _ := sp.ListSpendings(1)
	if len(got2) != 4 {
		t.Fatalf("expected 4 after delete, got %d", len(got2))
	}
}

func TestSpending_TotalSince(t *testing.T) {
	cat, sp, _ := newTestDB(t)

	_ = cat.AddOrUpdateCategory(CategoryInfo{UserID: 1, Name: "Food", Emoji: "🍔"})
	cats, _ := cat.ListCategories(1)
	categoryID := cats[0].ID

	now := time.Now()
	yesterday := now.Add(-24 * time.Hour)
	lastMonth := now.Add(-40 * 24 * time.Hour)

	mustAdd := func(amount float64, currency string, ts time.Time) {
		t.Helper()
		err := sp.AddSpending(SpendingInfo{
			UserID:     1,
			CategoryID: categoryID,
			Amount:     amount,
			Currency:   currency,
			Timestamp:  ts,
		})
		if err != nil {
			t.Fatalf("add spending: %v", err)
		}
	}
	mustAdd(10, "USD", now)
	mustAdd(20, "USD", yesterday)
	mustAdd(5, "EUR", yesterday)
	mustAdd(99, "USD", lastMonth) // outside the window

	since := now.Add(-7 * 24 * time.Hour)
	totals, err := sp.TotalSince(1, since)
	if err != nil {
		t.Fatalf("total: %v", err)
	}
	totalsByCurrency := map[string]float64{}
	for _, t := range totals {
		totalsByCurrency[t.Currency] = t.Total
	}
	if got := totalsByCurrency["USD"]; got != 30 {
		t.Fatalf("USD total: got %v, want 30", got)
	}
	if got := totalsByCurrency["EUR"]; got != 5 {
		t.Fatalf("EUR total: got %v, want 5", got)
	}
}

func TestSpending_MigrationAddsCurrencyColumn(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "legacy.db")
	db, err := NewSqliteDB(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	// Simulate an old DB without currency/description columns.
	if _, err := db.Exec(`CREATE TABLE spendings (
		id INTEGER PRIMARY KEY,
		user_id INTEGER NOT NULL,
		category_id INTEGER NOT NULL,
		amount REAL NOT NULL,
		timestamp DATETIME DEFAULT CURRENT_TIMESTAMP
	)`); err != nil {
		t.Fatalf("legacy create: %v", err)
	}

	sp, err := NewSpending(db)
	if err != nil {
		t.Fatalf("NewSpending after legacy: %v", err)
	}
	// We should now be able to insert with the new fields.
	if err := sp.AddSpending(SpendingInfo{
		UserID:      1,
		CategoryID:  1,
		Amount:      9.99,
		Currency:    "EUR",
		Description: "test",
		Timestamp:   time.Now(),
	}); err != nil {
		t.Fatalf("insert post-migration: %v", err)
	}
	got, err := sp.ListSpendings(1)
	if err != nil || len(got) != 1 || got[0].Currency != "EUR" {
		t.Fatalf("post-migration list mismatch: got=%+v err=%v", got, err)
	}
}

func TestCategory_UpdateByID(t *testing.T) {
	cat, _, _ := newTestDB(t)

	_ = cat.AddOrUpdateCategory(CategoryInfo{UserID: 1, Name: "Food", Emoji: "🍔"})
	_ = cat.AddOrUpdateCategory(CategoryInfo{UserID: 1, Name: "Transport", Emoji: "🚌"})
	cats, _ := cat.ListCategories(1)
	foodID := int64(0)
	for _, c := range cats {
		if c.Name == "Food" {
			foodID = c.ID
		}
	}

	// Rename collision: trying to rename "Food" → "Transport" must fail.
	if err := cat.UpdateCategoryByID(1, foodID, "Transport", "🚌"); !errors.Is(err, ErrCategoryNameTaken) {
		t.Fatalf("expected ErrCategoryNameTaken, got %v", err)
	}

	// Cross-user: user 2 can't update user 1's category.
	if err := cat.UpdateCategoryByID(2, foodID, "Whatever", "🥗"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound for cross-user, got %v", err)
	}

	// Happy path.
	if err := cat.UpdateCategoryByID(1, foodID, "Groceries", "🥗"); err != nil {
		t.Fatalf("update: %v", err)
	}
	updated, err := cat.GetCategoryForUser(1, foodID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if updated.Name != "Groceries" || updated.Emoji != "🥗" {
		t.Fatalf("got %+v", updated)
	}
}

func TestSpending_UpdateSpending(t *testing.T) {
	cat, sp, _ := newTestDB(t)

	_ = cat.AddOrUpdateCategory(CategoryInfo{UserID: 1, Name: "Food", Emoji: "🍔"})
	_ = cat.AddOrUpdateCategory(CategoryInfo{UserID: 1, Name: "Transport", Emoji: "🚌"})
	cats, _ := cat.ListCategories(1)
	foodID, transportID := cats[0].ID, cats[1].ID

	now := time.Now()
	if err := sp.AddSpending(SpendingInfo{
		UserID: 1, CategoryID: foodID, Amount: 10, Currency: "USD", Description: "old", Timestamp: now,
	}); err != nil {
		t.Fatalf("add: %v", err)
	}
	rows, _ := sp.ListSpendings(1)
	id := rows[0].ID

	newAmount := 20.5
	newCurrency := "EUR"
	newDesc := "updated"
	newTs := now.Add(-48 * time.Hour)
	if err := sp.UpdateSpending(1, id, SpendingUpdate{
		Amount:      &newAmount,
		Currency:    &newCurrency,
		Description: &newDesc,
		Timestamp:   &newTs,
		CategoryID:  &transportID,
	}); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, _ := sp.GetSpendingForUser(1, id)
	if got.Amount != 20.5 || got.Currency != "EUR" || got.Description != "updated" || got.CategoryID != transportID {
		t.Fatalf("got %+v", got)
	}

	// Cross-user.
	if err := sp.UpdateSpending(2, id, SpendingUpdate{Amount: &newAmount}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound for cross-user, got %v", err)
	}

	// Empty update is rejected.
	if err := sp.UpdateSpending(1, id, SpendingUpdate{}); err == nil {
		t.Fatalf("expected error on empty update")
	}
}

func TestUserState_WriteRead(t *testing.T) {
	_, _, us := newTestDB(t)

	if err := us.Write(UserStateInfo{UserID: 1, State: "Idle", DataJSON: "{}"}); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := us.Read(1)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if got.State != "Idle" || got.DataJSON != "{}" {
		t.Fatalf("got %+v", got)
	}

	if err := us.Write(UserStateInfo{UserID: 1, State: "AwaitingAmountInput", DataJSON: `{"x":1}`}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	got, _ = us.Read(1)
	if got.State != "AwaitingAmountInput" {
		t.Fatalf("expected upsert to overwrite, got %s", got.State)
	}
}
