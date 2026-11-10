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
	"strings"
	"time"

	"personal-offline-ai/assistant-api/internal/agent"
	"personal-offline-ai/assistant-api/internal/memory"
	"personal-offline-ai/assistant-api/internal/tools"
)

const ollamaURL = "http://ollama:11434/api/generate"
const embedURL = "http://ollama:11434/api/embed"
const chatModel = "qwen2.5:3b"
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
	Prompt   string      `json:"prompt"`
	Tool     ToolRequest `json:"tool,omitempty"`
	Stream   bool        `json:"stream,omitempty"`
	Approved bool        `json:"approved,omitempty"`
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
	Type     string `json:"type"`
	Tool     string `json:"tool,omitempty"`
	Output   string `json:"output,omitempty"`
	Error    string `json:"error,omitempty"`
	Response string `json:"response,omitempty"`
	Step     int    `json:"step,omitempty"`
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", 405)
		return
	}
	writeJSON(w, 200, map[string]string{"status": "ok", "service": "assistant-api"})
}
func chatHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	var req ChatRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2<<20)).Decode(&req); err != nil {
		http.Error(w, "invalid request: "+err.Error(), 400)
		return
	}
	req.Prompt = strings.TrimSpace(req.Prompt)
	if req.Tool.Tool != "" {
		out, err := executeTool(req.Tool, req.Approved || req.Tool.Approved)
		if err != nil {
			writeJSON(w, 400, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, 200, map[string]string{"response": out})
		return
	}
	if req.Prompt == "" {
		http.Error(w, "prompt is required", 400)
		return
	}
	if req.Stream {
		streamChat(w, r, req)
		return
	}
	response, err := runAgent(r.Context(), req.Prompt, req.Approved)
	if err != nil {
		log.Printf("chat failed: %v", err)
		http.Error(w, err.Error(), 502)
		return
	}
	writeJSON(w, 200, map[string]string{"response": response})
}
func streamChat(w http.ResponseWriter, r *http.Request, req ChatRequest) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", 500)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	send := func(v StepEvent) bool {
		b, _ := json.Marshal(v)
		if _, e := fmt.Fprintf(w, "data: %s\n\n", b); e != nil {
			return false
		}
		flusher.Flush()
		return true
	}
	response, err := runAgentWithEvents(r.Context(), req.Prompt, req.Approved, send)
	if err != nil {
		send(StepEvent{Type: "error", Error: err.Error()})
		return
	}
	send(StepEvent{Type: "done", Response: response})
}
func runAgent(ctx context.Context, prompt string, approved bool) (string, error) {
	return runAgentWithEvents(ctx, prompt, approved, nil)
}
func runAgentWithEvents(ctx context.Context, prompt string, approved bool, emit func(StepEvent) bool) (string, error) {
	queryEmbedding, _ := embed(prompt)
	summary, _ := memory.LatestSummary()
	recent, _ := memory.RecentMessages(8)
	relevant, _ := memory.RelevantMessages(queryEmbedding, 4)
	contextText := buildContext(summary, recent, relevant)
	// Save after retrieval so the current prompt is not retrieved as its own historical memory.
	_ = memory.SaveMessage("user", prompt, queryEmbedding)
	var observations []string
	var outputs []string
	decision, matched := agent.Route(prompt)
	for step := 1; step <= maxAgentSteps; step++ {
		var plan *agent.Plan
		var err error
		if step == 1 && matched {
			plan = &agent.Plan{UseTool: true, Tool: decision.Tool, Path: decision.Path, Command: decision.Command}
		} else {
			plan, err = agent.PlanNext(prompt, observations, step)
		}
		if err != nil {
			log.Printf("planner stopped at step %d: %v", step, err)
			break
		}
		if plan == nil || !plan.UseTool {
			break
		}
		tr := ToolRequest{Tool: plan.Tool, Path: plan.Path, Command: plan.Command, Content: plan.Content, Approved: approved}
		if (tr.Tool == "write_file" || tr.Tool == "shell") && !approved {
			return "This action requires explicit approval. Resubmit with approved=true after reviewing the requested operation.", nil
		}
		if emit != nil && !emit(StepEvent{Type: "tool_start", Tool: tr.Tool, Step: step}) {
			return "", ctx.Err()
		}
		out, toolErr := executeTool(tr, approved)
		if toolErr != nil {
			observation := "Tool " + tr.Tool + " failed: " + toolErr.Error()
			observations = append(observations, observation)
			outputs = append(outputs, observation)
			if emit != nil {
				emit(StepEvent{Type: "tool_error", Tool: tr.Tool, Error: toolErr.Error(), Step: step})
			}
			break
		}
		if len(out) > 24000 {
			out = out[:24000] + "\n...[truncated]"
		}
		observations = append(observations, "Tool "+tr.Tool+" output:\n"+out)
		outputs = append(outputs, "Tool: "+tr.Tool+"\n"+out)
		if emit != nil {
			emit(StepEvent{Type: "tool_result", Tool: tr.Tool, Output: out, Step: step})
		}
	}
	finalPrompt := "You are a helpful private local assistant. Use the conversation context and tool observations below. Be accurate; never claim a tool ran if no observation exists.\n\nConversation context:\n" + contextText + "\nUser request:\n" + prompt + "\n\nTool observations:\n" + strings.Join(outputs, "\n\n") + "\n\nRespond clearly and concisely."
	answer, err := generate(ctx, finalPrompt, false)
	if err != nil {
		return "", err
	}
	answer = strings.TrimSpace(answer)
	answerEmbedding, _ := embed(prompt + "\n" + answer)
	_ = memory.SaveMessage("assistant", answer, answerEmbedding)
	// Periodically refresh a compact conversation summary.
	messages, _ := memory.RecentMessages(20)
	messageCount, _ := memory.MessageCount()
	if messageCount >= 20 && messageCount%20 == 0 {
		if s, e := generate(ctx, "Summarize the following conversation facts and decisions for future context in under 180 words:\n"+formatMessages(messages), false); e == nil {
			_ = memory.SaveSummary(strings.TrimSpace(s))
		}
	}
	return answer, nil
}
func executeTool(t ToolRequest, approved bool) (string, error) {
	tool, ok := tools.Get(t.Tool)
	if !ok {
		return "", errors.New("unknown tool: " + t.Tool)
	}
	result := tool.Execute(tools.ExecuteParams{Path: t.Path, Command: t.Command, Content: t.Content, Approved: approved || t.Approved})
	if result.Error != "" {
		return result.Output, errors.New(result.Error)
	}
	return result.Output, nil
}
func generate(ctx context.Context, prompt string, stream bool) (string, error) {
	body, err := json.Marshal(OllamaRequest{Model: chatModel, Prompt: prompt, Stream: stream})
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
	var result OllamaResponse
	if err = json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	return result.Response, nil
}
func embed(input string) ([]float64, error) {
	body, _ := json.Marshal(EmbedRequest{Model: embedModel, Input: input})
	req, err := http.NewRequest(http.MethodPost, embedURL, bytes.NewReader(body))
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
	mux.HandleFunc("/", healthHandler)
	mux.HandleFunc("/chat", chatHandler)
	server := &http.Server{Addr: ":8000", Handler: mux, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 420 * time.Second, IdleTimeout: 60 * time.Second}
	log.Println("assistant-api running on :8000")
	log.Fatal(server.ListenAndServe())
}
