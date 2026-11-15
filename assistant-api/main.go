package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"personal-offline-ai/assistant-api/internal/llm"
	"personal-offline-ai/assistant-api/internal/memory"
	"personal-offline-ai/assistant-api/internal/vectors"
)

var chatModel = env("CHAT_MODEL", llm.DefaultModel)

type ChatRequest struct {
	Prompt         string `json:"prompt"`
	ConversationID string `json:"conversation_id"`
	Model          string `json:"model"`
	Stream         bool   `json:"stream,omitempty"`
}
type StepEvent struct {
	MessageID      int64  `json:"message_id,omitempty"`
	Type           string `json:"type"`
	Error          string `json:"error,omitempty"`
	Response       string `json:"response,omitempty"`
	Delta          string `json:"delta,omitempty"`
	RunID          string `json:"run_id,omitempty"`
	ConversationID string `json:"conversation_id,omitempty"`
}

func generate(ctx context.Context, model, prompt string, onToken func(string) error) (string, error) {
	return llm.FromEnv().Generate(ctx, model, prompt, onToken)
}
func buildContext(summary string, recent []memory.Message, relevant []memory.ScoredMessage) string {
	var b strings.Builder
	if summary != "" {
		b.WriteString("Long-term summary:\n" + summary + "\n")
	}
	b.WriteString("Recent messages:\n")
	for _, m := range recent {
		fmt.Fprintf(&b, "%s: %s\n", m.Role, m.Content)
	}
	if len(relevant) > 0 {
		b.WriteString("Relevant older messages:\n")
		for _, m := range relevant {
			fmt.Fprintf(&b, "%s: %s\n", m.Role, m.Content)
		}
	}
	return b.String()
}
func formatMessages(ms []memory.Message) string {
	var b strings.Builder
	for _, m := range ms {
		fmt.Fprintf(&b, "%s: %s\n", m.Role, m.Content)
	}
	return b.String()
}
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func main() {
	if err := llm.FromEnv().Validate(); err != nil {
		log.Fatal(err)
	}
	if err := vectors.FromEnv().Validate(); err != nil {
		log.Fatal(err)
	}
	if err := memory.InitDB(); err != nil {
		log.Fatal("memory initialization failed: ", err)
	}
	defer memory.DB.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	vectorClient := vectors.FromEnv()
	if err := vectorClient.Ensure(ctx); err != nil {
		cancel()
		log.Fatal("cloud memory initialization failed: ", err)
	}
	cancel()
	if vectorClient.Created {
		if err := memory.ResetCloudIndex(vectorClient.Identity()); err != nil {
			log.Fatal("cloud index reset failed: ", err)
		}
	}
	mux := http.NewServeMux()
	registerRoutes(mux)
	server := &http.Server{Addr: ":8000", Handler: mux, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 900 * time.Second, IdleTimeout: 60 * time.Second}
	log.Println("assistant-api running on :8000")
	log.Fatal(server.ListenAndServe())
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return strings.TrimRight(value, "/")
	}
	return fallback
}
