package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	"github.com/mtgo-labs/storage"
	_ "github.com/lib/pq"
)

type Postgres struct {
	db        *sql.DB
	q         *Queries
	cfg       Config
	initOnce  sync.Once
	initErr   error
	convOnce  sync.Once
	convReady bool
}

var (
	_ storage.Adapter           = (*Postgres)(nil)
	_ storage.ConversationStore = (*Postgres)(nil)
	_ storage.UpdateStateStore  = (*Postgres)(nil)
)

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
	return &Postgres{db: db, q: NewSqlcQueries()}, nil
}

func New(cfg Config) storage.Storage {
	return storage.NewAdapter(&Postgres{cfg: cfg, q: NewSqlcQueries()})
}

func (p *Postgres) init() error {
	p.initOnce.Do(func() {
		db, err := sql.Open("postgres", p.cfg.dsn())
		if err != nil {
			p.initErr = err
			return
		}
		if err := db.Ping(); err != nil {
			db.Close()
			p.initErr = err
			return
		}
		if err := initSchema(db); err != nil {
			db.Close()
			p.initErr = err
			return
		}
		p.db = db
	})
	return p.initErr
}

func initSchema(db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS sessions (
			session_id     TEXT DEFAULT '',
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
		`CREATE TABLE IF NOT EXISTS update_state (
			session_id TEXT PRIMARY KEY,
			pts INTEGER NOT NULL DEFAULT 0,
			qts INTEGER NOT NULL DEFAULT 0,
			date INTEGER NOT NULL DEFAULT 0,
			seq INTEGER NOT NULL DEFAULT 0,
			updated_at BIGINT NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS channel_update_state (
			session_id TEXT NOT NULL,
			channel_id BIGINT NOT NULL,
			pts INTEGER NOT NULL DEFAULT 0,
			updated_at BIGINT NOT NULL DEFAULT 0,
			PRIMARY KEY (session_id, channel_id)
		)`,
		`CREATE TABLE IF NOT EXISTS update_dedup (
			session_id TEXT NOT NULL,
			dedup_key TEXT NOT NULL,
			created_at BIGINT NOT NULL DEFAULT 0,
			PRIMARY KEY (session_id, dedup_key)
		)`,
		`CREATE TABLE IF NOT EXISTS durable_updates (
			session_id TEXT NOT NULL,
			id TEXT NOT NULL,
			payload BYTEA NOT NULL,
			attempts INTEGER NOT NULL DEFAULT 0,
			last_error TEXT NOT NULL DEFAULT '',
			created_at BIGINT NOT NULL DEFAULT 0,
			updated_at BIGINT NOT NULL DEFAULT 0,
			PRIMARY KEY (session_id, id)
		)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			return fmt.Errorf("postgres init: %w", err)
		}
	}
	return nil
}

func (p *Postgres) ensureConvTable() error {
	if err := p.init(); err != nil {
		return err
	}
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

func (p *Postgres) Close() error {
	if err := p.init(); err != nil {
		return err
	}
	return p.db.Close()
}

// --- SessionStore ---

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func (p *Postgres) LoadSession() (*storage.Session, error) {
	if err := p.init(); err != nil {
		return nil, err
	}
	row, err := p.q.GetSession(context.Background(), p.db)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &storage.Session{
		SessionID: row.SessionID, DC: int(row.DcID), APIID: int32(row.ApiID),
		APIHash: row.ApiHash, TestMode: row.TestMode != 0,
		AuthKey: row.AuthKey, State: row.State, UserID: row.UserID,
		IsBot: row.IsBot != 0, FirstName: row.FirstName, LastName: row.LastName,
		Username: row.Username, Date: int(row.Date), Addr: row.ServerAddress, Port: int(row.Port),
	}, nil
}

func (p *Postgres) SaveSession(s *storage.Session) error {
	if err := p.init(); err != nil {
		return err
	}
	return p.q.UpsertSession(context.Background(), p.db, UpsertSessionParams{
		SessionID: s.SessionID, DcID: int32(s.DC), ApiID: s.APIID,
		ApiHash: s.APIHash, TestMode: boolToInt(s.TestMode),
		AuthKey: s.AuthKey, State: s.State, UserID: s.UserID,
		IsBot: boolToInt(s.IsBot), FirstName: s.FirstName, LastName: s.LastName,
		Username: s.Username, Date: int32(s.Date), ServerAddress: s.Addr, Port: int32(s.Port),
	})
}

// --- PeerStore ---

func (p *Postgres) SavePeer(peer *storage.Peer) error {
	if err := p.init(); err != nil {
		return err
	}
	return p.q.UpsertPeer(context.Background(), p.db, UpsertPeerParams{
		ID: peer.ID, Type: int32(peer.Type), AccessHash: peer.AccessHash,
		Username: peer.Username, Usernames: peer.Usernames, FirstName: peer.FirstName,
		LastName: peer.LastName, PhoneNumber: peer.PhoneNumber, IsBot: boolToInt(peer.IsBot),
		PhotoID: peer.PhotoID, Language: peer.Language, LastUpdated: peer.LastUpdated,
	})
}

func (p *Postgres) GetPeer(id int64) (*storage.Peer, error) {
	if err := p.init(); err != nil {
		return nil, err
	}
	row, err := p.q.GetPeer(context.Background(), p.db, id)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &storage.Peer{
		ID: row.ID, Type: storage.PeerType(row.Type), AccessHash: row.AccessHash,
		Username: row.Username, Usernames: row.Usernames, FirstName: row.FirstName,
		LastName: row.LastName, PhoneNumber: row.PhoneNumber, IsBot: row.IsBot != 0,
		PhotoID: row.PhotoID, Language: row.Language, LastUpdated: row.LastUpdated,
	}, nil
}

func (p *Postgres) GetPeerByUsername(username string) (*storage.Peer, error) {
	if err := p.init(); err != nil {
		return nil, err
	}
	row, err := p.q.GetPeerByUsername(context.Background(), p.db, username)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &storage.Peer{
		ID: row.ID, Type: storage.PeerType(row.Type), AccessHash: row.AccessHash,
		Username: row.Username, Usernames: row.Usernames, FirstName: row.FirstName,
		LastName: row.LastName, PhoneNumber: row.PhoneNumber, IsBot: row.IsBot != 0,
		PhotoID: row.PhotoID, Language: row.Language, LastUpdated: row.LastUpdated,
	}, nil
}

func (p *Postgres) LoadPeers() ([]*storage.Peer, error) {
	if err := p.init(); err != nil {
		return nil, err
	}
	rows, err := p.q.ListPeers(context.Background(), p.db)
	if err != nil {
		return nil, err
	}
	out := make([]*storage.Peer, len(rows))
	for i, r := range rows {
		out[i] = &storage.Peer{
			ID: r.ID, Type: storage.PeerType(r.Type), AccessHash: r.AccessHash,
			Username: r.Username, Usernames: r.Usernames, FirstName: r.FirstName,
			LastName: r.LastName, PhoneNumber: r.PhoneNumber, IsBot: r.IsBot != 0,
			PhotoID: r.PhotoID, Language: r.Language, LastUpdated: r.LastUpdated,
		}
	}
	return out, nil
}

func (p *Postgres) DeletePeer(id int64) error {
	if err := p.init(); err != nil {
		return err
	}
	return p.q.DeletePeer(context.Background(), p.db, id)
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
	return p.q.UpsertConversation(context.Background(), p.db, UpsertConversationParams{
		ChatID: c.ChatID, UserID: c.UserID, Name: c.Name, Step: int32(c.Step),
		Data: c.Data, CreatedAt: createdAt, UpdatedAt: now,
	})
}

func (p *Postgres) LoadConversation(chatID, userID int64) (*storage.Conversation, error) {
	if err := p.ensureConvTable(); err != nil {
		return nil, err
	}
	row, err := p.q.GetConversation(context.Background(), p.db, GetConversationParams{
		ChatID: chatID, UserID: userID,
	})
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &storage.Conversation{
		ChatID: row.ChatID, UserID: row.UserID, Name: row.Name,
		Step: int(row.Step), Data: row.Data, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}, nil
}

func (p *Postgres) DeleteConversation(chatID, userID int64) error {
	if err := p.ensureConvTable(); err != nil {
		return err
	}
	return p.q.DeleteConversation(context.Background(), p.db, DeleteConversationParams{
		ChatID: chatID, UserID: userID,
	})
}

// --- UpdateStateStore ---

func (p *Postgres) LoadUpdateState(sessionID string) (*storage.UpdateState, error) {
	if err := p.init(); err != nil {
		return nil, err
	}
	row, err := p.q.GetUpdateState(context.Background(), p.db, sessionID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &storage.UpdateState{
		SessionID: row.SessionID, Pts: row.Pts, Qts: row.Qts,
		Date: row.Date, Seq: row.Seq,
	}, nil
}

func (p *Postgres) SaveUpdateState(s *storage.UpdateState) error {
	if err := p.init(); err != nil {
		return err
	}
	return p.q.UpsertUpdateState(context.Background(), p.db, UpsertUpdateStateParams{
		SessionID: s.SessionID, Pts: s.Pts, Qts: s.Qts,
		Date: s.Date, Seq: s.Seq, UpdatedAt: time.Now().Unix(),
	})
}

func (p *Postgres) LoadChannelUpdateState(sessionID string, channelID int64) (*storage.ChannelUpdateState, error) {
	if err := p.init(); err != nil {
		return nil, err
	}
	row, err := p.q.GetChannelUpdateState(context.Background(), p.db, GetChannelUpdateStateParams{
		SessionID: sessionID, ChannelID: channelID,
	})
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &storage.ChannelUpdateState{
		SessionID: row.SessionID, ChannelID: row.ChannelID, Pts: row.Pts,
	}, nil
}

func (p *Postgres) LoadAllChannelUpdateStates(sessionID string) ([]*storage.ChannelUpdateState, error) {
	if err := p.init(); err != nil {
		return nil, err
	}
	rows, err := p.q.ListChannelUpdateStates(context.Background(), p.db, sessionID)
	if err != nil {
		return nil, err
	}
	out := make([]*storage.ChannelUpdateState, len(rows))
	for i, r := range rows {
		out[i] = &storage.ChannelUpdateState{
			SessionID: r.SessionID, ChannelID: r.ChannelID, Pts: r.Pts,
		}
	}
	return out, nil
}

func (p *Postgres) SaveChannelUpdateState(s *storage.ChannelUpdateState) error {
	if err := p.init(); err != nil {
		return err
	}
	return p.q.UpsertChannelUpdateState(context.Background(), p.db, UpsertChannelUpdateStateParams{
		SessionID: s.SessionID, ChannelID: s.ChannelID, Pts: s.Pts,
		UpdatedAt: time.Now().Unix(),
	})
}

func (p *Postgres) SaveUpdateDedupKey(sessionID string, key string) (bool, error) {
	if err := p.init(); err != nil {
		return false, err
	}
	n, err := p.q.InsertDedupKey(context.Background(), p.db, InsertDedupKeyParams{
		SessionID: sessionID, DedupKey: key, CreatedAt: time.Now().Unix(),
	})
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func (p *Postgres) UpdateDedupKeyExists(sessionID string, key string) (bool, error) {
	if err := p.init(); err != nil {
		return false, err
	}
	count, err := p.q.ExistsDedupKey(context.Background(), p.db, ExistsDedupKeyParams{
		SessionID: sessionID, DedupKey: key,
	})
	return count > 0, err
}

func (p *Postgres) EnqueueDurableUpdate(u *storage.DurableUpdate) error {
	if err := p.init(); err != nil {
		return err
	}
	return p.q.UpsertDurableUpdate(context.Background(), p.db, UpsertDurableUpdateParams{
		SessionID: u.SessionID, ID: u.ID, Payload: u.Payload,
		Attempts: int32(u.Attempts), LastError: u.LastError,
		CreatedAt: u.CreatedAt, UpdatedAt: time.Now().Unix(),
	})
}

func (p *Postgres) DeleteDurableUpdate(sessionID string, id string) error {
	if err := p.init(); err != nil {
		return err
	}
	return p.q.DeleteDurableUpdate(context.Background(), p.db, DeleteDurableUpdateParams{
		SessionID: sessionID, ID: id,
	})
}

func (p *Postgres) LoadDurableUpdates(sessionID string, limit int) ([]*storage.DurableUpdate, error) {
	if err := p.init(); err != nil {
		return nil, err
	}
	rows, err := p.q.ListDurableUpdates(context.Background(), p.db, ListDurableUpdatesParams{
		SessionID: sessionID, Limit: int32(limit),
	})
	if err != nil {
		return nil, err
	}
	out := make([]*storage.DurableUpdate, len(rows))
	for i, r := range rows {
		out[i] = &storage.DurableUpdate{
			SessionID: r.SessionID, ID: r.ID, Payload: r.Payload,
			Attempts: int(r.Attempts), LastError: r.LastError,
			CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		}
	}
	return out, nil
}

func (p *Postgres) MarkDurableUpdateFailed(sessionID string, id string, attempts int, lastErr string) error {
	if err := p.init(); err != nil {
		return err
	}
	return p.q.MarkDurableUpdateFailed(context.Background(), p.db, MarkDurableUpdateFailedParams{
		Attempts: int32(attempts), LastError: lastErr, UpdatedAt: time.Now().Unix(),
		SessionID: sessionID, ID: id,
	})
}
