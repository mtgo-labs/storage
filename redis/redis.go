package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/mtgo-labs/storage"
)

type Config struct {
	KeyPrefix string
	TTL       time.Duration
}

type Client interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error
	Del(ctx context.Context, keys ...string) error
	Exists(ctx context.Context, keys ...string) (int64, error)
	Scan(ctx context.Context, match string, count int64) ([]string, error)
	MGet(ctx context.Context, keys ...string) ([]interface{}, error)
}

type redisAdapter struct {
	client      Client
	sessionName string
	prefix      string
	ttl         time.Duration
}

func New(client Client, cfg ...Config) storage.Storage {
	var c Config
	if len(cfg) > 0 {
		c = cfg[0]
	}
	if c.KeyPrefix == "" {
		c.KeyPrefix = "mtgo"
	}
	return storage.NewAdapter(&redisAdapter{
		client: client,
		prefix: c.KeyPrefix,
		ttl:    c.TTL,
	})
}

func (r *redisAdapter) key(parts ...string) string {
	all := make([]string, 0, len(parts)+2)
	all = append(all, r.prefix, r.sessionName)
	all = append(all, parts...)
	return strings.Join(all, ":")
}

func (r *redisAdapter) SetSessionName(name string) {
	r.sessionName = name
}

func (r *redisAdapter) LoadSession() (*storage.Session, error) {
	val, err := r.client.Get(context.Background(), r.key("session"))
	if err != nil {
		return nil, nil
	}
	var s storage.Session
	if err := json.Unmarshal([]byte(val), &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *redisAdapter) SaveSession(s *storage.Session) error {
	data, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return r.client.Set(context.Background(), r.key("session"), data, r.ttl)
}

func (r *redisAdapter) SavePeer(p *storage.Peer) error {
	data, err := json.Marshal(p)
	if err != nil {
		return err
	}
	ctx := context.Background()
	if err := r.client.Set(ctx, r.key("peer", strconv.FormatInt(p.ID, 10)), data, r.ttl); err != nil {
		return err
	}
	if p.Username != "" {
		if err := r.client.Set(ctx, r.key("peer_uname", p.Username), strconv.FormatInt(p.ID, 10), r.ttl); err != nil {
			return err
		}
	}
	return nil
}

func (r *redisAdapter) GetPeer(id int64) (*storage.Peer, error) {
	val, err := r.client.Get(context.Background(), r.key("peer", strconv.FormatInt(id, 10)))
	if err != nil {
		return nil, nil
	}
	var p storage.Peer
	if err := json.Unmarshal([]byte(val), &p); err != nil {
		return nil, err
	}
	return &p, nil
}

func (r *redisAdapter) GetPeerByUsername(username string) (*storage.Peer, error) {
	idStr, err := r.client.Get(context.Background(), r.key("peer_uname", username))
	if err != nil {
		return nil, nil
	}
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return nil, nil
	}
	return r.GetPeer(id)
}

func (r *redisAdapter) LoadPeers() ([]*storage.Peer, error) {
	ctx := context.Background()
	keys, err := r.client.Scan(ctx, r.key("peer", "*"), 100)
	if err != nil {
		return nil, err
	}
	if len(keys) == 0 {
		return nil, nil
	}
	vals, err := r.client.MGet(ctx, keys...)
	if err != nil {
		return nil, err
	}
	peers := make([]*storage.Peer, 0, len(vals))
	for _, v := range vals {
		if v == nil {
			continue
		}
		var b []byte
		switch val := v.(type) {
		case string:
			b = []byte(val)
		case []byte:
			b = val
		default:
			continue
		}
		var p storage.Peer
		if err := json.Unmarshal(b, &p); err != nil {
			continue
		}
		peers = append(peers, &p)
	}
	return peers, nil
}

func (r *redisAdapter) DeletePeer(id int64) error {
	ctx := context.Background()
	peer, _ := r.GetPeer(id)
	keys := []string{r.key("peer", strconv.FormatInt(id, 10))}
	if peer != nil && peer.Username != "" {
		keys = append(keys, r.key("peer_uname", peer.Username))
	}
	return r.client.Del(ctx, keys...)
}

func (r *redisAdapter) SaveConversation(c *storage.Conversation) error {
	data, err := json.Marshal(c)
	if err != nil {
		return err
	}
	k := r.key("conv", strconv.FormatInt(c.ChatID, 10), strconv.FormatInt(c.UserID, 10))
	return r.client.Set(context.Background(), k, data, r.ttl)
}

func (r *redisAdapter) LoadConversation(chatID, userID int64) (*storage.Conversation, error) {
	val, err := r.client.Get(context.Background(), r.key("conv", strconv.FormatInt(chatID, 10), strconv.FormatInt(userID, 10)))
	if err != nil {
		return nil, nil
	}
	var c storage.Conversation
	if err := json.Unmarshal([]byte(val), &c); err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *redisAdapter) DeleteConversation(chatID, userID int64) error {
	return r.client.Del(context.Background(), r.key("conv", strconv.FormatInt(chatID, 10), strconv.FormatInt(userID, 10)))
}

func (r *redisAdapter) LoadUpdateState(sessionID string) (*storage.UpdateState, error) {
	val, err := r.client.Get(context.Background(), r.key("update_state"))
	if err != nil {
		return nil, nil
	}
	var s storage.UpdateState
	if err := json.Unmarshal([]byte(val), &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *redisAdapter) SaveUpdateState(state *storage.UpdateState) error {
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return r.client.Set(context.Background(), r.key("update_state"), data, r.ttl)
}

func (r *redisAdapter) LoadChannelUpdateState(sessionID string, channelID int64) (*storage.ChannelUpdateState, error) {
	val, err := r.client.Get(context.Background(), r.key("ch_pts", strconv.FormatInt(channelID, 10)))
	if err != nil {
		return nil, nil
	}
	var s storage.ChannelUpdateState
	if err := json.Unmarshal([]byte(val), &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *redisAdapter) LoadAllChannelUpdateStates(sessionID string) ([]*storage.ChannelUpdateState, error) {
	keys, err := r.client.Scan(context.Background(), r.key("ch_pts", "*"), 100)
	if err != nil {
		return nil, err
	}
	if len(keys) == 0 {
		return nil, nil
	}
	vals, err := r.client.MGet(context.Background(), keys...)
	if err != nil {
		return nil, err
	}
	states := make([]*storage.ChannelUpdateState, 0, len(vals))
	for _, v := range vals {
		if v == nil {
			continue
		}
		var b []byte
		switch val := v.(type) {
		case string:
			b = []byte(val)
		case []byte:
			b = val
		default:
			continue
		}
		var s storage.ChannelUpdateState
		if err := json.Unmarshal(b, &s); err != nil {
			continue
		}
		states = append(states, &s)
	}
	return states, nil
}

func (r *redisAdapter) SaveChannelUpdateState(state *storage.ChannelUpdateState) error {
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return r.client.Set(context.Background(), r.key("ch_pts", strconv.FormatInt(state.ChannelID, 10)), data, r.ttl)
}

func (r *redisAdapter) SaveUpdateDedupKey(sessionID string, key string) (bool, error) {
	k := r.key("dedup", key)
	existing, err := r.client.Exists(context.Background(), k)
	if err != nil {
		return false, err
	}
	if existing > 0 {
		return false, nil
	}
	if err := r.client.Set(context.Background(), k, "1", r.ttl); err != nil {
		return false, err
	}
	return true, nil
}

func (r *redisAdapter) UpdateDedupKeyExists(sessionID string, key string) (bool, error) {
	n, err := r.client.Exists(context.Background(), r.key("dedup", key))
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func (r *redisAdapter) EnqueueDurableUpdate(update *storage.DurableUpdate) error {
	data, err := json.Marshal(update)
	if err != nil {
		return err
	}
	return r.client.Set(context.Background(), r.key("durable", update.ID), data, r.ttl)
}

func (r *redisAdapter) DeleteDurableUpdate(sessionID string, id string) error {
	return r.client.Del(context.Background(), r.key("durable", id))
}

func (r *redisAdapter) LoadDurableUpdates(sessionID string, limit int) ([]*storage.DurableUpdate, error) {
	keys, err := r.client.Scan(context.Background(), r.key("durable", "*"), int64(limit))
	if err != nil {
		return nil, err
	}
	if len(keys) == 0 {
		return nil, nil
	}
	if limit > 0 && len(keys) > limit {
		keys = keys[:limit]
	}
	vals, err := r.client.MGet(context.Background(), keys...)
	if err != nil {
		return nil, err
	}
	updates := make([]*storage.DurableUpdate, 0, len(vals))
	for _, v := range vals {
		if v == nil {
			continue
		}
		var b []byte
		switch val := v.(type) {
		case string:
			b = []byte(val)
		case []byte:
			b = val
		default:
			continue
		}
		var u storage.DurableUpdate
		if err := json.Unmarshal(b, &u); err != nil {
			continue
		}
		updates = append(updates, &u)
	}
	return updates, nil
}

func (r *redisAdapter) MarkDurableUpdateFailed(sessionID string, id string, attempts int, lastErr string) error {
	ctx := context.Background()
	k := r.key("durable", id)
	val, err := r.client.Get(ctx, k)
	if err != nil {
		return fmt.Errorf("durable update %s not found: %w", id, err)
	}
	var u storage.DurableUpdate
	if err := json.Unmarshal([]byte(val), &u); err != nil {
		return err
	}
	u.Attempts = attempts
	u.LastError = lastErr
	u.UpdatedAt = time.Now().Unix()
	data, err := json.Marshal(&u)
	if err != nil {
		return err
	}
	return r.client.Set(ctx, k, data, r.ttl)
}

func (r *redisAdapter) Close() error { return nil }
