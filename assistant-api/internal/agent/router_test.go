package agent

import "testing"

func TestRouteReadFile(t *testing.T) {
	got, ok := Route("Read assistant-api/main.go")
	if !ok || got.Tool != "read_file" || got.Path != "assistant-api/main.go" {
		t.Fatalf("unexpected route: %#v, matched=%v", got, ok)
	}
}

func TestRouteListFiles(t *testing.T) {
	got, ok := Route("List files inside assistant-api/internal")
	if !ok || got.Tool != "list_files" || got.Path != "assistant-api/internal" {
		t.Fatalf("unexpected route: %#v, matched=%v", got, ok)
	}
}

func TestRouteGeneralQuestion(t *testing.T) {
	if _, ok := Route("What is dependency injection?"); ok {
		t.Fatal("general question should not trigger a tool")
	}
}
