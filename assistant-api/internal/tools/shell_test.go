package tools

import "testing"

func TestShellRequiresApproval(t *testing.T) {
	got := (ShellTool{}).Execute(ExecuteParams{Command: "pwd"})
	if got.Error == "" {
		t.Fatal("expected approval error")
	}
}

func TestShellRejectsUnlistedCommand(t *testing.T) {
	got := (ShellTool{}).Execute(ExecuteParams{Command: "rm -rf /tmp/x", Approved: true})
	if got.Error == "" {
		t.Fatal("expected allowlist error")
	}
}

func TestWriteFileRequiresApproval(t *testing.T) {
	got := (WriteFileTool{}).Execute(ExecuteParams{Path: "new-file.txt", Content: "hello"})
	if got.Error == "" {
		t.Fatal("expected approval error")
	}
}

func TestShellRejectsDangerousGoFlags(t *testing.T) {
	got := (ShellTool{}).Execute(ExecuteParams{Command: "go test -exec=/bin/sh ./...", Approved: true})
	if got.Error == "" {
		t.Fatal("expected strict argument allowlist error")
	}
}
