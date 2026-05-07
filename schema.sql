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
    description TEXT,
    timestamp   DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (category_id) REFERENCES categories (id)
);

CREATE INDEX IF NOT EXISTS idx_spendings_user_id ON spendings (user_id);
CREATE INDEX IF NOT EXISTS idx_spendings_user_timestamp ON spendings (user_id, timestamp);
