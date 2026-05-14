-- name: GetSession :one
SELECT session_id, dc_id, api_id, api_hash, test_mode, auth_key, state, user_id, is_bot, first_name, last_name, username, date, server_address, port
FROM sessions
LIMIT 1;

-- name: UpsertSession :exec
INSERT OR REPLACE INTO sessions (session_id, dc_id, api_id, api_hash, test_mode, auth_key, state, user_id, is_bot, first_name, last_name, username, date, server_address, port)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetPeer :one
SELECT id, type, access_hash, username, usernames, first_name, last_name, phone_number, is_bot, photo_id, language, last_updated
FROM peers
WHERE id = ?;

-- name: GetPeerByUsername :one
SELECT id, type, access_hash, username, usernames, first_name, last_name, phone_number, is_bot, photo_id, language, last_updated
FROM peers
WHERE username = ?;

-- name: ListPeers :many
SELECT id, type, access_hash, username, usernames, first_name, last_name, phone_number, is_bot, photo_id, language, last_updated
FROM peers;

-- name: UpsertPeer :exec
INSERT OR REPLACE INTO peers (id, type, access_hash, username, usernames, first_name, last_name, phone_number, is_bot, photo_id, language, last_updated)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: DeletePeer :exec
DELETE FROM peers WHERE id = ?;

-- name: UpsertConversation :exec
INSERT OR REPLACE INTO conversations (chat_id, user_id, name, step, data, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?);

-- name: GetConversation :one
SELECT chat_id, user_id, name, step, data, created_at, updated_at
FROM conversations
WHERE chat_id = ? AND user_id = ?;

-- name: DeleteConversation :exec
DELETE FROM conversations WHERE chat_id = ? AND user_id = ?;

-- name: GetUpdateState :one
SELECT session_id, pts, qts, date, seq
FROM update_state
WHERE session_id = ?;

-- name: UpsertUpdateState :exec
INSERT INTO update_state (session_id, pts, qts, date, seq, updated_at)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT(session_id) DO UPDATE SET
    pts = excluded.pts, qts = excluded.qts, date = excluded.date,
    seq = excluded.seq, updated_at = excluded.updated_at;

-- name: GetChannelUpdateState :one
SELECT session_id, channel_id, pts
FROM channel_update_state
WHERE session_id = ? AND channel_id = ?;

-- name: ListChannelUpdateStates :many
SELECT session_id, channel_id, pts
FROM channel_update_state
WHERE session_id = ?;

-- name: UpsertChannelUpdateState :exec
INSERT INTO channel_update_state (session_id, channel_id, pts, updated_at)
VALUES (?, ?, ?, ?)
ON CONFLICT(session_id, channel_id) DO UPDATE SET
    pts = excluded.pts, updated_at = excluded.updated_at;

-- name: InsertDedupKey :execrows
INSERT OR IGNORE INTO update_dedup (session_id, dedup_key, created_at) VALUES (?, ?, ?);

-- name: ExistsDedupKey :one
SELECT COUNT(*) FROM update_dedup WHERE session_id = ? AND dedup_key = ?;

-- name: UpsertDurableUpdate :exec
INSERT OR REPLACE INTO durable_updates (session_id, id, payload, attempts, last_error, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?);

-- name: DeleteDurableUpdate :exec
DELETE FROM durable_updates WHERE session_id = ? AND id = ?;

-- name: ListDurableUpdates :many
SELECT session_id, id, payload, attempts, last_error, created_at, updated_at
FROM durable_updates
WHERE session_id = ?
ORDER BY created_at ASC
LIMIT ?;

-- name: MarkDurableUpdateFailed :exec
UPDATE durable_updates SET attempts = ?, last_error = ?, updated_at = ? WHERE session_id = ? AND id = ?;
