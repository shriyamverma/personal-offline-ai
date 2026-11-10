package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const WorkspaceRoot = "/workspace"

func resolveWorkspacePath(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("path is required")
	}
	root, err := filepath.EvalSymlinks(WorkspaceRoot)
	if err != nil {
		return "", fmt.Errorf("workspace unavailable: %w", err)
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return "", err
	}
	candidate := path
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(root, candidate)
	}
	candidate, err = filepath.Abs(filepath.Clean(candidate))
	if err != nil {
		return "", err
	}
	if err = ensureInsideWorkspace(root, candidate); err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return "", fmt.Errorf("unable to access path: %w", err)
	}
	if err = ensureInsideWorkspace(root, resolved); err != nil {
		return "", err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", err
	}
	if !info.IsDir() && !info.Mode().IsRegular() {
		return "", fmt.Errorf("unsupported file type")
	}
	return resolved, nil
}

func ensureInsideWorkspace(root, target string) error {
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return err
	}
	if rel == ".." || rel == "."+string(filepath.Separator)+".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return fmt.Errorf("path outside workspace is not allowed")
	}
	return nil
}
