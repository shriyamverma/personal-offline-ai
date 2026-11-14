package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"personal-offline-ai/assistant-api/internal/memory"
	"personal-offline-ai/assistant-api/internal/tools"
	"strings"
	"sync/atomic"
	"testing"
)

var probeCount atomic.Int32
var probeCommand string

type approvalProbe struct{}

func (approvalProbe) Name() string { return "approval_probe" }
func (approvalProbe) Execute(p tools.ExecuteParams) tools.ExecuteResult {
	probeCount.Add(1)
	probeCommand = p.Command
	return tools.ExecuteResult{Output: "ran approved operation"}
}
func init() { tools.Register(approvalProbe{}) }
func setup(t *testing.T) *http.ServeMux {
	t.Helper()
	if err := memory.Open(filepath.Join(t.TempDir(), "api.db")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { memory.DB.Close() })
	mux := http.NewServeMux()
	registerRoutes(mux)
	return mux
}
func request(t *testing.T, mux http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	return w
}
func mockOllama(t *testing.T, plan string) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/embed" {
			fmt.Fprint(w, `{"embeddings":[[1,0]]}`)
			return
		}
		var req struct {
			Format json.RawMessage `json:"format"`
			Stream bool            `json:"stream"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
			return
		}
		if len(req.Format) > 0 && string(req.Format) != "null" {
			_ = json.NewEncoder(w).Encode(map[string]string{"response": plan})
			return
		}
		if req.Stream {
			fmt.Fprintln(w, `{"response":"Hello ","done":false}`)
			fmt.Fprintln(w, `{"response":"locally.","done":true}`)
		} else {
			fmt.Fprint(w, `{"response":"Hello locally.","done":true}`)
		}
	}))
	t.Cleanup(server.Close)
	oldGenerate, oldEmbed := ollamaURL, embedURL
	ollamaURL = server.URL + "/api/generate"
	embedURL = server.URL + "/api/embed"
	t.Cleanup(func() { ollamaURL = oldGenerate; embedURL = oldEmbed })
	t.Setenv("OLLAMA_URL", server.URL)
}
func TestStreamingChatPersistsAndIsolates(t *testing.T) {
	mux := setup(t)
	mockOllama(t, `{"use_tool":false}`)
	a, _ := memory.CreateConversation("A", "model")
	b, _ := memory.CreateConversation("B", "model")
	if err := memory.SaveMessage(b.ID, "user", "secret in B", []float64{1, 0}); err != nil {
		t.Fatal(err)
	}
	w := request(t, mux, "POST", "/chat", fmt.Sprintf(`{"conversation_id":%q,"prompt":"Hello","stream":true}`, a.ID))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"type":"token"`) || !strings.Contains(w.Body.String(), `"type":"done"`) {
		t.Fatalf("bad stream: %d %s", w.Code, w.Body.String())
	}
	messages, _ := memory.Messages(a.ID)
	if len(messages) != 2 || messages[1].Content != "Hello locally." {
		t.Fatalf("not persisted: %v", messages)
	}
	records, _ := memory.Runs(a.ID)
	var state RunState
	_ = json.Unmarshal(records[0].State, &state)
	if strings.Contains(state.Context, "secret in B") {
		t.Fatal("other conversation in context")
	}
	w = request(t, mux, "GET", "/conversations/"+a.ID, "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), "Hello locally.") {
		t.Fatal(w.Body.String())
	}
}
func TestApprovalPreviewRejectAndNoBlanketApproval(t *testing.T) {
	mux := setup(t)
	mockOllama(t, `{"use_tool":true,"tool":"shell","command":"pwd"}`)
	c, _ := memory.CreateConversation("Approval", "model")
	w := request(t, mux, "POST", "/chat", fmt.Sprintf(`{"conversation_id":%q,"prompt":"Run pwd","approved":true}`, c.ID))
	if w.Code != 400 {
		t.Fatal("blanket approval accepted")
	}
	w = request(t, mux, "POST", "/chat", fmt.Sprintf(`{"conversation_id":%q,"prompt":"Run pwd","stream":true}`, c.ID))
	if !strings.Contains(w.Body.String(), `"type":"approval_required"`) || !strings.Contains(w.Body.String(), `"command":"pwd"`) {
		t.Fatal(w.Body.String())
	}
	runs, _ := memory.Runs(c.ID)
	if len(runs) != 1 || runs[0].Status != "awaiting_approval" {
		t.Fatal(runs)
	}
	w = request(t, mux, "POST", "/chat", fmt.Sprintf(`{"conversation_id":%q,"prompt":"another request"}`, c.ID))
	if w.Code != 409 {
		t.Fatalf("pending approval did not block chat: %d", w.Code)
	}
	w = request(t, mux, "POST", "/approvals/"+runs[0].ID, fmt.Sprintf(`{"approved":false,"approval_id":%q}`, func() string { var s RunState; _ = json.Unmarshal(runs[0].State, &s); return s.ApprovalID }()))
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	runs, _ = memory.Runs(c.ID)
	var state RunState
	_ = json.Unmarshal(runs[0].State, &state)
	if runs[0].Status != "rejected" {
		t.Fatal(runs)
	}
	for _, event := range state.Events {
		if event.Type == "tool_start" {
			t.Fatal("rejected tool ran")
		}
	}
	w = request(t, mux, "POST", "/approvals/"+runs[0].ID, `{"approved":true,"approval_id":"decision-1"}`)
	if w.Code != 409 {
		t.Fatal("rejected approval replayed")
	}
}
func TestApprovalExecutesStoredOperationOnce(t *testing.T) {
	mux := setup(t)
	mockOllama(t, `{"use_tool":false}`)
	probeCount.Store(0)
	c, _ := memory.CreateConversation("Approval", "model")
	state := RunState{Prompt: "Execute reviewed action", Model: "model", Step: 1, ApprovalID: "decision-1", Pending: &ToolRequest{Tool: "approval_probe", Command: "exact reviewed command"}}
	b, _ := json.Marshal(state)
	record := memory.RunRecord{ID: memory.NewID(), ConversationID: c.ID, Status: "awaiting_approval", State: b}
	if err := memory.CreateRun(record); err != nil {
		t.Fatal(err)
	}
	w := request(t, mux, "POST", "/approvals/"+record.ID, `{"approved":true,"command":"replacement"}`)
	if w.Code != 400 {
		t.Fatal("client could replace operation")
	}
	w = request(t, mux, "POST", "/approvals/"+record.ID, `{"approved":true,"approval_id":"decision-1","stream":true}`)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"type":"done"`) {
		t.Fatal(w.Body.String())
	}
	w = request(t, mux, "POST", "/approvals/"+record.ID, `{"approved":true,"approval_id":"decision-1"}`)
	if w.Code != 409 || probeCount.Load() != 1 || probeCommand != "exact reviewed command" {
		t.Fatalf("replay or mutated command: %d %d %s", w.Code, probeCount.Load(), probeCommand)
	}
}
func TestApprovalDoesNotAuthorizeNextAction(t *testing.T) {
	mux := setup(t)
	mockOllama(t, `{"use_tool":true,"tool":"shell","command":"pwd"}`)
	probeCount.Store(0)
	c, _ := memory.CreateConversation("Approval", "model")
	state := RunState{Prompt: "Two operations", Model: "model", Step: 1, ApprovalID: "decision-1", Pending: &ToolRequest{Tool: "approval_probe"}}
	b, _ := json.Marshal(state)
	record := memory.RunRecord{ID: memory.NewID(), ConversationID: c.ID, Status: "awaiting_approval", State: b}
	if err := memory.CreateRun(record); err != nil {
		t.Fatal(err)
	}
	w := request(t, mux, "POST", "/approvals/"+record.ID, `{"approved":true,"approval_id":"decision-1"}`)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	runs, _ := memory.Runs(c.ID)
	_ = json.Unmarshal(runs[0].State, &state)
	if runs[0].Status != "awaiting_approval" || state.Pending == nil || state.Pending.Tool != "shell" || state.Step != 2 || probeCount.Load() != 1 {
		t.Fatalf("next action inherited approval: %s %#v", runs[0].Status, state)
	}
	w = request(t, mux, "POST", "/approvals/"+record.ID, `{"approved":true,"approval_id":"decision-1"}`)
	if w.Code != 409 {
		t.Fatal("old approval consumed a later action")
	}
}

func TestPendingApprovalSurvivesDisconnectedStream(t *testing.T) {
	setup(t)
	mockOllama(t, `{"use_tool":true,"tool":"shell","command":"pwd"}`)
	c, _ := memory.CreateConversation("Pending", "model")
	e, err := startRun(context.Background(), c, "Run pwd", func(event StepEvent) bool { return event.Type != "approval_required" })
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.run(context.Background(), nil); err == nil {
		t.Fatal("expected disconnect")
	}
	runs, _ := memory.Runs(c.ID)
	if len(runs) != 1 || runs[0].Status != "awaiting_approval" {
		t.Fatalf("disconnect lost pending approval: %#v", runs)
	}
}

func TestRememberPhraseDoesNotPlanOrSearch(t *testing.T) {
	mux := setup(t)
	// Any attempted planning would fail; phrase storage must still succeed.
	mockOllama(t, `{"use_tool":"true","tool":"search_files","path":".","command":"copper mango lighthouse"}`)
	c, _ := memory.CreateConversation("Memory", "model")
	if err := memory.SaveMessage(c.ID, "assistant", "Previous git status results and a planner error", nil); err != nil {
		t.Fatal(err)
	}
	w := request(t, mux, "POST", "/chat", fmt.Sprintf(`{"conversation_id":%q,"prompt":"Remember this test phrase: copper mango lighthouse.","stream":true}`, c.ID))
	if w.Code != 200 || !strings.Contains(w.Body.String(), "Saved this phrase in this conversation") || strings.Contains(w.Body.String(), `"type":"tool_start"`) {
		t.Fatalf("memory phrase used a tool: %d %s", w.Code, w.Body.String())
	}
	messages, _ := memory.Messages(c.ID)
	if len(messages) != 3 || messages[2].Content != "Saved this phrase in this conversation:\n\n> copper mango lighthouse." {
		t.Fatalf("incorrect memory acknowledgement: %#v", messages)
	}
	runs, _ := memory.Runs(c.ID)
	if runs[0].Status != "completed" {
		t.Fatalf("memory run not completed: %#v", runs)
	}
}
func TestApprovedPwdReturnsOnlyObservedResult(t *testing.T) {
	mux := setup(t)
	// A follow-up planner call would propose an unrelated tool, and the answer
	// generator would say Hello locally. Neither may replace pwd's actual output.
	mockOllama(t, `{"use_tool":true,"tool":"git_status"}`)
	c, _ := memory.CreateConversation("Command", "model")
	_ = memory.SaveMessage(c.ID, "user", "Show git status", nil)
	_ = memory.SaveMessage(c.ID, "assistant", "There was a planning flag misconfiguration", nil)
	initial := request(t, mux, "POST", "/chat", fmt.Sprintf(`{"conversation_id":%q,"prompt":"Run the shell command pwd."}`, c.ID))
	var pending struct {
		RunID      string      `json:"run_id"`
		ApprovalID string      `json:"approval_id"`
		Action     ToolRequest `json:"action"`
	}
	if err := json.Unmarshal(initial.Body.Bytes(), &pending); err != nil {
		t.Fatal(err)
	}
	if pending.Action.Tool != "shell" || pending.Action.Command != "pwd" {
		t.Fatalf("wrong approval: %s", initial.Body.String())
	}
	resumed := request(t, mux, "POST", "/approvals/"+pending.RunID, fmt.Sprintf(`{"approved":true,"approval_id":%q}`, pending.ApprovalID))
	if resumed.Code != 200 {
		t.Fatalf("resume failed: %s", resumed.Body.String())
	}
	var result struct {
		Response string `json:"response"`
	}
	_ = json.Unmarshal(resumed.Body.Bytes(), &result)
	// On a host without /workspace pwd fails; on Docker it returns /workspace.
	// Both outcomes must be reported literally rather than inventing a git result.
	if !strings.HasPrefix(result.Response, "Command: `pwd`") || strings.Contains(result.Response, "git") || strings.Contains(result.Response, "Hello locally") || strings.Contains(result.Response, "misconfiguration") {
		t.Fatalf("ungrounded command answer: %s", result.Response)
	}
	runs, _ := memory.Runs(c.ID)
	var state RunState
	_ = json.Unmarshal(runs[0].State, &state)
	starts := 0
	for _, event := range state.Events {
		if event.Type == "tool_start" {
			starts++
			if event.Tool != "shell" {
				t.Fatal("unrelated tool ran")
			}
		}
	}
	if starts != 1 || runs[0].Status != "completed" {
		t.Fatalf("single command continued planning: %#v", state)
	}
}
func TestPlannerFailureDoesNotGenerateMisleadingAnswer(t *testing.T) {
	mux := setup(t)
	mockOllama(t, `{"use_tool":"true","tool":"git_status"}`)
	c, _ := memory.CreateConversation("Failure", "model")
	w := request(t, mux, "POST", "/chat", fmt.Sprintf(`{"conversation_id":%q,"prompt":"Inspect the repository"}`, c.ID))
	if w.Code != 502 || !strings.Contains(w.Body.String(), "tool planning failed") {
		t.Fatalf("planner error hidden: %d %s", w.Code, w.Body.String())
	}
	messages, _ := memory.Messages(c.ID)
	if len(messages) != 1 || messages[0].Role != "user" {
		t.Fatalf("planner failure saved fabricated assistant message: %#v", messages)
	}
}
