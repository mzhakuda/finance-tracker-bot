package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	tbapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/jmoiron/sqlx"
	"github.com/joho/godotenv"

	"github.com/nyanyamaga/finance-tracker-bot/app/events"
	"github.com/nyanyamaga/finance-tracker-bot/app/keyboards"
	"github.com/nyanyamaga/finance-tracker-bot/app/storage"
)

var revision = "local"

func main() {
	fmt.Printf("finance-tracker-bot %s\n", revision)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		stop := make(chan os.Signal, 1)
		signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
		<-stop
		log.Printf("[warn] interrupt signal")
		cancel()
	}()

	if err := execute(ctx); err != nil {
		log.Printf("[error] %v", err)
		os.Exit(1)
	}
}

func execute(ctx context.Context) error {
	if err := godotenv.Load(); err != nil && !os.IsNotExist(err) {
		log.Printf("[info] no .env file loaded: %v", err)
	}

	dataFilePath := os.Getenv("DATA_FILE_PATH")
	telegramToken := os.Getenv("TELEGRAM_TOKEN")
	defaultCurrency := os.Getenv("DEFAULT_CURRENCY")
	if defaultCurrency == "" {
		defaultCurrency = "USD"
	}
	if dataFilePath == "" {
		return errors.New("DATA_FILE_PATH is required")
	}
	if telegramToken == "" {
		return errors.New("TELEGRAM_TOKEN is required")
	}

	dataDB, err := storage.NewSqliteDB(dataFilePath)
	if err != nil {
		return fmt.Errorf("failed to open sqlite database: %w", err)
	}
	defer func(dataDB *sqlx.DB) {
		if err := dataDB.Close(); err != nil {
			log.Printf("[warn] error closing sqlite database: %v", err)
		}
	}(dataDB)

	categoryDB, err := storage.NewCategory(dataDB)
	if err != nil {
		return fmt.Errorf("failed to initialize category storage: %w", err)
	}

	userStateDB, err := storage.NewUserState(dataDB)
	if err != nil {
		return fmt.Errorf("failed to initialize user state storage: %w", err)
	}

	spendingDB, err := storage.NewSpending(dataDB)
	if err != nil {
		return fmt.Errorf("failed to initialize spending storage: %w", err)
	}

	tbAPI, err := tbapi.NewBotAPI(telegramToken)
	if err != nil {
		return fmt.Errorf("can't make telegram bot: %w", err)
	}
	tbAPI.Debug = false

	botKeyboardProvider := keyboards.NewTbKeyboardProvider(categoryDB, spendingDB)
	botStateManager := events.NewBotStateManager(tbAPI, botKeyboardProvider, userStateDB, categoryDB, spendingDB, defaultCurrency)

	commandHandler := &events.BotCommandHandler{
		TbAPI:        tbAPI,
		TbKeyboards:  botKeyboardProvider,
		StateManager: botStateManager,
		Categories:   categoryDB,
		Spendings:    spendingDB,
	}

	messageHandler := &events.BotMessageHandler{
		TbAPI:        tbAPI,
		StateManager: botStateManager,
		TbKeyboards:  botKeyboardProvider,
	}

	callbackQueryHandler := &events.BotCallbackQueryHandler{
		TbAPI:        tbAPI,
		StateManager: botStateManager,
		Categories:   categoryDB,
		Spendings:    spendingDB,
		TbKeyboards:  botKeyboardProvider,
	}

	listener := events.TelegramListener{
		TbAPI:                tbAPI,
		CommandHandler:       commandHandler,
		MessageHandler:       messageHandler,
		CallbackQueryHandler: callbackQueryHandler,
	}

	if err := listener.StartListening(ctx); err != nil && !errors.Is(err, context.Canceled) {
		return fmt.Errorf("failed to start listening: %w", err)
	}

	return nil
}
