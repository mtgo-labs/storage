// Package suite provides a conformance test suite for storage.Adapter
// implementations. Use it to verify that a custom adapter behaves correctly:
//
//	func TestMyAdapter(t *testing.T) {
//	    a := myadapter.Open()
//	    suite.Run(t, a)
//	}
//
// Individual sub-suites (session, peers, conversations) can also be run
// independently via RunSession, RunPeers, and RunConversations.
package suite

import (
	"testing"

	"github.com/mtgo-labs/storage"
)

// Run executes the full conformance suite against a: sessions, peers, and
// conversations (if a implements ConversationStore). It also calls Close.
func Run(t *testing.T, a storage.Adapter) {
	t.Helper()
	testSession(t, a)
	testPeers(t, a)
	if cs, ok := a.(storage.ConversationStore); ok {
		testConversations(t, cs)
	}
	if err := a.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

// RunSession tests only the SessionStore interface.
func RunSession(t *testing.T, s storage.SessionStore) {
	t.Helper()
	testSession(t, s)
}

// RunPeers tests only the PeerStore interface.
func RunPeers(t *testing.T, ps storage.PeerStore) {
	t.Helper()
	testPeers(t, ps)
}

// RunConversations tests only the ConversationStore interface.
func RunConversations(t *testing.T, cs storage.ConversationStore) {
	t.Helper()
	testConversations(t, cs)
}

func testSession(t *testing.T, s storage.SessionStore) {
	sess, err := s.LoadSession()
	if err != nil {
		t.Fatalf("LoadSession empty: %v", err)
	}
	if sess != nil {
		t.Fatalf("LoadSession empty: expected nil, got %+v", sess)
	}

	want := &storage.Session{
		DC:        2,
		APIID:     12345,
		APIHash:   "abc123",
		TestMode:  false,
		AuthKey:   []byte{1, 2, 3, 4},
		State:     []byte{5, 6},
		UserID:    999,
		IsBot:     true,
		FirstName: "TestBot",
		LastName:  "Bot",
		Username:  "testbot",
		Date:      1234567890,
		Addr:      "149.154.167.51",
		Port:      443,
	}
	if err := s.SaveSession(want); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}

	got, err := s.LoadSession()
	if err != nil {
		t.Fatalf("LoadSession: %v", err)
	}
	if got == nil {
		t.Fatal("LoadSession: expected non-nil")
	}
	if got.DC != want.DC || got.APIID != want.APIID || got.UserID != want.UserID || got.IsBot != want.IsBot || got.Username != want.Username {
		t.Fatalf("LoadSession: expected %+v, got %+v", want, got)
	}
	if len(got.AuthKey) != len(want.AuthKey) {
		t.Fatalf("LoadSession AuthKey: expected %d bytes, got %d", len(want.AuthKey), len(got.AuthKey))
	}
}

func testPeers(t *testing.T, ps storage.PeerStore) {
	_, err := ps.GetPeer(123)
	if err != nil {
		t.Fatalf("GetPeer nonexistent: %v", err)
	}

	p := &storage.Peer{
		ID:         123,
		Type:       storage.PeerTypeUser,
		AccessHash: 456789,
		Username:   "alice",
		FirstName:  "Alice",
		LastName:   "Smith",
	}
	if err := ps.SavePeer(p); err != nil {
		t.Fatalf("SavePeer: %v", err)
	}

	got, err := ps.GetPeer(123)
	if err != nil {
		t.Fatalf("GetPeer: %v", err)
	}
	if got == nil || got.ID != 123 || got.Username != "alice" {
		t.Fatalf("GetPeer: expected id=123 username=alice, got %+v", got)
	}

	byName, err := ps.GetPeerByUsername("alice")
	if err != nil {
		t.Fatalf("GetPeerByUsername: %v", err)
	}
	if byName == nil || byName.ID != 123 {
		t.Fatalf("GetPeerByUsername: expected id=123, got %+v", byName)
	}

	all, err := ps.LoadPeers()
	if err != nil {
		t.Fatalf("LoadPeers: %v", err)
	}
	if len(all) != 1 || all[0].ID != 123 {
		t.Fatalf("LoadPeers: expected 1 peer with id=123, got %+v", all)
	}

	if err := ps.DeletePeer(123); err != nil {
		t.Fatalf("DeletePeer: %v", err)
	}
	got, err = ps.GetPeer(123)
	if err != nil {
		t.Fatalf("GetPeer after delete: %v", err)
	}
	if got != nil {
		t.Fatalf("GetPeer after delete: expected nil, got %+v", got)
	}

	byName, err = ps.GetPeerByUsername("alice")
	if err != nil {
		t.Fatalf("GetPeerByUsername after delete: %v", err)
	}
	if byName != nil {
		t.Fatalf("GetPeerByUsername after delete: expected nil, got %+v", byName)
	}
}

func testConversations(t *testing.T, cs storage.ConversationStore) {
	got, err := cs.LoadConversation(100, 200)
	if err != nil {
		t.Fatalf("LoadConversation empty: %v", err)
	}
	if got != nil {
		t.Fatalf("LoadConversation empty: expected nil, got %+v", got)
	}

	conv := &storage.Conversation{
		ChatID: 100,
		UserID: 200,
		Name:   "survey",
		Step:   2,
		Data:   []byte(`{"lang":"go"}`),
	}
	if err := cs.SaveConversation(conv); err != nil {
		t.Fatalf("SaveConversation: %v", err)
	}

	got, err = cs.LoadConversation(100, 200)
	if err != nil {
		t.Fatalf("LoadConversation: %v", err)
	}
	if got == nil || got.Name != "survey" || got.Step != 2 {
		t.Fatalf("LoadConversation: expected name=survey step=2, got %+v", got)
	}
	if got.CreatedAt == 0 {
		t.Fatal("LoadConversation: expected CreatedAt to be set")
	}

	if err := cs.DeleteConversation(100, 200); err != nil {
		t.Fatalf("DeleteConversation: %v", err)
	}
	got, err = cs.LoadConversation(100, 200)
	if err != nil {
		t.Fatalf("LoadConversation after delete: %v", err)
	}
	if got != nil {
		t.Fatalf("LoadConversation after delete: expected nil, got %+v", got)
	}
}
