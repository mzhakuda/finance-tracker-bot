package events

import (
	"context"
	"errors"
	"log"
	"strconv"
	"strings"

	tbapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/nyanyamaga/finance-tracker-bot/app/keyboards"
	"github.com/nyanyamaga/finance-tracker-bot/app/storage"
)

type BotCallbackQueryHandler struct {
	TbAPI        TbAPI
	StateManager StateManager
	Categories   CategoriesRepository
	Spendings    SpendingsRepository
	TbKeyboards  TbKeyboards
}

func (h *BotCallbackQueryHandler) HandleCallbackQuery(ctx context.Context, update tbapi.Update) {
	if update.CallbackQuery == nil {
		log.Println("[warn] received an update that is not a callback query")
		return
	}
	cb := update.CallbackQuery
	userID := cb.From.ID
	callbackData := cb.Data

	log.Printf("[info] callback: user=%d data=%s", userID, callbackData)

	// Acknowledge the callback so Telegram clears the loading spinner.
	defer h.answer(cb.ID, "")

	switch {
	case callbackData == keyboards.CallbackNoop:
		return
	case strings.HasPrefix(callbackData, keyboards.CallbackDeleteCategory):
		h.handleDeleteCategory(userID, cb, callbackData)
		return
	case strings.HasPrefix(callbackData, keyboards.CallbackDeleteSpending):
		h.handleDeleteSpending(userID, cb, callbackData)
		return
	}

	// Otherwise, treat it as input for the FSM.
	currentState, err := h.StateManager.GetCurrentState(ctx, userID)
	if err != nil {
		log.Printf("[warn] error retrieving current state for user %d: %v", userID, err)
		return
	}
	nextStates := currentState.AvailableTransitions()
	switch {
	case len(nextStates) == 0:
		log.Printf("[warn] no available transitions from state %s for user %d", currentState.Current(), userID)
		return
	case len(nextStates) > 1:
		log.Printf("[warn] more than one transition from %s for user %d: %v", currentState.Current(), userID, nextStates)
		return
	}
	if err := h.StateManager.TriggerStateChange(ctx, userID, nextStates[0], callbackData); err != nil {
		log.Printf("[warn] error triggering state change: %v", err)
	}
}

func (h *BotCallbackQueryHandler) handleDeleteCategory(userID int64, cb *tbapi.CallbackQuery, payload string) {
	idStr := strings.TrimPrefix(payload, keyboards.CallbackDeleteCategory)
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		log.Printf("[warn] malformed delete-category payload %q", payload)
		return
	}
	if err := h.Categories.DeleteCategory(userID, id); err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			h.answer(cb.ID, "Category not found.")
			return
		}
		log.Printf("[warn] failed to delete category id=%d user=%d: %v", id, userID, err)
		h.answer(cb.ID, "Failed to delete.")
		return
	}
	h.answer(cb.ID, "Deleted.")
	// Refresh the management keyboard in place.
	newMarkup := h.TbKeyboards.GetCategoriesManagementKeyboard(userID)
	edit := tbapi.NewEditMessageReplyMarkup(cb.Message.Chat.ID, cb.Message.MessageID, newMarkup)
	if _, err := h.TbAPI.Request(edit); err != nil {
		log.Printf("[warn] failed to update categories markup: %v", err)
	}
}

func (h *BotCallbackQueryHandler) handleDeleteSpending(userID int64, cb *tbapi.CallbackQuery, payload string) {
	idStr := strings.TrimPrefix(payload, keyboards.CallbackDeleteSpending)
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		log.Printf("[warn] malformed delete-spending payload %q", payload)
		return
	}
	if err := h.Spendings.DeleteSpending(userID, id); err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			h.answer(cb.ID, "Spending not found.")
			return
		}
		log.Printf("[warn] failed to delete spending id=%d user=%d: %v", id, userID, err)
		h.answer(cb.ID, "Failed to delete.")
		return
	}
	h.answer(cb.ID, "Deleted.")
	newMarkup := h.TbKeyboards.GetSpendingsManagementKeyboard(userID, 10)
	edit := tbapi.NewEditMessageReplyMarkup(cb.Message.Chat.ID, cb.Message.MessageID, newMarkup)
	if _, err := h.TbAPI.Request(edit); err != nil {
		log.Printf("[warn] failed to update spendings markup: %v", err)
	}
}

func (h *BotCallbackQueryHandler) answer(callbackID, text string) {
	cfg := tbapi.NewCallback(callbackID, text)
	if _, err := h.TbAPI.Request(cfg); err != nil {
		// Telegram returns "query is too old" if we miss the 30s window — log at debug level only.
		log.Printf("[debug] failed to answer callback %s: %v", callbackID, err)
	}
}

