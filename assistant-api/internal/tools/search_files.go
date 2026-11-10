package tools

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

type SearchFilesTool struct{}

func init()                          { Register(SearchFilesTool{}) }
func (SearchFilesTool) Name() string { return "search_files" }
func (SearchFilesTool) Execute(p ExecuteParams) ExecuteResult {
	if strings.TrimSpace(p.Command) == "" {
		return ExecuteResult{Error: "search_files requires Command as the text to find"}
	}
	start := p.Path
	if start == "" {
		start = "."
	}
	root, err := resolveWorkspacePath(start)
	if err != nil {
		return ExecuteResult{Error: err.Error()}
	}
	needle := strings.ToLower(p.Command)
	matches := 0
	var out strings.Builder
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if d.IsDir() {
			if d.Name() == ".git" || d.Name() == "vendor" || d.Name() == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if matches >= 100 {
			return filepath.SkipAll
		}
		if !d.Type().IsRegular() {
			return nil
		}
		info, e := d.Info()
		if e != nil || info.Size() > 1<<20 {
			return nil
		}
		f, e := os.Open(path)
		if e != nil {
			return nil
		}
		defer f.Close()
		s := bufio.NewScanner(f)
		s.Buffer(make([]byte, 4096), 256*1024)
		line := 0
		for s.Scan() {
			line++
			if strings.Contains(strings.ToLower(s.Text()), needle) {
				rel, _ := filepath.Rel(WorkspaceRoot, path)
				fmt.Fprintf(&out, "%s:%d: %s\n", rel, line, strings.TrimSpace(s.Text()))
				matches++
				if matches >= 100 {
					break
				}
			}
		}
		return nil
	})
	if err != nil {
		return ExecuteResult{Error: err.Error()}
	}
	if matches == 0 {
		return ExecuteResult{Output: "No matches found."}
	}
	return ExecuteResult{Output: out.String()}
}
