//go:build !js

// Package sqlite provides a storage adapter backed by a SQLite database
// (pure-Go via modernc.org/sqlite, no CGO required).
//
// It implements [storage.Adapter] and [storage.ConversationStore]. Session and
// peer tables are created on Open; the conversations table is created lazily
// on first use.
//
// Basic usage:
//
//	store, err := sqlite.Open("session.db")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer store.Close()
//
//	client, _ := telegram.NewClient(apiID, apiHash, telegram.WithStorage(store))
//
// Exporting a portable session string (for StringSession):
//
//	str, _ := sqlite.ExportSessionString(sess)
package sqlite

import (
	"bytes"
	"database/sql"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"sync"
	"time"

	"github.com/mtgo-labs/storage"
	_ "modernc.org/sqlite"
)

// SQLite is a storage adapter backed by a SQLite database.
type SQLite struct {
	db        *sql.DB
	convOnce  sync.Once
	convReady bool
}

var (
	_ storage.Adapter           = (*SQLite)(nil)
	_ storage.ConversationStore = (*SQLite)(nil)
)

// Open opens (or creates) a SQLite database at path and initializes
// the sessions and peers tables. Conversation tables are created lazily
// when first accessed.
func Open(path string) (*SQLite, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	if err := initSchema(db); err != nil {
		db.Close()
		return nil, err
	}
	return &SQLite{db: db}, nil
}

func initSchema(db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS sessions (
			dc_id          INTEGER PRIMARY KEY,
			api_id         INTEGER DEFAULT 0,
			api_hash       TEXT DEFAULT '',
			test_mode      INTEGER DEFAULT 0,
			auth_key       BLOB,
			state          BLOB,
			user_id        INTEGER DEFAULT 0,
			is_bot         INTEGER DEFAULT 0,
			first_name     TEXT DEFAULT '',
			last_name      TEXT DEFAULT '',
			username       TEXT DEFAULT '',
			date           INTEGER DEFAULT 0,
			server_address TEXT DEFAULT '',
			port           INTEGER DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS peers (
			id           INTEGER PRIMARY KEY,
			type         INTEGER NOT NULL,
			access_hash  INTEGER DEFAULT 0,
			username     TEXT DEFAULT '',
			usernames    TEXT DEFAULT '',
			first_name   TEXT DEFAULT '',
			last_name    TEXT DEFAULT '',
			phone_number TEXT DEFAULT '',
			is_bot       INTEGER DEFAULT 0,
			photo_id     INTEGER DEFAULT 0,
			language     TEXT DEFAULT '',
			last_updated INTEGER DEFAULT 0
		)`,
		`CREATE INDEX IF NOT EXISTS idx_peers_username ON peers(username)`,
		`CREATE INDEX IF NOT EXISTS idx_peers_phone ON peers(phone_number)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			return fmt.Errorf("sqlite init: %w", err)
		}
	}
	return nil
}

func (a *SQLite) ensureConvTable() error {
	if a.convReady {
		return nil
	}
	_, err := a.db.Exec(`CREATE TABLE IF NOT EXISTS conversations (
		chat_id    INTEGER NOT NULL,
		user_id    INTEGER NOT NULL,
		name       TEXT    NOT NULL,
		step       INTEGER DEFAULT 0,
		data       BLOB,
		created_at INTEGER DEFAULT 0,
		updated_at INTEGER DEFAULT 0,
		PRIMARY KEY (chat_id, user_id)
	)`)
	if err == nil {
		a.convReady = true
	}
	return err
}

// --- SessionStore ---

func (a *SQLite) LoadSession() (*storage.Session, error) {
	var dcID, apiID, date, port int
	var testMode, isBot int
	var authKey, state []byte
	var userID int64
	var apiHash, firstName, lastName, username, serverAddress string

	err := a.db.QueryRow(
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

func (a *SQLite) SaveSession(s *storage.Session) error {
	tm := 0
	if s.TestMode {
		tm = 1
	}
	ib := 0
	if s.IsBot {
		ib = 1
	}
	_, err := a.db.Exec(`INSERT OR REPLACE INTO sessions
		(dc_id, api_id, api_hash, test_mode, auth_key, state, user_id, is_bot, first_name, last_name, username, date, server_address, port)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		s.DC, s.APIID, s.APIHash, tm, s.AuthKey, s.State, s.UserID, ib,
		s.FirstName, s.LastName, s.Username, s.Date, s.Addr, s.Port)
	return err
}

// --- PeerStore ---

func (a *SQLite) SavePeer(p *storage.Peer) error {
	ib := 0
	if p.IsBot {
		ib = 1
	}
	_, err := a.db.Exec(`INSERT OR REPLACE INTO peers
		(id, type, access_hash, username, usernames, first_name, last_name, phone_number, is_bot, photo_id, language, last_updated)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.ID, p.Type, p.AccessHash, p.Username, p.Usernames, p.FirstName, p.LastName,
		p.PhoneNumber, ib, p.PhotoID, p.Language, p.LastUpdated)
	return err
}

func (a *SQLite) GetPeer(id int64) (*storage.Peer, error) {
	p := &storage.Peer{}
	var isBot int
	err := a.db.QueryRow(
		`SELECT id, type, access_hash, username, usernames, first_name, last_name, phone_number, is_bot, photo_id, language, last_updated FROM peers WHERE id = ?`, id).
		Scan(&p.ID, &p.Type, &p.AccessHash, &p.Username, &p.Usernames, &p.FirstName, &p.LastName,
			&p.PhoneNumber, &isBot, &p.PhotoID, &p.Language, &p.LastUpdated)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	p.IsBot = isBot != 0
	return p, nil
}

func (a *SQLite) GetPeerByUsername(username string) (*storage.Peer, error) {
	p := &storage.Peer{}
	var isBot int
	err := a.db.QueryRow(
		`SELECT id, type, access_hash, username, usernames, first_name, last_name, phone_number, is_bot, photo_id, language, last_updated FROM peers WHERE username = ?`, username).
		Scan(&p.ID, &p.Type, &p.AccessHash, &p.Username, &p.Usernames, &p.FirstName, &p.LastName,
			&p.PhoneNumber, &isBot, &p.PhotoID, &p.Language, &p.LastUpdated)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	p.IsBot = isBot != 0
	return p, nil
}

func (a *SQLite) LoadPeers() ([]*storage.Peer, error) {
	rows, err := a.db.Query(`SELECT id, type, access_hash, username, usernames, first_name, last_name, phone_number, is_bot, photo_id, language, last_updated FROM peers`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var peers []*storage.Peer
	for rows.Next() {
		p := &storage.Peer{}
		var isBot int
		if err := rows.Scan(&p.ID, &p.Type, &p.AccessHash, &p.Username, &p.Usernames, &p.FirstName, &p.LastName,
			&p.PhoneNumber, &isBot, &p.PhotoID, &p.Language, &p.LastUpdated); err != nil {
			return nil, err
		}
		p.IsBot = isBot != 0
		peers = append(peers, p)
	}
	return peers, nil
}

func (a *SQLite) DeletePeer(id int64) error {
	_, err := a.db.Exec(`DELETE FROM peers WHERE id = ?`, id)
	return err
}

// --- ConversationStore (lazy) ---

func (a *SQLite) SaveConversation(c *storage.Conversation) error {
	if err := a.ensureConvTable(); err != nil {
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
	_, err := a.db.Exec(`INSERT OR REPLACE INTO conversations
		(chat_id, user_id, name, step, data, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		c.ChatID, c.UserID, c.Name, c.Step, c.Data, createdAt, now)
	return err
}

func (a *SQLite) LoadConversation(chatID, userID int64) (*storage.Conversation, error) {
	if err := a.ensureConvTable(); err != nil {
		return nil, err
	}
	c := &storage.Conversation{}
	err := a.db.QueryRow(
		`SELECT chat_id, user_id, name, step, data, created_at, updated_at FROM conversations WHERE chat_id = ? AND user_id = ?`, chatID, userID).
		Scan(&c.ChatID, &c.UserID, &c.Name, &c.Step, &c.Data, &c.CreatedAt, &c.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return c, nil
}

func (a *SQLite) DeleteConversation(chatID, userID int64) error {
	if err := a.ensureConvTable(); err != nil {
		return err
	}
	_, err := a.db.Exec(`DELETE FROM conversations WHERE chat_id = ? AND user_id = ?`, chatID, userID)
	return err
}

// --- Close ---

func (a *SQLite) Close() error {
	if _, err := a.db.Exec("PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		a.db.Close()
		return err
	}
	return a.db.Close()
}

// --- ExportSessionString ---

// ExportSessionString encodes session data into a portable string.
func ExportSessionString(s *storage.Session) (string, error) {
	buf := new(bytes.Buffer)
	if err := binary.Write(buf, binary.LittleEndian, uint8(1)); err != nil {
		return "", err
	}
	if err := binary.Write(buf, binary.LittleEndian, uint8(s.DC)); err != nil {
		return "", err
	}
	if err := binary.Write(buf, binary.LittleEndian, uint32(s.APIID)); err != nil {
		return "", err
	}
	tm := uint8(0)
	if s.TestMode {
		tm = 1
	}
	if err := binary.Write(buf, binary.LittleEndian, tm); err != nil {
		return "", err
	}
	if err := binary.Write(buf, binary.LittleEndian, uint64(s.UserID)); err != nil {
		return "", err
	}
	ib := uint8(0)
	if s.IsBot {
		ib = 1
	}
	if err := binary.Write(buf, binary.LittleEndian, ib); err != nil {
		return "", err
	}
	if _, err := buf.Write(s.AuthKey); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(buf.Bytes()), nil
}
