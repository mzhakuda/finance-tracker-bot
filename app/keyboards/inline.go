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
	CallbackEditCategory   = "editcat_"         // start editing a category
	CallbackEditSpending   = "editsp_"          // start editing a spending
	CallbackEditField      = "editfield_"       // pick which spending field to edit
	CallbackPickCategory   = "pickcat_"         // pick a category as the new value
	CallbackNoop           = "noop"             // placeholder for non-actionable buttons
)

// Spending fields that the user can edit, used as the suffix of CallbackEditField.
const (
	EditFieldAmount      = "amount"
	EditFieldDescription = "description"
	EditFieldDate        = "date"
	EditFieldCategory    = "category"
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

// GetCategoriesManagementKeyboard renders the user's categories with edit/delete buttons.
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
			tbapi.NewInlineKeyboardButtonData("Edit", fmt.Sprintf("%s%d", CallbackEditCategory, category.ID)),
			tbapi.NewInlineKeyboardButtonData("Delete", fmt.Sprintf("%s%d", CallbackDeleteCategory, category.ID)),
		}
		rows = append(rows, row)
	}
	return tbapi.NewInlineKeyboardMarkup(rows...)
}

// GetCategoryPickKeyboard returns the same categories as GetCategoryKeyboard but with a
// distinct callback prefix used while editing a spending's category field.
func (tbk *TbKeyboardProvider) GetCategoryPickKeyboard(userID int64) tbapi.InlineKeyboardMarkup {
	categories, err := tbk.Storage.ListCategories(userID)
	if err != nil {
		log.Printf("[warn] error retrieving categories: %v", err)
		return tbapi.NewInlineKeyboardMarkup()
	}
	rows := make([][]tbapi.InlineKeyboardButton, 0, len(categories))
	for _, category := range categories {
		label := category.Emoji + " " + category.Name
		row := []tbapi.InlineKeyboardButton{
			tbapi.NewInlineKeyboardButtonData(label, fmt.Sprintf("%s%d", CallbackPickCategory, category.ID)),
		}
		rows = append(rows, row)
	}
	return tbapi.NewInlineKeyboardMarkup(rows...)
}

// GetEditSpendingFieldKeyboard returns an inline keyboard for picking which field to edit.
func (tbk *TbKeyboardProvider) GetEditSpendingFieldKeyboard() tbapi.InlineKeyboardMarkup {
	return tbapi.NewInlineKeyboardMarkup(
		[]tbapi.InlineKeyboardButton{
			tbapi.NewInlineKeyboardButtonData("Amount", CallbackEditField+EditFieldAmount),
			tbapi.NewInlineKeyboardButtonData("Description", CallbackEditField+EditFieldDescription),
		},
		[]tbapi.InlineKeyboardButton{
			tbapi.NewInlineKeyboardButtonData("Date", CallbackEditField+EditFieldDate),
			tbapi.NewInlineKeyboardButtonData("Category", CallbackEditField+EditFieldCategory),
		},
	)
}
