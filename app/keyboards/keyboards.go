package keyboards

import (
	"fmt"
	"log"

	tbapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/nyanyamaga/finance-tracker-bot/app/storage"
)

// CategoriesLister abstracts category lookups for keyboard rendering.
type CategoriesLister interface {
	ListCategories(userID int64) ([]storage.CategoryInfo, error)
}

// SpendingsLister abstracts recent spending lookups for keyboard rendering.
type SpendingsLister interface {
	RecentSpendings(userID int64, limit int) ([]storage.SpendingDisplay, error)
}

type TbKeyboardProvider struct {
	Storage   CategoriesLister
	Spendings SpendingsLister
}

func NewTbKeyboardProvider(storage CategoriesLister, spendings SpendingsLister) *TbKeyboardProvider {
	return &TbKeyboardProvider{Storage: storage, Spendings: spendings}
}

// GetSpendingsManagementKeyboard renders the latest spendings with a delete button per row.
func (tbk *TbKeyboardProvider) GetSpendingsManagementKeyboard(userID int64, limit int) tbapi.InlineKeyboardMarkup {
	rows, err := tbk.Spendings.RecentSpendings(userID, limit)
	if err != nil {
		log.Printf("[warn] error loading recent spendings: %v", err)
		return tbapi.NewInlineKeyboardMarkup()
	}
	if len(rows) == 0 {
		return tbapi.NewInlineKeyboardMarkup()
	}

	keyboard := make([][]tbapi.InlineKeyboardButton, 0, len(rows))
	for _, sp := range rows {
		category := "?"
		if sp.CategoryEmoji.Valid && sp.CategoryEmoji.String != "" {
			category = sp.CategoryEmoji.String
		}
		if sp.CategoryName.Valid && sp.CategoryName.String != "" {
			category = category + " " + sp.CategoryName.String
		}
		amount := fmt.Sprintf("%.2f", sp.Amount)
		if sp.Currency != "" {
			amount = amount + " " + sp.Currency
		}
		label := fmt.Sprintf("%s • %s • %s", sp.Timestamp.Local().Format("02 Jan 15:04"), category, amount)

		row := []tbapi.InlineKeyboardButton{
			tbapi.NewInlineKeyboardButtonData(label, CallbackNoop),
			tbapi.NewInlineKeyboardButtonData("Delete", fmt.Sprintf("%s%d", CallbackDeleteSpending, sp.ID)),
		}
		keyboard = append(keyboard, row)
	}
	return tbapi.NewInlineKeyboardMarkup(keyboard...)
}
