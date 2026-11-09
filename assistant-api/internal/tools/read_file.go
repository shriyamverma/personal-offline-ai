package tools

import (
	"fmt"
	"os"
)

type ReadFileTool struct{}

func init() {
	Register(ReadFileTool{})
}

func (t ReadFileTool) Name() string {
	return "read_file"
}

func (t ReadFileTool) Execute(params ExecuteParams) ExecuteResult {
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

	if !info.Mode().IsRegular() {
		return ExecuteResult{
			Error: "path is not a regular file",
		}
	}

	// Avoid unexpectedly loading very large files into memory.
	const maxFileSize = 2 * 1024 * 1024

	if info.Size() > maxFileSize {
		return ExecuteResult{
			Error: fmt.Sprintf("file exceeds maximum allowed size of %d bytes", maxFileSize),
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
