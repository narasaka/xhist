package cli

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/prosights/xhist/internal/excel"
	"github.com/prosights/xhist/internal/format"
)

func xhistPath(xlsxPath string) string {
	ext := filepath.Ext(xlsxPath)
	return strings.TrimSuffix(xlsxPath, ext) + ".xhist"
}

func ensureInit(xlsxPath string) (string, error) {
	xhp := xhistPath(xlsxPath)

	if _, err := os.Stat(xlsxPath); os.IsNotExist(err) {
		if err := excel.CreateWorkbook(xlsxPath); err != nil {
			return "", err
		}
	}

	if _, err := os.Stat(xhp); err == nil {
		return xhp, nil
	}

	f, err := os.Create(xhp)
	if err != nil {
		return "", err
	}
	defer f.Close()

	w, err := format.NewWriter(f)
	if err != nil {
		return "", err
	}
	if err := w.WriteHeader(time.Now().UnixMilli(), filepath.Base(xlsxPath)); err != nil {
		return "", err
	}
	return xhp, nil
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
