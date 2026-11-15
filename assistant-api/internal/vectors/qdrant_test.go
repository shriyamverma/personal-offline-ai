package vectors

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCloudInferenceAndConversationFilter(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("api-key") != "test-key" {
			t.Error("missing Qdrant authentication")
		}
		var body map[string]json.RawMessage
		_ = json.NewDecoder(r.Body).Decode(&body)
		if strings.HasSuffix(r.URL.Path, "/points") {
			var points struct {
				Points []struct {
					ID     string `json:"id"`
					Vector struct {
						Text  string `json:"text"`
						Model string `json:"model"`
					} `json:"vector"`
					Payload map[string]any `json:"payload"`
				} `json:"points"`
			}
			raw, _ := json.Marshal(body)
			_ = json.Unmarshal(raw, &points)
			if len(points.Points) != 1 || points.Points[0].ID != PointID("store", 42) || points.Points[0].Vector.Model != EmbeddingModel || points.Points[0].Vector.Text != "remember this" {
				t.Errorf("incorrect inference upsert: %+v", points)
			}
			if _, ok := points.Points[0].Payload["content"]; ok {
				t.Error("unnecessary plaintext content persisted in payload")
			}
			fmt.Fprint(w, `{"status":"ok"}`)
			return
		}
		if !strings.Contains(string(body["query"]), EmbeddingModel) || !strings.Contains(string(body["filter"]), `"store_id"`) || !strings.Contains(string(body["filter"]), `"conversation_id"`) {
			t.Error("missing inference or isolation filter")
		}
		fmt.Fprint(w, `{"result":{"points":[{"score":0.9,"payload":{"message_id":42,"store_id":"store","conversation_id":"chat"}},{"score":1,"payload":{"message_id":99,"store_id":"other","conversation_id":"chat"}}]}}`)
	}))
	defer server.Close()
	c := &Client{URL: server.URL, Key: "test-key", Collection: "test", HTTP: server.Client()}
	if err := c.Upsert(context.Background(), "store", "chat", []Document{{ID: 42, Content: "remember this"}}); err != nil {
		t.Fatal(err)
	}
	matches, err := c.Query(context.Background(), "store", "chat", "remember", 4)
	if err != nil || len(matches) != 1 || matches[0].ID != 42 {
		t.Fatalf("cross-store result not rejected: %+v %v", matches, err)
	}
	if PointID("store", 42) == PointID("other", 42) {
		t.Fatal("point IDs collide across installations")
	}
}
func TestEnsureCreatesCollectionAndFilterIndexes(t *testing.T) {
	puts := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			w.WriteHeader(404)
			return
		}
		puts = append(puts, r.URL.Path)
		fmt.Fprint(w, `{"status":"ok"}`)
	}))
	defer server.Close()
	c := &Client{URL: server.URL, Key: "key", Collection: "test", HTTP: server.Client()}
	if err := c.Ensure(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(puts) != 3 {
		t.Fatalf("collection or filter indexes missing: %v", puts)
	}
}
func TestRejectsIncompatibleExistingCollection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"result":{"config":{"params":{"vectors":{"size":768,"distance":"Cosine"}}}}}`)
	}))
	defer server.Close()
	c := &Client{URL: server.URL, Key: "key", Collection: "test", HTTP: server.Client()}
	if err := c.Ensure(context.Background()); err == nil {
		t.Fatal("old embedding dimensions accepted")
	}
}
