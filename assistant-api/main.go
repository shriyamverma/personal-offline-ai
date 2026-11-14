package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"personal-offline-ai/assistant-api/internal/memory"
	"personal-offline-ai/assistant-api/internal/tools"
)

var ollamaBase = env("OLLAMA_URL", "http://ollama:11434")
var ollamaURL = ollamaBase + "/api/generate"
var embedURL = ollamaBase + "/api/embed"
var chatModel = env("CHAT_MODEL", "qwen2.5:3b")

const embedModel = "nomic-embed-text"
const maxAgentSteps = 4

var httpClient = &http.Client{Timeout: 120 * time.Second}

type ToolRequest struct {
	Tool     string `json:"tool"`
	Path     string `json:"path,omitempty"`
	Command  string `json:"command,omitempty"`
	Content  string `json:"content,omitempty"`
	Approved bool   `json:"approved,omitempty"`
}
type ChatRequest struct {
	Prompt         string      `json:"prompt"`
	ConversationID string      `json:"conversation_id"`
	Model          string      `json:"model"`
	Tool           ToolRequest `json:"tool,omitempty"`
	Stream         bool        `json:"stream,omitempty"`
	Approved       bool        `json:"approved,omitempty"`
}
type OllamaRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"`
}
type OllamaResponse struct {
	Response string `json:"response"`
	Done     bool   `json:"done"`
}
type EmbedRequest struct {
	Model string `json:"model"`
	Input string `json:"input"`
}
type EmbedResponse struct {
	Embeddings [][]float64 `json:"embeddings"`
}
type StepEvent struct {
	ApprovalID     string       `json:"approval_id,omitempty"`
	MessageID      int64        `json:"message_id,omitempty"`
	Type           string       `json:"type"`
	Tool           string       `json:"tool,omitempty"`
	Output         string       `json:"output,omitempty"`
	Error          string       `json:"error,omitempty"`
	Response       string       `json:"response,omitempty"`
	Step           int          `json:"step,omitempty"`
	Delta          string       `json:"delta,omitempty"`
	RunID          string       `json:"run_id,omitempty"`
	ConversationID string       `json:"conversation_id,omitempty"`
	Action         *ToolRequest `json:"action,omitempty"`
}

func executeToolContext(ctx context.Context, t ToolRequest, approved bool) (string, error) {
	tool, ok := tools.Get(t.Tool)
	if !ok {
		return "", errors.New("unknown tool: " + t.Tool)
	}
	result := tool.Execute(tools.ExecuteParams{Context: ctx, Path: t.Path, Command: t.Command, Content: t.Content, Approved: approved || t.Approved})
	if result.Error != "" {
		return result.Output, errors.New(result.Error)
	}
	return result.Output, nil
}
func generate(ctx context.Context, model, prompt string, onToken func(string) error) (string, error) {
	body, err := json.Marshal(OllamaRequest{Model: model, Prompt: prompt, Stream: onToken != nil})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ollamaURL, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("ollama returned %s: %s", resp.Status, string(b))
	}
	decoder := json.NewDecoder(resp.Body)
	var answer strings.Builder
	for {
		var chunk struct {
			Response string `json:"response"`
			Done     bool   `json:"done"`
			Error    string `json:"error"`
		}
		if err = decoder.Decode(&chunk); err != nil {
			return "", fmt.Errorf("incomplete model response: %w", err)
		}
		if chunk.Error != "" {
			return "", errors.New(chunk.Error)
		}
		answer.WriteString(chunk.Response)
		if onToken != nil && chunk.Response != "" {
			if err = onToken(chunk.Response); err != nil {
				return "", err
			}
		}
		if onToken == nil || chunk.Done {
			return answer.String(), nil
		}
	}
}
func embed(ctx context.Context, input string) ([]float64, error) {
	body, _ := json.Marshal(EmbedRequest{Model: embedModel, Input: input})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, embedURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, fmt.Errorf("embedding model unavailable: %s", string(b))
	}
	var result EmbedResponse
	if err = json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	if len(result.Embeddings) == 0 {
		return nil, errors.New("empty embedding response")
	}
	return result.Embeddings[0], nil
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
	if err := memory.InitDB(); err != nil {
		log.Fatal("memory initialization failed: ", err)
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
