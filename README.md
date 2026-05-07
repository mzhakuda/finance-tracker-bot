# Finance Tracker Bot

The Finance Tracker Bot is a sophisticated tool designed in Go, aimed at providing individuals with a comprehensive
solution for managing their finances. It automates the tracking of expenses, budgets, and generates insightful financial
reports, all stored securely in a local database.

## Features

- **Expense Tracking**: Log expenses with category, amount, currency and optional description.
- **Recent spendings & totals**: View the latest entries (`/list`) and a per-currency monthly total (`/total`).
- **Category management**: Create and delete categories with `/categories`.
- **Cancel safely**: `/cancel` aborts any in-progress dialog without losing other data.

## Getting Started

These instructions will get you a copy of the project up and running on your local machine for development and testing
purposes.

### Prerequisites

- Go 1.15 or later. You can download it from [here](https://golang.org/dl/).
- SQLite3 for the database. Installation instructions can be found [here](https://www.sqlite.org/download.html).

### Installation

1. Clone the repository:
    ```bash
    git clone https://github.com/qfpeeeer/finance-tracker-bot.git
    cd finance-tracker-bot
    ```

2. Install Go dependencies:
    ```bash
    go mod tidy
    ```

3. Initialize the database. Run the following command to create `data.db` and set up the necessary tables or you can run
   the bot, and it will create the database for you.
    ```bash
    sqlite3 data.db < schema.sql
    ```

### Configuration

- **Environment Variables**: Set up your environment variables (if any) in a `.env` file or your preferred configuration
  method.
    - `DATA_FILE_PATH`: Path to your SQLite database file (e.g., `./data.db`). **Required.**
    - `TELEGRAM_TOKEN`: Telegram Bot API token from [BotFather](https://core.telegram.org/bots#6-botfather). **Required.**
    - `DEFAULT_CURRENCY`: Currency assigned to spendings when the user doesn't specify one (default: `USD`).

### Running Locally

To start the bot, run:

```bash
go run app/main.go
```

Replace app/main.go with the correct path to your application's entry point.

## Usage

Available commands inside the chat:

- `/start` — show the main menu.
- `/cancel` — abort the current dialog.
- `/list` — show the 10 most recent spendings (with delete buttons).
- `/total` — total spent since the start of the current month, broken down by currency.
- `/categories` — manage existing categories (with delete buttons).
- `/help` — list all commands.

Typical flow: tap *New spending category* to create at least one category, then tap *Add spending*, pick a category, type the amount (e.g. `12.50` or `12.50 EUR`), and an optional description.

## Deployment

Check the deployments folder for scripts and configurations needed to deploy the Finance Tracker Bot. Include specific
instructions for deploying to popular platforms if available.