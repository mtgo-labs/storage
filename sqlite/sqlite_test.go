package sqlite_test

import (
	"os"
	"testing"

	"github.com/mtgo-labs/storage"
	"github.com/mtgo-labs/storage/internal/suite"
	"github.com/mtgo-labs/storage/sqlite"
)

func TestSQLite(t *testing.T) {
	path := t.TempDir() + "/test.db"
	a, err := sqlite.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	suite.Run(t, a)
}

func TestSQLiteSessionOnly(t *testing.T) {
	path := t.TempDir() + "/session_only.db"
	a, err := sqlite.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()

	suite.RunSession(t, a)

	sess, err := a.LoadSession()
	if err != nil {
		t.Fatal(err)
	}
	if sess == nil {
		t.Fatal("expected session")
	}
	if sess.DC != 2 {
		t.Fatalf("expected DC=2, got %d", sess.DC)
	}
}

func TestSQLiteConversationsLazy(t *testing.T) {
	path := t.TempDir() + "/lazy.db"
	a, err := sqlite.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()

	var cs storage.ConversationStore = a
	suite.RunConversations(t, cs)
}

func TestSQLiteCloseRemovesWALSidecars(t *testing.T) {
	path := t.TempDir() + "/clean.db"
	a, err := sqlite.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.SaveSession(&storage.Session{SessionID: "clean", DC: 2}); err != nil {
		t.Fatal(err)
	}
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}

	for _, suffix := range []string{"-wal", "-shm"} {
		if _, err := os.Stat(path + suffix); !os.IsNotExist(err) {
			t.Fatalf("%s exists after Close: %v", path+suffix, err)
		}
	}
}
