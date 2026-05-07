# Finance Tracker Bot

A Telegram bot, written in Go, for keeping a manual log of personal expenses.
Each spending has a category, an amount with currency, an optional description,
and a timestamp. Everything is stored locally in SQLite.

The bot is intentionally minimal: it does **not** connect to any bank, send
reminders, set budget limits, or run analytics. It is a journal you fill in by
hand, with quick-access reports.

## Features

- Log a spending in three taps: pick category → type amount → add description (or skip).
- Multi-currency: the amount can be entered as `12.50`, `12.50 EUR`, or `12,50EUR`. A default currency is configurable.
- Recent spendings (`/list`) with inline *Delete* buttons.
- Per-currency total since the 1st of the current month (`/total`).
- Category management (`/categories`) with inline *Delete* buttons.
- Safe `/cancel` from any step of any dialog.
- State is persisted in SQLite, so a bot restart doesn't strand users mid-flow.

## How it works (user flow)

Below is what a real user sees in the Telegram chat. Square brackets denote
inline buttons; **bold** text denotes reply-keyboard buttons.

### First run

```
User: /start
Bot:  Welcome — choose an option:
      [Add spending] [New spending category]
      [Recent spendings] [Manage categories]
```

The four reply-keyboard buttons stay visible until the bot needs to take them
away (e.g. while waiting for a description).

### Step 1 — create at least one category

`Add spending` doesn't work without categories. If the user taps it on a fresh
account, the bot refuses upfront:

```
User: [Add spending]
Bot:  You don't have any categories yet. Tap *New spending category* to create one.
```

Creating a category is a two-step dialog:

```
User: [New spending category]
Bot:  Please enter the name of the new category (or /cancel):
User: Food
Bot:  Please enter the emoji for the new category:
User: 🍔
Bot:  Category saved: 🍔 Food
      Choose an option: [...main menu...]
```

Validation:

- Name is trimmed, must be non-empty, ≤ 32 characters, must not start with `/`,
  and must not collide with main-menu labels (`Add spending`, `Recent spendings`,
  …) — otherwise the bot would shadow its own buttons.
- Emoji must be non-empty and ≤ 8 runes.
- A new category with the same name as an existing one **overwrites the emoji**
  (this is intentional — it's how you "edit" an emoji).

### Step 2 — log a spending

```
User: [Add spending]
Bot:  Please select a category:
      [🍔 Food]
      [🚌 Transport]
User: [🍔 Food]
Bot:  Please enter the amount (e.g. `12.50` or `12.50 EUR`). Default currency: USD.
User: 12.50 EUR
Bot:  Please enter a description, or tap [Skip]:
User: lunch with Anna
Bot:  Spending saved: 12.50 EUR — lunch with Anna
      Choose an option: [...main menu...]
```

The amount step accepts `12.50`, `12,50`, `12.50 EUR`, or `12.50EUR`. If the
suffix is alphabetic, it's treated as the currency (uppercased); otherwise the
configured `DEFAULT_CURRENCY` is used.

If the user types something that isn't a positive number, the bot stays in the
same step and asks again instead of dropping out:

```
User: lol
Bot:  Invalid amount: not a number: "lol". Try again, or /cancel.
```

The same applies to category-name and emoji validation during category
creation.

### Step 3 — review

```
User: /list
Bot:  Recent spendings:
      • 06 May 19:42 — 🍔 Food — 12.50 EUR (lunch with Anna)
      • 05 May 12:08 — 🚌 Transport — 3.40 USD
      [06 May 19:42 • 🍔 Food • 12.50 EUR] [Delete]
      [05 May 12:08 • 🚌 Transport • 3.40 USD] [Delete]
```

Tapping *Delete* removes the row and rebuilds the inline keyboard in place.
Cross-user delete is not possible — the SQL update is filtered by `user_id`,
and a forged callback id returns `not found`.

```
User: /total
Bot:  Total since 01 May 2026:
      • 12.50 EUR
      • 3.40 USD
```

Totals are grouped by currency. There is no FX conversion.

### Managing categories

```
User: /categories
Bot:  Your categories:
      • 🍔 Food
      • 🚌 Transport
      [🍔 Food] [Delete]
      [🚌 Transport] [Delete]
```

Deleting a category does **not** delete the spendings that referenced it —
those rows keep their `category_id`, but `/list` will render the category as
`?` because the join no longer finds it. (Soft-delete or cascading is a
deliberate non-feature for now; ask before adding it.)

### Cancel and restart

`/cancel` from any step returns the user to the main menu without saving the
half-finished entry. The FSM resets to `Idle` and the user value buffer is
cleared.

`/start` also resets the FSM to `Idle`. Note that this **discards in-progress
data** — if the user is mid-spending and taps `/start`, the partially-entered
amount is lost.

If the bot process restarts, the user's last persisted state is rehydrated from
SQLite the next time they send a message, so they pick up exactly where they
left off.

## Commands

| Command       | What it does                                            |
| ------------- | ------------------------------------------------------- |
| `/start`      | Reset to the main menu.                                 |
| `/cancel`     | Abort the current dialog, return to the main menu.      |
| `/list`       | Show the last 10 spendings with delete buttons.         |
| `/total`      | Sum of spendings since the 1st of the current month, grouped by currency. |
| `/categories` | List categories with delete buttons.                    |
| `/help`       | Print this list.                                        |

## Configuration

Configuration is via environment variables. A `.env` file at the working
directory is auto-loaded if present (see `deployments/example.env`).

| Variable           | Required | Default | Description                                                              |
| ------------------ | -------- | ------- | ------------------------------------------------------------------------ |
| `DATA_FILE_PATH`   | yes      | —       | Path to the SQLite database file (created on first run).                 |
| `TELEGRAM_TOKEN`   | yes      | —       | Bot token from [BotFather](https://core.telegram.org/bots#6-botfather).  |
| `DEFAULT_CURRENCY` | no       | `USD`   | Used when the user doesn't specify a currency in the amount.             |

## Running locally

Prerequisites: Go 1.21 or later. SQLite is **not** required as a system
dependency — the bot uses pure-Go [`modernc.org/sqlite`](https://gitlab.com/cznic/sqlite),
so `CGO_ENABLED=0` is fine.

```bash
git clone https://github.com/mzhakuda/finance-tracker-bot.git
cd finance-tracker-bot

go mod download

cp deployments/example.env .env
# edit .env: paste TELEGRAM_TOKEN, set DATA_FILE_PATH

go run ./app
```

The schema is created automatically on first run; you don't need to
`sqlite3 data.db < schema.sql` manually. `schema.sql` is kept as a reference
of the canonical layout.

## Running with Docker

```bash
cd deployments
cp example.env .env
# edit .env

docker compose up -d --build
docker compose logs -f bot
```

The image is multi-stage (`golang:1.21-alpine` → `alpine:3.19`), runs as a
non-root user, and stores the SQLite file under `/data`, mounted as a named
volume so the data survives container recreation.

## Architecture overview

```
app/
├── main.go                       # entrypoint: env, DB, wiring, signal handling
├── events/
│   ├── events.go                 # interfaces (TbAPI, repos, handlers, StateManager)
│   ├── listener.go               # Telegram update loop
│   ├── command_handler.go        # /start, /cancel, /list, /total, /categories, /help
│   ├── message_handler.go        # main-menu buttons + per-state input validation
│   ├── callback_query_handler.go # inline buttons (category pick, deletes)
│   └── state_manager.go          # FSM definition, transitions, state persistence
├── keyboards/                    # reply + inline keyboard builders
└── storage/                      # SQLite repos (user_states, categories, spendings)
```

The dialog is driven by a finite state machine (one per user, kept in memory
and rehydrated from `user_states` on first interaction after a restart). The
states are:

```
Idle ──[ChooseAddSpending]──► AwaitingCategorySelection
                              ──[CategorySelected]──► AwaitingAmountInput
                              ──[AmountEntered]──► AwaitingDescriptionInput
                              ──[DescriptionEntered]──► SaveSpending
                              ──[SpendingSaved]──► Idle

Idle ──[ChooseAddCategory]──► AwaitingNewCategoryName
                              ──[NewCategoryNameEntered]──► AwaitingNewCategoryEmoji
                              ──[NewCategoryEmojiEntered]──► AwaitingSaveCategoryName
                              ──[SaveNewCategory]──► Idle

(any state) ──[ResetToIdle]──► Idle    // used by /cancel and validation failures
```

The `BotStateManager` guards its in-memory FSM map with a `sync.Mutex`, so
concurrent updates from a single user (multiple devices, retried webhooks)
don't race.

## What the bot does NOT do (yet)

This is the explicit non-feature list, so expectations are clear:

- No edit of an existing spending or category (only delete + recreate).
- No FX conversion in `/total` — totals are per-currency.
- No custom date for a spending — `Timestamp` is always `time.Now()`.
- No budgets, alerts, or recurring expenses.
- No multi-user / shared accounts — each Telegram user is isolated.
- No CSV export, no reporting beyond `/list` and `/total`.

If any of these matters for your use case, open an issue.
