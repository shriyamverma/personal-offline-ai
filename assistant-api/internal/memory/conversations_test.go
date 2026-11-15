package memory

import (
	"database/sql"
	"encoding/json"
	"path/filepath"
	"testing"
)

func database(t *testing.T) {
	t.Helper()
	if err := Open(filepath.Join(t.TempDir(), "test.db")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { DB.Close() })
}
func TestConversationIsolationAndDeletion(t *testing.T) {
	database(t)
	a, err := CreateConversation("A", "model")
	if err != nil {
		t.Fatal(err)
	}
	b, err := CreateConversation("B", "model")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []Conversation{a, b} {
		if err := SaveMessage(c.ID, "user", c.Title, []float64{1, 0}); err != nil {
			t.Fatal(err)
		}
		if err := SaveSummary(c.ID, c.Title); err != nil {
			t.Fatal(err)
		}
	}
	recent, err := RecentMessages(a.ID, 8)
	if err != nil || len(recent) != 1 || recent[0].Content != "A" {
		t.Fatalf("recent leaked: %#v %v", recent, err)
	}
	relevant, err := RelevantMessages(b.ID, []float64{1, 0}, 4)
	if err != nil || len(relevant) != 1 || relevant[0].Content != "B" {
		t.Fatalf("retrieval leaked: %#v %v", relevant, err)
	}
	summary, err := LatestSummary(a.ID)
	if err != nil || summary != "A" {
		t.Fatalf("summary leaked: %q %v", summary, err)
	}
	r := RunRecord{ID: NewID(), ConversationID: a.ID, Status: "completed", State: json.RawMessage(`{"prompt":"hello","events":[]}`)}
	if err := CreateRun(r); err != nil {
		t.Fatal(err)
	}
	if err := DeleteConversation(a.ID); err != nil {
		t.Fatal(err)
	}
	runs, err := Runs(a.ID)
	if err != nil || len(runs) != 0 {
		t.Fatalf("orphan runs: %v %v", runs, err)
	}
	messages, err := Messages(a.ID)
	if err != nil || len(messages) != 0 {
		t.Fatal("orphan messages")
	}
	if _, err := GetConversation(b.ID); err != nil {
		t.Fatal("deleted other conversation")
	}
}
func TestRestartRetiresLegacyPendingActionsAndRunningRequests(t *testing.T) {
	database(t)
	// Reproduce the index used by databases from the former workspace mode.
	for _, q := range []string{`DROP INDEX one_active_run`, `CREATE UNIQUE INDEX one_active_run ON runs(conversation_id) WHERE status IN ('running','awaiting_approval')`} {
		if _, err := DB.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	for _, status := range []string{"running", "awaiting_approval"} {
		c, _ := CreateConversation("Existing chat", "model")
		if err := SaveMessage(c.ID, "user", "Keep my history", nil); err != nil {
			t.Fatal(err)
		}
		r := RunRecord{ID: NewID(), ConversationID: c.ID, Status: status, State: json.RawMessage(`{"prompt":"old request","events":[]}`)}
		if err := CreateRun(r); err != nil {
			t.Fatal(err)
		}
		if err := initConversations(); err != nil {
			t.Fatal(err)
		}
		runs, _ := Runs(c.ID)
		if len(runs) != 1 || runs[0].Status != "interrupted" {
			t.Fatal("old request remained active", runs)
		}
		messages, _ := Messages(c.ID)
		if len(messages) != 1 || messages[0].Content != "Keep my history" {
			t.Fatal("history changed")
		}
		next := RunRecord{ID: NewID(), ConversationID: c.ID, Status: "running", State: json.RawMessage(`{}`)}
		if err := CreateRun(next); err != nil {
			t.Fatal("new chat blocked", err)
		}
		next.ID = NewID()
		if err := CreateRun(next); err != ErrConflict {
			t.Fatal("concurrent request accepted", err)
		}
	}
}
func TestMigratesLegacyDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	old, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{`CREATE TABLE messages(id INTEGER PRIMARY KEY,role TEXT,content TEXT,created_at DATETIME DEFAULT CURRENT_TIMESTAMP)`, `INSERT INTO messages(role,content) VALUES('user','old message')`, `CREATE TABLE conversation_summaries(id INTEGER PRIMARY KEY,summary TEXT,created_at DATETIME DEFAULT CURRENT_TIMESTAMP)`, `INSERT INTO conversation_summaries(summary) VALUES('old summary')`} {
		if _, err := old.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	old.Close()
	if err := Open(path); err != nil {
		t.Fatal(err)
	}
	defer DB.Close()
	messages, err := Messages("legacy")
	if err != nil || len(messages) != 1 || messages[0].Content != "old message" {
		t.Fatalf("history lost: %v %v", messages, err)
	}
	if _, err := GetConversation("legacy"); err != nil {
		t.Fatal(err)
	}
	summary, err := LatestSummary("legacy")
	if err != nil || summary != "old summary" {
		t.Fatalf("summary lost: %q %v", summary, err)
	}
	if err := SaveMessage("legacy", "assistant", "new answer", []float64{1}); err != nil {
		t.Fatal(err)
	}
	if err := initConversations(); err != nil {
		t.Fatal("migration not idempotent:", err)
	}
}
