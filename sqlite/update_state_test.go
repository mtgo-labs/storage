package sqlite_test

import (
	"path/filepath"
	"testing"

	"github.com/mtgo-labs/storage"
	"github.com/mtgo-labs/storage/sqlite"
)

func TestSQLiteUpdateStateSurvivesReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.db")
	db, err := sqlite.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	store := any(db).(storage.UpdateStateStore)
	if err := store.SaveUpdateState(&storage.UpdateState{SessionID: "s1", Pts: 11, Qts: 2, Date: 100, Seq: 5}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveChannelUpdateState(&storage.ChannelUpdateState{SessionID: "s1", ChannelID: 1001, Pts: 77}); err != nil {
		t.Fatal(err)
	}
	db.Close()

	db, err = sqlite.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store = any(db).(storage.UpdateStateStore)

	state, err := store.LoadUpdateState("s1")
	if err != nil {
		t.Fatal(err)
	}
	if state.Pts != 11 || state.Qts != 2 || state.Date != 100 || state.Seq != 5 {
		t.Fatalf("state = %+v", state)
	}
	channel, err := store.LoadChannelUpdateState("s1", 1001)
	if err != nil {
		t.Fatal(err)
	}
	if channel.Pts != 77 {
		t.Fatalf("channel state = %+v", channel)
	}
}

func TestSQLiteUpdateStateDedupAndDurableQueue(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.db")
	db, err := sqlite.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := any(db).(storage.UpdateStateStore)

	inserted, err := store.SaveUpdateDedupKey("s1", "msg:1:100")
	if err != nil || !inserted {
		t.Fatalf("first SaveUpdateDedupKey inserted=%v err=%v", inserted, err)
	}
	inserted, err = store.SaveUpdateDedupKey("s1", "msg:1:100")
	if err != nil || inserted {
		t.Fatalf("duplicate SaveUpdateDedupKey inserted=%v err=%v", inserted, err)
	}

	if err := store.EnqueueDurableUpdate(&storage.DurableUpdate{SessionID: "s1", ID: "u1", Payload: []byte{1, 2, 3}}); err != nil {
		t.Fatal(err)
	}
	items, err := store.LoadDurableUpdates("s1", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != "u1" {
		t.Fatalf("items = %+v", items)
	}

	if err := store.MarkDurableUpdateFailed("s1", "u1", 2, "timeout"); err != nil {
		t.Fatal(err)
	}
	items, _ = store.LoadDurableUpdates("s1", 10)
	if len(items) != 1 || items[0].Attempts != 2 || items[0].LastError != "timeout" {
		t.Fatalf("after mark failed = %+v", items[0])
	}

	if err := store.DeleteDurableUpdate("s1", "u1"); err != nil {
		t.Fatal(err)
	}
	items, _ = store.LoadDurableUpdates("s1", 10)
	if len(items) != 0 {
		t.Fatalf("after delete = %+v", items)
	}
}
