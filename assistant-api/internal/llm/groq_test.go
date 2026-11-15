package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func testClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return &Client{BaseURL: server.URL, APIKey: "secret-test-key", HTTP: server.Client()}
}
func TestCompletionUsesGroqAuthAndChatMessages(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" || r.Header.Get("Authorization") != "Bearer secret-test-key" {
			t.Error("missing endpoint/auth")
		}
		var body map[string]json.RawMessage
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["prompt"] != nil || !strings.Contains(string(body["messages"]), "Explain semantic search") {
			t.Error("request is not a chat completion")
		}
		if body["response_format"] != nil || body["tools"] != nil {
			t.Error("unexpected tool configuration")
		}
		fmt.Fprint(w, `{"choices":[{"message":{"content":"Semantic search uses meaning."},"finish_reason":"stop"}]}`)
	})
	answer, err := client.Generate(context.Background(), DefaultModel, "Explain semantic search", nil)
	if err != nil || answer != "Semantic search uses meaning." {
		t.Fatalf("completion failed: %s %v", answer, err)
	}
}
func TestStreamingHandlesUnicodeCommentsAndDone(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, ": heartbeat\r\n\r\ndata: {\"choices\":[{\"delta\":{\"content\":\"नमस्ते\"}}]}\r\n\r\ndata: {\"choices\":[{\"delta\":{\"content\":\"!\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	})
	var chunks []string
	answer, err := client.Generate(context.Background(), DefaultModel, "Hello", func(s string) error { chunks = append(chunks, s); return nil })
	if err != nil || answer != "नमस्ते!" || len(chunks) != 2 {
		t.Fatalf("bad stream: %q %v %#v", answer, err, chunks)
	}
}
func TestStreamTruncationAndCallbackErrors(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"partial\"}}]}\n\n")
	})
	if _, err := client.Generate(context.Background(), DefaultModel, "Hello", func(string) error { return nil }); err == nil {
		t.Fatal("incomplete response accepted")
	}
	stopped := errors.New("stopped")
	if _, err := client.Generate(context.Background(), DefaultModel, "Hello", func(string) error { return stopped }); !errors.Is(err, stopped) {
		t.Fatalf("callback failure lost: %v", err)
	}
}
func TestRateLimitsDoNotLeakCredentialsOrRetryGeneration(t *testing.T) {
	calls := 0
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Retry-After", "30")
		w.WriteHeader(429)
		fmt.Fprint(w, `{"error":"secret-test-key"}`)
	})
	_, err := client.Generate(context.Background(), DefaultModel, "private prompt", nil)
	var apiErr *HTTPError
	if !errors.As(err, &apiErr) || apiErr.Status != 429 || apiErr.RetryAfter != "30" || calls != 1 || strings.Contains(err.Error(), "secret-test-key") {
		t.Fatalf("unsafe error: %v calls=%d", err, calls)
	}
}
func TestModelListExcludesNonChatModels(t *testing.T) {
	t.Setenv("GROQ_CHAT_MODELS", "")
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"data":[{"id":"openai/gpt-oss-20b","active":true},{"id":"whisper-large-v3"},{"id":"openai/gpt-oss-120b","active":false}]}`)
	})
	models, err := client.Models(context.Background())
	if err != nil || len(models) != 1 || models[0] != DefaultModel {
		t.Fatalf("bad models: %v %v", models, err)
	}
}
