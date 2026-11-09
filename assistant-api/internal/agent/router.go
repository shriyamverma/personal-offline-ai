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

var (
	filePathPattern = regexp.MustCompile(
		`(?:/[A-Za-z0-9_.-]+|[A-Za-z0-9_.-]+)(?:/[A-Za-z0-9_.-]+)*(?:\.[A-Za-z0-9_-]+)`,
	)

	listIntentPattern = regexp.MustCompile(
		`\b(list|show|display)\b.*\b(files|directories|folders|contents)\b`,
	)

	fileIntentPattern = regexp.MustCompile(
		`\b(read|open|summarize|explain|debug|analyze|inspect)\b`,
	)
)

func Route(prompt string) (RouteDecision, bool) {
	lowerPrompt := strings.ToLower(prompt)

	// Directory listing requests.
	if listIntentPattern.MatchString(lowerPrompt) ||
		strings.Contains(lowerPrompt, "ls -") {

		path := extractFilePath(prompt)
		if path == "" {
			path = "."
		}

		return RouteDecision{
			UseTool: true,
			Tool:    "list_files",
			Path:    path,
		}, true
	}

	// File-related requests need an identifiable path.
	if fileIntentPattern.MatchString(lowerPrompt) {
		path := extractFilePath(prompt)

		if path != "" {
			return RouteDecision{
				UseTool: true,
				Tool:    "read_file",
				Path:    path,
			}, true
		}
	}

	// Let the LLM planner handle ambiguous requests.
	return RouteDecision{}, false
}

func extractFilePath(prompt string) string {
	matches := filePathPattern.FindAllString(prompt, -1)

	if len(matches) == 0 {
		return ""
	}

	path := matches[0]

	path = strings.TrimRight(path, ".,;:!?)]}\"'")

	return path
}
