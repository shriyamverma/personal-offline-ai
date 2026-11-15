package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"personal-offline-ai/assistant-api/internal/memory"
	"personal-offline-ai/assistant-api/internal/vectors"
	"strings"
	"testing"
)

func configureQdrantTest(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	t.Setenv("QDRANT_URL", server.URL)
	t.Setenv("QDRANT_API_KEY", "qdrant-test-key")
	t.Setenv("QDRANT_COLLECTION", "test")
}
func TestIndexRetryKeepsUnsentMessages(t *testing.T) {
	setup(t)
	conversation, _ := memory.CreateConversation("Retry", "model")
	_ = memory.SaveMessage(conversation.ID, "user", "durable memory", nil)
	unavailable := true
	configureQdrantTest(t, func(w http.ResponseWriter, r *http.Request) {
		if unavailable {
			w.WriteHeader(503)
			return
		}
		fmt.Fprint(w, `{"status":"ok"}`)
	})
	if err := syncMemory(context.Background(), conversation.ID); err == nil {
		t.Fatal("unavailable Qdrant accepted")
	}
	pending, _ := memory.UnindexedMessages(conversation.ID, vectors.FromEnv().Identity(), 32)
	if len(pending) != 1 {
		t.Fatal("failed index marked complete")
	}
	unavailable = false
	if err := syncMemory(context.Background(), conversation.ID); err != nil {
		t.Fatal(err)
	}
	pending, _ = memory.UnindexedMessages(conversation.ID, vectors.FromEnv().Identity(), 32)
	if len(pending) != 0 {
		t.Fatal("successful index was not checkpointed")
	}
}
func TestQdrantMatchesAreHydratedFromCorrectConversation(t *testing.T) {
	setup(t)
	a, _ := memory.CreateConversation("A", "model")
	b, _ := memory.CreateConversation("B", "model")
	_ = memory.SaveMessage(a.ID, "user", "fact in A", nil)
	_ = memory.SaveMessage(b.ID, "user", "secret in B", nil)
	am, _ := memory.Messages(a.ID)
	bm, _ := memory.Messages(b.ID)
	store, _ := memory.StoreID()
	configureQdrantTest(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/query") {
			fmt.Fprint(w, `{"status":"ok"}`)
			return
		}
		var query struct {
			Filter struct {
				Must []struct {
					Key   string `json:"key"`
					Match struct {
						Value string `json:"value"`
					} `json:"match"`
				} `json:"must"`
			} `json:"filter"`
		}
		_ = json.NewDecoder(r.Body).Decode(&query)
		if len(query.Filter.Must) != 2 {
			t.Error("missing scope filters")
		}
		// Even a malformed remote result claiming chat A cannot expose a chat B row.
		_ = json.NewEncoder(w).Encode(map[string]any{"result": map[string]any{"points": []any{map[string]any{"score": .9, "payload": map[string]any{"store_id": store, "conversation_id": a.ID, "message_id": am[0].ID}}, map[string]any{"score": 1, "payload": map[string]any{"store_id": store, "conversation_id": a.ID, "message_id": bm[0].ID}}}}})
	})
	matches, err := relevantMemory(context.Background(), a.ID, "fact")
	if err != nil || len(matches) != 1 || matches[0].Content != "fact in A" {
		t.Fatalf("cross-conversation retrieval: %+v %v", matches, err)
	}
}
func TestCloudOutageWarnsButRetainsChat(t *testing.T) {
	mux := setup(t)
	mockGroq(t)
	c, _ := memory.CreateConversation("Fallback", "model")
	configureQdrantTest(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(503) })
	w := request(t, mux, "POST", "/chat", fmt.Sprintf(`{"conversation_id":%q,"prompt":"Hello","stream":true}`, c.ID))
	if !strings.Contains(w.Body.String(), `"type":"memory_warning"`) || !strings.Contains(w.Body.String(), `"type":"done"`) {
		t.Fatal(w.Body.String())
	}
	messages, _ := memory.Messages(c.ID)
	if len(messages) != 2 {
		t.Fatal("cloud failure lost chat history")
	}
	pending, _ := memory.UnindexedMessages(c.ID, vectors.FromEnv().Identity(), 32)
	if len(pending) != 2 {
		t.Fatal("cloud failure lost indexing retries")
	}
}
func TestConversationDeletionRequiresCloudCleanup(t *testing.T) {
	mux := setup(t)
	c, _ := memory.CreateConversation("Delete", "model")
	_ = memory.SaveMessage(c.ID, "user", "private fact", nil)
	unavailable := true
	deleted := false
	configureQdrantTest(t, func(w http.ResponseWriter, r *http.Request) {
		if unavailable {
			w.WriteHeader(503)
			return
		}
		if !strings.HasSuffix(r.URL.Path, "/delete") {
			t.Error("unexpected deletion endpoint")
		}
		deleted = true
		fmt.Fprint(w, `{"status":"ok"}`)
	})
	w := request(t, mux, "DELETE", "/conversations/"+c.ID, "")
	if w.Code != 503 {
		t.Fatal("cloud deletion failure hidden")
	}
	if _, err := memory.GetConversation(c.ID); err != nil {
		t.Fatal("conversation lost before cloud cleanup")
	}
	unavailable = false
	w = request(t, mux, "DELETE", "/conversations/"+c.ID, "")
	if w.Code != 204 || !deleted {
		t.Fatalf("deletion failed: %d %s", w.Code, w.Body.String())
	}
	if _, err := memory.GetConversation(c.ID); !memory.IsNotFound(err) {
		t.Fatal("conversation retained after successful deletion")
	}
}
