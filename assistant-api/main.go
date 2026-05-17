package main

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
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

	err := json.NewDecoder(r.Body).Decode(&chatReq)
	if err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	ollamaReq := OllamaRequest{
		Model:  "qwen2.5:3b",
		Prompt: chatReq.Prompt,
		Stream: false,
	}

	reqBody, err := json.Marshal(ollamaReq)
	if err != nil {
		http.Error(w, "failed to marshal request", http.StatusInternalServerError)
		return
	}

	resp, err := http.Post(
		ollamaURL,
		"application/json",
		bytes.NewBuffer(reqBody),
	)

	if err != nil {
		http.Error(w, "failed to call ollama", http.StatusInternalServerError)
		return
	}

	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		http.Error(w, "failed to read response", http.StatusInternalServerError)
		return
	}

	var ollamaResp OllamaResponse

	err = json.Unmarshal(body, &ollamaResp)
	if err != nil {
		http.Error(w, "failed to parse ollama response", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	json.NewEncoder(w).Encode(map[string]string{
		"response": ollamaResp.Response,
	})
}

func main() {
	http.HandleFunc("/", healthHandler)
	http.HandleFunc("/chat", chatHandler)

	log.Println("assistant-api running on :8000")

	err := http.ListenAndServe(":8000", nil)
	if err != nil {
		log.Fatal(err)
	}
}
