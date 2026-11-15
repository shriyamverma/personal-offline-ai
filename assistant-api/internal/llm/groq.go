// Package llm implements Groq chat completions without exposing credentials to the browser.
package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const DefaultModel = "openai/gpt-oss-20b"

type Client struct {
	BaseURL, APIKey string
	HTTP            *http.Client
}

func FromEnv() *Client {
	base := strings.TrimRight(os.Getenv("GROQ_BASE_URL"), "/")
	if base == "" {
		base = "https://api.groq.com/openai/v1"
	}
	return &Client{BaseURL: base, APIKey: os.Getenv("GROQ_API_KEY"), HTTP: &http.Client{Timeout: 120 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}
func (c *Client) Validate() error {
	if strings.TrimSpace(c.APIKey) == "" {
		return errors.New("GROQ_API_KEY is required")
	}
	parsed, err := url.Parse(c.BaseURL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.User != nil {
		return errors.New("GROQ_BASE_URL must be an HTTP(S) URL without credentials")
	}
	return nil
}

// HTTPError deliberately excludes upstream response bodies, prompts, and credentials.
type HTTPError struct {
	Status     int
	RetryAfter string
}

func (e *HTTPError) Error() string {
	detail := "request failed"
	switch e.Status {
	case 401, 403:
		detail = "check the API key and model permissions"
	case 404:
		detail = "model or endpoint not found"
	case 429:
		detail = "rate limit reached; retry later"
	case 400:
		detail = "request rejected; check model and request configuration"
	}
	return fmt.Sprintf("Groq HTTP %d: %s", e.Status, detail)
}
func (c *Client) request(ctx context.Context, method, path string, payload any) (*http.Response, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	var body io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, body)
	if err != nil {
		return nil, errors.New("invalid Groq request")
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, errors.New("Groq connection failed or timed out")
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return nil, &HTTPError{Status: resp.StatusCode, RetryAfter: resp.Header.Get("Retry-After")}
	}
	return resp, nil
}
func (c *Client) Generate(ctx context.Context, model, prompt string, onToken func(string) error) (string, error) {
	payload := map[string]any{"model": model, "messages": []map[string]string{{"role": "user", "content": prompt}}, "stream": onToken != nil, "max_completion_tokens": 4096}
	resp, err := c.request(ctx, http.MethodPost, "/chat/completions", payload)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if onToken == nil {
		var result struct {
			Choices []struct {
				Message struct {
					Content string `json:"content"`
					Refusal string `json:"refusal"`
				} `json:"message"`
				FinishReason string `json:"finish_reason"`
			} `json:"choices"`
		}
		if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&result); err != nil {
			return "", errors.New("invalid Groq completion response")
		}
		if len(result.Choices) == 0 {
			return "", errors.New("Groq returned no completion")
		}
		choice := result.Choices[0]
		if choice.Message.Refusal != "" {
			return "", errors.New("Groq declined the request")
		}
		if choice.FinishReason != "stop" {
			return "", errors.New("Groq completion stopped before completion")
		}
		if choice.Message.Content == "" {
			return "", errors.New("Groq returned empty content")
		}
		return choice.Message.Content, nil
	}
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 4096), 1<<20)
	var answer strings.Builder
	var data []string
	finished := false
	consume := func() (bool, error) {
		if len(data) == 0 {
			return false, nil
		}
		frame := strings.Join(data, "\n")
		data = nil
		if frame == "[DONE]" {
			if !finished {
				return false, errors.New("Groq stream ended without a completion marker")
			}
			return true, nil
		}
		var chunk struct {
			Error   json.RawMessage `json:"error"`
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
				FinishReason *string `json:"finish_reason"`
			} `json:"choices"`
		}
		if json.Unmarshal([]byte(frame), &chunk) != nil {
			return false, errors.New("invalid Groq stream frame")
		}
		if len(chunk.Error) > 0 && string(chunk.Error) != "null" {
			return false, errors.New("Groq reported a streaming error")
		}
		for _, choice := range chunk.Choices {
			if choice.Delta.Content != "" {
				answer.WriteString(choice.Delta.Content)
				if err := onToken(choice.Delta.Content); err != nil {
					return false, err
				}
			}
			if choice.FinishReason != nil {
				if *choice.FinishReason != "stop" {
					return false, errors.New("Groq response stopped before completion")
				}
				finished = true
			}
		}
		return false, nil
	}
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			done, err := consume()
			if err != nil {
				return "", err
			}
			if done {
				return answer.String(), nil
			}
		} else if strings.HasPrefix(line, "data:") {
			data = append(data, strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
		}
	}
	if scanner.Err() != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", errors.New("failed reading Groq stream")
	}
	if done, err := consume(); err != nil {
		return "", err
	} else if done {
		return answer.String(), nil
	}
	return "", errors.New("Groq stream ended before [DONE]")
}
func (c *Client) Models(ctx context.Context) ([]string, error) {
	resp, err := c.request(ctx, http.MethodGet, "/models", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var result struct {
		Data []struct {
			ID     string `json:"id"`
			Active *bool  `json:"active"`
		} `json:"data"`
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&result) != nil {
		return nil, errors.New("invalid Groq model list")
	}
	configured := os.Getenv("GROQ_CHAT_MODELS")
	if configured == "" {
		configured = DefaultModel + ",openai/gpt-oss-120b"
	}
	allowed := map[string]bool{}
	for _, model := range strings.Split(configured, ",") {
		allowed[strings.TrimSpace(model)] = true
	}
	out := []string{}
	for _, model := range result.Data {
		if allowed[model.ID] && (model.Active == nil || *model.Active) {
			out = append(out, model.ID)
		}
	}
	return out, nil
}
