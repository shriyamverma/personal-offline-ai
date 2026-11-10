package tools

import "strings"

type GitStatusTool struct{}
type GitDiffTool struct{}

func init()                        { Register(GitStatusTool{}); Register(GitDiffTool{}) }
func (GitStatusTool) Name() string { return "git_status" }
func (GitStatusTool) Execute(ExecuteParams) ExecuteResult {
	return runGit("status", "--short", "--branch")
}
func (GitDiffTool) Name() string                        { return "git_diff" }
func (GitDiffTool) Execute(ExecuteParams) ExecuteResult { return runGit("diff", "--no-ext-diff", "--") }
func runGit(args ...string) ExecuteResult {
	p := ExecuteParams{Command: "git " + strings.Join(args, " "), Approved: true}
	return (ShellTool{}).Execute(p)
}
