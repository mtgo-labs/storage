-- name: GetSession :one
SELECT session_id, dc_id, api_id, api_hash, test_mode, auth_key, state, user_id, is_bot, first_name, last_name, username, date, server_address, port
FROM sessions
LIMIT 1;

-- name: UpsertSession :exec
INSERT INTO sessions (session_id, dc_id, api_id, api_hash, test_mode, auth_key, state, user_id, is_bot, first_name, last_name, username, date, server_address, port)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
ON CONFLICT (dc_id) DO UPDATE SET
    api_id = EXCLUDED.api_id, api_hash = EXCLUDED.api_hash, test_mode = EXCLUDED.test_mode,
    auth_key = EXCLUDED.auth_key, state = EXCLUDED.state, user_id = EXCLUDED.user_id,
    is_bot = EXCLUDED.is_bot, first_name = EXCLUDED.first_name, last_name = EXCLUDED.last_name,
    username = EXCLUDED.username, date = EXCLUDED.date, server_address = EXCLUDED.server_address,
    port = EXCLUDED.port;

-- name: GetPeer :one
SELECT id, type, access_hash, username, usernames, first_name, last_name, phone_number, is_bot, photo_id, language, last_updated
FROM peers
WHERE id = $1;

-- name: GetPeerByUsername :one
SELECT id, type, access_hash, username, usernames, first_name, last_name, phone_number, is_bot, photo_id, language, last_updated
FROM peers
WHERE username = $1;

-- name: ListPeers :many
SELECT id, type, access_hash, username, usernames, first_name, last_name, phone_number, is_bot, photo_id, language, last_updated
FROM peers;

-- name: UpsertPeer :exec
INSERT INTO peers (id, type, access_hash, username, usernames, first_name, last_name, phone_number, is_bot, photo_id, language, last_updated)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
ON CONFLICT (id) DO UPDATE SET
    type = EXCLUDED.type, access_hash = EXCLUDED.access_hash, username = EXCLUDED.username,
    usernames = EXCLUDED.usernames, first_name = EXCLUDED.first_name, last_name = EXCLUDED.last_name,
    phone_number = EXCLUDED.phone_number, is_bot = EXCLUDED.is_bot, photo_id = EXCLUDED.photo_id,
    language = EXCLUDED.language, last_updated = EXCLUDED.last_updated;

-- name: DeletePeer :exec
DELETE FROM peers WHERE id = $1;

-- name: UpsertConversation :exec
INSERT INTO conversations (chat_id, user_id, name, step, data, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (chat_id, user_id) DO UPDATE SET
    name = EXCLUDED.name, step = EXCLUDED.step, data = EXCLUDED.data,
    created_at = EXCLUDED.created_at, updated_at = EXCLUDED.updated_at;

-- name: GetConversation :one
SELECT chat_id, user_id, name, step, data, created_at, updated_at
FROM conversations
WHERE chat_id = $1 AND user_id = $2;

-- name: DeleteConversation :exec
DELETE FROM conversations WHERE chat_id = $1 AND user_id = $2;

-- name: GetUpdateState :one
SELECT session_id, pts, qts, date, seq
FROM update_state
WHERE session_id = $1;

-- name: UpsertUpdateState :exec
INSERT INTO update_state (session_id, pts, qts, date, seq, updated_at)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (session_id) DO UPDATE SET
    pts = EXCLUDED.pts, qts = EXCLUDED.qts, date = EXCLUDED.date,
    seq = EXCLUDED.seq, updated_at = EXCLUDED.updated_at;

-- name: GetChannelUpdateState :one
SELECT session_id, channel_id, pts
FROM channel_update_state
WHERE session_id = $1 AND channel_id = $2;

-- name: ListChannelUpdateStates :many
SELECT session_id, channel_id, pts
FROM channel_update_state
WHERE session_id = $1;

-- name: UpsertChannelUpdateState :exec
INSERT INTO channel_update_state (session_id, channel_id, pts, updated_at)
VALUES ($1, $2, $3, $4)
ON CONFLICT (session_id, channel_id) DO UPDATE SET
    pts = EXCLUDED.pts, updated_at = EXCLUDED.updated_at;

-- name: InsertDedupKey :execrows
INSERT INTO update_dedup (session_id, dedup_key, created_at) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING;

-- name: ExistsDedupKey :one
SELECT COUNT(*) FROM update_dedup WHERE session_id = $1 AND dedup_key = $2;

-- name: UpsertDurableUpdate :exec
INSERT INTO durable_updates (session_id, id, payload, attempts, last_error, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (session_id, id) DO UPDATE SET
    payload = EXCLUDED.payload, attempts = EXCLUDED.attempts,
    last_error = EXCLUDED.last_error, updated_at = EXCLUDED.updated_at;

-- name: DeleteDurableUpdate :exec
DELETE FROM durable_updates WHERE session_id = $1 AND id = $2;

-- name: ListDurableUpdates :many
SELECT session_id, id, payload, attempts, last_error, created_at, updated_at
FROM durable_updates
WHERE session_id = $1
ORDER BY created_at ASC
LIMIT $2;

-- name: MarkDurableUpdateFailed :exec
UPDATE durable_updates SET attempts = $1, last_error = $2, updated_at = $3 WHERE session_id = $4 AND id = $5;
