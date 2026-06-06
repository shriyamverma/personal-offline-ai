package tools

import (
	"os"
	"path/filepath"
	"strings"
)

const WorkspaceRoot = "/workspace"

type ReadFileTool struct{}

func init() {
	Register(ReadFileTool{})
}

func (t ReadFileTool) Name() string {
	return "read_file"
}

func (t ReadFileTool) Execute(params ExecuteParams) ExecuteResult {

	cleanPath := filepath.Clean(params.Path)

	fullPath := filepath.Join(WorkspaceRoot, cleanPath)

	if !strings.HasPrefix(fullPath, WorkspaceRoot) {
		return ExecuteResult{
			Error: "invalid path",
		}
	}

	data, err := os.ReadFile(fullPath)
	if err != nil {
		return ExecuteResult{
			Error: err.Error(),
		}
	}

	return ExecuteResult{
		Output: string(data),
	}
}
