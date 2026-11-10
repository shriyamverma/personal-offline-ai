package tools

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

type ShellTool struct{}

func init()                    { Register(ShellTool{}) }
func (ShellTool) Name() string { return "shell" }
func (ShellTool) Execute(p ExecuteParams) ExecuteResult {
	if !p.Approved {
		return ExecuteResult{Error: "shell requires explicit approval=true"}
	}
	args := strings.Fields(p.Command)
	if len(args) == 0 {
		return ExecuteResult{Error: "command is required"}
	}
	if !isAllowedCommand(args) {
		return ExecuteResult{Error: "command and arguments are not allowlisted"}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Dir = WorkspaceRoot
	if args[0] == "go" {
		cmd.Dir = WorkspaceRoot + "/assistant-api"
	}
	out, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return ExecuteResult{Error: "command timed out after 60 seconds"}
	}
	const max = 64 * 1024
	if len(out) > max {
		out = out[:max]
		out = append(out, []byte("\n...[truncated]")...)
	}
	if err != nil {
		return ExecuteResult{Output: string(out), Error: fmt.Sprintf("command failed: %v", err)}
	}
	return ExecuteResult{Output: string(out)}
}

func isAllowedCommand(args []string) bool {
	joined := strings.Join(args, " ")
	switch joined {
	case "pwd", "go version", "go test ./...", "go vet ./...",
		"git status", "git status --short --branch",
		"git diff", "git diff --no-ext-diff --",
		"git log -5", "git log -5 --oneline", "git branch", "ls":
		return true
	default:
		return false
	}
}
