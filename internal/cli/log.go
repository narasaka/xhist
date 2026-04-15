package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/prosights/xhist/internal/format"
	"github.com/urfave/cli/v3"
)

// logRecord represents either an Op or CommentOp for display purposes.
type logRecord struct {
	Op        *format.Op
	CommentOp *format.CommentOp
}

func (lr logRecord) sequence() uint32 {
	if lr.Op != nil {
		return lr.Op.Sequence
	}
	return lr.CommentOp.Sequence
}

func (lr logRecord) timestamp() int64 {
	if lr.Op != nil {
		return lr.Op.Timestamp
	}
	return lr.CommentOp.Timestamp
}

func (lr logRecord) sheet() string {
	if lr.Op != nil {
		return lr.Op.Sheet
	}
	return lr.CommentOp.Sheet
}

func newLogCmd() *cli.Command {
	return &cli.Command{
		Name:  "log",
		Usage: "Show operation history",
		Flags: []cli.Flag{
			&cli.BoolFlag{Name: "follow", Usage: "Stream new ops in real-time (NDJSON)"},
			&cli.StringFlag{Name: "sheet", Usage: "Filter by sheet name"},
			&cli.StringFlag{Name: "action", Usage: "Filter by action type (read|write)"},
			&cli.StringFlag{Name: "since", Usage: "Ops after this time (ISO 8601 or Unix ms)"},
			&cli.IntFlag{Name: "last", Usage: "Show only the last N ops"},
			&cli.BoolFlag{Name: "with-values", Usage: "Include cell values in output"},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			errW := cmdErr(cmd)
			xlsxPath := cmd.Args().Get(0)
			if xlsxPath == "" {
				return outputErrorTo(errW, "missing required argument: <file.xlsx>")
			}

			xhp := xhistPath(xlsxPath)
			if _, err := os.Stat(xhp); os.IsNotExist(err) {
				return outputErrorTo(errW, fmt.Sprintf("%s not found", xhp))
			}

			records, err := scanRecords(xhp)
			if err != nil {
				return outputErrorTo(errW, fmt.Sprintf("reading log: %v", err))
			}

			records = filterRecords(records, cmd)

			outW := cmdOut(cmd)
			human := cmd.Bool("human") || cmd.Root().Bool("human")

			if cmd.Bool("follow") {
				for _, lr := range records {
					entry := recordToMap(lr, cmd.Bool("with-values"))
					if err := outputNDJSON(outW, entry); err != nil {
						return err
					}
				}
				return followLog(ctx, xhp, cmd)
			}

			if human {
				printHumanLog(outW, records)
				return nil
			}

			entries := make([]map[string]any, len(records))
			for i, lr := range records {
				entries[i] = recordToMap(lr, cmd.Bool("with-values"))
			}
			return outputJSON(outW, entries)
		},
	}
}

func scanOps(xhp string) ([]format.Op, error) {
	f, err := os.Open(xhp)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	rd, err := format.NewReader(f)
	if err != nil {
		return nil, err
	}

	var ops []format.Op
	for {
		rec, err := rd.Next()
		if err != nil {
			if err == io.EOF {
				break
			}
			return ops, nil
		}
		if op, ok := rec.Parsed.(format.Op); ok {
			ops = append(ops, op)
		}
	}
	return ops, nil
}

func scanRecords(xhp string) ([]logRecord, error) {
	f, err := os.Open(xhp)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	rd, err := format.NewReader(f)
	if err != nil {
		return nil, err
	}

	var records []logRecord
	for {
		rec, err := rd.Next()
		if err != nil {
			if err == io.EOF {
				break
			}
			return records, nil
		}
		switch v := rec.Parsed.(type) {
		case format.Op:
			op := v
			records = append(records, logRecord{Op: &op})
		case format.CommentOp:
			cop := v
			records = append(records, logRecord{CommentOp: &cop})
		}
	}
	return records, nil
}

func filterOps(ops []format.Op, cmd *cli.Command) []format.Op {
	var filtered []format.Op
	sheetFilter := cmd.String("sheet")
	actionFilter := cmd.String("action")
	sinceStr := cmd.String("since")
	var sinceTS int64
	if sinceStr != "" {
		if t, err := time.Parse(time.RFC3339, sinceStr); err == nil {
			sinceTS = t.UnixMilli()
		} else if ms, err := strconv.ParseInt(sinceStr, 10, 64); err == nil {
			sinceTS = ms
		}
	}

	for _, op := range ops {
		if sheetFilter != "" && op.Sheet != sheetFilter {
			continue
		}
		if actionFilter != "" && actionString(op.Action) != actionFilter {
			continue
		}
		if sinceTS > 0 && op.Timestamp <= sinceTS {
			continue
		}
		filtered = append(filtered, op)
	}

	last := cmd.Int("last")
	if last > 0 && int(last) < len(filtered) {
		filtered = filtered[len(filtered)-int(last):]
	}
	return filtered
}

func filterRecords(records []logRecord, cmd *cli.Command) []logRecord {
	var filtered []logRecord
	sheetFilter := cmd.String("sheet")
	actionFilter := cmd.String("action")
	sinceStr := cmd.String("since")
	var sinceTS int64
	if sinceStr != "" {
		if t, err := time.Parse(time.RFC3339, sinceStr); err == nil {
			sinceTS = t.UnixMilli()
		} else if ms, err := strconv.ParseInt(sinceStr, 10, 64); err == nil {
			sinceTS = ms
		}
	}

	for _, lr := range records {
		if sheetFilter != "" && lr.sheet() != sheetFilter {
			continue
		}
		if actionFilter != "" {
			if lr.Op != nil && actionString(lr.Op.Action) != actionFilter {
				continue
			}
			if lr.CommentOp != nil && commentActionString(lr.CommentOp.Action) != actionFilter {
				continue
			}
		}
		if sinceTS > 0 && lr.timestamp() <= sinceTS {
			continue
		}
		filtered = append(filtered, lr)
	}

	last := cmd.Int("last")
	if last > 0 && int(last) < len(filtered) {
		filtered = filtered[len(filtered)-int(last):]
	}
	return filtered
}

func actionString(a uint8) string {
	if a == format.ActionRead {
		return "read"
	}
	return "write"
}

func commentActionString(a uint8) string {
	switch a {
	case format.ActionCommentSet:
		return "comment_set"
	case format.ActionCommentGet:
		return "comment_get"
	case format.ActionCommentDelete:
		return "comment_delete"
	default:
		return fmt.Sprintf("comment_unknown(%d)", a)
	}
}

func opToMap(op format.Op, withValues bool) map[string]any {
	m := map[string]any{
		"seq":     op.Sequence,
		"ts":      time.UnixMilli(op.Timestamp).UTC().Format(time.RFC3339),
		"action":  actionString(op.Action),
		"sheet":   op.Sheet,
		"range":   op.Range,
		"message": op.Message,
	}
	if withValues {
		m["values"] = flatToGrid(op.Cells, int(op.NumRows), int(op.NumCols))
	}
	return m
}

func recordToMap(lr logRecord, withValues bool) map[string]any {
	if lr.Op != nil {
		return opToMap(*lr.Op, withValues)
	}
	cop := lr.CommentOp
	m := map[string]any{
		"seq":     cop.Sequence,
		"ts":      time.UnixMilli(cop.Timestamp).UTC().Format(time.RFC3339),
		"type":    "comment",
		"action":  commentActionString(cop.Action),
		"sheet":   cop.Sheet,
		"range":   cop.Range,
		"message": cop.Message,
	}
	if cop.NumEntries > 0 {
		entries := make([]map[string]any, len(cop.Entries))
		for i, e := range cop.Entries {
			entries[i] = map[string]any{
				"cell":   e.Cell,
				"author": e.Author,
				"text":   e.Text,
			}
		}
		m["entries"] = entries
	}
	return m
}

func flatToGrid(cells []format.Cell, rows, cols int) [][]any {
	grid := make([][]any, rows)
	for r := range rows {
		grid[r] = make([]any, cols)
		for c := range cols {
			idx := r*cols + c
			if idx < len(cells) {
				grid[r][c] = cellToJSON(cells[idx])
			}
		}
	}
	return grid
}

func printHumanLog(w io.Writer, records []logRecord) {
	fmt.Fprintf(w, " %-5s %-22s %-14s %-10s %-12s %s\n", "SEQ", "TIME", "ACTION", "SHEET", "RANGE", "MESSAGE")
	for _, lr := range records {
		var seq uint32
		var ts, action, sheet, rng, msg string
		if lr.Op != nil {
			seq = lr.Op.Sequence
			ts = time.UnixMilli(lr.Op.Timestamp).UTC().Format(time.RFC3339)
			action = strings.ToUpper(actionString(lr.Op.Action))
			sheet = lr.Op.Sheet
			rng = lr.Op.Range
			msg = lr.Op.Message
		} else {
			seq = lr.CommentOp.Sequence
			ts = time.UnixMilli(lr.CommentOp.Timestamp).UTC().Format(time.RFC3339)
			action = strings.ToUpper(commentActionString(lr.CommentOp.Action))
			sheet = lr.CommentOp.Sheet
			rng = lr.CommentOp.Range
			msg = lr.CommentOp.Message
		}
		fmt.Fprintf(w, " %-5d %-22s %-14s %-10s %-12s %s\n", seq, ts, action, sheet, rng, msg)
	}
}

func followLog(ctx context.Context, xhp string, cmd *cli.Command) error {
	outW := cmdOut(cmd)
	var lastSeq uint32
	records, _ := scanRecords(xhp)
	if len(records) > 0 {
		lastSeq = records[len(records)-1].sequence()
	}

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			records, err := scanRecords(xhp)
			if err != nil {
				continue
			}
			for _, lr := range records {
				if lr.sequence() <= lastSeq {
					continue
				}
				sheetFilter := cmd.String("sheet")
				actionFilter := cmd.String("action")
				if sheetFilter != "" && lr.sheet() != sheetFilter {
					continue
				}
				if actionFilter != "" {
					if lr.Op != nil && actionString(lr.Op.Action) != actionFilter {
						continue
					}
					if lr.CommentOp != nil && commentActionString(lr.CommentOp.Action) != actionFilter {
						continue
					}
				}
				entry := recordToMap(lr, cmd.Bool("with-values"))
				if err := outputNDJSON(outW, entry); err != nil {
					return err
				}
				lastSeq = lr.sequence()
			}
		}
	}
}
