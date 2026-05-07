package events

import (
	"context"
	"log"

	tbapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type BotCallbackQueryHandler struct {
	StateManager StateManager
}

func (h *BotCallbackQueryHandler) HandleCallbackQuery(ctx context.Context, update tbapi.Update) {
	if update.CallbackQuery == nil {
		log.Println("[warn] received an update that is not a callback query")
		return
	}

	userID := update.CallbackQuery.From.ID
	callbackData := update.CallbackQuery.Data

	log.Printf("[info] handling callback query: user %d, data %s", userID, callbackData)

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
		log.Printf("[warn] more than one available transition from state %s for user %d: %v", currentState.Current(), userID, nextStates)
		return
	}

	if err := h.StateManager.TriggerStateChange(ctx, userID, nextStates[0], callbackData); err != nil {
		log.Printf("[warn] error triggering state change: %v", err)
	}
}
