package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/prosights/xhist/internal/format"
	"github.com/prosights/xhist/internal/workspace"
	"github.com/urfave/cli/v3"
)

func resolveWorkspace(cmd *cli.Command) (string, string, error) {
	if ws := cmd.Root().String("workspace"); ws != "" {
		if !strings.HasSuffix(ws, ".xhist") {
			return "", "", fmt.Errorf("workspace path must end with .xhist: %s", ws)
		}
		abs, err := filepath.Abs(ws)
		if err != nil {
			return "", "", err
		}
		if _, err := os.Stat(abs); os.IsNotExist(err) {
			f, err := os.Create(abs)
			if err != nil {
				return "", "", fmt.Errorf("creating workspace: %v", err)
			}
			defer f.Close()
			w, err := format.NewWriter(f)
			if err != nil {
				return "", "", err
			}
			name := strings.TrimSuffix(filepath.Base(abs), ".xhist")
			if err := w.WriteHeader(time.Now().UnixMilli(), name); err != nil {
				return "", "", err
			}
		}
		return abs, filepath.Dir(abs), nil
	}

	cwd, err := os.Getwd()
	if err != nil {
		return "", "", fmt.Errorf("getting cwd: %v", err)
	}

	found, err := workspace.Discover(cwd)
	if err == nil {
		abs, _ := filepath.Abs(found)
		return abs, filepath.Dir(abs), nil
	}

	name := workspace.DefaultName(cwd)
	xhp := filepath.Join(cwd, name+".xhist")
	f, err := os.Create(xhp)
	if err != nil {
		return "", "", fmt.Errorf("creating workspace: %v", err)
	}
	defer f.Close()
	w, err := format.NewWriter(f)
	if err != nil {
		return "", "", err
	}
	if err := w.WriteHeader(time.Now().UnixMilli(), name); err != nil {
		return "", "", err
	}
	return xhp, cwd, nil
}

func resolveWorkspaceReadOnly(cmd *cli.Command) (string, string, error) {
	if ws := cmd.Root().String("workspace"); ws != "" {
		if !strings.HasSuffix(ws, ".xhist") {
			return "", "", fmt.Errorf("workspace path must end with .xhist: %s", ws)
		}
		abs, err := filepath.Abs(ws)
		if err != nil {
			return "", "", err
		}
		if _, err := os.Stat(abs); os.IsNotExist(err) {
			return "", "", fmt.Errorf("workspace file not found: %s", abs)
		}
		return abs, filepath.Dir(abs), nil
	}

	cwd, err := os.Getwd()
	if err != nil {
		return "", "", fmt.Errorf("getting cwd: %v", err)
	}

	found, err := workspace.Discover(cwd)
	if err != nil {
		return "", "", fmt.Errorf("no workspace found: %v", err)
	}
	abs, _ := filepath.Abs(found)
	return abs, filepath.Dir(abs), nil
}

func resolveTargetFile(workspaceRoot, xlsxPath string) (string, error) {
	return workspace.RelativePath(workspaceRoot, xlsxPath)
}

func requireV2(xhistPath string) error {
	f, err := os.Open(xhistPath)
	if err != nil {
		return err
	}
	defer f.Close()

	var buf [7]byte
	if _, err := io.ReadFull(f, buf[:]); err != nil {
		return fmt.Errorf("reading preamble: %v", err)
	}
	if buf[6] == 0x01 {
		return fmt.Errorf("v1 log detected — run `xhist migrate` to upgrade")
	}
	return nil
}

func openWorkspaceForAppend(cmd *cli.Command) (*os.File, *format.Writer, string, error) {
	xhp, wsRoot, err := resolveWorkspace(cmd)
	if err != nil {
		return nil, nil, "", err
	}
	if err := requireV2(xhp); err != nil {
		return nil, nil, "", err
	}
	f, w, err := openLogForAppend(xhp)
	if err != nil {
		return nil, nil, "", err
	}
	return f, w, wsRoot, nil
}

func lastSequence(xhistPath string) (uint32, error) {
	f, err := os.Open(xhistPath)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	rd, err := format.NewReader(f)
	if err != nil {
		return 0, err
	}

	var maxSeq uint32
	for {
		rec, err := rd.Next()
		if err != nil {
			if err == io.EOF {
				break
			}
			return maxSeq, nil
		}
		switch v := rec.Parsed.(type) {
		case format.Op:
			if v.Sequence > maxSeq {
				maxSeq = v.Sequence
			}
		case format.CommentOp:
			if v.Sequence > maxSeq {
				maxSeq = v.Sequence
			}
		case format.ConfuseOp:
			if v.Sequence > maxSeq {
				maxSeq = v.Sequence
			}
		}
	}
	return maxSeq, nil
}

func openLogForAppend(xhistPath string) (*os.File, *format.Writer, error) {
	f, err := os.OpenFile(xhistPath, os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, nil, err
	}
	w := format.NewAppendWriter(f)
	return f, w, nil
}
