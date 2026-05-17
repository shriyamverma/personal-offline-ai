package main

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"personal-offline-ai/assistant-api/internal/memory"
)

const ollamaURL = "http://ollama:11434/api/generate"

type ChatRequest struct {
	Prompt string `json:"prompt"`
}

type OllamaRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"`
}

type OllamaResponse struct {
	Response string `json:"response"`
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("assistant-api-running"))
}

func chatHandler(w http.ResponseWriter, r *http.Request) {
	var chatReq ChatRequest

	json.NewDecoder(r.Body).Decode(&chatReq)

	// Save user message
	saveMessage("user", chatReq.Prompt)

	// Get memory
	messages := getLastMessages(10)

	// Build context
	context := ""

	for _, m := range messages {
		context += m.Role + ": " + m.Content + "\n"
	}

	ollamaReq := OllamaRequest{
		Model:  "qwen2.5:3b",
		Prompt: context + "\nassistant:",
		Stream: false,
	}

	reqBody, _ := json.Marshal(ollamaReq)

	resp, err := http.Post(
		ollamaURL,
		"application/json",
		bytes.NewBuffer(reqBody),
	)

	if err != nil {
		http.Error(w, "ollama error", 500)
		return
	}

	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	var result map[string]interface{}
	json.Unmarshal(body, &result)

	responseText := ""
	if r, ok := result["response"].(string); ok {
		responseText = r
	}

	// Save assistant response
	saveMessage("assistant", responseText)

	json.NewEncoder(w).Encode(map[string]string{
		"response": responseText,
	})
}

func saveMessage(role, content string) {
	_, err := memory.DB.Exec(
		"INSERT INTO messages(role, content) VALUES(?, ?)",
		role, content,
	)

	if err != nil {
		log.Println("DB insert error:", err)
	}
}

func getLastMessages(limit int) []memory.Message {
	rows, err := memory.DB.Query(
		"SELECT role, content FROM messages ORDER BY id DESC LIMIT ?",
		limit,
	)

	if err != nil {
		log.Println(err)
		return nil
	}
	defer rows.Close()

	var messages []memory.Message

	for rows.Next() {
		var m memory.Message
		rows.Scan(&m.Role, &m.Content)
		messages = append(messages, m)
	}

	// reverse order
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}

	return messages
}

func main() {
	memory.InitDB()

	http.HandleFunc("/", healthHandler)
	http.HandleFunc("/chat", chatHandler)

	log.Println("assistant-api running on :8000")

	err := http.ListenAndServe(":8000", nil)
	if err != nil {
		log.Fatal(err)
	}
}
