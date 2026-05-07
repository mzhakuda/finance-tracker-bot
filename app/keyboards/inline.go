package keyboards

import (
	"fmt"
	"log"

	tbapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Callback prefixes for inline buttons. Keep them short and unambiguous.
const (
	CallbackCategory       = "category_"        // category selection during AddSpending
	CallbackDeleteCategory = "delcat_"          // delete category from management view
	CallbackDeleteSpending = "delspending_"     // delete recent spending
	CallbackNoop           = "noop"             // placeholder for non-actionable buttons
)

func (tbk *TbKeyboardProvider) GetCategoryKeyboard(userID int64) tbapi.InlineKeyboardMarkup {
	categories, err := tbk.Storage.ListCategories(userID)
	if err != nil {
		log.Printf("[warn] error retrieving categories: %v", err)
		return tbapi.NewInlineKeyboardMarkup()
	}

	var rows [][]tbapi.InlineKeyboardButton
	for _, category := range categories {
		buttonText := category.Emoji + " " + category.Name
		callbackData := fmt.Sprintf("%s%d", CallbackCategory, category.ID)

		row := []tbapi.InlineKeyboardButton{tbapi.NewInlineKeyboardButtonData(buttonText, callbackData)}
		rows = append(rows, row)
	}

	return tbapi.NewInlineKeyboardMarkup(rows...)
}

// GetCategoriesManagementKeyboard renders the user's categories with a delete button per row.
func (tbk *TbKeyboardProvider) GetCategoriesManagementKeyboard(userID int64) tbapi.InlineKeyboardMarkup {
	categories, err := tbk.Storage.ListCategories(userID)
	if err != nil {
		log.Printf("[warn] error retrieving categories: %v", err)
		return tbapi.NewInlineKeyboardMarkup()
	}
	if len(categories) == 0 {
		return tbapi.NewInlineKeyboardMarkup()
	}

	rows := make([][]tbapi.InlineKeyboardButton, 0, len(categories))
	for _, category := range categories {
		label := category.Emoji + " " + category.Name
		row := []tbapi.InlineKeyboardButton{
			tbapi.NewInlineKeyboardButtonData(label, CallbackNoop),
			tbapi.NewInlineKeyboardButtonData("Delete", fmt.Sprintf("%s%d", CallbackDeleteCategory, category.ID)),
		}
		rows = append(rows, row)
	}
	return tbapi.NewInlineKeyboardMarkup(rows...)
}
