package workspace_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/prosights/xhist/internal/workspace"
)

func TestDiscoverFindsXhist(t *testing.T) {
	dir := t.TempDir()
	xhistPath := filepath.Join(dir, "test.xhist")
	if err := os.WriteFile(xhistPath, []byte{}, 0644); err != nil {
		t.Fatal(err)
	}

	got, err := workspace.Discover(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != xhistPath {
		t.Errorf("got %q, want %q", got, xhistPath)
	}
}

func TestDiscoverWalksUp(t *testing.T) {
	parent := t.TempDir()
	xhistPath := filepath.Join(parent, "test.xhist")
	if err := os.WriteFile(xhistPath, []byte{}, 0644); err != nil {
		t.Fatal(err)
	}

	child := filepath.Join(parent, "subdir", "deep")
	if err := os.MkdirAll(child, 0755); err != nil {
		t.Fatal(err)
	}

	got, err := workspace.Discover(child)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != xhistPath {
		t.Errorf("got %q, want %q", got, xhistPath)
	}
}

func TestDiscoverErrorNoXhist(t *testing.T) {
	dir := t.TempDir()

	got, err := workspace.Discover(dir)
	if err == nil {
		if strings.HasPrefix(got, dir) {
			t.Fatal("expected error or result outside temp dir, got file in temp dir")
		}
		t.Skipf("found .xhist in ancestor directory: %s", got)
	}
	if !strings.Contains(err.Error(), "no .xhist file found") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestDiscoverErrorMultiple(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a.xhist", "b.xhist"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte{}, 0644); err != nil {
			t.Fatal(err)
		}
	}

	_, err := workspace.Discover(dir)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "multiple .xhist files found") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestDiscoverIgnoresBareXhist(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".xhist"), []byte{}, 0644); err != nil {
		t.Fatal(err)
	}

	got, err := workspace.Discover(dir)
	if err == nil {
		if strings.HasPrefix(got, dir) {
			t.Fatal("bare .xhist should be ignored, but was returned")
		}
		t.Skipf("bare .xhist ignored correctly; found .xhist in ancestor: %s", got)
	}
	if !strings.Contains(err.Error(), "no .xhist file found") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestDiscoverIgnoresIdxAndLock(t *testing.T) {
	dir := t.TempDir()
	xhistPath := filepath.Join(dir, "test.xhist")
	for _, name := range []string{"test.xhist", "test.xhist.idx", "test.xhist.lock"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte{}, 0644); err != nil {
			t.Fatal(err)
		}
	}

	got, err := workspace.Discover(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != xhistPath {
		t.Errorf("got %q, want %q", got, xhistPath)
	}
}

func TestRelativePathSimple(t *testing.T) {
	dir := t.TempDir()
	xlsx := filepath.Join(dir, "budget.xlsx")

	got, err := workspace.RelativePath(dir, xlsx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "budget.xlsx" {
		t.Errorf("got %q, want %q", got, "budget.xlsx")
	}
}

func TestRelativePathNested(t *testing.T) {
	dir := t.TempDir()
	xlsx := filepath.Join(dir, "reports", "q1.xlsx")

	got, err := workspace.RelativePath(dir, xlsx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "reports/q1.xlsx" {
		t.Errorf("got %q, want %q", got, "reports/q1.xlsx")
	}
}

func TestRelativePathFromXhistFile(t *testing.T) {
	dir := t.TempDir()
	xhistFile := filepath.Join(dir, "proj.xhist")
	xlsx := filepath.Join(dir, "budget.xlsx")

	got, err := workspace.RelativePath(xhistFile, xlsx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "budget.xlsx" {
		t.Errorf("got %q, want %q", got, "budget.xlsx")
	}
}

func TestRelativePathOutsideRoot(t *testing.T) {
	dir := t.TempDir()
	other := t.TempDir()
	xlsx := filepath.Join(other, "budget.xlsx")

	_, err := workspace.RelativePath(dir, xlsx)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "outside workspace root") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestRelativePathForwardSlashes(t *testing.T) {
	dir := t.TempDir()
	xlsx := filepath.Join(dir, "reports", "q1.xlsx")

	got, err := workspace.RelativePath(dir, xlsx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(got, "\\") {
		t.Errorf("result contains backslash: %q", got)
	}
}

func TestRelativePathNoLeadingDotSlash(t *testing.T) {
	dir := t.TempDir()
	xlsx := filepath.Join(dir, "budget.xlsx")

	got, err := workspace.RelativePath(dir, xlsx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.HasPrefix(got, "./") {
		t.Errorf("result has leading ./: %q", got)
	}
}

func TestDefaultName(t *testing.T) {
	got := workspace.DefaultName("/work/my-project")
	if got != "my-project" {
		t.Errorf("got %q, want %q", got, "my-project")
	}
}

func TestDefaultNameTrailingSlash(t *testing.T) {
	got := workspace.DefaultName("/work/my-project/")
	if got != "my-project" {
		t.Errorf("got %q, want %q", got, "my-project")
	}
}
