package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type WriteFileTool struct{}

func init()                        { Register(WriteFileTool{}) }
func (WriteFileTool) Name() string { return "write_file" }
func (WriteFileTool) Execute(p ExecuteParams) ExecuteResult {
	if !p.Approved {
		return ExecuteResult{Error: "write_file requires explicit approval=true"}
	}
	if strings.TrimSpace(p.Path) == "" {
		return ExecuteResult{Error: "path is required"}
	}
	root, err := filepath.EvalSymlinks(WorkspaceRoot)
	if err != nil {
		return ExecuteResult{Error: err.Error()}
	}
	dir := filepath.Dir(p.Path)
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(root, dir)
	}
	resolvedDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return ExecuteResult{Error: fmt.Sprintf("parent directory must already exist: %v", err)}
	}
	if err = ensureInsideWorkspace(root, resolvedDir); err != nil {
		return ExecuteResult{Error: err.Error()}
	}
	name := filepath.Base(p.Path)
	if name == "." || name == string(filepath.Separator) {
		return ExecuteResult{Error: "invalid filename"}
	}
	target := filepath.Join(resolvedDir, name)
	if _, err = os.Lstat(target); err == nil {
		return ExecuteResult{Error: "refusing to overwrite an existing file; choose a new path"}
	} else if !os.IsNotExist(err) {
		return ExecuteResult{Error: err.Error()}
	}
	if len(p.Content) > 1<<20 {
		return ExecuteResult{Error: "content exceeds 1 MiB limit"}
	}
	f, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return ExecuteResult{Error: err.Error()}
	}
	if _, err = f.WriteString(p.Content); err != nil {
		f.Close()
		os.Remove(target)
		return ExecuteResult{Error: err.Error()}
	}
	if err = f.Close(); err != nil {
		return ExecuteResult{Error: err.Error()}
	}
	return ExecuteResult{Output: "Created " + target}
}
