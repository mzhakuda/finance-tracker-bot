CREATE TABLE IF NOT EXISTS user_states
(
    id        INTEGER PRIMARY KEY,
    user_id   INTEGER UNIQUE,
    state     TEXT,
    data      TEXT,
    timestamp DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS categories
(
    id      INTEGER PRIMARY KEY,
    user_id INTEGER NOT NULL,
    name    TEXT    NOT NULL,
    emoji   TEXT,
    UNIQUE (user_id, name) ON CONFLICT REPLACE
);

CREATE INDEX IF NOT EXISTS idx_categories_user_id ON categories (user_id);

CREATE TABLE IF NOT EXISTS spendings
(
    id          INTEGER PRIMARY KEY,
    user_id     INTEGER NOT NULL,
    category_id INTEGER NOT NULL,
    amount      REAL    NOT NULL,
    currency    TEXT    NOT NULL DEFAULT '',
    description TEXT    NOT NULL DEFAULT '',
    timestamp   DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (category_id) REFERENCES categories (id)
);

CREATE INDEX IF NOT EXISTS idx_spendings_user_id ON spendings (user_id);
CREATE INDEX IF NOT EXISTS idx_spendings_user_timestamp ON spendings (user_id, timestamp);

CREATE TABLE IF NOT EXISTS budgets
(
    id          INTEGER PRIMARY KEY,
    user_id     INTEGER NOT NULL,
    category_id INTEGER NOT NULL DEFAULT 0, -- 0 means overall budget for the user
    amount      REAL    NOT NULL,
    currency    TEXT    NOT NULL,
    period      TEXT    NOT NULL DEFAULT 'month',
    UNIQUE (user_id, category_id, period) ON CONFLICT REPLACE
);

CREATE INDEX IF NOT EXISTS idx_budgets_user_id ON budgets (user_id);
