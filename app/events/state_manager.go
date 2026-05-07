package events

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	tbapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/looplab/fsm"
	"github.com/nyanyamaga/finance-tracker-bot/app/storage"
)

const (
	stateIdle                       = "Idle"
	stateAwaitingCategorySelection  = "AwaitingCategorySelection"
	stateAwaitingAmountInput        = "AwaitingAmountInput"
	stateSaveSpending               = "SaveSpending"
	stateAwaitingNewCategoryName    = "AwaitingNewCategoryName"
	stateAwaitingNewCategoryEmoji   = "AwaitingNewCategoryEmoji"
	stateAwaitingSaveCategoryName   = "AwaitingSaveCategoryName"
)

type BotStateManager struct {
	TbAPI       TbAPI
	TbKeyboards TbKeyboards
	UserState   UserStateRepository
	Categories  CategoriesRepository
	Spendings   SpendingsRepository

	mu         sync.Mutex
	UserFSMs   map[int64]*fsm.FSM
	UserValues map[int64]string
}

func NewBotStateManager(tbAPI TbAPI, tbKeyboards TbKeyboards, usRepository UserStateRepository, cRepository CategoriesRepository, sRepository SpendingsRepository) *BotStateManager {
	return &BotStateManager{
		TbAPI:       tbAPI,
		TbKeyboards: tbKeyboards,
		UserState:   usRepository,
		Categories:  cRepository,
		Spendings:   sRepository,
		UserFSMs:    make(map[int64]*fsm.FSM),
		UserValues:  make(map[int64]string),
	}
}

// newUserFSM constructs a fresh FSM for a user, starting at the supplied state.
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
			{Name: "AmountEntered", Src: []string{stateAwaitingAmountInput}, Dst: stateSaveSpending},
			{Name: "SpendingSaved", Src: []string{stateSaveSpending}, Dst: stateIdle},

			// Allows recovery from an invalid amount: stay in AwaitingAmountInput.
			{Name: "ResetToIdle", Src: []string{
				stateAwaitingCategorySelection,
				stateAwaitingAmountInput,
				stateSaveSpending,
				stateAwaitingNewCategoryName,
				stateAwaitingNewCategoryEmoji,
				stateAwaitingSaveCategoryName,
			}, Dst: stateIdle},
		},
		fsm.Callbacks{
			"leave_state":                                    func(ctx context.Context, e *fsm.Event) { sm.leaveState(e, userID) },
			"enter_" + stateIdle:                             func(ctx context.Context, e *fsm.Event) { sm.promptEnterIdle(userID) },
			"enter_" + stateAwaitingCategorySelection:        func(ctx context.Context, e *fsm.Event) { sm.promptCategorySelection(userID) },
			"enter_" + stateAwaitingAmountInput:              func(ctx context.Context, e *fsm.Event) { sm.promptAmountInput(userID) },
			"enter_" + stateSaveSpending:                     func(ctx context.Context, e *fsm.Event) { sm.saveSpending(ctx, userID) },
			"enter_" + stateAwaitingNewCategoryName:          func(ctx context.Context, e *fsm.Event) { sm.promptNewCategoryName(userID) },
			"enter_" + stateAwaitingNewCategoryEmoji:         func(ctx context.Context, e *fsm.Event) { sm.promptNewCategoryEmoji(userID) },
			"enter_" + stateAwaitingSaveCategoryName:         func(ctx context.Context, e *fsm.Event) { sm.promptSaveNewCategory(ctx, userID) },
		},
	)
}

// getOrCreateFSM returns the FSM for the user, creating it from persisted state when possible.
// Caller must hold sm.mu.
func (sm *BotStateManager) getOrCreateFSM(userID int64) *fsm.FSM {
	if existing, ok := sm.UserFSMs[userID]; ok {
		return existing
	}

	initial := stateIdle
	if persisted, err := sm.UserState.Read(userID); err == nil && persisted != nil && persisted.State != "" {
		// Only restore states the FSM can actually be in. Event names are written
		// to the state column by leaveState, so we only trust transition destinations.
		switch persisted.State {
		case stateIdle,
			stateAwaitingCategorySelection,
			stateAwaitingAmountInput,
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
		log.Printf("[error] Failed to create initial state for user %d: %v", userID, err)
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

// resetToIdle forces the user's FSM back to the Idle state. Used to recover from
// invalid input without losing the bot's grip on the user session.
func (sm *BotStateManager) resetToIdle(ctx context.Context, userID int64) {
	sm.mu.Lock()
	userFSM, ok := sm.UserFSMs[userID]
	sm.mu.Unlock()
	if !ok {
		sm.SetIdleState(ctx, userID)
		return
	}
	if userFSM.Current() == stateIdle {
		return
	}
	if err := userFSM.Event(ctx, "ResetToIdle"); err != nil {
		log.Printf("[warn] failed to reset user %d to Idle: %v", userID, err)
		sm.SetIdleState(ctx, userID)
	}
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
		log.Printf("[error] Failed to marshal updated state data to JSON for user %d: %v", userID, err)
		return
	}

	stateInfo := storage.UserStateInfo{
		UserID:   userID,
		State:    e.Dst,
		DataJSON: string(dataJSON),
	}

	if err := sm.UserState.Write(stateInfo); err != nil {
		log.Printf("[error] Failed to save updated user state for user %d: %v", userID, err)
		return
	}

	log.Printf("[info] User %d entered state %s, data: %s", userID, e.Dst, dataJSON)
}

func (sm *BotStateManager) promptEnterIdle(userID int64) {
	if err := sm.sendBotResponse(userID, "Choose an option:", sm.TbKeyboards.GetMainKeyboard()); err != nil {
		log.Printf("[warn] error sending main message: %v", err)
	}
}

func (sm *BotStateManager) promptCategorySelection(userID int64) {
	keyboard := sm.TbKeyboards.GetCategoryKeyboard(userID)
	if err := sm.sendBotResponse(userID, "Please select a category:", &keyboard); err != nil {
		log.Printf("[warn] error sending category selection prompt: %v", err)
	}
}

func (sm *BotStateManager) promptNewCategoryName(userID int64) {
	if err := sm.sendBotResponse(userID, "Please enter the name of the new category:", nil); err != nil {
		log.Printf("[warn] error sending new category name prompt: %v", err)
	}
}

func (sm *BotStateManager) promptNewCategoryEmoji(userID int64) {
	if err := sm.sendBotResponse(userID, "Please enter the emoji for the new category:", nil); err != nil {
		log.Printf("[warn] error sending new category emoji prompt: %v", err)
	}
}

func (sm *BotStateManager) promptSaveNewCategory(ctx context.Context, userID int64) {
	stateData, err := sm.getStateData(userID)
	if err != nil {
		log.Printf("[warn] error fetching state data: %v", err)
		sm.resetToIdle(ctx, userID)
		return
	}

	name, ok := stringField(stateData, "NewCategoryNameEntered")
	if !ok || strings.TrimSpace(name) == "" {
		_ = sm.sendBotResponse(userID, "Category name is missing. Returning to the main menu.", nil)
		sm.resetToIdle(ctx, userID)
		return
	}
	emoji, _ := stringField(stateData, "NewCategoryEmojiEntered")

	category := storage.CategoryInfo{
		UserID: userID,
		Name:   strings.TrimSpace(name),
		Emoji:  strings.TrimSpace(emoji),
	}

	if err = sm.Categories.AddOrUpdateCategory(category); err != nil {
		log.Printf("[warn] error saving new category: %v", err)
		_ = sm.sendBotResponse(userID, "Failed to save category. Please try again.", nil)
		sm.resetToIdle(ctx, userID)
		return
	}

	if err := sm.sendBotResponse(userID, "Category saved!", nil); err != nil {
		log.Printf("[warn] error sending new category save prompt: %v", err)
	}

	sm.mu.Lock()
	userFSM := sm.UserFSMs[userID]
	sm.mu.Unlock()
	if userFSM == nil {
		return
	}
	if err := userFSM.Event(ctx, "SaveNewCategory"); err != nil {
		log.Printf("[error] Failed to transition to Idle state for user %d: %v", userID, err)
	}
}

func (sm *BotStateManager) sendBotResponse(chatID int64, text string, keyboard interface{}) error {
	tbMsg := tbapi.NewMessage(chatID, text)
	tbMsg.ParseMode = tbapi.ModeMarkdown
	tbMsg.DisableWebPagePreview = true
	tbMsg.ReplyMarkup = keyboard

	if keyboard == nil {
		removeKeyboard := tbapi.NewRemoveKeyboard(true)
		tbMsg.ReplyMarkup = removeKeyboard
	}

	if err := send(tbMsg, sm.TbAPI); err != nil {
		return fmt.Errorf("can't send message to telegram %s, %d: %w", text, chatID, err)
	}
	return nil
}

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
	if err := sm.sendBotResponse(userID, "Please enter the amount:", nil); err != nil {
		log.Printf("[warn] error sending amount prompt: %v", err)
	}
}

func (sm *BotStateManager) saveSpending(ctx context.Context, userID int64) {
	stateData, err := sm.getStateData(userID)
	if err != nil {
		log.Printf("[warn] error fetching state data: %v", err)
		sm.resetToIdle(ctx, userID)
		return
	}

	categoryID, err := extractCategoryID(stateData)
	if err != nil {
		log.Printf("[warn] error parsing category for user %d: %v", userID, err)
		_ = sm.sendBotResponse(userID, "Failed to read selected category. Returning to the main menu.", nil)
		sm.resetToIdle(ctx, userID)
		return
	}

	amountFloat, err := parseAmount(stateData)
	if err != nil {
		log.Printf("[info] invalid amount for user %d: %v", userID, err)
		_ = sm.sendBotResponse(userID, fmt.Sprintf("Invalid amount: %s. Returning to the main menu.", err.Error()), nil)
		sm.resetToIdle(ctx, userID)
		return
	}

	spending := storage.SpendingInfo{
		UserID:      userID,
		CategoryID:  categoryID,
		Amount:      amountFloat,
		Description: "",
		Timestamp:   time.Now(),
	}

	if err := sm.Spendings.AddSpending(spending); err != nil {
		log.Printf("[warn] error saving spending for user %d: %v", userID, err)
		_ = sm.sendBotResponse(userID, "Failed to save spending. Please try again.", nil)
		sm.resetToIdle(ctx, userID)
		return
	}

	if err := sm.sendBotResponse(userID, fmt.Sprintf("Spending saved: %.2f", amountFloat), nil); err != nil {
		log.Printf("[warn] error sending spending save prompt: %v", err)
	}

	sm.mu.Lock()
	userFSM := sm.UserFSMs[userID]
	sm.mu.Unlock()
	if userFSM == nil {
		return
	}
	if err := userFSM.Event(ctx, "SpendingSaved"); err != nil {
		log.Printf("[warn] error transitioning to Idle after saving spending for user %d: %v", userID, err)
	}
}

// stringField safely extracts a string-typed value from the user state data map.
func stringField(data map[string]interface{}, key string) (string, bool) {
	raw, ok := data[key]
	if !ok || raw == nil {
		return "", false
	}
	s, ok := raw.(string)
	return s, ok
}

// extractCategoryID parses the "category_<id>" callback payload stored under CategorySelected.
func extractCategoryID(data map[string]interface{}) (int64, error) {
	raw, ok := stringField(data, "CategorySelected")
	if !ok {
		return 0, errors.New("category not selected")
	}
	parts := strings.SplitN(raw, "_", 2)
	if len(parts) != 2 || parts[0] != "category" || parts[1] == "" {
		return 0, fmt.Errorf("malformed category payload %q", raw)
	}
	id, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid category id %q: %w", parts[1], err)
	}
	if id <= 0 {
		return 0, fmt.Errorf("non-positive category id: %d", id)
	}
	return id, nil
}

// parseAmount validates and parses the AmountEntered field. It accepts both "." and "," as
// decimal separators, rejects non-positive and non-finite values, and trims whitespace.
func parseAmount(data map[string]interface{}) (float64, error) {
	raw, ok := stringField(data, "AmountEntered")
	if !ok {
		return 0, errors.New("amount not provided")
	}
	cleaned := strings.ReplaceAll(strings.TrimSpace(raw), ",", ".")
	if cleaned == "" {
		return 0, errors.New("amount is empty")
	}
	value, err := strconv.ParseFloat(cleaned, 64)
	if err != nil {
		return 0, fmt.Errorf("not a number: %q", raw)
	}
	if value <= 0 {
		return 0, fmt.Errorf("amount must be positive, got %g", value)
	}
	// Reject NaN/Inf — ParseFloat happily returns +Inf for very large literals.
	if value != value || value-value != 0 {
		return 0, fmt.Errorf("amount is not a finite number")
	}
	return value, nil
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
