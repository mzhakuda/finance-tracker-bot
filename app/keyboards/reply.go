package keyboards

import tbapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

// Action identifiers
const (
	ActionAddSpending         = "ADD_SPENDING"
	ActionNewSpendingCategory = "NEW_SPENDING_CATEGORY"
	ActionShowSpendings       = "SHOW_SPENDINGS"
	ActionShowCategories      = "SHOW_CATEGORIES"
)

// SkipDescriptionLabel is the label of the inline reply button used to omit a spending description.
const SkipDescriptionLabel = "Skip"

// ActionMessages maps action identifiers to user-facing text.
var ActionMessages = map[string]string{
	ActionAddSpending:         "Add spending",
	ActionNewSpendingCategory: "New spending category",
	ActionShowSpendings:       "Recent spendings",
	ActionShowCategories:      "Manage categories",
}

// reservedLabels lists user-facing strings that must not be used as category names —
// otherwise they'd shadow main-menu reply buttons.
var reservedLabels = func() map[string]struct{} {
	m := make(map[string]struct{}, len(ActionMessages)+1)
	for _, v := range ActionMessages {
		m[v] = struct{}{}
	}
	m[SkipDescriptionLabel] = struct{}{}
	return m
}()

// IsReservedActionLabel reports whether the supplied text matches any main-menu action label.
func (tbk *TbKeyboardProvider) IsReservedActionLabel(text string) bool {
	_, ok := reservedLabels[text]
	return ok
}

// GetMainKeyboard generates the main keyboard with dynamic actions.
func (tbk *TbKeyboardProvider) GetMainKeyboard() tbapi.ReplyKeyboardMarkup {
	return tbapi.ReplyKeyboardMarkup{
		Keyboard: [][]tbapi.KeyboardButton{
			{{Text: ActionMessages[ActionAddSpending]}, {Text: ActionMessages[ActionNewSpendingCategory]}},
			{{Text: ActionMessages[ActionShowSpendings]}, {Text: ActionMessages[ActionShowCategories]}},
		},
		ResizeKeyboard: true,
	}
}

// GetSkipKeyboard returns a one-button reply keyboard used for optional inputs.
func (tbk *TbKeyboardProvider) GetSkipKeyboard() tbapi.ReplyKeyboardMarkup {
	return tbapi.ReplyKeyboardMarkup{
		Keyboard:        [][]tbapi.KeyboardButton{{{Text: SkipDescriptionLabel}}},
		ResizeKeyboard:  true,
		OneTimeKeyboard: true,
	}
}
