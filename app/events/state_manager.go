package events

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	tbapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/looplab/fsm"

	"github.com/nyanyamaga/finance-tracker-bot/app/keyboards"
	"github.com/nyanyamaga/finance-tracker-bot/app/storage"
)

const (
	stateIdle                       = "Idle"
	stateAwaitingCategorySelection  = "AwaitingCategorySelection"
	stateAwaitingAmountInput        = "AwaitingAmountInput"
	stateAwaitingDescriptionInput   = "AwaitingDescriptionInput"
	stateSaveSpending               = "SaveSpending"
	stateAwaitingNewCategoryName    = "AwaitingNewCategoryName"
	stateAwaitingNewCategoryEmoji   = "AwaitingNewCategoryEmoji"
	stateAwaitingSaveCategoryName   = "AwaitingSaveCategoryName"
)

const (
	maxCategoryNameLength = 32
	maxDescriptionLength  = 200
	defaultRecentLimit    = 10
)

// dataKey* are the JSON keys used to persist user-state values across FSM transitions.
const (
	dataKeyCategorySelected        = "CategorySelected"
	dataKeyAmountEntered           = "AmountEntered"
	dataKeyDescriptionEntered      = "DescriptionEntered"
	dataKeyAmountValue             = "AmountValue"   // numeric, validated
	dataKeyAmountCurrency          = "AmountCurrency"
	dataKeyNewCategoryNameEntered  = "NewCategoryNameEntered"
	dataKeyNewCategoryEmojiEntered = "NewCategoryEmojiEntered"
)

type BotStateManager struct {
	TbAPI           TbAPI
	TbKeyboards     TbKeyboards
	UserState       UserStateRepository
	Categories      CategoriesRepository
	Spendings       SpendingsRepository
	DefaultCurrency string

	mu         sync.Mutex
	UserFSMs   map[int64]*fsm.FSM
	UserValues map[int64]string
}

func NewBotStateManager(
	tbAPI TbAPI,
	tbKeyboards TbKeyboards,
	usRepository UserStateRepository,
	cRepository CategoriesRepository,
	sRepository SpendingsRepository,
	defaultCurrency string,
) *BotStateManager {
	return &BotStateManager{
		TbAPI:           tbAPI,
		TbKeyboards:     tbKeyboards,
		UserState:       usRepository,
		Categories:      cRepository,
		Spendings:       sRepository,
		DefaultCurrency: defaultCurrency,
		UserFSMs:        make(map[int64]*fsm.FSM),
		UserValues:      make(map[int64]string),
	}
}

func (sm *BotStateManager) newUserFSM(userID int64, initialState string) *fsm.FSM {
	return fsm.NewFSM(
		initialState,
		fsm.Events{
			{Name: "ChooseAddSpending", Src: []string{stateIdle}, Dst: stateAwaitingCategorySelection},
			{Name: "ChooseAddCategory", Src: []string{stateIdle}, Dst: stateAwaitingNewCategoryName},

			{Name: "NewCategoryNameEntered", Src: []string{stateAwaitingNewCategoryName}, Dst: stateAwaitingNewCategoryEmoji},
			{Name: "NewCategoryEmojiEntered", Src: []string{stateAwaitingNewCategoryEmoji}, Dst: stateAwaitingSaveCategoryName},
			{Name: "SaveNewCategory", Src: []string{stateAwaitingSaveCategoryName}, Dst: stateIdle},

			{Name: "CategorySelected", Src: []string{stateAwaitingCategorySelection}, Dst: stateAwaitingAmountInput},
			{Name: "AmountEntered", Src: []string{stateAwaitingAmountInput}, Dst: stateAwaitingDescriptionInput},
			{Name: "DescriptionEntered", Src: []string{stateAwaitingDescriptionInput}, Dst: stateSaveSpending},
			{Name: "SpendingSaved", Src: []string{stateSaveSpending}, Dst: stateIdle},

			// Universal escape hatch — used by /cancel and on validation failure.
			{Name: "ResetToIdle", Src: []string{
				stateAwaitingCategorySelection,
				stateAwaitingAmountInput,
				stateAwaitingDescriptionInput,
				stateSaveSpending,
				stateAwaitingNewCategoryName,
				stateAwaitingNewCategoryEmoji,
				stateAwaitingSaveCategoryName,
			}, Dst: stateIdle},
		},
		fsm.Callbacks{
			"leave_state":                              func(ctx context.Context, e *fsm.Event) { sm.leaveState(e, userID) },
			"enter_" + stateIdle:                       func(ctx context.Context, e *fsm.Event) { sm.promptEnterIdle(userID) },
			"enter_" + stateAwaitingCategorySelection:  func(ctx context.Context, e *fsm.Event) { sm.promptCategorySelection(userID) },
			"enter_" + stateAwaitingAmountInput:        func(ctx context.Context, e *fsm.Event) { sm.promptAmountInput(userID) },
			"enter_" + stateAwaitingDescriptionInput:   func(ctx context.Context, e *fsm.Event) { sm.promptDescriptionInput(userID) },
			"enter_" + stateSaveSpending:               func(ctx context.Context, e *fsm.Event) { sm.saveSpending(ctx, userID) },
			"enter_" + stateAwaitingNewCategoryName:    func(ctx context.Context, e *fsm.Event) { sm.promptNewCategoryName(userID) },
			"enter_" + stateAwaitingNewCategoryEmoji:   func(ctx context.Context, e *fsm.Event) { sm.promptNewCategoryEmoji(userID) },
			"enter_" + stateAwaitingSaveCategoryName:   func(ctx context.Context, e *fsm.Event) { sm.promptSaveNewCategory(ctx, userID) },
		},
	)
}

func (sm *BotStateManager) getOrCreateFSM(userID int64) *fsm.FSM {
	if existing, ok := sm.UserFSMs[userID]; ok {
		return existing
	}

	initial := stateIdle
	if persisted, err := sm.UserState.Read(userID); err == nil && persisted != nil && persisted.State != "" {
		switch persisted.State {
		case stateIdle,
			stateAwaitingCategorySelection,
			stateAwaitingAmountInput,
			stateAwaitingDescriptionInput,
			stateSaveSpending,
			stateAwaitingNewCategoryName,
			stateAwaitingNewCategoryEmoji,
			stateAwaitingSaveCategoryName:
			initial = persisted.State
		}
	}

	userFSM := sm.newUserFSM(userID, initial)
	sm.UserFSMs[userID] = userFSM
	return userFSM
}

func (sm *BotStateManager) InitializeUserFSM(ctx context.Context, userID int64) {
	sm.mu.Lock()
	sm.UserFSMs[userID] = sm.newUserFSM(userID, stateIdle)
	sm.mu.Unlock()

	initialState := storage.UserStateInfo{
		UserID:   userID,
		State:    stateIdle,
		DataJSON: "{}",
	}
	if err := sm.UserState.Write(initialState); err != nil {
		log.Printf("[error] failed to create initial state for user %d: %v", userID, err)
	}
}

func (sm *BotStateManager) TriggerStateChange(ctx context.Context, userID int64, action, value string) error {
	sm.mu.Lock()
	userFSM := sm.getOrCreateFSM(userID)
	sm.UserValues[userID] = value
	sm.mu.Unlock()

	if !userFSM.Can(action) {
		return fmt.Errorf("can't trigger %s event from state %s", action, userFSM.Current())
	}
	if err := userFSM.Event(ctx, action, value); err != nil {
		return fmt.Errorf("fsm event %s failed: %w", action, err)
	}
	return nil
}

func (sm *BotStateManager) GetCurrentState(ctx context.Context, userID int64) (*fsm.FSM, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if existing, ok := sm.UserFSMs[userID]; ok {
		return existing, nil
	}
	return sm.getOrCreateFSM(userID), nil
}

func (sm *BotStateManager) SetIdleState(ctx context.Context, userID int64) {
	sm.InitializeUserFSM(ctx, userID)
}

// ResetToIdle forces the user's FSM back to Idle. Used by /cancel and validation failures.
func (sm *BotStateManager) ResetToIdle(ctx context.Context, userID int64) {
	sm.mu.Lock()
	userFSM, ok := sm.UserFSMs[userID]
	sm.mu.Unlock()
	if !ok {
		sm.SetIdleState(ctx, userID)
		return
	}
	if userFSM.Current() == stateIdle {
		sm.promptEnterIdle(userID)
		return
	}
	if err := userFSM.Event(ctx, "ResetToIdle"); err != nil {
		log.Printf("[warn] failed to reset user %d to Idle: %v", userID, err)
		sm.SetIdleState(ctx, userID)
	}
}

// HasCategories reports whether the user has at least one category.
func (sm *BotStateManager) HasCategories(userID int64) (bool, error) {
	cats, err := sm.Categories.ListCategories(userID)
	if err != nil {
		return false, err
	}
	return len(cats) > 0, nil
}

func (sm *BotStateManager) leaveState(e *fsm.Event, userID int64) {
	sm.mu.Lock()
	userValue := sm.UserValues[userID]
	delete(sm.UserValues, userID)
	sm.mu.Unlock()

	updatedData := make(map[string]interface{})
	if e.Dst != stateIdle {
		fetched, err := sm.getStateData(userID)
		if err == nil && fetched != nil {
			updatedData = fetched
		}
		updatedData[e.Event] = userValue
	}

	dataJSON, err := json.Marshal(updatedData)
	if err != nil {
		log.Printf("[error] failed to marshal updated state data to JSON for user %d: %v", userID, err)
		return
	}

	stateInfo := storage.UserStateInfo{
		UserID:   userID,
		State:    e.Dst,
		DataJSON: string(dataJSON),
	}

	if err := sm.UserState.Write(stateInfo); err != nil {
		log.Printf("[error] failed to save updated user state for user %d: %v", userID, err)
		return
	}

	log.Printf("[info] user %d entered state %s", userID, e.Dst)
}

func (sm *BotStateManager) promptEnterIdle(userID int64) {
	keyboard := sm.TbKeyboards.GetMainKeyboard()
	if err := sm.sendBotResponse(userID, "Choose an option:", keyboard); err != nil {
		log.Printf("[warn] error sending main message: %v", err)
	}
}

func (sm *BotStateManager) promptCategorySelection(userID int64) {
	categories, err := sm.Categories.ListCategories(userID)
	if err != nil {
		log.Printf("[warn] error listing categories for user %d: %v", userID, err)
		_ = sm.sendBotResponse(userID, "Failed to load categories. Please try again later.", sm.TbKeyboards.GetMainKeyboard())
		// Reset asynchronously via FSM event.
		if userFSM := sm.fsmFor(userID); userFSM != nil {
			_ = userFSM.Event(context.Background(), "ResetToIdle")
		}
		return
	}
	if len(categories) == 0 {
		_ = sm.sendBotResponse(userID, "You don't have any categories yet. Tap *New spending category* to create one.", sm.TbKeyboards.GetMainKeyboard())
		if userFSM := sm.fsmFor(userID); userFSM != nil {
			_ = userFSM.Event(context.Background(), "ResetToIdle")
		}
		return
	}

	keyboard := sm.TbKeyboards.GetCategoryKeyboard(userID)
	if err := sm.sendBotResponse(userID, "Please select a category:", &keyboard); err != nil {
		log.Printf("[warn] error sending category selection prompt: %v", err)
	}
}

func (sm *BotStateManager) promptNewCategoryName(userID int64) {
	if err := sm.sendBotResponse(userID, "Please enter the name of the new category (or /cancel):", noKeyboard()); err != nil {
		log.Printf("[warn] error sending new category name prompt: %v", err)
	}
}

func (sm *BotStateManager) promptNewCategoryEmoji(userID int64) {
	if err := sm.sendBotResponse(userID, "Please enter the emoji for the new category:", noKeyboard()); err != nil {
		log.Printf("[warn] error sending new category emoji prompt: %v", err)
	}
}

func (sm *BotStateManager) promptSaveNewCategory(ctx context.Context, userID int64) {
	stateData, err := sm.getStateData(userID)
	if err != nil {
		log.Printf("[warn] error fetching state data: %v", err)
		sm.ResetToIdle(ctx, userID)
		return
	}

	name, _ := stringField(stateData, dataKeyNewCategoryNameEntered)
	emoji, _ := stringField(stateData, dataKeyNewCategoryEmojiEntered)
	if reason, ok := validateCategoryName(name, sm.TbKeyboards.IsReservedActionLabel); !ok {
		_ = sm.sendBotResponse(userID, "Invalid category name: "+reason, sm.TbKeyboards.GetMainKeyboard())
		sm.ResetToIdle(ctx, userID)
		return
	}
	if reason, ok := validateEmoji(emoji); !ok {
		_ = sm.sendBotResponse(userID, "Invalid emoji: "+reason, sm.TbKeyboards.GetMainKeyboard())
		sm.ResetToIdle(ctx, userID)
		return
	}

	category := storage.CategoryInfo{
		UserID: userID,
		Name:   strings.TrimSpace(name),
		Emoji:  strings.TrimSpace(emoji),
	}

	if err = sm.Categories.AddOrUpdateCategory(category); err != nil {
		log.Printf("[warn] error saving new category: %v", err)
		_ = sm.sendBotResponse(userID, "Failed to save category. Please try again.", sm.TbKeyboards.GetMainKeyboard())
		sm.ResetToIdle(ctx, userID)
		return
	}

	if err := sm.sendBotResponse(userID, fmt.Sprintf("Category saved: %s %s", category.Emoji, category.Name), nil); err != nil {
		log.Printf("[warn] error sending new category save prompt: %v", err)
	}

	if userFSM := sm.fsmFor(userID); userFSM != nil {
		if err := userFSM.Event(ctx, "SaveNewCategory"); err != nil {
			log.Printf("[error] failed to transition to Idle for user %d: %v", userID, err)
		}
	}
}

// sendBotResponse delivers a message. The keyboard parameter follows these conventions:
//   - tbapi.ReplyKeyboardMarkup / *InlineKeyboardMarkup: shown to user
//   - nil: do not touch the existing keyboard (Telegram keeps the previous reply keyboard)
//   - noKeyboard() sentinel: explicitly remove the reply keyboard
func (sm *BotStateManager) sendBotResponse(chatID int64, text string, keyboard interface{}) error {
	tbMsg := tbapi.NewMessage(chatID, text)
	tbMsg.ParseMode = tbapi.ModeMarkdown
	tbMsg.DisableWebPagePreview = true

	switch v := keyboard.(type) {
	case removeKeyboardSentinel:
		tbMsg.ReplyMarkup = tbapi.NewRemoveKeyboard(true)
		_ = v
	case nil:
		// leave ReplyMarkup unset — keeps previous keyboard visible
	default:
		tbMsg.ReplyMarkup = keyboard
	}

	if err := send(tbMsg, sm.TbAPI); err != nil {
		return fmt.Errorf("can't send message to telegram chat=%d: %w", chatID, err)
	}
	return nil
}

// removeKeyboardSentinel is a typed marker so callers can ask to drop the reply keyboard
// while keeping `nil` to mean "don't touch the keyboard".
type removeKeyboardSentinel struct{}

func noKeyboard() removeKeyboardSentinel { return removeKeyboardSentinel{} }

func (sm *BotStateManager) getStateData(userID int64) (map[string]interface{}, error) {
	currentStateInfo, err := sm.UserState.Read(userID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch current state data for user %d: %w", userID, err)
	}

	data, err := unmarshalUserData(currentStateInfo.DataJSON)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal data: %w", err)
	}
	return data, nil
}

func (sm *BotStateManager) promptAmountInput(userID int64) {
	hint := fmt.Sprintf("Please enter the amount (e.g. `12.50` or `12.50 EUR`). Default currency: %s.", sm.defaultCurrency())
	if err := sm.sendBotResponse(userID, hint, noKeyboard()); err != nil {
		log.Printf("[warn] error sending amount prompt: %v", err)
	}
}

func (sm *BotStateManager) promptDescriptionInput(userID int64) {
	if err := sm.sendBotResponse(userID, "Please enter a description, or tap *Skip*:", sm.TbKeyboards.GetSkipKeyboard()); err != nil {
		log.Printf("[warn] error sending description prompt: %v", err)
	}
}

func (sm *BotStateManager) defaultCurrency() string {
	if sm.DefaultCurrency == "" {
		return "USD"
	}
	return sm.DefaultCurrency
}

func (sm *BotStateManager) saveSpending(ctx context.Context, userID int64) {
	stateData, err := sm.getStateData(userID)
	if err != nil {
		log.Printf("[warn] error fetching state data: %v", err)
		sm.ResetToIdle(ctx, userID)
		return
	}

	categoryID, err := extractCategoryID(stateData)
	if err != nil {
		log.Printf("[warn] error parsing category for user %d: %v", userID, err)
		_ = sm.sendBotResponse(userID, "Failed to read selected category. Returning to the main menu.", sm.TbKeyboards.GetMainKeyboard())
		sm.ResetToIdle(ctx, userID)
		return
	}

	// Verify the user actually owns the selected category. Without this, a forged callback
	// payload could attach a spending to someone else's category.
	if _, err := sm.Categories.GetCategoryForUser(userID, categoryID); err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			log.Printf("[warn] user %d attempted to use category %d not belonging to them", userID, categoryID)
			_ = sm.sendBotResponse(userID, "Selected category is no longer available.", sm.TbKeyboards.GetMainKeyboard())
		} else {
			log.Printf("[warn] error verifying category ownership: %v", err)
			_ = sm.sendBotResponse(userID, "Failed to verify category. Please try again.", sm.TbKeyboards.GetMainKeyboard())
		}
		sm.ResetToIdle(ctx, userID)
		return
	}

	amountValue, currency, err := readAmountFromState(stateData, sm.defaultCurrency())
	if err != nil {
		log.Printf("[info] invalid amount for user %d: %v", userID, err)
		_ = sm.sendBotResponse(userID, fmt.Sprintf("Invalid amount: %s. Returning to the main menu.", err.Error()), sm.TbKeyboards.GetMainKeyboard())
		sm.ResetToIdle(ctx, userID)
		return
	}

	description, _ := stringField(stateData, dataKeyDescriptionEntered)
	description = strings.TrimSpace(description)
	if description == keyboards.SkipDescriptionLabel {
		description = ""
	}
	if utf8.RuneCountInString(description) > maxDescriptionLength {
		description = string([]rune(description)[:maxDescriptionLength])
	}

	spending := storage.SpendingInfo{
		UserID:      userID,
		CategoryID:  categoryID,
		Amount:      amountValue,
		Currency:    currency,
		Description: description,
		Timestamp:   time.Now().UTC(),
	}

	if err := sm.Spendings.AddSpending(spending); err != nil {
		log.Printf("[warn] error saving spending for user %d: %v", userID, err)
		_ = sm.sendBotResponse(userID, "Failed to save spending. Please try again.", sm.TbKeyboards.GetMainKeyboard())
		sm.ResetToIdle(ctx, userID)
		return
	}

	confirmation := fmt.Sprintf("Spending saved: %.2f %s", amountValue, currency)
	if description != "" {
		confirmation = confirmation + " — " + description
	}
	if err := sm.sendBotResponse(userID, confirmation, nil); err != nil {
		log.Printf("[warn] error sending spending save prompt: %v", err)
	}

	if userFSM := sm.fsmFor(userID); userFSM != nil {
		if err := userFSM.Event(ctx, "SpendingSaved"); err != nil {
			log.Printf("[warn] error transitioning to Idle for user %d: %v", userID, err)
		}
	}
}

func (sm *BotStateManager) fsmFor(userID int64) *fsm.FSM {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	return sm.UserFSMs[userID]
}

// ---------- helpers ----------

func stringField(data map[string]interface{}, key string) (string, bool) {
	raw, ok := data[key]
	if !ok || raw == nil {
		return "", false
	}
	s, ok := raw.(string)
	return s, ok
}

func extractCategoryID(data map[string]interface{}) (int64, error) {
	raw, ok := stringField(data, dataKeyCategorySelected)
	if !ok {
		return 0, errors.New("category not selected")
	}
	const prefix = keyboards.CallbackCategory
	if !strings.HasPrefix(raw, prefix) {
		return 0, fmt.Errorf("malformed category payload %q", raw)
	}
	idStr := strings.TrimPrefix(raw, prefix)
	if idStr == "" {
		return 0, fmt.Errorf("missing category id in %q", raw)
	}
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid category id %q: %w", idStr, err)
	}
	if id <= 0 {
		return 0, fmt.Errorf("non-positive category id: %d", id)
	}
	return id, nil
}

// parseAmount validates and parses an amount expression. It accepts:
//   - "12.50"
//   - "12,50"
//   - "12.50 EUR"  (uppercase 3-letter ISO 4217-ish; we don't validate the alphabet)
//   - "12.50EUR"
//
// Currency is uppercased and trimmed; if absent, the supplied default is used.
func parseAmount(raw, defaultCurrency string) (float64, string, error) {
	cleaned := strings.TrimSpace(raw)
	if cleaned == "" {
		return 0, "", errors.New("amount is empty")
	}

	// Split on the last whitespace; the suffix (if alphabetic) is treated as currency.
	currency := strings.ToUpper(strings.TrimSpace(defaultCurrency))
	amountPart := cleaned

	if idx := strings.LastIndexAny(cleaned, " \t"); idx >= 0 {
		head := strings.TrimSpace(cleaned[:idx])
		tail := strings.TrimSpace(cleaned[idx+1:])
		if isCurrencyToken(tail) {
			amountPart = head
			currency = strings.ToUpper(tail)
		}
	} else {
		// Try trailing letters without space, e.g. "12.50EUR".
		i := len(cleaned)
		for i > 0 && isCurrencyRune(rune(cleaned[i-1])) {
			i--
		}
		if i < len(cleaned) && i > 0 {
			suffix := cleaned[i:]
			if isCurrencyToken(suffix) {
				amountPart = strings.TrimSpace(cleaned[:i])
				currency = strings.ToUpper(suffix)
			}
		}
	}

	amountPart = strings.ReplaceAll(amountPart, ",", ".")
	if amountPart == "" {
		return 0, "", errors.New("amount is empty")
	}
	value, err := strconv.ParseFloat(amountPart, 64)
	if err != nil {
		return 0, "", fmt.Errorf("not a number: %q", raw)
	}
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, "", errors.New("amount is not a finite number")
	}
	if value <= 0 {
		return 0, "", fmt.Errorf("amount must be positive, got %g", value)
	}
	if currency == "" {
		return 0, "", errors.New("currency is empty")
	}
	return value, currency, nil
}

// readAmountFromState extracts a previously-validated amount/currency from state, falling
// back to re-parsing the raw user input if the validated values aren't there.
func readAmountFromState(data map[string]interface{}, defaultCurrency string) (float64, string, error) {
	if v, ok := data[dataKeyAmountValue]; ok {
		if f, ok := v.(float64); ok && f > 0 {
			currency, _ := stringField(data, dataKeyAmountCurrency)
			if currency == "" {
				currency = strings.ToUpper(strings.TrimSpace(defaultCurrency))
			}
			return f, currency, nil
		}
	}
	raw, ok := stringField(data, dataKeyAmountEntered)
	if !ok {
		return 0, "", errors.New("amount not provided")
	}
	return parseAmount(raw, defaultCurrency)
}

func isCurrencyToken(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !isCurrencyRune(r) {
			return false
		}
	}
	return true
}

func isCurrencyRune(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
}

func validateCategoryName(raw string, isReserved func(string) bool) (string, bool) {
	name := strings.TrimSpace(raw)
	if name == "" {
		return "name cannot be empty", false
	}
	if utf8.RuneCountInString(name) > maxCategoryNameLength {
		return fmt.Sprintf("name is too long (max %d characters)", maxCategoryNameLength), false
	}
	if strings.HasPrefix(name, "/") {
		return "name cannot start with '/'", false
	}
	if isReserved != nil && isReserved(name) {
		return "this name is reserved by the main menu", false
	}
	return "", true
}

func validateEmoji(raw string) (string, bool) {
	emoji := strings.TrimSpace(raw)
	if emoji == "" {
		return "emoji cannot be empty", false
	}
	if utf8.RuneCountInString(emoji) > 8 {
		return "emoji is too long", false
	}
	return "", true
}

func unmarshalUserData(jsonData string) (map[string]interface{}, error) {
	data := make(map[string]interface{})
	if jsonData == "" {
		return data, nil
	}
	if err := json.Unmarshal([]byte(jsonData), &data); err != nil {
		return nil, fmt.Errorf("failed to unmarshal data: %w", err)
	}
	return data, nil
}
