// custom_storage demonstrates how to implement a custom storage adapter
// that persists data as JSON files on disk.
//
// A custom adapter must implement the storage.Adapter interface (SessionStore
// + PeerStore + Close). To support the conversations plugin, also implement
// storage.ConversationStore.
//
// Use the conformance test suite to verify correctness:
//
//	func TestMyAdapter(t *testing.T) { suite.Run(t, New("testdata")) }
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"

	"github.com/mtgo-labs/storage"
)

// JSONStore is a file-based storage adapter that persists sessions, peers,
// and conversations as JSON files under a root directory. It is safe for
// concurrent use.
//
// Directory layout:
//
//	<dir>/session.json
//	<dir>/peers/<id>.json
//	<dir>/conversations/<chatID>_<userID>.json
type JSONStore struct {
	mu  sync.RWMutex
	dir string
}

// New creates a JSONStore backed by dir. The directory and subdirectories are
// created lazily on first write.
func New(dir string) *JSONStore {
	return &JSONStore{dir: dir}
}

func (s *JSONStore) ensureDir(sub string) error {
	return os.MkdirAll(filepath.Join(s.dir, sub), 0o755)
}

func (s *JSONStore) readJSON(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return json.Unmarshal(data, v)
}

func (s *JSONStore) writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// --- SessionStore ---

func (s *JSONStore) LoadSession() (*storage.Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var sess *storage.Session
	return sess, s.readJSON(filepath.Join(s.dir, "session.json"), &sess)
}

func (s *JSONStore) SaveSession(sess *storage.Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureDir(""); err != nil {
		return err
	}
	return s.writeJSON(filepath.Join(s.dir, "session.json"), sess)
}

// --- PeerStore ---

func (s *JSONStore) SavePeer(p *storage.Peer) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureDir("peers"); err != nil {
		return err
	}
	return s.writeJSON(filepath.Join(s.dir, "peers", fmt.Sprintf("%d.json", p.ID)), p)
}

func (s *JSONStore) GetPeer(id int64) (*storage.Peer, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var p *storage.Peer
	return p, s.readJSON(filepath.Join(s.dir, "peers", fmt.Sprintf("%d.json", id)), &p)
}

func (s *JSONStore) GetPeerByUsername(username string) (*storage.Peer, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entries, err := os.ReadDir(filepath.Join(s.dir, "peers"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	for _, e := range entries {
		var p *storage.Peer
		if err := s.readJSON(filepath.Join(s.dir, "peers", e.Name()), &p); err != nil {
			continue
		}
		if p != nil && p.Username == username {
			return p, nil
		}
	}
	return nil, nil
}

func (s *JSONStore) LoadPeers() ([]*storage.Peer, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entries, err := os.ReadDir(filepath.Join(s.dir, "peers"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var peers []*storage.Peer
	for _, e := range entries {
		var p *storage.Peer
		if err := s.readJSON(filepath.Join(s.dir, "peers", e.Name()), &p); err != nil {
			continue
		}
		if p != nil {
			peers = append(peers, p)
		}
	}
	return peers, nil
}

func (s *JSONStore) DeletePeer(id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	err := os.Remove(filepath.Join(s.dir, "peers", fmt.Sprintf("%d.json", id)))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// --- ConversationStore ---

func convPath(dir string, chatID, userID int64) string {
	return filepath.Join(dir, "conversations", fmt.Sprintf("%d_%d.json", chatID, userID))
}

func (s *JSONStore) SaveConversation(c *storage.Conversation) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureDir("conversations"); err != nil {
		return err
	}
	return s.writeJSON(convPath(s.dir, c.ChatID, c.UserID), c)
}

func (s *JSONStore) LoadConversation(chatID, userID int64) (*storage.Conversation, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var c *storage.Conversation
	return c, s.readJSON(convPath(s.dir, chatID, userID), &c)
}

func (s *JSONStore) DeleteConversation(chatID, userID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	err := os.Remove(convPath(s.dir, chatID, userID))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// --- Close ---

func (s *JSONStore) Close() error { return nil }

// Compile-time interface checks.
var (
	_ storage.Adapter           = (*JSONStore)(nil)
	_ storage.ConversationStore = (*JSONStore)(nil)
)

func main() {
	dir := filepath.Join(os.TempDir(), "mtgo-json-store")
	defer os.RemoveAll(dir)

	store := New(dir)

	// Save and load a session → <dir>/session.json
	sess := &storage.Session{
		DC:        2,
		APIID:     12345,
		APIHash:   "deadbeef",
		UserID:    42,
		IsBot:     true,
		FirstName: "Demo",
		Username:  "demo_bot",
	}
	if err := store.SaveSession(sess); err != nil {
		log.Fatal(err)
	}

	loaded, err := store.LoadSession()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("session: dc=%d uid=%d bot=%v user=%s\n",
		loaded.DC, loaded.UserID, loaded.IsBot, loaded.Username)

	// Save and query a peer → <dir>/peers/<id>.json
	peer := &storage.Peer{
		ID:         100,
		Type:       storage.PeerTypeUser,
		AccessHash: 999888777,
		Username:   "alice",
		FirstName:  "Alice",
	}
	if err := store.SavePeer(peer); err != nil {
		log.Fatal(err)
	}

	found, err := store.GetPeerByUsername("alice")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("peer: id=%d name=%s type=%s\n", found.ID, found.FirstName, found.Type)

	// Save and load a conversation → <dir>/conversations/<chatID>_<userID>.json
	conv := &storage.Conversation{
		ChatID: 100,
		UserID: 42,
		Name:   "signup",
		Step:   2,
	}
	if err := store.SaveConversation(conv); err != nil {
		log.Fatal(err)
	}

	got, err := store.LoadConversation(100, 42)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("conversation: chat=%d user=%d name=%s step=%d\n",
		got.ChatID, got.UserID, got.Name, got.Step)

	fmt.Println("done")
}
