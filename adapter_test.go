package storage

import "testing"

func TestAdapterWrapperForwardsConversations(t *testing.T) {
	store := NewMemory()
	cs, ok := store.(ConversationStore)
	if !ok {
		t.Fatal("NewMemory storage should expose ConversationStore")
	}

	conv := &Conversation{ChatID: 100, UserID: 200, Name: "note:hello", Step: 1}
	if err := cs.SaveConversation(conv); err != nil {
		t.Fatalf("SaveConversation: %v", err)
	}

	got, err := cs.LoadConversation(100, 200)
	if err != nil {
		t.Fatalf("LoadConversation: %v", err)
	}
	if got == nil || got.Name != conv.Name || got.Step != conv.Step {
		t.Fatalf("LoadConversation = %+v", got)
	}

	if err := cs.DeleteConversation(100, 200); err != nil {
		t.Fatalf("DeleteConversation: %v", err)
	}
	got, err = cs.LoadConversation(100, 200)
	if err != nil {
		t.Fatalf("LoadConversation after delete: %v", err)
	}
	if got != nil {
		t.Fatalf("LoadConversation after delete = %+v", got)
	}
}
