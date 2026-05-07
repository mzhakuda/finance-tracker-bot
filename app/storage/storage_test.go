package storage

import (
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
	// Same user can have multiple categories — this used to fail because of UNIQUE on user_id.
	cats, err := cat.ListCategories(1)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(cats) != 2 {
		t.Fatalf("expected 2 categories, got %d", len(cats))
	}

	// Update emoji via conflict.
	if err := cat.AddOrUpdateCategory(CategoryInfo{UserID: 1, Name: "Food", Emoji: "🥗"}); err != nil {
		t.Fatalf("update food: %v", err)
	}
	cats, _ = cat.ListCategories(1)
	if len(cats) != 2 {
		t.Fatalf("expected 2 after update, got %d", len(cats))
	}

	// Different users isolated.
	if err := cat.AddOrUpdateCategory(CategoryInfo{UserID: 2, Name: "Food", Emoji: "🍔"}); err != nil {
		t.Fatalf("add for user 2: %v", err)
	}
	if c2, _ := cat.ListCategories(2); len(c2) != 1 {
		t.Fatalf("expected 1 category for user 2, got %d", len(c2))
	}
}

func TestSpending_MultiplePerUser(t *testing.T) {
	cat, sp, _ := newTestDB(t)

	if err := cat.AddOrUpdateCategory(CategoryInfo{UserID: 1, Name: "Food", Emoji: "🍔"}); err != nil {
		t.Fatalf("add cat: %v", err)
	}
	cats, _ := cat.ListCategories(1)
	categoryID := cats[0].ID

	for i := 0; i < 5; i++ {
		err := sp.AddSpending(SpendingInfo{
			UserID:     1,
			CategoryID: categoryID,
			Amount:     float64(i + 1),
			Timestamp:  time.Now(),
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
		t.Fatalf("expected 5 spendings, got %d", len(got))
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

	// Upsert.
	if err := us.Write(UserStateInfo{UserID: 1, State: "AwaitingAmountInput", DataJSON: `{"x":1}`}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	got, _ = us.Read(1)
	if got.State != "AwaitingAmountInput" {
		t.Fatalf("expected upsert to overwrite, got %s", got.State)
	}
}
