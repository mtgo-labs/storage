CREATE TABLE sessions (
    session_id     TEXT DEFAULT '' NOT NULL,
    dc_id          INTEGER NOT NULL DEFAULT 0,
    api_id         INTEGER NOT NULL DEFAULT 0,
    api_hash       TEXT NOT NULL DEFAULT '',
    test_mode      INTEGER NOT NULL DEFAULT 0,
    auth_key       BYTEA,
    state          BYTEA,
    user_id        BIGINT NOT NULL DEFAULT 0,
    is_bot         INTEGER NOT NULL DEFAULT 0,
    first_name     TEXT NOT NULL DEFAULT '',
    last_name      TEXT NOT NULL DEFAULT '',
    username       TEXT NOT NULL DEFAULT '',
    date           INTEGER NOT NULL DEFAULT 0,
    server_address TEXT NOT NULL DEFAULT '',
    port           INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE peers (
    id           BIGINT PRIMARY KEY,
    type         INTEGER NOT NULL,
    access_hash  BIGINT NOT NULL DEFAULT 0,
    username     TEXT NOT NULL DEFAULT '',
    usernames    TEXT NOT NULL DEFAULT '',
    first_name   TEXT NOT NULL DEFAULT '',
    last_name    TEXT NOT NULL DEFAULT '',
    phone_number TEXT NOT NULL DEFAULT '',
    is_bot       INTEGER NOT NULL DEFAULT 0,
    photo_id     BIGINT NOT NULL DEFAULT 0,
    language     TEXT NOT NULL DEFAULT '',
    last_updated BIGINT NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_peers_username ON peers(username);
CREATE INDEX IF NOT EXISTS idx_peers_phone ON peers(phone_number);

CREATE TABLE conversations (
    chat_id    BIGINT NOT NULL,
    user_id    BIGINT NOT NULL,
    name       TEXT NOT NULL,
    step       INTEGER NOT NULL DEFAULT 0,
    data       BYTEA,
    created_at BIGINT NOT NULL DEFAULT 0,
    updated_at BIGINT NOT NULL DEFAULT 0,
    PRIMARY KEY (chat_id, user_id)
);

CREATE TABLE update_state (
    session_id TEXT PRIMARY KEY,
    pts        INTEGER NOT NULL DEFAULT 0,
    qts        INTEGER NOT NULL DEFAULT 0,
    date       INTEGER NOT NULL DEFAULT 0,
    seq        INTEGER NOT NULL DEFAULT 0,
    updated_at BIGINT NOT NULL DEFAULT 0
);

CREATE TABLE channel_update_state (
    session_id TEXT    NOT NULL,
    channel_id BIGINT  NOT NULL,
    pts        INTEGER NOT NULL DEFAULT 0,
    updated_at BIGINT  NOT NULL DEFAULT 0,
    PRIMARY KEY (session_id, channel_id)
);

CREATE TABLE update_dedup (
    session_id TEXT NOT NULL,
    dedup_key  TEXT NOT NULL,
    created_at BIGINT NOT NULL DEFAULT 0,
    PRIMARY KEY (session_id, dedup_key)
);

CREATE TABLE durable_updates (
    session_id TEXT  NOT NULL,
    id         TEXT  NOT NULL,
    payload    BYTEA NOT NULL,
    attempts   INTEGER NOT NULL DEFAULT 0,
    last_error TEXT  NOT NULL DEFAULT '',
    created_at BIGINT NOT NULL DEFAULT 0,
    updated_at BIGINT NOT NULL DEFAULT 0,
    PRIMARY KEY (session_id, id)
);
