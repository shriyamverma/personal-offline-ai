package tools

import (
	"fmt"
	"os"
	"path/filepath"
)

const WorkspaceRoot = "/workspace"

func resolveWorkspacePath(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("path is required")
	}

	root, err := filepath.Abs(WorkspaceRoot)
	if err != nil {
		return "", fmt.Errorf("failed to resolve workspace: %w", err)
	}

	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return "", fmt.Errorf("failed to access workspace: %w", err)
	}

	var fullPath string

	if filepath.IsAbs(path) {
		fullPath = filepath.Clean(path)
	} else {
		fullPath = filepath.Join(root, filepath.Clean(path))
	}

	fullPath, err = filepath.Abs(fullPath)
	if err != nil {
		return "", fmt.Errorf("invalid path: %w", err)
	}

	// Check the lexical path before resolving symlinks.
	if err := ensureInsideWorkspace(root, fullPath); err != nil {
		return "", err
	}

	// Resolve symlinks to prevent escaping the workspace.
	resolvedPath, err := filepath.EvalSymlinks(fullPath)
	if err != nil {
		return "", fmt.Errorf("unable to access path: %w", err)
	}

	if err := ensureInsideWorkspace(root, resolvedPath); err != nil {
		return "", fmt.Errorf("path outside workspace is not allowed")
	}

	info, err := os.Stat(resolvedPath)
	if err != nil {
		return "", err
	}

	if !info.Mode().IsRegular() && !info.IsDir() {
		return "", fmt.Errorf("unsupported file type")
	}

	return resolvedPath, nil
}

func ensureInsideWorkspace(root, target string) error {
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return fmt.Errorf("invalid path: %w", err)
	}

	if rel == ".." || filepath.IsAbs(rel) ||
		(len(rel) > 3 && rel[:3] == ".."+string(filepath.Separator)) {
		return fmt.Errorf("path outside workspace is not allowed")
	}

	return nil
}
