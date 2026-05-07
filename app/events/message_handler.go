package events

import (
	"context"
	"log"

	tbapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/nyanyamaga/finance-tracker-bot/app/keyboards"
)

type BotMessageHandler struct {
	TbAPI        TbAPI
	StateManager StateManager
}

func (h *BotMessageHandler) HandleMessages(ctx context.Context, update tbapi.Update) {
	if update.Message == nil || update.Message.From == nil {
		return
	}

	userID := update.Message.From.ID
	messageText := update.Message.Text

	var err error
	switch messageText {
	case keyboards.ActionMessages[keyboards.ActionAddSpending]:
		err = h.StateManager.TriggerStateChange(ctx, userID, "ChooseAddSpending", "")
	case keyboards.ActionMessages[keyboards.ActionNewSpendingCategory]:
		err = h.StateManager.TriggerStateChange(ctx, userID, "ChooseAddCategory", "")
	default:
		currentState, stateErr := h.StateManager.GetCurrentState(ctx, userID)
		if stateErr != nil {
			log.Printf("[warn] failed to get current state for user %d: %v", userID, stateErr)
			return
		}

		nextStates := currentState.AvailableTransitions()
		switch len(nextStates) {
		case 0:
			log.Printf("[info] ignoring message %q from user %d in terminal state %s", messageText, userID, currentState.Current())
			return
		case 1:
			err = h.StateManager.TriggerStateChange(ctx, userID, nextStates[0], messageText)
		default:
			log.Printf("[warn] more than one available transition from state %s for user %d: %v", currentState.Current(), userID, nextStates)
			return
		}
	}

	if err != nil {
		log.Printf("[warn] error triggering state change: %v", err)
	}
}
