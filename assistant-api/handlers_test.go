package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"personal-offline-ai/assistant-api/internal/memory"
	"strings"
	"sync/atomic"
	"testing"
)

func setup(t *testing.T) *http.ServeMux {
	t.Helper()
	t.Setenv("QDRANT_URL", "")
	if err := memory.Open(filepath.Join(t.TempDir(), "api.db")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { memory.DB.Close() })
	mux := http.NewServeMux()
	registerRoutes(mux)
	return mux
}
func request(t *testing.T, mux http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	return w
}
func mockGroq(t *testing.T) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Stream bool `json:"stream"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
			return
		}
		if req.Stream {
			fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Hello \"}}]}\n\n")
			fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"locally.\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
		} else {
			fmt.Fprint(w, `{"choices":[{"message":{"content":"Hello locally."},"finish_reason":"stop"}]}`)
		}
	}))
	t.Cleanup(server.Close)
	t.Setenv("GROQ_API_KEY", "test-key")
	t.Setenv("GROQ_BASE_URL", server.URL)
}
func TestStreamingChatPersistsAndIsolates(t *testing.T) {
	mux := setup(t)
	mockGroq(t)
	a, _ := memory.CreateConversation("A", "model")
	b, _ := memory.CreateConversation("B", "model")
	if err := memory.SaveMessage(b.ID, "user", "secret in B", []float64{1, 0}); err != nil {
		t.Fatal(err)
	}
	w := request(t, mux, "POST", "/chat", fmt.Sprintf(`{"conversation_id":%q,"prompt":"Hello","stream":true}`, a.ID))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"type":"token"`) || !strings.Contains(w.Body.String(), `"type":"done"`) {
		t.Fatalf("bad stream: %d %s", w.Code, w.Body.String())
	}
	messages, _ := memory.Messages(a.ID)
	if len(messages) != 2 || messages[1].Content != "Hello locally." {
		t.Fatalf("not persisted: %v", messages)
	}
	records, _ := memory.Runs(a.ID)
	var state RunState
	_ = json.Unmarshal(records[0].State, &state)
	if strings.Contains(state.Context, "secret in B") {
		t.Fatal("other conversation in context")
	}
	w = request(t, mux, "GET", "/conversations/"+a.ID, "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), "Hello locally.") {
		t.Fatal(w.Body.String())
	}
}
func TestRememberPhrasePersistsWithoutGeneration(t *testing.T) {
	mux := setup(t)
	t.Setenv("GROQ_API_KEY", "") // Saving a phrase needs no generation call.
	c, _ := memory.CreateConversation("Memory", "model")
	if err := memory.SaveMessage(c.ID, "assistant", "Previous git status results and a planner error", nil); err != nil {
		t.Fatal(err)
	}
	w := request(t, mux, "POST", "/chat", fmt.Sprintf(`{"conversation_id":%q,"prompt":"Remember this test phrase: copper mango lighthouse.","stream":true}`, c.ID))
	if w.Code != 200 || !strings.Contains(w.Body.String(), "Saved this phrase in this conversation") || strings.Contains(w.Body.String(), `"type":"tool_start"`) {
		t.Fatalf("memory phrase used a tool: %d %s", w.Code, w.Body.String())
	}
	messages, _ := memory.Messages(c.ID)
	if len(messages) != 3 || messages[2].Content != "Saved this phrase in this conversation:\n\n> copper mango lighthouse." {
		t.Fatalf("incorrect memory acknowledgement: %#v", messages)
	}
	runs, _ := memory.Runs(c.ID)
	if runs[0].Status != "completed" {
		t.Fatalf("memory run not completed: %#v", runs)
	}
}
func TestMissingConversationReturnsFriendlyNotFound(t *testing.T) {
	mux := setup(t)
	for _, method := range []string{"GET", "PATCH", "DELETE"} {
		w := request(t, mux, method, "/conversations/old-offline-conversation", "")
		if w.Code != 404 || strings.Contains(w.Body.String(), "sql:") || !strings.Contains(w.Body.String(), "no longer exists") {
			t.Fatalf("raw SQL error exposed: %d %s", w.Code, w.Body.String())
		}
	}
}

func TestRemovedToolRequestsAreRejected(t *testing.T) {
	mux := setup(t)
	for _, body := range []string{`{"prompt":"hello","tool":{"tool":"shell","command":"pwd"}}`, `{"prompt":"hello","approved":true}`} {
		w := request(t, mux, "POST", "/chat", body)
		if w.Code != 400 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	for _, path := range []string{"/approvals/old-run", "/capabilities"} {
		w := request(t, mux, "POST", path, `{"approved":true}`)
		if w.Code != 404 {
			t.Fatal("removed endpoint still exists", path, w.Code)
		}
	}
}

func TestChatRequestsOnlyGenerateAnswers(t *testing.T) {
	mux := setup(t)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var req struct {
			Format   json.RawMessage `json:"response_format"`
			Tools    json.RawMessage `json:"tools"`
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
		}
		if len(req.Format) > 0 || len(req.Tools) > 0 {
			t.Error("unexpected tool planning")
		}
		found := false
		for _, message := range req.Messages {
			if strings.Contains(message.Content, "chat-only assistant") {
				found = true
			}
		}
		if !found {
			t.Error("missing chat-only model instruction")
		}
		fmt.Fprint(w, `{"choices":[{"message":{"content":"I can explain commands you can run yourself."},"finish_reason":"stop"}]}`)
	}))
	defer server.Close()
	t.Setenv("GROQ_API_KEY", "test-key")
	t.Setenv("GROQ_BASE_URL", server.URL)
	for _, prompt := range []string{"Run pwd", "Inspect my project and explain its architecture"} {
		w := request(t, mux, "POST", "/chat", fmt.Sprintf(`{"prompt":%q}`, prompt))
		if w.Code != 200 || !strings.Contains(w.Body.String(), `"status":"completed"`) {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	if calls.Load() != 2 {
		t.Fatalf("unexpected model calls: %d", calls.Load())
	}
}

func TestDisconnectedStreamIsMarkedFailed(t *testing.T) {
	setup(t)
	c, _ := memory.CreateConversation("Interrupted stream", "model")
	e, err := startRun(context.Background(), c, "Hello", func(StepEvent) bool { return false })
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.run(context.Background()); err == nil {
		t.Fatal("disconnect was ignored")
	}
	records, _ := memory.Runs(c.ID)
	if len(records) != 1 || records[0].Status != "failed" {
		t.Fatal("disconnected request remained active", records)
	}
}

func TestGenerationFailureDoesNotSaveInventedAnswer(t *testing.T) {
	mux := setup(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(429)
		fmt.Fprint(w, `{"error":"private-provider-detail secret-key"}`)
	}))
	defer server.Close()
	t.Setenv("GROQ_API_KEY", "secret-key")
	t.Setenv("GROQ_BASE_URL", server.URL)
	c, _ := memory.CreateConversation("Rate limit", "model")
	w := request(t, mux, "POST", "/chat", fmt.Sprintf(`{"conversation_id":%q,"prompt":"Hello"}`, c.ID))
	if w.Code != 502 || !strings.Contains(w.Body.String(), "rate limit") || strings.Contains(w.Body.String(), "secret-key") || strings.Contains(w.Body.String(), "private-provider-detail") {
		t.Fatal(w.Code, w.Body.String())
	}
	messages, _ := memory.Messages(c.ID)
	runs, _ := memory.Runs(c.ID)
	if len(messages) != 1 || messages[0].Role != "user" || len(runs) != 1 || runs[0].Status != "failed" {
		t.Fatal("failed generation left incorrect state", messages, runs)
	}
}
