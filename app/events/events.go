package events

import (
	"context"
	"fmt"
	"log"
	"time"

	tbapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/looplab/fsm"

	"github.com/nyanyamaga/finance-tracker-bot/app/storage"
)

// TbAPI is an interface for telegram bot API, only subset of methods used.
type TbAPI interface {
	GetUpdatesChan(config tbapi.UpdateConfig) tbapi.UpdatesChannel
	Send(c tbapi.Chattable) (tbapi.Message, error)
	Request(c tbapi.Chattable) (*tbapi.APIResponse, error)
	GetChat(config tbapi.ChatInfoConfig) (tbapi.Chat, error)
}

type TbKeyboards interface {
	GetMainKeyboard() tbapi.ReplyKeyboardMarkup
	GetCategoryKeyboard(userID int64) tbapi.InlineKeyboardMarkup
	GetCategoriesManagementKeyboard(userID int64) tbapi.InlineKeyboardMarkup
	GetSpendingsManagementKeyboard(userID int64, limit int) tbapi.InlineKeyboardMarkup
	GetSkipKeyboard() tbapi.ReplyKeyboardMarkup
	GetDateKeyboard() tbapi.ReplyKeyboardMarkup
	IsReservedActionLabel(text string) bool
}

type UserStateRepository interface {
	Write(entry storage.UserStateInfo) error
	Read(userID int64) (*storage.UserStateInfo, error)
}

type CategoriesRepository interface {
	AddOrUpdateCategory(info storage.CategoryInfo) error
	ListCategories(userID int64) ([]storage.CategoryInfo, error)
	GetCategoryForUser(userID, categoryID int64) (*storage.CategoryInfo, error)
	DeleteCategory(userID, categoryID int64) error
}

type SpendingsRepository interface {
	AddSpending(info storage.SpendingInfo) error
	ListSpendings(userID int64) ([]storage.SpendingInfo, error)
	RecentSpendings(userID int64, limit int) ([]storage.SpendingDisplay, error)
	AllSpendingsWithCategory(userID int64) ([]storage.SpendingDisplay, error)
	TotalSince(userID int64, since time.Time) ([]storage.CurrencyTotal, error)
	DeleteSpending(userID, spendingID int64) error
}

type CommandHandler interface {
	HandleCommands(ctx context.Context, update tbapi.Update)
}

type MessageHandler interface {
	HandleMessages(ctx context.Context, update tbapi.Update)
}

type CallbackQueryHandler interface {
	HandleCallbackQuery(ctx context.Context, update tbapi.Update)
}

type StateManager interface {
	InitializeUserFSM(ctx context.Context, userID int64)
	SetIdleState(ctx context.Context, userID int64)
	ResetToIdle(ctx context.Context, userID int64)
	TriggerStateChange(ctx context.Context, userID int64, action, value string) error
	GetCurrentState(ctx context.Context, userID int64) (*fsm.FSM, error)
	HasCategories(userID int64) (bool, error)
}

// send a message to telegram trying markdown first, falling back to plain text on error.
func send(tbMsg tbapi.Chattable, tbAPI TbAPI) error {
	withParseMode := func(tbMsg tbapi.Chattable, parseMode string) tbapi.Chattable {
		switch msg := tbMsg.(type) {
		case tbapi.MessageConfig:
			msg.ParseMode = parseMode
			msg.DisableWebPagePreview = true
			return msg
		case tbapi.EditMessageTextConfig:
			msg.ParseMode = parseMode
			msg.DisableWebPagePreview = true
			return msg
		case tbapi.EditMessageReplyMarkupConfig:
			return msg
		}
		return tbMsg
	}

	msg := withParseMode(tbMsg, tbapi.ModeMarkdown)
	if _, err := tbAPI.Send(msg); err != nil {
		log.Printf("[warn] failed to send message as markdown, %v", err)
		msg = withParseMode(tbMsg, "")
		if _, err := tbAPI.Send(msg); err != nil {
			return fmt.Errorf("can't send message to telegram: %w", err)
		}
	}
	return nil
}
