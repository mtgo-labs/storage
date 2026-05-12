// Package postgres provides a storage adapter backed by a PostgreSQL database.
//
// It implements [storage.Adapter] and [storage.ConversationStore]. Session and
// peer tables are created on Open; the conversations table is created lazily
// on first use.
//
// Basic usage:
//
//	store, err := postgres.Open(postgres.Config{
//	    Host:     "localhost",
//	    Port:     5432,
//	    User:     "mtgo",
//	    Password: "secret",
//	    Database: "mtgo",
//	    SSLMode:  "require",
//	})
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer store.Close()
package postgres

import (
	"database/sql"
	"fmt"
	"sync"
	"time"

	"github.com/mtgo-labs/storage"
	_ "github.com/lib/pq"
)

// Postgres is a storage adapter backed by a PostgreSQL database.
type Postgres struct {
	db        *sql.DB
	convOnce  sync.Once
	convReady bool
}

var (
	_ storage.Adapter           = (*Postgres)(nil)
	_ storage.ConversationStore = (*Postgres)(nil)
)

// Config holds PostgreSQL connection parameters.
type Config struct {
	Host     string
	Port     int
	User     string
	Password string
	Database string
	SSLMode  string
}

func (c Config) dsn() string {
	ssl := c.SSLMode
	if ssl == "" {
		ssl = "disable"
	}
	return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		c.Host, c.Port, c.User, c.Password, c.Database, ssl)
}

// Open connects to a PostgreSQL database and initializes the sessions
// and peers tables. Conversation tables are created lazily when first accessed.
func Open(cfg Config) (*Postgres, error) {
	db, err := sql.Open("postgres", cfg.dsn())
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	if err := initSchema(db); err != nil {
		db.Close()
		return nil, err
	}
	return &Postgres{db: db}, nil
}

func initSchema(db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS sessions (
			dc_id          INTEGER PRIMARY KEY,
			api_id         INTEGER DEFAULT 0,
			api_hash       TEXT DEFAULT '',
			test_mode      INTEGER DEFAULT 0,
			auth_key       BYTEA,
			state          BYTEA,
			user_id        BIGINT DEFAULT 0,
			is_bot         INTEGER DEFAULT 0,
			first_name     TEXT DEFAULT '',
			last_name      TEXT DEFAULT '',
			username       TEXT DEFAULT '',
			date           INTEGER DEFAULT 0,
			server_address TEXT DEFAULT '',
			port           INTEGER DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS peers (
			id           BIGINT PRIMARY KEY,
			type         INTEGER NOT NULL,
			access_hash  BIGINT DEFAULT 0,
			username     TEXT DEFAULT '',
			usernames    TEXT DEFAULT '',
			first_name   TEXT DEFAULT '',
			last_name    TEXT DEFAULT '',
			phone_number TEXT DEFAULT '',
			is_bot       INTEGER DEFAULT 0,
			photo_id     BIGINT DEFAULT 0,
			language     TEXT DEFAULT '',
			last_updated BIGINT DEFAULT 0
		)`,
		`CREATE INDEX IF NOT EXISTS idx_peers_username ON peers(username)`,
		`CREATE INDEX IF NOT EXISTS idx_peers_phone ON peers(phone_number)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			return fmt.Errorf("postgres init: %w", err)
		}
	}
	return nil
}

func (p *Postgres) ensureConvTable() error {
	if p.convReady {
		return nil
	}
	_, err := p.db.Exec(`CREATE TABLE IF NOT EXISTS conversations (
		chat_id    BIGINT NOT NULL,
		user_id    BIGINT NOT NULL,
		name       TEXT   NOT NULL,
		step       INTEGER DEFAULT 0,
		data       BYTEA,
		created_at BIGINT DEFAULT 0,
		updated_at BIGINT DEFAULT 0,
		PRIMARY KEY (chat_id, user_id)
	)`)
	if err == nil {
		p.convReady = true
	}
	return err
}

// --- SessionStore ---

func (p *Postgres) LoadSession() (*storage.Session, error) {
	var dcID, apiID, date, port int
	var testMode, isBot int
	var authKey, state []byte
	var userID int64
	var apiHash, firstName, lastName, username, serverAddress string

	err := p.db.QueryRow(
		`SELECT dc_id, api_id, api_hash, test_mode, auth_key, state, user_id, is_bot, first_name, last_name, username, date, server_address, port FROM sessions LIMIT 1`,
	).Scan(&dcID, &apiID, &apiHash, &testMode, &authKey, &state, &userID, &isBot, &firstName, &lastName, &username, &date, &serverAddress, &port)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &storage.Session{
		DC: dcID, APIID: apiID, APIHash: apiHash, TestMode: testMode != 0,
		AuthKey: authKey, State: state, UserID: userID, IsBot: isBot != 0,
		FirstName: firstName, LastName: lastName, Username: username,
		Date: date, Addr: serverAddress, Port: port,
	}, nil
}

func (p *Postgres) SaveSession(s *storage.Session) error {
	tm := 0
	if s.TestMode {
		tm = 1
	}
	ib := 0
	if s.IsBot {
		ib = 1
	}
	_, err := p.db.Exec(`INSERT INTO sessions
		(dc_id, api_id, api_hash, test_mode, auth_key, state, user_id, is_bot, first_name, last_name, username, date, server_address, port)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		ON CONFLICT (dc_id) DO UPDATE SET
			api_id = EXCLUDED.api_id, api_hash = EXCLUDED.api_hash, test_mode = EXCLUDED.test_mode,
			auth_key = EXCLUDED.auth_key, state = EXCLUDED.state, user_id = EXCLUDED.user_id,
			is_bot = EXCLUDED.is_bot, first_name = EXCLUDED.first_name, last_name = EXCLUDED.last_name,
			username = EXCLUDED.username, date = EXCLUDED.date, server_address = EXCLUDED.server_address, port = EXCLUDED.port`,
		s.DC, s.APIID, s.APIHash, tm, s.AuthKey, s.State, s.UserID, ib,
		s.FirstName, s.LastName, s.Username, s.Date, s.Addr, s.Port)
	return err
}

// --- PeerStore ---

func (p *Postgres) SavePeer(peer *storage.Peer) error {
	ib := 0
	if peer.IsBot {
		ib = 1
	}
	_, err := p.db.Exec(`INSERT INTO peers
		(id, type, access_hash, username, usernames, first_name, last_name, phone_number, is_bot, photo_id, language, last_updated)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		ON CONFLICT (id) DO UPDATE SET
			type = EXCLUDED.type, access_hash = EXCLUDED.access_hash, username = EXCLUDED.username,
			usernames = EXCLUDED.usernames, first_name = EXCLUDED.first_name, last_name = EXCLUDED.last_name,
			phone_number = EXCLUDED.phone_number, is_bot = EXCLUDED.is_bot, photo_id = EXCLUDED.photo_id,
			language = EXCLUDED.language, last_updated = EXCLUDED.last_updated`,
		peer.ID, peer.Type, peer.AccessHash, peer.Username, peer.Usernames, peer.FirstName, peer.LastName,
		peer.PhoneNumber, ib, peer.PhotoID, peer.Language, peer.LastUpdated)
	return err
}

func (p *Postgres) GetPeer(id int64) (*storage.Peer, error) {
	peer := &storage.Peer{}
	var isBot int
	err := p.db.QueryRow(
		`SELECT id, type, access_hash, username, usernames, first_name, last_name, phone_number, is_bot, photo_id, language, last_updated FROM peers WHERE id = $1`, id).
		Scan(&peer.ID, &peer.Type, &peer.AccessHash, &peer.Username, &peer.Usernames, &peer.FirstName, &peer.LastName,
			&peer.PhoneNumber, &isBot, &peer.PhotoID, &peer.Language, &peer.LastUpdated)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	peer.IsBot = isBot != 0
	return peer, nil
}

func (p *Postgres) GetPeerByUsername(username string) (*storage.Peer, error) {
	peer := &storage.Peer{}
	var isBot int
	err := p.db.QueryRow(
		`SELECT id, type, access_hash, username, usernames, first_name, last_name, phone_number, is_bot, photo_id, language, last_updated FROM peers WHERE username = $1`, username).
		Scan(&peer.ID, &peer.Type, &peer.AccessHash, &peer.Username, &peer.Usernames, &peer.FirstName, &peer.LastName,
			&peer.PhoneNumber, &isBot, &peer.PhotoID, &peer.Language, &peer.LastUpdated)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	peer.IsBot = isBot != 0
	return peer, nil
}

func (p *Postgres) LoadPeers() ([]*storage.Peer, error) {
	rows, err := p.db.Query(`SELECT id, type, access_hash, username, usernames, first_name, last_name, phone_number, is_bot, photo_id, language, last_updated FROM peers`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var peers []*storage.Peer
	for rows.Next() {
		peer := &storage.Peer{}
		var isBot int
		if err := rows.Scan(&peer.ID, &peer.Type, &peer.AccessHash, &peer.Username, &peer.Usernames, &peer.FirstName, &peer.LastName,
			&peer.PhoneNumber, &isBot, &peer.PhotoID, &peer.Language, &peer.LastUpdated); err != nil {
			return nil, err
		}
		peer.IsBot = isBot != 0
		peers = append(peers, peer)
	}
	return peers, nil
}

func (p *Postgres) DeletePeer(id int64) error {
	_, err := p.db.Exec(`DELETE FROM peers WHERE id = $1`, id)
	return err
}

// --- ConversationStore (lazy) ---

func (p *Postgres) SaveConversation(c *storage.Conversation) error {
	if err := p.ensureConvTable(); err != nil {
		return err
	}
	now := c.UpdatedAt
	if now == 0 {
		now = time.Now().Unix()
	}
	createdAt := c.CreatedAt
	if createdAt == 0 {
		createdAt = now
	}
	_, err := p.db.Exec(`INSERT INTO conversations
		(chat_id, user_id, name, step, data, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (chat_id, user_id) DO UPDATE SET
			name = EXCLUDED.name, step = EXCLUDED.step, data = EXCLUDED.data,
			created_at = EXCLUDED.created_at, updated_at = EXCLUDED.updated_at`,
		c.ChatID, c.UserID, c.Name, c.Step, c.Data, createdAt, now)
	return err
}

func (p *Postgres) LoadConversation(chatID, userID int64) (*storage.Conversation, error) {
	if err := p.ensureConvTable(); err != nil {
		return nil, err
	}
	c := &storage.Conversation{}
	err := p.db.QueryRow(
		`SELECT chat_id, user_id, name, step, data, created_at, updated_at FROM conversations WHERE chat_id = $1 AND user_id = $2`, chatID, userID).
		Scan(&c.ChatID, &c.UserID, &c.Name, &c.Step, &c.Data, &c.CreatedAt, &c.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return c, nil
}

func (p *Postgres) DeleteConversation(chatID, userID int64) error {
	if err := p.ensureConvTable(); err != nil {
		return err
	}
	_, err := p.db.Exec(`DELETE FROM conversations WHERE chat_id = $1 AND user_id = $2`, chatID, userID)
	return err
}

// --- Close ---

func (p *Postgres) Close() error {
	return p.db.Close()
}
