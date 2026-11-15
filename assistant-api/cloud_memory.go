package main

import (
	"context"
	"log"
	"personal-offline-ai/assistant-api/internal/memory"
	"personal-offline-ai/assistant-api/internal/vectors"
)

func syncMemory(ctx context.Context, conversation string) error {
	client := vectors.FromEnv()
	if !client.Configured() {
		return nil
	}
	store, err := memory.StoreID()
	if err != nil {
		return err
	}
	messages, err := memory.UnindexedMessages(conversation, client.Identity(), 32)
	if err != nil {
		return err
	}
	documents := make([]vectors.Document, 0, len(messages))
	ids := make([]int64, 0, len(messages))
	for _, m := range messages {
		documents = append(documents, vectors.Document{ID: m.ID, Content: m.Content})
		ids = append(ids, m.ID)
	}
	if err := client.Upsert(ctx, store, conversation, documents); err != nil {
		return err
	}
	return memory.MarkIndexed(client.Identity(), ids)
}
func relevantMemory(ctx context.Context, conversation, prompt string) ([]memory.ScoredMessage, error) {
	client := vectors.FromEnv()
	if !client.Configured() {
		return nil, nil
	}
	if err := syncMemory(ctx, conversation); err != nil {
		return nil, err
	}
	store, err := memory.StoreID()
	if err != nil {
		return nil, err
	}
	matches, err := client.Query(ctx, store, conversation, prompt, 4)
	if err != nil {
		return nil, err
	}
	out := []memory.ScoredMessage{}
	seen := map[int64]bool{}
	for _, match := range matches {
		if seen[match.ID] {
			continue
		}
		seen[match.ID] = true
		message, err := memory.MessageInConversation(conversation, match.ID)
		if memory.IsNotFound(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		out = append(out, memory.ScoredMessage{Message: message, Score: match.Score})
	}
	return out, nil
}
func (e *execution) syncSavedMemory(ctx context.Context) {
	if err := syncMemory(ctx, e.record.ConversationID); err != nil {
		log.Printf("semantic memory indexing deferred: %v", err)
		e.state.MemoryWarning = "Cloud semantic memory is unavailable. Recent chat history is still available; indexing will retry on a later request."
	}
}
