package agent

import (
	"regexp"
	"strings"
)

type RouteDecision struct {
	UseTool bool
	Tool    string
	Path    string
	Command string
}

var pathPattern = regexp.MustCompile(`(?:[A-Za-z0-9_.-]+/)*[A-Za-z0-9_.-]+(?:\.[A-Za-z0-9_-]+)?`)

func Route(prompt string) (RouteDecision, bool) {
	p := strings.ToLower(prompt)
	if strings.Contains(p, "git status") {
		return RouteDecision{UseTool: true, Tool: "git_status"}, true
	}
	if strings.Contains(p, "git diff") {
		return RouteDecision{UseTool: true, Tool: "git_diff"}, true
	}
	if (strings.Contains(p, "list") || strings.Contains(p, "show") || strings.Contains(p, "display")) && (strings.Contains(p, "files") || strings.Contains(p, "folders") || strings.Contains(p, "directory")) {
		path := extractPath(prompt)
		if path == "" {
			path = "."
		}
		return RouteDecision{UseTool: true, Tool: "list_files", Path: path}, true
	}
	if strings.Contains(p, "read ") || strings.Contains(p, "summarize ") || strings.Contains(p, "explain ") || strings.Contains(p, "debug ") || strings.Contains(p, "inspect ") {
		path := extractPath(prompt)
		if path != "" {
			return RouteDecision{UseTool: true, Tool: "read_file", Path: path}, true
		}
	}
	return RouteDecision{}, false
}
func extractPath(prompt string) string {
	for _, m := range pathPattern.FindAllString(prompt, -1) {
		if strings.Contains(m, "/") || strings.Contains(m, ".") {
			return strings.TrimRight(m, ".,;:!?)]}\"'")
		}
	}
	return ""
}
