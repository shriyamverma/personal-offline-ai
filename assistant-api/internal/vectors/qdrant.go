// Package vectors uses Qdrant Cloud's server-side inference; no local embedding model is needed.
package vectors

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
)

const EmbeddingModel = "sentence-transformers/all-minilm-l6-v2"
const Dimensions = 384

type Client struct {
	Created              bool
	URL, Key, Collection string
	HTTP                 *http.Client
}

func FromEnv() *Client {
	collection := os.Getenv("QDRANT_COLLECTION")
	if collection == "" {
		collection = "shriyam_ai_memory_minilm_v1"
	}
	return &Client{URL: strings.TrimRight(os.Getenv("QDRANT_URL"), "/"), Key: os.Getenv("QDRANT_API_KEY"), Collection: collection, HTTP: &http.Client{Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}
func (c *Client) Configured() bool { return c.URL != "" }
func (c *Client) Validate() error {
	u, err := url.Parse(c.URL)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil {
		return errors.New("QDRANT_URL must be an HTTP(S) cluster URL without credentials")
	}
	if c.Key == "" {
		return errors.New("QDRANT_API_KEY is required")
	}
	if !regexp.MustCompile(`^[A-Za-z0-9_-]+$`).MatchString(c.Collection) {
		return errors.New("invalid QDRANT_COLLECTION name")
	}
	return nil
}
func (c *Client) Identity() string { return c.URL + ":" + c.Collection + ":" + EmbeddingModel }
func (c *Client) request(ctx context.Context, method, path string, payload any, out any) error {
	if err := c.Validate(); err != nil {
		return err
	}
	var body io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.URL+path, body)
	if err != nil {
		return errors.New("invalid Qdrant request")
	}
	req.Header.Set("api-key", c.Key)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New("Qdrant connection failed or timed out")
	}
	defer resp.Body.Close()
	if resp.StatusCode == 404 {
		return ErrNotFound
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("Qdrant HTTP %d; check cluster permissions, inference, and quota", resp.StatusCode)
	}
	if out != nil {
		if json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(out) != nil {
			return errors.New("invalid Qdrant response")
		}
	} else {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	}
	return nil
}

var ErrNotFound = errors.New("Qdrant collection not found")

func (c *Client) path() string { return "/collections/" + c.Collection }
func (c *Client) Ensure(ctx context.Context) error {
	var collection struct {
		Result struct {
			Config struct {
				Params struct {
					Vectors struct {
						Size     int    `json:"size"`
						Distance string `json:"distance"`
					} `json:"vectors"`
				} `json:"params"`
			} `json:"config"`
		} `json:"result"`
	}
	err := c.request(ctx, "GET", c.path(), nil, &collection)
	if errors.Is(err, ErrNotFound) {
		c.Created = true
		if err = c.request(ctx, "PUT", c.path(), map[string]any{"vectors": map[string]any{"size": Dimensions, "distance": "Cosine"}}, nil); err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else if collection.Result.Config.Params.Vectors.Size != Dimensions || !strings.EqualFold(collection.Result.Config.Params.Vectors.Distance, "Cosine") {
		return errors.New("Qdrant collection uses a different vector configuration; choose a new QDRANT_COLLECTION")
	}
	for _, field := range []string{"store_id", "conversation_id"} {
		if err = c.request(ctx, "PUT", c.path()+"/index?wait=true", map[string]string{"field_name": field, "field_schema": "keyword"}, nil); err != nil {
			return err
		}
	}
	return nil
}
func PointID(store string, id int64) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s:%d", store, id)))
	b := sum[:16]
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:])
}

type Document struct {
	ID      int64
	Content string
}
type Match struct {
	ID    int64
	Score float64
}

func (c *Client) Upsert(ctx context.Context, store, conversation string, documents []Document) error {
	if len(documents) == 0 {
		return nil
	}
	points := make([]any, 0, len(documents))
	for _, doc := range documents {
		points = append(points, map[string]any{"id": PointID(store, doc.ID), "vector": map[string]string{"text": doc.Content, "model": EmbeddingModel}, "payload": map[string]any{"store_id": store, "conversation_id": conversation, "message_id": doc.ID}})
	}
	return c.request(ctx, "PUT", c.path()+"/points?wait=true", map[string]any{"points": points}, nil)
}
func filter(store, conversation string) map[string]any {
	return map[string]any{"must": []any{map[string]any{"key": "store_id", "match": map[string]string{"value": store}}, map[string]any{"key": "conversation_id", "match": map[string]string{"value": conversation}}}}
}
func (c *Client) Query(ctx context.Context, store, conversation, text string, limit int) ([]Match, error) {
	var result struct {
		Result struct {
			Points []struct {
				Score   float64 `json:"score"`
				Payload struct {
					Store        string `json:"store_id"`
					Conversation string `json:"conversation_id"`
					ID           int64  `json:"message_id"`
				} `json:"payload"`
			} `json:"points"`
		} `json:"result"`
	}
	err := c.request(ctx, "POST", c.path()+"/points/query", map[string]any{"query": map[string]string{"text": text, "model": EmbeddingModel}, "filter": filter(store, conversation), "limit": limit, "with_payload": true, "with_vector": false}, &result)
	if err != nil {
		return nil, err
	}
	out := []Match{}
	for _, point := range result.Result.Points {
		if point.Payload.Store == store && point.Payload.Conversation == conversation && point.Payload.ID > 0 {
			out = append(out, Match{ID: point.Payload.ID, Score: point.Score})
		}
	}
	return out, nil
}
func (c *Client) DeleteConversation(ctx context.Context, store, conversation string) error {
	err := c.request(ctx, "POST", c.path()+"/points/delete?wait=true", map[string]any{"filter": filter(store, conversation)}, nil)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	return err
}
