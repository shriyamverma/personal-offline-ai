package agent

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"
)

const ollamaURL = "http://ollama:11434/api/generate"

type Plan struct {
	UseTool bool   `json:"use_tool"`
	Tool    string `json:"tool"`
	Path    string `json:"path"`
	Command string `json:"command"`
}

type ollamaRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"`
}

func PlanTool(userPrompt string) (*Plan, error) {

	plannerPrompt := `
You are a STRICT tool router for a coding assistant.

YOU MUST follow these rules:

RULE 1:
If the user mentions ANY file, code, path, or repository content,
YOU MUST use the read_file tool.

RULE 2:
If the user asks to summarize, explain, debug, or analyze code,
YOU MUST use read_file.

RULE 3:
NEVER answer from memory.

RULE 4:
If unsure, default to use_tool = true with read_file.

TOOLS:

read_file:
- reads a file
- requires "path"

list_files:
- lists directory
- requires "path"

OUTPUT FORMAT (STRICT JSON ONLY):

If tool needed:
{
  "use_tool": true,
  "tool": "read_file",
  "path": "assistant-api/main.go"
}

If no tool needed:
{
  "use_tool": false
}

USER REQUEST:
` + userPrompt

	req := ollamaRequest{
		Model:  "qwen2.5:3b",
		Prompt: plannerPrompt,
		Stream: false,
	}

	body, _ := json.Marshal(req)

	resp, err := http.Post(
		ollamaURL,
		"application/json",
		bytes.NewBuffer(body),
	)

	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	var raw struct {
		Response string `json:"response"`
	}

	json.Unmarshal(respBody, &raw)
	log.Println("🧠 Planner raw response:")
	log.Println(raw.Response)

	response := strings.TrimSpace(raw.Response)

	response = strings.TrimPrefix(response, "```json")
	response = strings.TrimPrefix(response, "```")
	response = strings.TrimSuffix(response, "```")
	response = strings.TrimSpace(response)

	var plan Plan

	err = json.Unmarshal([]byte(response), &plan)
	if err != nil {
		return nil, errors.New("planner returned invalid JSON")
	}

	return &plan, nil
}
