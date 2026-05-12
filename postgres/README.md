# postgres

PostgreSQL storage adapter for [mtgo] Telegram clients.

## Install

```bash
go get github.com/mtgo-labs/storage/postgres
```

## Usage

```go
store, err := postgres.Open(postgres.Config{
    Host:     "localhost",
    Port:     5432,
    User:     "mtgo",
    Password: "secret",
    Database: "mtgo",
    SSLMode:  "require",
})
if err != nil {
    log.Fatal(err)
}
defer store.Close()

client, _ := telegram.NewClient(apiID, apiHash, telegram.WithStorage(store))
```

`Open` connects to the database, verifies connectivity with `Ping`, and initializes the `sessions` and `peers` tables. The `conversations` table is created lazily on first use.

`SSLMode` defaults to `"disable"` if empty.

## Schema

**sessions**

| Column          | Type    |
|-----------------|---------|
| dc_id           | INTEGER |
| api_id          | INTEGER |
| api_hash        | TEXT    |
| test_mode       | INTEGER |
| auth_key        | BYTEA   |
| state           | BYTEA   |
| user_id         | BIGINT  |
| is_bot          | INTEGER |
| first_name      | TEXT    |
| last_name       | TEXT    |
| username        | TEXT    |
| date            | INTEGER |
| server_address  | TEXT    |
| port            | INTEGER |

**peers**

| Column        | Type    | Index              |
|---------------|---------|--------------------|
| id            | BIGINT  | PRIMARY KEY        |
| type          | INTEGER |                    |
| access_hash   | BIGINT  |                    |
| username      | TEXT    | idx_peers_username |
| usernames     | TEXT    |                    |
| first_name    | TEXT    |                    |
| last_name     | TEXT    |                    |
| phone_number  | TEXT    | idx_peers_phone    |
| is_bot        | INTEGER |                    |
| photo_id      | BIGINT  |                    |
| language      | TEXT    |                    |
| last_updated  | BIGINT  |                    |

**conversations** (created lazily)

| Column     | Type    |
|------------|---------|
| chat_id    | BIGINT  |
| user_id    | BIGINT  |
| name       | TEXT    |
| step       | INTEGER |
| data       | BYTEA   |
| created_at | BIGINT  |
| updated_at | BIGINT  |

## Interfaces

```go
var _ storage.Adapter           = (*Postgres)(nil)
var _ storage.ConversationStore = (*Postgres)(nil)
```
