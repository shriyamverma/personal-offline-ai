package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"personal-offline-ai/assistant-api/internal/llm"
	"personal-offline-ai/assistant-api/internal/memory"
	"personal-offline-ai/assistant-api/internal/vectors"
	"strings"
	"sync"
)

var conversationLocks sync.Map

func lockConversation(id string) (func(), bool) {
	value, _ := conversationLocks.LoadOrStore(id, &sync.Mutex{})
	m := value.(*sync.Mutex)
	if !m.TryLock() {
		return nil, false
	}
	return m.Unlock, true
}
func registerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/", healthHandler)
	mux.HandleFunc("/chat", chatHandler)
	mux.HandleFunc("/conversations", conversationsHandler)
	mux.HandleFunc("/conversations/", conversationHandler)
	mux.HandleFunc("/models", modelsHandler)
}
func healthHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	if !method(w, r, "GET") {
		return
	}
	writeJSON(w, 200, map[string]string{"status": "ok", "service": "assistant-api"})
}
func method(w http.ResponseWriter, r *http.Request, allowed string) bool {
	if r.Method != allowed {
		w.Header().Set("Allow", allowed)
		writeJSON(w, 405, map[string]string{"error": "method not allowed"})
		return false
	}
	return true
}
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2<<20))
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid request: " + err.Error()})
		return false
	}
	if err := d.Decode(new(any)); err != io.EOF {
		writeJSON(w, 400, map[string]string{"error": "expected one JSON object"})
		return false
	}
	return true
}
func apiError(w http.ResponseWriter, err error) {
	status := 500
	if memory.IsNotFound(err) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "The requested conversation no longer exists."})
		return
	}
	if errors.Is(err, memory.ErrConflict) {
		status = 409
	}
	writeJSON(w, status, map[string]string{"error": err.Error()})
}
func conversationsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		items, err := memory.ListConversations()
		if err != nil {
			apiError(w, err)
			return
		}
		writeJSON(w, 200, items)
	case "POST":
		var req struct {
			Title string `json:"title"`
			Model string `json:"model"`
		}
		if !decode(w, r, &req) {
			return
		}
		if req.Model == "" {
			req.Model = chatModel
		}
		if len(req.Title) > 240 || len(req.Model) > 200 {
			writeJSON(w, 400, map[string]string{"error": "title or model too long"})
			return
		}
		c, err := memory.CreateConversation(req.Title, req.Model)
		if err != nil {
			apiError(w, err)
			return
		}
		writeJSON(w, 201, c)
	default:
		w.Header().Set("Allow", "GET, POST")
		writeJSON(w, 405, map[string]string{"error": "method not allowed"})
	}
}
func conversationHandler(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/conversations/")
	if id == "" || strings.Contains(id, "/") {
		http.NotFound(w, r)
		return
	}
	c, err := memory.GetConversation(id)
	if err != nil {
		apiError(w, err)
		return
	}
	switch r.Method {
	case "GET":
		messages, err := memory.Messages(id)
		if err != nil {
			apiError(w, err)
			return
		}
		runs, err := memory.Runs(id)
		if err != nil {
			apiError(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"conversation": c, "messages": messages, "runs": runs})
	case "PATCH":
		unlock, ok := lockConversation(id)
		if !ok {
			apiError(w, memory.ErrConflict)
			return
		}
		defer unlock()
		var req struct {
			Title *string `json:"title"`
			Model *string `json:"model"`
		}
		if !decode(w, r, &req) {
			return
		}
		if req.Title != nil {
			c.Title = strings.TrimSpace(*req.Title)
		}
		if req.Model != nil {
			c.Model = strings.TrimSpace(*req.Model)
		}
		if c.Title == "" || c.Model == "" || len(c.Title) > 240 || len(c.Model) > 200 {
			writeJSON(w, 400, map[string]string{"error": "invalid title or model"})
			return
		}
		if err = memory.UpdateConversation(id, c.Title, c.Model); err != nil {
			apiError(w, err)
			return
		}
		c, _ = memory.GetConversation(id)
		writeJSON(w, 200, c)
	case "DELETE":
		unlock, ok := lockConversation(id)
		if !ok {
			apiError(w, memory.ErrConflict)
			return
		}
		defer unlock()
		client := vectors.FromEnv()
		if client.Configured() {
			store, err := memory.StoreID()
			if err != nil {
				apiError(w, err)
				return
			}
			if err := client.DeleteConversation(r.Context(), store, id); err != nil {
				writeJSON(w, 503, map[string]string{"error": "Cloud memory deletion failed. Conversation retained; retry deletion later."})
				return
			}
		}
		if err := memory.DeleteConversation(id); err != nil {
			apiError(w, err)
			return
		}
		w.WriteHeader(204)
	default:
		w.Header().Set("Allow", "GET, PATCH, DELETE")
		writeJSON(w, 405, map[string]string{"error": "method not allowed"})
	}
}
func chatHandler(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, "POST") {
		return
	}
	var req ChatRequest
	if !decode(w, r, &req) {
		return
	}
	req.Prompt = strings.TrimSpace(req.Prompt)
	if req.Prompt == "" || len(req.Prompt) > 64000 {
		writeJSON(w, 400, map[string]string{"error": "prompt must contain 1–64000 bytes"})
		return
	}
	var c memory.Conversation
	var err error
	if req.ConversationID == "" {
		if req.Model == "" {
			req.Model = chatModel
		}
		c, err = memory.CreateConversation("", req.Model)
	} else {
		c, err = memory.GetConversation(req.ConversationID)
	}
	if err != nil {
		apiError(w, err)
		return
	}
	unlock, ok := lockConversation(c.ID)
	if !ok {
		apiError(w, memory.ErrConflict)
		return
	}
	defer unlock()
	if req.Model != "" && req.Model != c.Model {
		writeJSON(w, 400, map[string]string{"error": "update the conversation model first"})
		return
	}
	e, err := startRun(r.Context(), c, req.Prompt, nil)
	if err != nil {
		apiError(w, err)
		return
	}
	respondRun(w, r, e, req.Stream)
}
func respondRun(w http.ResponseWriter, r *http.Request, e *execution, stream bool) {
	if stream {
		flusher, ok := w.(http.Flusher)
		if !ok {
			err := errors.New("streaming unsupported")
			e.fail(err)
			apiError(w, err)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("X-Accel-Buffering", "no")
		e.emit = func(event StepEvent) bool {
			b, _ := json.Marshal(event)
			if _, err := fmt.Fprintf(w, "data: %s\n\n", b); err != nil {
				return false
			}
			flusher.Flush()
			return true
		}
		// Flush immediately, including when model loading takes time.
		flusher.Flush()
		if _, err := e.run(r.Context()); err != nil {
			e.emit(StepEvent{Type: "error", Error: err.Error(), RunID: e.record.ID, ConversationID: e.record.ConversationID})
		}
		return
	}
	answer, err := e.run(r.Context())
	if err != nil {
		writeJSON(w, 502, map[string]string{"error": err.Error(), "conversation_id": e.record.ConversationID, "run_id": e.record.ID})
		return
	}
	writeJSON(w, 200, map[string]any{"response": answer, "conversation_id": e.record.ConversationID, "run_id": e.record.ID, "status": e.record.Status})
}
func modelsHandler(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, "GET") {
		return
	}
	names, err := llm.FromEnv().Models(r.Context())
	if err != nil {
		writeJSON(w, 502, map[string]string{"error": err.Error()})
		return
	}
	if len(names) == 0 {
		writeJSON(w, 502, map[string]string{"error": "No supported chat models are active. Check Groq model permissions and GROQ_CHAT_MODELS."})
		return
	}
	writeJSON(w, 200, map[string]any{"models": names, "default": chatModel, "provider": "groq", "memory_provider": "qdrant"})
}
