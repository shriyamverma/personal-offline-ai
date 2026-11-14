package tools

import "strings"

type GitStatusTool struct{}
type GitDiffTool struct{}

func init()                        { Register(GitStatusTool{}); Register(GitDiffTool{}) }
func (GitStatusTool) Name() string { return "git_status" }
func (GitStatusTool) Execute(p ExecuteParams) ExecuteResult {
	return runGit(p, "status", "--short", "--branch")
}
func (GitDiffTool) Name() string { return "git_diff" }
func (GitDiffTool) Execute(p ExecuteParams) ExecuteResult {
	return runGit(p, "diff", "--no-ext-diff", "--")
}
func runGit(p ExecuteParams, args ...string) ExecuteResult {
	p.Command = "git " + strings.Join(args, " ")
	p.Approved = true
	return (ShellTool{}).Execute(p)
}
