package events

import (
	"context"
	"log"
	"strings"
	"time"

	tbapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/nyanyamaga/finance-tracker-bot/app/keyboards"
)

type BotMessageHandler struct {
	TbAPI        TbAPI
	StateManager StateManager
	TbKeyboards  TbKeyboards
}

func (h *BotMessageHandler) HandleMessages(ctx context.Context, update tbapi.Update) {
	if update.Message == nil || update.Message.From == nil {
		return
	}

	userID := update.Message.From.ID
	chatID := update.Message.Chat.ID
	messageText := strings.TrimSpace(update.Message.Text)

	switch messageText {
	case keyboards.ActionMessages[keyboards.ActionAddSpending]:
		h.startAddSpending(ctx, userID, chatID)
		return
	case keyboards.ActionMessages[keyboards.ActionNewSpendingCategory]:
		if err := h.StateManager.TriggerStateChange(ctx, userID, "ChooseAddCategory", ""); err != nil {
			log.Printf("[warn] error starting add category: %v", err)
		}
		return
	case keyboards.ActionMessages[keyboards.ActionShowSpendings]:
		// Re-route to /list via a synthetic command pass-through.
		h.sendSimple(chatID, "Use /list to view recent spendings.")
		return
	case keyboards.ActionMessages[keyboards.ActionShowCategories]:
		h.sendSimple(chatID, "Use /categories to manage your categories.")
		return
	}

	currentState, stateErr := h.StateManager.GetCurrentState(ctx, userID)
	if stateErr != nil {
		log.Printf("[warn] failed to get current state for user %d: %v", userID, stateErr)
		return
	}

	// Per-state input validation: catch bad input *before* it lands in FSM data, so the
	// user can correct without dropping out of the flow.
	switch currentState.Current() {
	case stateAwaitingAmountInput, stateAwaitingBudgetAmount:
		if _, _, err := parseAmount(messageText, ""); err != nil {
			h.sendSimple(chatID, "Invalid amount: "+err.Error()+". Try again, or /cancel.")
			return
		}
	case stateAwaitingDateInput:
		if _, err := parseDate(messageText, time.Now()); err != nil {
			h.sendSimple(chatID, "Invalid date: "+err.Error()+". Try again, or /cancel.")
			return
		}
	case stateAwaitingNewCategoryName, stateAwaitingEditCategoryName:
		if reason, ok := validateCategoryName(messageText, h.TbKeyboards.IsReservedActionLabel); !ok {
			h.sendSimple(chatID, "Invalid name: "+reason+". Try again, or /cancel.")
			return
		}
	case stateAwaitingNewCategoryEmoji, stateAwaitingEditCategoryEmoji:
		if reason, ok := validateEmoji(messageText); !ok {
			h.sendSimple(chatID, "Invalid emoji: "+reason+". Try again, or /cancel.")
			return
		}
	case stateAwaitingEditSpendingValue:
		// Validation depends on the chosen field (stored in FSM data), and is performed
		// in saveEditedSpending. On failure the bot drops back to Idle with an
		// explanatory message.
	case stateAwaitingDescriptionInput:
		// Description is freeform; "Skip" is a recognized sentinel handled in saveSpending.
	}

	nextStates := currentState.AvailableTransitions()
	switch len(nextStates) {
	case 0:
		log.Printf("[info] ignoring message %q from user %d in terminal state %s", messageText, userID, currentState.Current())
		return
	case 1:
		if err := h.StateManager.TriggerStateChange(ctx, userID, nextStates[0], messageText); err != nil {
			log.Printf("[warn] error triggering state change: %v", err)
		}
	default:
		log.Printf("[warn] more than one transition from state %s for user %d: %v", currentState.Current(), userID, nextStates)
	}
}

func (h *BotMessageHandler) startAddSpending(ctx context.Context, userID, chatID int64) {
	hasCats, err := h.StateManager.HasCategories(userID)
	if err != nil {
		log.Printf("[warn] failed to check categories for user %d: %v", userID, err)
		h.sendSimple(chatID, "Failed to load categories. Please try again later.")
		return
	}
	if !hasCats {
		h.sendSimple(chatID, "You don't have any categories yet. Tap *New spending category* to create one.")
		return
	}
	if err := h.StateManager.TriggerStateChange(ctx, userID, "ChooseAddSpending", ""); err != nil {
		log.Printf("[warn] error starting add spending: %v", err)
	}
}

func (h *BotMessageHandler) sendSimple(chatID int64, text string) {
	msg := tbapi.NewMessage(chatID, text)
	msg.ParseMode = tbapi.ModeMarkdown
	if err := send(msg, h.TbAPI); err != nil {
		log.Printf("[warn] failed to send response: %v", err)
	}
}
