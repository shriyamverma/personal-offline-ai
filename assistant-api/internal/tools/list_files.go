package tools

import (
	"os"
	"strings"
)

type ListFilesTool struct{}

func init() {
	Register(ListFilesTool{})
}

func (t ListFilesTool) Name() string {
	return "list_files"
}

func (t ListFilesTool) Execute(params ExecuteParams) ExecuteResult {
	fullPath, err := resolveWorkspacePath(params.Path)
	if err != nil {
		return ExecuteResult{
			Error: err.Error(),
		}
	}

	info, err := os.Stat(fullPath)
	if err != nil {
		return ExecuteResult{
			Error: err.Error(),
		}
	}

	if !info.IsDir() {
		return ExecuteResult{
			Error: "path is not a directory",
		}
	}

	entries, err := os.ReadDir(fullPath)
	if err != nil {
		return ExecuteResult{
			Error: err.Error(),
		}
	}

	var result strings.Builder

	for _, entry := range entries {
		name := entry.Name()

		if entry.IsDir() {
			name += "/"
		}

		result.WriteString(name)
		result.WriteString("\n")
	}

	return ExecuteResult{
		Output: result.String(),
	}
}
