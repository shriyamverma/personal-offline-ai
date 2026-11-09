package agent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
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

type ollamaResponse struct {
	Response string `json:"response"`
}

var plannerHTTPClient = &http.Client{
	Timeout: 60 * time.Second,
}

func PlanTool(userPrompt string) (*Plan, error) {
	plannerPrompt := `
You are a tool planner for a coding assistant.

Available tools:

1. read_file
   Purpose: Read the contents of a file.
   Required parameter: path

2. list_files
   Purpose: List the contents of a directory.
   Required parameter: path

Rules:

- If the user asks to read, inspect, summarize, explain, debug,
  or analyze a specific file, use read_file.
- If the user asks to list or show files or directories,
  use list_files.
- For general programming questions that do not require
  inspecting repository files, do not use a tool.
- Never invent a file path.
- Use the path mentioned by the user.
- If the user asks about a file but does not provide a path,
  use an empty path only if you cannot determine one.
- Do not use tools for ordinary conversations.

Return STRICT JSON ONLY.

Tool request example:
{"use_tool":true,"tool":"read_file","path":"assistant-api/main.go"}

Directory listing example:
{"use_tool":true,"tool":"list_files","path":"assistant-api/internal"}

No tool example:
{"use_tool":false,"tool":"","path":""}

User request:
` + userPrompt

	req := ollamaRequest{
		Model:  "qwen2.5:3b",
		Prompt: plannerPrompt,
		Stream: false,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to encode planner request: %w", err)
	}

	httpReq, err := http.NewRequest(
		http.MethodPost,
		ollamaURL,
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create planner request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := plannerHTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("planner request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		responseBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf(
			"ollama planner returned status %d: %s",
			resp.StatusCode,
			string(responseBody),
		)
	}

	var raw ollamaResponse

	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("failed to decode planner response: %w", err)
	}

	response := cleanJSONResponse(raw.Response)

	var plan Plan

	if err := json.Unmarshal([]byte(response), &plan); err != nil {
		return nil, fmt.Errorf("planner returned invalid JSON: %w", err)
	}

	if !plan.UseTool {
		return &plan, nil
	}

	switch plan.Tool {
	case "read_file", "list_files":
	default:
		return nil, fmt.Errorf("planner selected unsupported tool: %s", plan.Tool)
	}

	if strings.TrimSpace(plan.Path) == "" {
		return nil, fmt.Errorf("planner did not provide a path")
	}

	return &plan, nil
}

func cleanJSONResponse(response string) string {
	response = strings.TrimSpace(response)

	response = strings.TrimPrefix(response, "```json")
	response = strings.TrimPrefix(response, "```")
	response = strings.TrimSuffix(response, "```")
	response = strings.TrimSpace(response)

	// Handle cases where the model adds text around the JSON.
	start := strings.Index(response, "{")
	end := strings.LastIndex(response, "}")

	if start >= 0 && end > start {
		response = response[start : end+1]
	}

	return strings.TrimSpace(response)
}
