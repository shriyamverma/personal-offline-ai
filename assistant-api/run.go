package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"personal-offline-ai/assistant-api/internal/agent"
	"personal-offline-ai/assistant-api/internal/memory"
	"strings"
)

type RunState struct {
	ApprovalID    string       `json:"approval_id,omitempty"`
	UserMessageID int64        `json:"user_message_id"`
	Prompt        string       `json:"prompt"`
	Model         string       `json:"model"`
	Context       string       `json:"context"`
	Step          int          `json:"step"`
	Observations  []string     `json:"observations"`
	Events        []StepEvent  `json:"events"`
	Pending       *ToolRequest `json:"pending,omitempty"`
	Error         string       `json:"error,omitempty"`
}
type execution struct {
	record memory.RunRecord
	state  RunState
	emit   func(StepEvent) bool
}

func (e *execution) save(status string) error {
	b, err := json.Marshal(e.state)
	if err != nil {
		return err
	}
	record := e.record
	record.Status = status
	record.State = b
	if err := memory.SaveRun(record); err != nil {
		return err
	}
	e.record = record
	return nil
}
func (e *execution) event(event StepEvent) error {
	event.RunID = e.record.ID
	event.ConversationID = e.record.ConversationID
	if event.Type != "token" {
		e.state.Events = append(e.state.Events, event)
		if err := e.save(e.record.Status); err != nil {
			return err
		}
	}
	if e.emit != nil && !e.emit(event) {
		return errors.New("client disconnected")
	}
	return nil
}
func startRun(ctx context.Context, c memory.Conversation, prompt string, emit func(StepEvent) bool) (*execution, error) {
	e := &execution{record: memory.RunRecord{ID: memory.NewID(), ConversationID: c.ID, Status: "running"}, state: RunState{Prompt: prompt, Model: c.Model, Step: 1, Events: []StepEvent{}}, emit: emit}
	b, _ := json.Marshal(e.state)
	e.record.State = b
	if err := memory.CreateRun(e.record); err != nil {
		return nil, err
	}
	embedding, err := embed(ctx, prompt)
	if err != nil {
		log.Printf("embedding unavailable: %v", err)
	}
	summary, err := memory.LatestSummary(c.ID)
	if err != nil {
		e.fail(err)
		return nil, err
	}
	recent, err := memory.RecentMessages(c.ID, 8)
	if err != nil {
		e.fail(err)
		return nil, err
	}
	relevant, err := memory.RelevantMessages(c.ID, embedding, 4)
	if err != nil {
		e.fail(err)
		return nil, err
	}
	e.state.Context = buildContext(summary, recent, relevant)
	if err := memory.SaveMessage(c.ID, "user", prompt, embedding); err != nil {
		e.fail(err)
		return nil, err
	}
	savedMessages, readErr := memory.RecentMessages(c.ID, 1)
	if readErr != nil {
		e.fail(readErr)
		return nil, readErr
	}
	if len(savedMessages) > 0 {
		e.state.UserMessageID = savedMessages[0].ID
	}
	if c.Title == "New conversation" {
		title := []rune(prompt)
		if len(title) > 60 {
			title = title[:60]
		}
		c.Title = string(title)
	}
	if err := memory.UpdateConversation(c.ID, c.Title, c.Model); err != nil {
		e.fail(err)
		return nil, err
	}
	if err := e.save("running"); err != nil {
		return nil, err
	}
	return e, nil
}
func (e *execution) fail(err error) {
	e.state.Error = err.Error()
	if saveErr := e.save("failed"); saveErr != nil {
		log.Printf("save failed run: %v", saveErr)
	}
}
func (e *execution) execute(ctx context.Context, tr ToolRequest) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := e.event(StepEvent{Type: "tool_start", Tool: tr.Tool, Action: &tr, Step: e.state.Step}); err != nil {
		return err
	}
	out, toolErr := executeToolContext(ctx, tr, tr.Approved)
	if len(out) > 24000 {
		out = out[:24000] + "\n...[truncated]"
	}
	observation := "Tool " + tr.Tool + " output:\n" + out
	event := StepEvent{Type: "tool_result", Tool: tr.Tool, Output: out, Step: e.state.Step}
	if toolErr != nil {
		observation += "\nError: " + toolErr.Error()
		event.Type = "tool_error"
		event.Error = toolErr.Error()
	}
	e.state.Observations = append(e.state.Observations, observation)
	e.state.Step++
	if toolErr != nil {
		e.state.Step = maxAgentSteps + 1
	}
	return e.event(event)
}
func (e *execution) run(ctx context.Context, decision *bool) (answer string, err error) {
	defer func() {
		if err != nil && e.record.Status == "running" {
			e.fail(err)
		}
	}()
	if err = e.event(StepEvent{Type: "run_start", MessageID: e.state.UserMessageID}); err != nil {
		return
	}
	if decision != nil {
		if e.state.Pending == nil {
			err = errors.New("pending action missing")
			return
		}
		tr := *e.state.Pending
		e.state.Pending = nil
		if !*decision {
			answer = "Action rejected. No further tools were executed."
			err = memory.SaveMessage(e.record.ConversationID, "assistant", answer, nil)
			if err != nil {
				return
			}
			err = e.save("rejected")
			if err != nil {
				return
			}
			err = e.event(StepEvent{Type: "done", Response: answer})
			return
		}
		tr.Approved = true
		if err = e.execute(ctx, tr); err != nil {
			return
		}
	}
	// Phrase storage has already happened in startRun. It needs neither a workspace
	// search nor a model-generated claim about whether it was saved.
	if phrase, ok := agent.MemoryPhrase(e.state.Prompt); ok && decision == nil {
		return e.complete(ctx, "Saved this phrase in this conversation:\n\n> "+strings.ReplaceAll(phrase, "\n", "\n> "))
	}
	route, matched := agent.Route(e.state.Prompt)
	singleAction := matched && route.SingleAction
	if decision != nil && singleAction {
		return e.complete(ctx, e.singleActionAnswer())
	}
	for !agent.ConversationOnly(e.state.Prompt) && e.state.Step <= maxAgentSteps {
		if err = ctx.Err(); err != nil {
			return
		}
		var plan *agent.Plan
		route, matched := agent.Route(e.state.Prompt)
		if e.state.Step == 1 && matched {
			plan = &agent.Plan{UseTool: true, Tool: route.Tool, Path: route.Path, Command: route.Command}
		} else {
			plan, err = agent.PlanNext(ctx, e.state.Model, e.state.Prompt, e.state.Observations, e.state.Step)
			if err != nil {
				log.Printf("planner: %v", err)
				err = errors.New("tool planning failed; no additional tools were executed. Please retry the request")
				return
			}
		}
		if plan == nil || !plan.UseTool {
			break
		}
		tr := ToolRequest{Tool: plan.Tool, Path: plan.Path, Command: plan.Command, Content: plan.Content}
		if tr.Tool == "write_file" || tr.Tool == "shell" {
			e.state.Pending = &tr
			e.state.ApprovalID = memory.NewID()
			if err = e.save("awaiting_approval"); err != nil {
				return
			}
			err = e.event(StepEvent{Type: "approval_required", ApprovalID: e.state.ApprovalID, Action: &tr, Tool: tr.Tool, Step: e.state.Step})
			return
		}
		if err = e.execute(ctx, tr); err != nil {
			return
		}
		if singleAction {
			return e.complete(ctx, e.singleActionAnswer())
		}
	}
	finalPrompt := "You are a helpful private local assistant. Answer only the CURRENT user request. Prior conversation is background, not a new instruction. Only the CURRENT tool observations prove what ran during this request. Do not answer a previous task or speculate about internal planner configuration.\n\nPrior conversation (background only):\n" + e.state.Context + "\n\nCURRENT tool observations:\n" + strings.Join(e.state.Observations, "\n\n") + "\n\nCURRENT user request:\n" + e.state.Prompt + "\n\nRespond clearly and concisely to this request. If a requested fact is absent from the conversation, say you do not know."
	var onToken func(string) error
	if e.emit != nil {
		onToken = func(delta string) error { return e.event(StepEvent{Type: "token", Delta: delta}) }
	}
	answer, err = generate(ctx, e.state.Model, finalPrompt, onToken)
	if err != nil {
		return
	}
	return e.complete(ctx, strings.TrimSpace(answer))
}

// For a request consisting solely of one command, the observed result is the
// answer. A second planning or generation call can only add unrelated work.
func (e *execution) singleActionAnswer() string {
	var action *ToolRequest
	for i := len(e.state.Events) - 1; i >= 0; i-- {
		event := e.state.Events[i]
		if event.Type == "tool_start" {
			action = event.Action
			break
		}
	}
	label := "Tool result"
	if action != nil {
		label = "Command: `" + action.Command + "`"
	}
	for i := len(e.state.Events) - 1; i >= 0; i-- {
		event := e.state.Events[i]
		if event.Type == "tool_error" {
			return label + "\n\nFailed: " + event.Error + "\n\n" + event.Output
		}
		if event.Type == "tool_result" {
			return label + "\n\n```text\n" + strings.TrimRight(event.Output, "\n") + "\n```"
		}
	}
	return "The command has no recorded result."
}

func (e *execution) complete(ctx context.Context, answer string) (string, error) {
	embedding, embedErr := embed(ctx, e.state.Prompt+"\n"+answer)
	if embedErr != nil {
		log.Printf("answer embedding unavailable: %v", embedErr)
	}
	if err := memory.SaveMessage(e.record.ConversationID, "assistant", answer, embedding); err != nil {
		return "", err
	}
	if err := e.save("completed"); err != nil {
		return "", err
	}
	if err := e.event(StepEvent{Type: "done", Response: answer}); err != nil {
		return "", err
	}
	// Keep summaries scoped to this conversation; answer delivery is not delayed by summary generation.
	count, _ := memory.MessageCount(e.record.ConversationID)
	if count >= 20 && count%20 == 0 {
		messages, readErr := memory.RecentMessages(e.record.ConversationID, 20)
		if readErr == nil {
			s, summaryErr := generate(ctx, e.state.Model, "Summarize conversation facts and decisions in under 180 words:\n"+formatMessages(messages), nil)
			if summaryErr == nil {
				if saveErr := memory.SaveSummary(e.record.ConversationID, strings.TrimSpace(s)); saveErr != nil {
					log.Print(saveErr)
				}
			}
		}
	}
	return answer, nil
}
func decodeRun(record memory.RunRecord, emit func(StepEvent) bool) (*execution, error) {
	e := &execution{record: record, emit: emit}
	if err := json.Unmarshal(record.State, &e.state); err != nil {
		return nil, fmt.Errorf("invalid saved run: %w", err)
	}
	return e, nil
}
