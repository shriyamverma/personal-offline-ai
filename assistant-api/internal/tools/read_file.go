package tools

import (
	"fmt"
	"os"
)

type ReadFileTool struct{}

func init()                       { Register(ReadFileTool{}) }
func (ReadFileTool) Name() string { return "read_file" }
func (ReadFileTool) Execute(p ExecuteParams) ExecuteResult {
	path, err := resolveWorkspacePath(p.Path)
	if err != nil {
		return ExecuteResult{Error: err.Error()}
	}
	info, err := os.Stat(path)
	if err != nil {
		return ExecuteResult{Error: err.Error()}
	}
	if !info.Mode().IsRegular() {
		return ExecuteResult{Error: "path is not a regular file"}
	}
	const maxSize = 2 << 20
	if info.Size() > maxSize {
		return ExecuteResult{Error: fmt.Sprintf("file exceeds %d-byte limit", maxSize)}
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return ExecuteResult{Error: err.Error()}
	}
	return ExecuteResult{Output: string(b)}
}
