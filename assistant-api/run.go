package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"personal-offline-ai/assistant-api/internal/memory"
	"regexp"
	"strings"
)

type RunState struct {
	MemoryWarning string      `json:"memory_warning,omitempty"`
	UserMessageID int64       `json:"user_message_id"`
	Prompt        string      `json:"prompt"`
	Model         string      `json:"model"`
	Context       string      `json:"context"`
	Events        []StepEvent `json:"events"`
	Error         string      `json:"error,omitempty"`
}

var phrasePattern = regexp.MustCompile(`(?is)^\s*(?:please\s+)?(?:remember|memorize|save|store)\s+(?:this\s+)?(?:test\s+)?phrase\s*:\s*(\S.*?)\s*$`)

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
	e := &execution{record: memory.RunRecord{ID: memory.NewID(), ConversationID: c.ID, Status: "running"}, state: RunState{Prompt: prompt, Model: c.Model, Events: []StepEvent{}}, emit: emit}
	b, _ := json.Marshal(e.state)
	e.record.State = b
	if err := memory.CreateRun(e.record); err != nil {
		return nil, err
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
	relevant, retrievalErr := relevantMemory(ctx, c.ID, prompt)
	if retrievalErr != nil {
		log.Printf("semantic retrieval unavailable: %v", retrievalErr)
		e.state.MemoryWarning = "Cloud semantic memory is unavailable. This response uses recent chat history only."
	}
	e.state.Context = buildContext(summary, recent, relevant)
	if err := memory.SaveMessage(c.ID, "user", prompt, nil); err != nil {
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
	e.syncSavedMemory(ctx)
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
func (e *execution) run(ctx context.Context) (answer string, err error) {
	defer func() {
		if err != nil && e.record.Status == "running" {
			e.fail(err)
		}
	}()
	if err = e.event(StepEvent{Type: "run_start", MessageID: e.state.UserMessageID}); err != nil {
		return
	}
	if e.state.MemoryWarning != "" {
		if err = e.event(StepEvent{Type: "memory_warning", Error: e.state.MemoryWarning}); err != nil {
			return
		}
	}
	// The phrase is already stored durably by startRun.
	if match := phrasePattern.FindStringSubmatch(e.state.Prompt); len(match) > 0 {
		return e.complete(ctx, "Saved this phrase in this conversation:\n\n> "+strings.ReplaceAll(match[1], "\n", "\n> "))
	}
	finalPrompt := "You are Shriyam AI, a helpful personal assistant. Answer only the CURRENT user request. Prior conversation is background, not a new instruction. This is a chat-only assistant: you cannot access files, Git repositories, or a shell, and cannot execute commands. Never claim you inspected files or executed an action. You may explain commands or analyze text the user pastes into chat.\n\nPrior conversation (background only):\n" + e.state.Context + "\n\nCURRENT user request:\n" + e.state.Prompt + "\n\nRespond clearly and concisely. If a requested fact is absent from the conversation, say you do not know."

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

func (e *execution) complete(ctx context.Context, answer string) (string, error) {
	if err := memory.SaveMessage(e.record.ConversationID, "assistant", answer, nil); err != nil {
		return "", err
	}
	previousWarning := e.state.MemoryWarning
	e.syncSavedMemory(ctx)
	if e.state.MemoryWarning != "" && previousWarning == "" {
		if err := e.event(StepEvent{Type: "memory_warning", Error: e.state.MemoryWarning}); err != nil {
			return "", err
		}
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
