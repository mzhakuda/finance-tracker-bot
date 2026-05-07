package events

import (
	"context"
	"log"

	tbapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type BotCommandHandler struct {
	TbAPI        TbAPI
	TbKeyboards  TbKeyboards
	StateManager StateManager
}

func (h *BotCommandHandler) HandleCommands(ctx context.Context, update tbapi.Update) {
	if update.Message == nil || update.Message.From == nil {
		return
	}
	userID := update.Message.From.ID

	switch update.Message.Command() {
	case "start":
		h.StateManager.SetIdleState(ctx, userID)

		msg := tbapi.NewMessage(update.Message.Chat.ID, "Welcome! Choose an option.")
		msg.ReplyMarkup = h.TbKeyboards.GetMainKeyboard()

		if _, err := h.TbAPI.Send(msg); err != nil {
			log.Printf("[warn] error sending welcome message: %v", err)
		}
	default:
		log.Printf("[info] unknown command %q from user %d", update.Message.Command(), userID)
	}
}
