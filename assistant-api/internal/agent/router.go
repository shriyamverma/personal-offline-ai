package agent

import (
	"regexp"
	"strings"
)

type RouteDecision struct {
	UseTool      bool
	Tool         string
	Path         string
	Command      string
	SingleAction bool
}

var pathPattern = regexp.MustCompile(`(?:[A-Za-z0-9_.-]+/)*[A-Za-z0-9_.-]+(?:\.[A-Za-z0-9_-]+)?`)

var phrasePattern = regexp.MustCompile(`(?is)^\s*(?:please\s+)?(?:remember|memorize|save|store)\s+(?:this\s+)?(?:test\s+)?phrase\s*:\s*(\S.*?)\s*$`)
var pwdPattern = regexp.MustCompile("(?i)^\\s*(?:please\\s+)?(?:(?:run|execute)\\s+(?:(?:the\\s+)?shell\\s+)?(?:the\\s+)?(?:command\\s+)?)?[`\"']?pwd[`\"']?[.!]?\\s*$")

// Phrase payloads are conversation data, even if they contain tool names or paths.
func MemoryPhrase(prompt string) (string, bool) {
	match := phrasePattern.FindStringSubmatch(prompt)
	if len(match) == 0 {
		return "", false
	}
	return match[1], true
}
func ConversationOnly(prompt string) bool {
	if _, ok := MemoryPhrase(prompt); ok {
		return true
	}
	p := strings.ToLower(strings.TrimSpace(prompt))
	for _, prefix := range []string{"what test phrase did i give you", "what phrase did i give you", "what do you remember about me"} {
		if strings.TrimRight(p, "?.!") == prefix {
			return true
		}
	}
	return false
}

func Route(prompt string) (RouteDecision, bool) {
	if ConversationOnly(prompt) {
		return RouteDecision{}, false
	}
	if pwdPattern.MatchString(prompt) {
		return RouteDecision{UseTool: true, Tool: "shell", Command: "pwd", SingleAction: true}, true
	}
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
		candidate := strings.TrimRight(m, ".,;:!?)]}\"'")
		if strings.Contains(candidate, "/") || strings.Contains(candidate, ".") {
			return candidate
		}
	}
	return ""
}
