package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func Discover(startDir string) (string, error) {
	dir, err := filepath.Abs(startDir)
	if err != nil {
		return "", fmt.Errorf("resolving start directory: %w", err)
	}

	origStart := dir
	for {
		matches, err := filepath.Glob(filepath.Join(dir, "*.xhist"))
		if err != nil {
			return "", fmt.Errorf("globbing in %s: %w", dir, err)
		}

		var found []string
		for _, m := range matches {
			base := filepath.Base(m)
			if !strings.HasSuffix(base, ".xhist") {
				continue
			}
			name := strings.TrimSuffix(base, ".xhist")
			if name == "" {
				continue
			}
			info, err := os.Stat(m)
			if err != nil || info.IsDir() {
				continue
			}
			found = append(found, m)
		}

		switch len(found) {
		case 1:
			return found[0], nil
		case 0:
		default:
			names := make([]string, len(found))
			for i, f := range found {
				names[i] = filepath.Base(f)
			}
			return "", fmt.Errorf("multiple .xhist files found in %s: %v", dir, names)
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no .xhist file found (searched from %s to filesystem root)", origStart)
		}
		dir = parent
	}
}

func RelativePath(workspaceRoot string, xlsxPath string) (string, error) {
	absRoot, err := filepath.Abs(workspaceRoot)
	if err != nil {
		return "", fmt.Errorf("resolving workspace root: %w", err)
	}

	if strings.HasSuffix(absRoot, ".xhist") {
		absRoot = filepath.Dir(absRoot)
	}

	absXlsx, err := filepath.Abs(xlsxPath)
	if err != nil {
		return "", fmt.Errorf("resolving xlsx path: %w", err)
	}

	rel, err := filepath.Rel(absRoot, absXlsx)
	if err != nil {
		return "", fmt.Errorf("computing relative path: %w", err)
	}

	if strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("%s is outside workspace root %s", xlsxPath, workspaceRoot)
	}

	result := filepath.ToSlash(rel)
	result = strings.TrimPrefix(result, "./")
	return result, nil
}

func DefaultName(dir string) string {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return filepath.Base(dir)
	}
	return filepath.Base(absDir)
}
