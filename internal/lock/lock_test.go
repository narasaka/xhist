package lock

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAcquireAndRelease(t *testing.T) {
	tmpDir := t.TempDir()
	xhistPath := filepath.Join(tmpDir, "data.xhist")
	xlsxPath := filepath.Join(tmpDir, "data.xlsx")

	if err := os.WriteFile(xhistPath, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to create xhist file: %v", err)
	}
	if err := os.WriteFile(xlsxPath, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to create xlsx file: %v", err)
	}

	ctx := context.Background()
	unlocker, err := Acquire(ctx, xhistPath, xlsxPath)
	if err != nil {
		t.Fatalf("Acquire failed: %v", err)
	}

	if _, err := os.Stat(xhistPath + ".lock"); err != nil {
		t.Errorf("xhist lock file not created: %v", err)
	}
	if _, err := os.Stat(xlsxPath + ".lock"); err != nil {
		t.Errorf("xlsx lock file not created: %v", err)
	}

	if err := unlocker.Release(); err != nil {
		t.Errorf("Release failed: %v", err)
	}
}

func TestAcquireBlocksWhileHeld(t *testing.T) {
	tmpDir := t.TempDir()
	xhistPath := filepath.Join(tmpDir, "data.xhist")
	xlsxPath := filepath.Join(tmpDir, "data.xlsx")

	if err := os.WriteFile(xhistPath, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to create xhist file: %v", err)
	}
	if err := os.WriteFile(xlsxPath, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to create xlsx file: %v", err)
	}

	ctx := context.Background()

	unlocker1, err := Acquire(ctx, xhistPath, xlsxPath)
	if err != nil {
		t.Fatalf("First Acquire failed: %v", err)
	}
	defer unlocker1.Release()

	shortCtx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
	defer cancel()

	_, err = Acquire(shortCtx, xhistPath, xlsxPath)
	if err == nil {
		t.Error("Second Acquire should have failed while locks are held")
	}
}

func TestReleaseIdempotent(t *testing.T) {
	tmpDir := t.TempDir()
	xhistPath := filepath.Join(tmpDir, "data.xhist")
	xlsxPath := filepath.Join(tmpDir, "data.xlsx")

	if err := os.WriteFile(xhistPath, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to create xhist file: %v", err)
	}
	if err := os.WriteFile(xlsxPath, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to create xlsx file: %v", err)
	}

	ctx := context.Background()
	unlocker, err := Acquire(ctx, xhistPath, xlsxPath)
	if err != nil {
		t.Fatalf("Acquire failed: %v", err)
	}

	if err := unlocker.Release(); err != nil {
		t.Errorf("First Release failed: %v", err)
	}

	if err := unlocker.Release(); err != nil {
		t.Errorf("Second Release failed: %v", err)
	}
}

func TestLockFilesAreSidecars(t *testing.T) {
	tmpDir := t.TempDir()
	xhistPath := filepath.Join(tmpDir, "data.xhist")
	xlsxPath := filepath.Join(tmpDir, "data.xlsx")

	if err := os.WriteFile(xhistPath, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to create xhist file: %v", err)
	}
	if err := os.WriteFile(xlsxPath, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to create xlsx file: %v", err)
	}

	ctx := context.Background()
	unlocker, err := Acquire(ctx, xhistPath, xlsxPath)
	if err != nil {
		t.Fatalf("Acquire failed: %v", err)
	}
	defer unlocker.Release()

	expectedXhistLock := xhistPath + ".lock"
	expectedXlsxLock := xlsxPath + ".lock"

	if _, err := os.Stat(expectedXhistLock); err != nil {
		t.Errorf("Expected xhist lock file at %s: %v", expectedXhistLock, err)
	}
	if _, err := os.Stat(expectedXlsxLock); err != nil {
		t.Errorf("Expected xlsx lock file at %s: %v", expectedXlsxLock, err)
	}
}

func TestConcurrentAcquire(t *testing.T) {
	tmpDir := t.TempDir()
	xhistPath := filepath.Join(tmpDir, "data.xhist")
	xlsxPath := filepath.Join(tmpDir, "data.xlsx")

	if err := os.WriteFile(xhistPath, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to create xhist file: %v", err)
	}
	if err := os.WriteFile(xlsxPath, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to create xlsx file: %v", err)
	}

	ctx := context.Background()
	acquired := make(chan bool, 2)

	go func() {
		unlocker, err := Acquire(ctx, xhistPath, xlsxPath)
		if err != nil {
			acquired <- false
			return
		}
		acquired <- true
		time.Sleep(300 * time.Millisecond)
		unlocker.Release()
	}()

	if !<-acquired {
		t.Fatal("First goroutine failed to acquire lock")
	}

	shortCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()

	start := time.Now()
	unlocker2, err := Acquire(shortCtx, xhistPath, xlsxPath)
	elapsed := time.Since(start)

	if err != nil {
		if elapsed < 200*time.Millisecond {
			t.Logf("Second acquire failed quickly as expected (elapsed: %v)", elapsed)
		}
	} else {
		if elapsed < 200*time.Millisecond {
			t.Error("Second acquire succeeded too quickly - should have waited")
		}
		unlocker2.Release()
	}
}
