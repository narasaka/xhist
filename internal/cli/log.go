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

			ops, err := scanOps(xhp)
			if err != nil {
				return outputErrorTo(errW, fmt.Sprintf("reading log: %v", err))
			}

			ops = filterOps(ops, cmd)

			outW := cmdOut(cmd)
			human := cmd.Bool("human") || cmd.Root().Bool("human")

			if cmd.Bool("follow") {
				for _, op := range ops {
					entry := opToMap(op, cmd.Bool("with-values"))
					if err := outputNDJSON(outW, entry); err != nil {
						return err
					}
				}
				return followLog(ctx, xhp, cmd)
			}

			if human {
				printHumanLog(outW, ops)
				return nil
			}

			entries := make([]map[string]any, len(ops))
			for i, op := range ops {
				entries[i] = opToMap(op, cmd.Bool("with-values"))
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

func actionString(a uint8) string {
	if a == format.ActionRead {
		return "read"
	}
	return "write"
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

func printHumanLog(w io.Writer, ops []format.Op) {
	fmt.Fprintf(w, " %-5s %-22s %-6s %-10s %-12s %s\n", "SEQ", "TIME", "ACTION", "SHEET", "RANGE", "MESSAGE")
	for _, op := range ops {
		ts := time.UnixMilli(op.Timestamp).UTC().Format(time.RFC3339)
		fmt.Fprintf(w, " %-5d %-22s %-6s %-10s %-12s %s\n",
			op.Sequence, ts, strings.ToUpper(actionString(op.Action)), op.Sheet, op.Range, op.Message)
	}
}

func followLog(ctx context.Context, xhp string, cmd *cli.Command) error {
	outW := cmdOut(cmd)
	var lastSeq uint32
	ops, _ := scanOps(xhp)
	if len(ops) > 0 {
		lastSeq = ops[len(ops)-1].Sequence
	}

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			ops, err := scanOps(xhp)
			if err != nil {
				continue
			}
			for _, op := range ops {
				if op.Sequence <= lastSeq {
					continue
				}
				sheetFilter := cmd.String("sheet")
				actionFilter := cmd.String("action")
				if sheetFilter != "" && op.Sheet != sheetFilter {
					continue
				}
				if actionFilter != "" && actionString(op.Action) != actionFilter {
					continue
				}
				entry := opToMap(op, cmd.Bool("with-values"))
				if err := outputNDJSON(outW, entry); err != nil {
					return err
				}
				lastSeq = op.Sequence
			}
		}
	}
}
