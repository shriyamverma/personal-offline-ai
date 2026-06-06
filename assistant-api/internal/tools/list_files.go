package tools

import (
	"os"
	"path/filepath"
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

	fullPath := filepath.Join(WorkspaceRoot, params.Path)

	if !strings.HasPrefix(fullPath, WorkspaceRoot) {
		return ExecuteResult{
			Error: "invalid path",
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
		result.WriteString(entry.Name())
		result.WriteString("\n")
	}

	return ExecuteResult{
		Output: result.String(),
	}
}