package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"personal-offline-ai/assistant-api/internal/agent"
	"personal-offline-ai/assistant-api/internal/memory"
	"personal-offline-ai/assistant-api/internal/tools"
	"strings"
)

const ollamaURL = "http://ollama:11434/api/generate"

type ToolRequest struct {
	Tool string `json:"tool"`
	Path string `json:"path,omitempty"`
	Cmd  string `json:"command,omitempty"`
}

type ChatRequest struct {
	Prompt string      `json:"prompt"`
	Tool   ToolRequest `json:"tool"`
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

	err := json.NewDecoder(r.Body).Decode(&chatReq)
	if err != nil {
		log.Println("❌ JSON decode error:", err)
		http.Error(w, "invalid request", 400)
		return
	}

	log.Printf("📥 REQUEST: %+v\n", chatReq)

	var toolOutput string
	var toolUsed bool

	// =====================================================
	// 1. DIRECT TOOL CALL (HIGHEST PRIORITY)
	// =====================================================
	if strings.TrimSpace(chatReq.Tool.Tool) != "" {

		log.Println("🚀 DIRECT TOOL:", chatReq.Tool.Tool)

		output, err := executeTool(chatReq.Tool)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}

		// ⚠️ DIRECT TOOL MUST EXIT IMMEDIATELY
		json.NewEncoder(w).Encode(map[string]string{
			"response": output,
		})
		return
	} else {

		// =====================================================
		// 2. HYBRID ROUTER (FAST PATH)
		// =====================================================
		decision, ok := agent.Route(chatReq.Prompt)

		if ok && decision.UseTool {

			log.Println("⚡ ROUTER TOOL:", decision.Tool)

			output, err := executeTool(ToolRequest{
				Tool: decision.Tool,
				Path: decision.Path,
				Cmd:  decision.Command,
			})

			if err != nil {
				http.Error(w, err.Error(), 500)
				return
			}

			toolOutput = output
			toolUsed = true

		} else {

			// =====================================================
			// 3. PLANNER (LLM FALLBACK)
			// =====================================================
			plan, err := agent.PlanTool(chatReq.Prompt)

			if err == nil && plan != nil && plan.UseTool && plan.Tool != "" {

				log.Println("🤖 PLANNER TOOL:", plan.Tool)

				output, err := executeTool(ToolRequest{
					Tool: plan.Tool,
					Path: plan.Path,
					Cmd:  plan.Command,
				})

				if err != nil {
					http.Error(w, err.Error(), 500)
					return
				}

				toolOutput = output
				toolUsed = true
			}
		}
	}

	// =====================================================
	// 4. TOOL-AWARE LLM RESPONSE
	// =====================================================
	if toolUsed {

		finalPrompt := `
You are a helpful assistant.

User request:
` + chatReq.Prompt + `

Tool output:
` + toolOutput + `

Now answer the user clearly and concisely.
`

		ollamaReq := OllamaRequest{
			Model:  "qwen2.5:3b",
			Prompt: finalPrompt,
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

		saveMessage("assistant", responseText)

		json.NewEncoder(w).Encode(map[string]string{
			"response": responseText,
		})
		return
	}

	// =====================================================
	// 5. NORMAL CHAT
	// =====================================================
	isToolTask := strings.TrimSpace(chatReq.Tool.Tool) != "" || strings.Contains(chatReq.Prompt, "read") || strings.Contains(chatReq.Prompt, "summarize")

	var messages []memory.Message

	if !isToolTask {
		saveMessage("user", chatReq.Prompt)
		messages = getLastMessages(10)
	}

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

	saveMessage("assistant", responseText)

	json.NewEncoder(w).Encode(map[string]string{
		"response": responseText,
	})
}

func executeTool(t ToolRequest) (string, error) {

	tool, exists := tools.Registry[t.Tool]
	if !exists {
		return "", errors.New("unknown tool: " + t.Tool)
	}

	result := tool.Execute(tools.ExecuteParams{
		Path:    t.Path,
		Command: t.Cmd,
	})

	if result.Error != "" {
		return "", errors.New(result.Error)
	}

	return result.Output, nil
}

// =========================
// MEMORY
// =========================

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

	log.Fatal(http.ListenAndServe(":8000", nil))
}
