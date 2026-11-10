package tools

import (
	"os"
	"strings"
)

type ListFilesTool struct{}

func init()                        { Register(ListFilesTool{}) }
func (ListFilesTool) Name() string { return "list_files" }
func (ListFilesTool) Execute(p ExecuteParams) ExecuteResult {
	path, err := resolveWorkspacePath(p.Path)
	if err != nil {
		return ExecuteResult{Error: err.Error()}
	}
	info, err := os.Stat(path)
	if err != nil {
		return ExecuteResult{Error: err.Error()}
	}
	if !info.IsDir() {
		return ExecuteResult{Error: "path is not a directory"}
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return ExecuteResult{Error: err.Error()}
	}
	var b strings.Builder
	for i, e := range entries {
		if i >= 500 {
			b.WriteString("... output truncated at 500 entries\n")
			break
		}
		name := e.Name()
		if e.IsDir() {
			name += "/"
		}
		b.WriteString(name + "\n")
	}
	return ExecuteResult{Output: b.String()}
}
