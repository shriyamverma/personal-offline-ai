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
const modelName = "qwen2.5:3b"

type Plan struct {
	UseTool bool   `json:"use_tool"`
	Tool    string `json:"tool"`
	Path    string `json:"path,omitempty"`
	Command string `json:"command,omitempty"`
	Content string `json:"content,omitempty"`
}
type ollamaRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"`
	Format string `json:"format,omitempty"`
}
type ollamaResponse struct {
	Response string `json:"response"`
}

var plannerHTTPClient = &http.Client{Timeout: 60 * time.Second}

// PlanNext asks the model for one action. Repeated calls form the observe-plan-execute loop.
func PlanNext(userPrompt string, observations []string, step int) (*Plan, error) {
	var history strings.Builder
	for i, v := range observations {
		fmt.Fprintf(&history, "Observation %d:\n%s\n", i+1, v)
	}
	prompt := `You are a local coding assistant agent. Choose at most ONE next tool action.
Available tools:
- read_file(path): read a workspace file
- list_files(path): list a workspace directory
- search_files(path, command): find literal text in files under path; command is search text
- git_status(): inspect git status
- git_diff(): inspect git diff
- shell(command): only safe allowlisted commands such as "go test ./assistant-api/..." or "git log -5"
- write_file(path, content): create a NEW file only; never overwrite

Rules: Use a tool only when it materially helps. Never invent paths. Do not use shell for arbitrary commands. Never request write_file or shell unless the user explicitly asked for that operation; those tools still require approval. After observations, either choose the next useful action or set use_tool=false and provide no tool. Max planned actions is 4. Return JSON only with keys use_tool, tool, path, command, content.

User request:\n` + userPrompt + `\n\nPrevious tool observations:\n` + history.String() + fmt.Sprintf("\nThis is planning step %d of 4.", step)
	body, _ := json.Marshal(ollamaRequest{Model: modelName, Prompt: prompt, Stream: false, Format: "json"})
	req, err := http.NewRequest(http.MethodPost, ollamaURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := plannerHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("planner returned %s: %s", resp.Status, string(b))
	}
	var raw ollamaResponse
	if err = json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}
	clean := cleanJSON(raw.Response)
	var plan Plan
	if err = json.Unmarshal([]byte(clean), &plan); err != nil {
		return nil, fmt.Errorf("invalid planner JSON: %w (%s)", err, clean)
	}
	if plan.UseTool {
		switch plan.Tool {
		case "read_file", "list_files", "search_files", "git_status", "git_diff", "shell", "write_file":
		default:
			return nil, fmt.Errorf("unsupported tool: %s", plan.Tool)
		}
	}
	return &plan, nil
}
func PlanTool(prompt string) (*Plan, error) { return PlanNext(prompt, nil, 1) }
func cleanJSON(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	s = strings.TrimSpace(s)
	a := strings.Index(s, "{")
	b := strings.LastIndex(s, "}")
	if a >= 0 && b > a {
		s = s[a : b+1]
	}
	return strings.TrimSpace(s)
}
