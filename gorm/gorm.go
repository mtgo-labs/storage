package gorm

import (
	"time"

	"github.com/mtgo-labs/storage"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Config struct {
	AutoMigrate bool
	TablePrefix string
}

type SessionRecord struct {
	SessionID string `gorm:"primaryKey;size:255"`
	DC        int
	APIID     int32
	APIHash   string
	TestMode  bool
	AuthKey   []byte `gorm:"type:bytes"`
	State     []byte `gorm:"type:bytes"`
	UserID    int64
	IsBot     bool
	FirstName string
	LastName  string
	Username  string
	Date      int
	Addr      string
	Port      int
}

func (SessionRecord) TableName() string { return "mtgo_sessions" }

type PeerRecord struct {
	SessionID   string `gorm:"primaryKey;size:255;index"`
	ID          int64  `gorm:"primaryKey"`
	Type        int
	AccessHash  int64
	Username    string `gorm:"index"`
	Usernames   string
	FirstName   string
	LastName    string
	PhoneNumber string
	IsBot       bool
	PhotoID     int64
	Language    string
	LastUpdated int64
}

func (PeerRecord) TableName() string { return "mtgo_peers" }

type ConversationRecord struct {
	SessionID string `gorm:"primaryKey;size:255;index"`
	ChatID    int64  `gorm:"primaryKey"`
	UserID    int64  `gorm:"primaryKey"`
	Name      string
	Step      int
	Data      []byte `gorm:"type:bytes"`
	CreatedAt int64
	UpdatedAt int64
}

func (ConversationRecord) TableName() string { return "mtgo_conversations" }

type UpdateStateRecord struct {
	SessionID string `gorm:"primaryKey;size:255"`
	Pts       int32
	Qts       int32
	Date      int32
	Seq       int32
	UpdatedAt int64
}

func (UpdateStateRecord) TableName() string { return "mtgo_update_state" }

type ChannelUpdateStateRecord struct {
	SessionID string `gorm:"primaryKey;size:255;index"`
	ChannelID int64  `gorm:"primaryKey"`
	Pts       int32
	UpdatedAt int64
}

func (ChannelUpdateStateRecord) TableName() string { return "mtgo_channel_update_state" }

type UpdateDedupRecord struct {
	SessionID string `gorm:"primaryKey;size:255;index"`
	DedupKey  string `gorm:"primaryKey;size:255"`
	CreatedAt int64
}

func (UpdateDedupRecord) TableName() string { return "mtgo_update_dedup" }

type DurableUpdateRecord struct {
	SessionID string `gorm:"primaryKey;size:255;index"`
	ID        string `gorm:"primaryKey;size:255"`
	Payload   []byte `gorm:"type:bytes"`
	Attempts  int
	LastError string
	CreatedAt int64
	UpdatedAt int64
}

func (DurableUpdateRecord) TableName() string { return "mtgo_durable_updates" }

type gormAdapter struct {
	db          *gorm.DB
	sessionName string
	ownDB       bool
	prefix      string
}

func New(db *gorm.DB, cfg ...Config) storage.Storage {
	var c Config
	if len(cfg) > 0 {
		c = cfg[0]
	}
	if c.TablePrefix == "" {
		c.TablePrefix = "mtgo_"
	}
	if !c.AutoMigrate && len(cfg) == 0 {
		c.AutoMigrate = true
	}

	a := &gormAdapter{
		db:     db,
		prefix: c.TablePrefix,
	}

	if c.AutoMigrate {
		_ = db.AutoMigrate(
			&SessionRecord{},
			&PeerRecord{},
			&ConversationRecord{},
			&UpdateStateRecord{},
			&ChannelUpdateStateRecord{},
			&UpdateDedupRecord{},
			&DurableUpdateRecord{},
		)
	}

	return storage.NewAdapter(a)
}

func (a *gormAdapter) SetSessionName(name string) {
	a.sessionName = name
}

func (a *gormAdapter) st() string   { return a.prefix + "sessions" }
func (a *gormAdapter) pt() string   { return a.prefix + "peers" }
func (a *gormAdapter) ct() string   { return a.prefix + "conversations" }
func (a *gormAdapter) ust() string  { return a.prefix + "update_state" }
func (a *gormAdapter) cust() string { return a.prefix + "channel_update_state" }
func (a *gormAdapter) udt() string  { return a.prefix + "update_dedup" }
func (a *gormAdapter) dut() string  { return a.prefix + "durable_updates" }

// --- SessionStore ---

func (a *gormAdapter) LoadSession() (*storage.Session, error) {
	if a.sessionName == "" {
		return nil, nil
	}
	var rec SessionRecord
	result := a.db.Table(a.st()).Where("session_id = ?", a.sessionName).First(&rec)
	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, result.Error
	}
	return a.sessionToDomain(&rec), nil
}

func (a *gormAdapter) SaveSession(s *storage.Session) error {
	rec := a.sessionToRecord(s)
	if rec.SessionID == "" {
		rec.SessionID = a.sessionName
	}
	result := a.db.Table(a.st()).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "session_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"dc", "api_id", "api_hash", "test_mode", "auth_key", "state",
			"user_id", "is_bot", "first_name", "last_name", "username",
			"date", "addr", "port",
		}),
	}).Create(rec)
	return result.Error
}

// --- PeerStore ---

func (a *gormAdapter) SavePeer(p *storage.Peer) error {
	rec := a.peerToRecord(p)
	result := a.db.Table(a.pt()).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "session_id"}, {Name: "id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"type", "access_hash", "username", "usernames", "first_name",
			"last_name", "phone_number", "is_bot", "photo_id", "language", "last_updated",
		}),
	}).Create(rec)
	return result.Error
}

func (a *gormAdapter) GetPeer(id int64) (*storage.Peer, error) {
	var rec PeerRecord
	result := a.db.Table(a.pt()).Where("session_id = ? AND id = ?", a.sessionName, id).First(&rec)
	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, result.Error
	}
	return a.peerToDomain(&rec), nil
}

func (a *gormAdapter) GetPeerByUsername(username string) (*storage.Peer, error) {
	var rec PeerRecord
	result := a.db.Table(a.pt()).Where("session_id = ? AND username = ?", a.sessionName, username).First(&rec)
	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, result.Error
	}
	return a.peerToDomain(&rec), nil
}

func (a *gormAdapter) LoadPeers() ([]*storage.Peer, error) {
	var recs []PeerRecord
	result := a.db.Table(a.pt()).Where("session_id = ?", a.sessionName).Find(&recs)
	if result.Error != nil {
		return nil, result.Error
	}
	peers := make([]*storage.Peer, len(recs))
	for i := range recs {
		peers[i] = a.peerToDomain(&recs[i])
	}
	return peers, nil
}

func (a *gormAdapter) DeletePeer(id int64) error {
	return a.db.Table(a.pt()).Where("session_id = ? AND id = ?", a.sessionName, id).Delete(nil).Error
}

// --- ConversationStore ---

func (a *gormAdapter) SaveConversation(c *storage.Conversation) error {
	rec := a.conversationToRecord(c)
	result := a.db.Table(a.ct()).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "session_id"}, {Name: "chat_id"}, {Name: "user_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"name", "step", "data", "updated_at",
		}),
	}).Create(rec)
	return result.Error
}

func (a *gormAdapter) LoadConversation(chatID, userID int64) (*storage.Conversation, error) {
	var rec ConversationRecord
	result := a.db.Table(a.ct()).Where("session_id = ? AND chat_id = ? AND user_id = ?", a.sessionName, chatID, userID).First(&rec)
	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, result.Error
	}
	return a.conversationToDomain(&rec), nil
}

func (a *gormAdapter) DeleteConversation(chatID, userID int64) error {
	return a.db.Table(a.ct()).Where("session_id = ? AND chat_id = ? AND user_id = ?", a.sessionName, chatID, userID).Delete(nil).Error
}

// --- UpdateStateStore ---

func (a *gormAdapter) LoadUpdateState(sessionID string) (*storage.UpdateState, error) {
	var rec UpdateStateRecord
	result := a.db.Table(a.ust()).Where("session_id = ?", sessionID).First(&rec)
	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, result.Error
	}
	return &storage.UpdateState{
		SessionID: rec.SessionID, Pts: rec.Pts, Qts: rec.Qts, Date: rec.Date, Seq: rec.Seq,
	}, nil
}

func (a *gormAdapter) SaveUpdateState(state *storage.UpdateState) error {
	rec := &UpdateStateRecord{
		SessionID: state.SessionID, Pts: state.Pts, Qts: state.Qts, Date: state.Date, Seq: state.Seq,
		UpdatedAt: time.Now().Unix(),
	}
	return a.db.Table(a.ust()).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "session_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"pts", "qts", "date", "seq", "updated_at"}),
	}).Create(rec).Error
}

func (a *gormAdapter) LoadChannelUpdateState(sessionID string, channelID int64) (*storage.ChannelUpdateState, error) {
	var rec ChannelUpdateStateRecord
	result := a.db.Table(a.cust()).Where("session_id = ? AND channel_id = ?", sessionID, channelID).First(&rec)
	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, result.Error
	}
	return &storage.ChannelUpdateState{SessionID: rec.SessionID, ChannelID: rec.ChannelID, Pts: rec.Pts}, nil
}

func (a *gormAdapter) LoadAllChannelUpdateStates(sessionID string) ([]*storage.ChannelUpdateState, error) {
	var recs []ChannelUpdateStateRecord
	if err := a.db.Table(a.cust()).Where("session_id = ?", sessionID).Find(&recs).Error; err != nil {
		return nil, err
	}
	out := make([]*storage.ChannelUpdateState, len(recs))
	for i := range recs {
		out[i] = &storage.ChannelUpdateState{SessionID: recs[i].SessionID, ChannelID: recs[i].ChannelID, Pts: recs[i].Pts}
	}
	return out, nil
}

func (a *gormAdapter) SaveChannelUpdateState(state *storage.ChannelUpdateState) error {
	rec := &ChannelUpdateStateRecord{
		SessionID: state.SessionID, ChannelID: state.ChannelID, Pts: state.Pts, UpdatedAt: time.Now().Unix(),
	}
	return a.db.Table(a.cust()).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "session_id"}, {Name: "channel_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"pts", "updated_at"}),
	}).Create(rec).Error
}

func (a *gormAdapter) SaveUpdateDedupKey(sessionID string, key string) (bool, error) {
	rec := &UpdateDedupRecord{SessionID: sessionID, DedupKey: key, CreatedAt: time.Now().Unix()}
	result := a.db.Table(a.udt()).Clauses(clause.OnConflict{DoNothing: true}).Create(rec)
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

func (a *gormAdapter) UpdateDedupKeyExists(sessionID string, key string) (bool, error) {
	var count int64
	result := a.db.Table(a.udt()).Where("session_id = ? AND dedup_key = ?", sessionID, key).Count(&count)
	return count > 0, result.Error
}

func (a *gormAdapter) EnqueueDurableUpdate(update *storage.DurableUpdate) error {
	rec := &DurableUpdateRecord{
		SessionID: update.SessionID, ID: update.ID, Payload: update.Payload,
		Attempts: update.Attempts, LastError: update.LastError, CreatedAt: update.CreatedAt, UpdatedAt: update.UpdatedAt,
	}
	return a.db.Table(a.dut()).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "session_id"}, {Name: "id"}},
		DoUpdates: clause.AssignmentColumns([]string{"payload", "attempts", "last_error", "updated_at"}),
	}).Create(rec).Error
}

func (a *gormAdapter) DeleteDurableUpdate(sessionID string, id string) error {
	return a.db.Table(a.dut()).Where("session_id = ? AND id = ?", sessionID, id).Delete(nil).Error
}

func (a *gormAdapter) LoadDurableUpdates(sessionID string, limit int) ([]*storage.DurableUpdate, error) {
	var recs []DurableUpdateRecord
	q := a.db.Table(a.dut()).Where("session_id = ?", sessionID).Order("created_at ASC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	if err := q.Find(&recs).Error; err != nil {
		return nil, err
	}
	out := make([]*storage.DurableUpdate, len(recs))
	for i := range recs {
		out[i] = &storage.DurableUpdate{
			SessionID: recs[i].SessionID, ID: recs[i].ID, Payload: recs[i].Payload,
			Attempts: recs[i].Attempts, LastError: recs[i].LastError, CreatedAt: recs[i].CreatedAt, UpdatedAt: recs[i].UpdatedAt,
		}
	}
	return out, nil
}

func (a *gormAdapter) MarkDurableUpdateFailed(sessionID string, id string, attempts int, lastErr string) error {
	return a.db.Table(a.dut()).Where("session_id = ? AND id = ?", sessionID, id).
		Updates(map[string]interface{}{"attempts": attempts, "last_error": lastErr, "updated_at": time.Now().Unix()}).Error
}

// --- Close ---

func (a *gormAdapter) Close() error {
	if a.ownDB {
		sqlDB, err := a.db.DB()
		if err != nil {
			return err
		}
		return sqlDB.Close()
	}
	return nil
}

// --- Conversion helpers ---

func (a *gormAdapter) sessionToDomain(rec *SessionRecord) *storage.Session {
	return &storage.Session{
		SessionID: rec.SessionID, DC: rec.DC, APIID: rec.APIID, APIHash: rec.APIHash,
		TestMode: rec.TestMode, AuthKey: rec.AuthKey, State: rec.State, UserID: rec.UserID,
		IsBot: rec.IsBot, FirstName: rec.FirstName, LastName: rec.LastName, Username: rec.Username,
		Date: rec.Date, Addr: rec.Addr, Port: rec.Port,
	}
}

func (a *gormAdapter) sessionToRecord(s *storage.Session) *SessionRecord {
	return &SessionRecord{
		SessionID: s.SessionID, DC: s.DC, APIID: s.APIID, APIHash: s.APIHash,
		TestMode: s.TestMode, AuthKey: s.AuthKey, State: s.State, UserID: s.UserID,
		IsBot: s.IsBot, FirstName: s.FirstName, LastName: s.LastName, Username: s.Username,
		Date: s.Date, Addr: s.Addr, Port: s.Port,
	}
}

func (a *gormAdapter) peerToDomain(rec *PeerRecord) *storage.Peer {
	return &storage.Peer{
		ID: rec.ID, Type: storage.PeerType(rec.Type), AccessHash: rec.AccessHash,
		Username: rec.Username, Usernames: rec.Usernames, FirstName: rec.FirstName, LastName: rec.LastName,
		PhoneNumber: rec.PhoneNumber, IsBot: rec.IsBot, PhotoID: rec.PhotoID, Language: rec.Language, LastUpdated: rec.LastUpdated,
	}
}

func (a *gormAdapter) peerToRecord(p *storage.Peer) *PeerRecord {
	return &PeerRecord{
		SessionID: a.sessionName, ID: p.ID, Type: int(p.Type), AccessHash: p.AccessHash,
		Username: p.Username, Usernames: p.Usernames, FirstName: p.FirstName, LastName: p.LastName,
		PhoneNumber: p.PhoneNumber, IsBot: p.IsBot, PhotoID: p.PhotoID, Language: p.Language, LastUpdated: p.LastUpdated,
	}
}

func (a *gormAdapter) conversationToDomain(rec *ConversationRecord) *storage.Conversation {
	return &storage.Conversation{
		ChatID: rec.ChatID, UserID: rec.UserID, Name: rec.Name, Step: rec.Step,
		Data: rec.Data, CreatedAt: rec.CreatedAt, UpdatedAt: rec.UpdatedAt,
	}
}

func (a *gormAdapter) conversationToRecord(c *storage.Conversation) *ConversationRecord {
	return &ConversationRecord{
		SessionID: a.sessionName, ChatID: c.ChatID, UserID: c.UserID, Name: c.Name,
		Step: c.Step, Data: c.Data, CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt,
	}
}
