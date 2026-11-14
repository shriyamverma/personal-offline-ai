package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPlannerRequestsTypedSchema(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Prompt string `json:"prompt"`
			Format struct {
				Properties map[string]struct {
					Type string `json:"type"`
				} `json:"properties"`
				Required []string `json:"required"`
			} `json:"format"`
			Options map[string]float64 `json:"options"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
			http.Error(w, "bad request", 400)
			return
		}
		if req.Format.Properties["use_tool"].Type != "boolean" {
			t.Error("planner did not constrain use_tool to boolean")
		}
		if _, ok := req.Options["temperature"]; !ok || req.Options["temperature"] != 0 {
			t.Error("planner is not deterministic")
		}
		if !strings.Contains(req.Prompt, "User request:\nInspect the repository") || strings.Contains(req.Prompt, `User request:\n`) {
			t.Error("request sections have escaped newlines")
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"response": `{"use_tool":false,"tool":""}`})
	}))
	defer server.Close()
	t.Setenv("OLLAMA_URL", server.URL)
	plan, err := PlanNext(context.Background(), "model", "Inspect the repository", nil, 1)
	if err != nil || plan.UseTool {
		t.Fatalf("unexpected plan: %#v %v", plan, err)
	}
}
func TestPlannerRejectsMalformedFlags(t *testing.T) {
	for _, raw := range []string{`{"use_tool":"true","tool":"shell","command":"pwd"}`, `{"use_tool":"false","tool":""}`, `{"tool":"shell","command":"pwd"}`, `{"use_tool":null,"tool":""}`} {
		t.Run(raw, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]string{"response": raw})
			}))
			defer server.Close()
			t.Setenv("OLLAMA_URL", server.URL)
			if _, err := PlanNext(context.Background(), "model", "Inspect the repository", nil, 1); err == nil {
				t.Fatal("malformed flag accepted")
			}
		})
	}
}
