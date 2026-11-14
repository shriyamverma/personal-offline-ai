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

func TestMemoryPhrasesAreNotWorkspaceRequests(t *testing.T) {
	for _, prompt := range []string{
		"Remember this test phrase: copper mango lighthouse.",
		"Remember this phrase: git status and read README.md",
		"Please remember this phrase: search_files .",
	} {
		if !ConversationOnly(prompt) {
			t.Fatalf("not conversation-only: %s", prompt)
		}
		if _, matched := Route(prompt); matched {
			t.Fatalf("memory phrase triggered a tool: %s", prompt)
		}
	}
	for _, prompt := range []string{"Read README.md", "Remember this phrase and then run pwd", "What test phrase did I give you? Also read README.md"} {
		if ConversationOnly(prompt) {
			t.Fatalf("workspace/compound request suppressed: %s", prompt)
		}
	}
}
func TestPwdIsAnExactSingleAction(t *testing.T) {
	for _, prompt := range []string{"pwd", "Run pwd", "Run the shell command pwd.", "Please execute `pwd`", "Run command \"pwd\""} {
		got, matched := Route(prompt)
		if !matched || got.Tool != "shell" || got.Command != "pwd" || !got.SingleAction {
			t.Fatalf("unexpected route for %q: %#v", prompt, got)
		}
	}
	for _, prompt := range []string{"Run pwd and then show git status", "Explain what pwd does", "Do not run pwd"} {
		got, _ := Route(prompt)
		if got.SingleAction {
			t.Fatalf("compound/nonexecution request treated as single command: %s", prompt)
		}
	}
}

func TestSentencePunctuationIsNotAFilePath(t *testing.T) {
	for _, prompt := range []string{"Explain dependency injection in one sentence.", "Explain Docker.", "Summarize our conversation.", "Remember this test phrase: copper mango lighthouse."} {
		if route, matched := Route(prompt); matched {
			t.Fatalf("sentence punctuation became path: %q %#v", prompt, route)
		}
	}
	route, matched := Route("Read assistant-api/main.go.")
	if !matched || route.Path != "assistant-api/main.go" {
		t.Fatalf("file path lost with sentence punctuation: %#v", route)
	}
}
