package events

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	tbapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

const helpText = "*Finance Tracker Bot*\n" +
	"\n" +
	"Available commands:\n" +
	"/start — show the main menu\n" +
	"/cancel — cancel the current operation\n" +
	"/list — show recent spendings\n" +
	"/total — total spent this month\n" +
	"/categories — manage categories\n" +
	"/help — this message"

type BotCommandHandler struct {
	TbAPI        TbAPI
	TbKeyboards  TbKeyboards
	StateManager StateManager
	Categories   CategoriesRepository
	Spendings    SpendingsRepository
}

func (h *BotCommandHandler) HandleCommands(ctx context.Context, update tbapi.Update) {
	if update.Message == nil || update.Message.From == nil {
		return
	}
	userID := update.Message.From.ID
	chatID := update.Message.Chat.ID

	switch update.Message.Command() {
	case "start":
		h.StateManager.SetIdleState(ctx, userID)
		// SetIdleState already prompts via FSM enter_Idle callback; nothing else to do.
	case "cancel":
		h.StateManager.ResetToIdle(ctx, userID)
	case "help":
		h.sendMarkdown(chatID, helpText, h.TbKeyboards.GetMainKeyboard())
	case "list", "spendings":
		h.handleList(ctx, userID, chatID)
	case "total":
		h.handleTotal(ctx, userID, chatID)
	case "categories":
		h.handleCategories(ctx, userID, chatID)
	default:
		h.sendMarkdown(chatID, "Unknown command. Try /help.", nil)
	}
}

func (h *BotCommandHandler) handleList(_ context.Context, userID, chatID int64) {
	rows, err := h.Spendings.RecentSpendings(userID, 10)
	if err != nil {
		log.Printf("[warn] failed to load recent spendings for user %d: %v", userID, err)
		h.sendMarkdown(chatID, "Failed to load recent spendings.", nil)
		return
	}
	if len(rows) == 0 {
		h.sendMarkdown(chatID, "No spendings yet. Add one via the main menu.", h.TbKeyboards.GetMainKeyboard())
		return
	}

	var b strings.Builder
	b.WriteString("*Recent spendings:*\n")
	for _, sp := range rows {
		category := "?"
		if sp.CategoryEmoji.Valid && sp.CategoryEmoji.String != "" {
			category = sp.CategoryEmoji.String
		}
		if sp.CategoryName.Valid && sp.CategoryName.String != "" {
			category = category + " " + sp.CategoryName.String
		}
		line := fmt.Sprintf("• %s — %s — %.2f %s",
			sp.Timestamp.Local().Format("02 Jan 15:04"),
			category,
			sp.Amount,
			sp.Currency,
		)
		if strings.TrimSpace(sp.Description) != "" {
			line += " (" + sp.Description + ")"
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	keyboard := h.TbKeyboards.GetSpendingsManagementKeyboard(userID, 10)
	h.sendMarkdown(chatID, b.String(), &keyboard)
}

func (h *BotCommandHandler) handleTotal(_ context.Context, userID, chatID int64) {
	now := time.Now()
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())

	totals, err := h.Spendings.TotalSince(userID, monthStart)
	if err != nil {
		log.Printf("[warn] failed to compute totals for user %d: %v", userID, err)
		h.sendMarkdown(chatID, "Failed to load totals.", nil)
		return
	}
	if len(totals) == 0 {
		h.sendMarkdown(chatID, fmt.Sprintf("No spendings since %s.", monthStart.Format("02 Jan 2006")), h.TbKeyboards.GetMainKeyboard())
		return
	}
	sort.Slice(totals, func(i, j int) bool { return totals[i].Currency < totals[j].Currency })

	var b strings.Builder
	b.WriteString(fmt.Sprintf("*Total since %s:*\n", monthStart.Format("02 Jan 2006")))
	for _, t := range totals {
		currency := t.Currency
		if currency == "" {
			currency = "(unspecified)"
		}
		b.WriteString(fmt.Sprintf("• %.2f %s\n", t.Total, currency))
	}
	h.sendMarkdown(chatID, b.String(), h.TbKeyboards.GetMainKeyboard())
}

func (h *BotCommandHandler) handleCategories(_ context.Context, userID, chatID int64) {
	cats, err := h.Categories.ListCategories(userID)
	if err != nil {
		log.Printf("[warn] failed to list categories for user %d: %v", userID, err)
		h.sendMarkdown(chatID, "Failed to load categories.", nil)
		return
	}
	if len(cats) == 0 {
		h.sendMarkdown(chatID, "You don't have any categories yet. Tap *New spending category* to create one.", h.TbKeyboards.GetMainKeyboard())
		return
	}

	var b strings.Builder
	b.WriteString("*Your categories:*\n")
	for _, cat := range cats {
		b.WriteString(fmt.Sprintf("• %s %s\n", cat.Emoji, cat.Name))
	}
	keyboard := h.TbKeyboards.GetCategoriesManagementKeyboard(userID)
	h.sendMarkdown(chatID, b.String(), &keyboard)
}

func (h *BotCommandHandler) sendMarkdown(chatID int64, text string, keyboard interface{}) {
	msg := tbapi.NewMessage(chatID, text)
	msg.ParseMode = tbapi.ModeMarkdown
	msg.DisableWebPagePreview = true
	if keyboard != nil {
		msg.ReplyMarkup = keyboard
	}
	if err := send(msg, h.TbAPI); err != nil {
		log.Printf("[warn] failed to send markdown response: %v", err)
	}
}

