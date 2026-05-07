package events

import (
	"context"
	"database/sql"
	"encoding/csv"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	tbapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/nyanyamaga/finance-tracker-bot/app/storage"
)

// fakeTbAPI captures sent messages and documents instead of talking to Telegram.
type fakeTbAPI struct {
	mu        sync.Mutex
	messages  []tbapi.MessageConfig
	documents []tbapi.DocumentConfig
}

func (f *fakeTbAPI) GetUpdatesChan(_ tbapi.UpdateConfig) tbapi.UpdatesChannel { return nil }

func (f *fakeTbAPI) Send(c tbapi.Chattable) (tbapi.Message, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch v := c.(type) {
	case tbapi.MessageConfig:
		f.messages = append(f.messages, v)
	case tbapi.DocumentConfig:
		f.documents = append(f.documents, v)
	}
	return tbapi.Message{}, nil
}

func (f *fakeTbAPI) Request(_ tbapi.Chattable) (*tbapi.APIResponse, error) {
	return &tbapi.APIResponse{Ok: true}, nil
}

func (f *fakeTbAPI) GetChat(_ tbapi.ChatInfoConfig) (tbapi.Chat, error) { return tbapi.Chat{}, nil }

// stubKeyboards is the minimum TbKeyboards stub needed by /export's failure path.
type stubKeyboards struct{}

func (stubKeyboards) GetMainKeyboard() tbapi.ReplyKeyboardMarkup        { return tbapi.ReplyKeyboardMarkup{} }
func (stubKeyboards) GetCategoryKeyboard(int64) tbapi.InlineKeyboardMarkup {
	return tbapi.NewInlineKeyboardMarkup()
}
func (stubKeyboards) GetCategoriesManagementKeyboard(int64) tbapi.InlineKeyboardMarkup {
	return tbapi.NewInlineKeyboardMarkup()
}
func (stubKeyboards) GetSpendingsManagementKeyboard(int64, int) tbapi.InlineKeyboardMarkup {
	return tbapi.NewInlineKeyboardMarkup()
}
func (stubKeyboards) GetSkipKeyboard() tbapi.ReplyKeyboardMarkup { return tbapi.ReplyKeyboardMarkup{} }
func (stubKeyboards) GetDateKeyboard() tbapi.ReplyKeyboardMarkup { return tbapi.ReplyKeyboardMarkup{} }
func (stubKeyboards) GetCategoryPickKeyboard(int64) tbapi.InlineKeyboardMarkup {
	return tbapi.NewInlineKeyboardMarkup()
}
func (stubKeyboards) GetEditSpendingFieldKeyboard() tbapi.InlineKeyboardMarkup {
	return tbapi.NewInlineKeyboardMarkup()
}
func (stubKeyboards) GetBudgetCategoryKeyboard(int64) tbapi.InlineKeyboardMarkup {
	return tbapi.NewInlineKeyboardMarkup()
}
func (stubKeyboards) GetBudgetsManagementKeyboard(int64) tbapi.InlineKeyboardMarkup {
	return tbapi.NewInlineKeyboardMarkup()
}
func (stubKeyboards) IsReservedActionLabel(string) bool { return false }

// stubSpendings supplies canned spendings for /export.
type stubSpendings struct {
	rows []storage.SpendingDisplay
	err  error
}

func (s *stubSpendings) AddSpending(storage.SpendingInfo) error { return nil }
func (s *stubSpendings) ListSpendings(int64) ([]storage.SpendingInfo, error) {
	return nil, nil
}
func (s *stubSpendings) RecentSpendings(int64, int) ([]storage.SpendingDisplay, error) {
	return s.rows, s.err
}
func (s *stubSpendings) AllSpendingsWithCategory(int64) ([]storage.SpendingDisplay, error) {
	return s.rows, s.err
}
func (s *stubSpendings) TotalSince(int64, time.Time) ([]storage.CurrencyTotal, error) {
	return nil, nil
}
func (s *stubSpendings) DeleteSpending(int64, int64) error { return nil }
func (s *stubSpendings) GetSpendingForUser(int64, int64) (*storage.SpendingInfo, error) {
	return nil, storage.ErrNotFound
}
func (s *stubSpendings) UpdateSpending(int64, int64, storage.SpendingUpdate) error { return nil }
func (s *stubSpendings) TotalSinceForCategory(int64, int64, time.Time) ([]storage.CurrencyTotal, error) {
	return nil, nil
}

func TestHandleExport_Empty(t *testing.T) {
	api := &fakeTbAPI{}
	h := &BotCommandHandler{
		TbAPI:       api,
		TbKeyboards: stubKeyboards{},
		Spendings:   &stubSpendings{rows: nil},
	}
	h.handleExport(context.Background(), 1, 100)
	if len(api.documents) != 0 {
		t.Fatalf("expected no documents on empty export, got %d", len(api.documents))
	}
	if len(api.messages) == 0 || !strings.Contains(api.messages[0].Text, "Nothing to export") {
		t.Fatalf("expected 'Nothing to export' message, got %+v", api.messages)
	}
}

func TestHandleExport_WritesCSV(t *testing.T) {
	api := &fakeTbAPI{}
	rows := []storage.SpendingDisplay{
		{
			SpendingInfo: storage.SpendingInfo{
				ID:          1,
				UserID:      1,
				CategoryID:  10,
				Amount:      12.5,
				Currency:    "EUR",
				Description: "lunch, with comma",
				Timestamp:   time.Date(2026, 5, 7, 10, 0, 0, 0, time.UTC),
			},
			CategoryName:  sql.NullString{String: "Food", Valid: true},
			CategoryEmoji: sql.NullString{String: "🍔", Valid: true},
		},
		{
			SpendingInfo: storage.SpendingInfo{
				ID:        2,
				UserID:    1,
				Amount:    3,
				Currency:  "USD",
				Timestamp: time.Date(2026, 5, 8, 8, 0, 0, 0, time.UTC),
			},
			// orphaned category — left NULL
		},
	}
	h := &BotCommandHandler{
		TbAPI:       api,
		TbKeyboards: stubKeyboards{},
		Spendings:   &stubSpendings{rows: rows},
	}
	h.handleExport(context.Background(), 1, 100)

	if len(api.documents) != 1 {
		t.Fatalf("expected 1 document, got %d", len(api.documents))
	}
	doc := api.documents[0]
	fb, ok := doc.File.(tbapi.FileBytes)
	if !ok {
		t.Fatalf("expected FileBytes, got %T", doc.File)
	}

	r := csv.NewReader(strings.NewReader(string(fb.Bytes)))
	records, err := r.ReadAll()
	if err != nil {
		t.Fatalf("csv parse: %v", err)
	}
	if len(records) != 3 {
		t.Fatalf("expected header + 2 rows, got %d", len(records))
	}
	if got := records[0]; got[0] != "id" || got[2] != "category" {
		t.Fatalf("unexpected header: %v", got)
	}
	// Comma in description must be quoted by encoding/csv.
	if !strings.Contains(records[1][6], "lunch, with comma") {
		t.Fatalf("description not preserved: %q", records[1][6])
	}
	// Orphaned category is empty, not "?".
	if records[2][2] != "" {
		t.Fatalf("expected empty category for orphaned row, got %q", records[2][2])
	}
}

func TestHandleExport_StorageError(t *testing.T) {
	api := &fakeTbAPI{}
	h := &BotCommandHandler{
		TbAPI:       api,
		TbKeyboards: stubKeyboards{},
		Spendings:   &stubSpendings{err: errors.New("boom")},
	}
	h.handleExport(context.Background(), 1, 100)
	if len(api.documents) != 0 {
		t.Fatalf("expected no documents on error")
	}
	if len(api.messages) == 0 || !strings.Contains(api.messages[0].Text, "Failed to build export") {
		t.Fatalf("expected failure message, got %+v", api.messages)
	}
}
