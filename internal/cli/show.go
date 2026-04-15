package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"time"

	"github.com/prosights/xhist/internal/format"
	"github.com/urfave/cli/v3"
)

func newShowCmd() *cli.Command {
	return &cli.Command{
		Name:  "show",
		Usage: "Show full details of a specific operation",
		Action: func(_ context.Context, cmd *cli.Command) error {
			errW := cmdErr(cmd)
			xlsxPath := cmd.Args().Get(0)
			if xlsxPath == "" {
				return outputErrorTo(errW, "missing required argument: <file.xlsx>")
			}
			seqStr := cmd.Args().Get(1)
			if seqStr == "" {
				return outputErrorTo(errW, "missing required argument: <seq>")
			}
			targetSeq, err := strconv.ParseUint(seqStr, 10, 32)
			if err != nil {
				return outputErrorTo(errW, fmt.Sprintf("invalid sequence number: %s", seqStr))
			}

			xhp := xhistPath(xlsxPath)
			f, err := os.Open(xhp)
			if err != nil {
				return outputErrorTo(errW, fmt.Sprintf("opening %s: %v", xhp, err))
			}
			defer f.Close()

			rd, err := format.NewReader(f)
			if err != nil {
				return outputErrorTo(errW, fmt.Sprintf("reading %s: %v", xhp, err))
			}

			for {
				rec, err := rd.Next()
				if err != nil {
					if err == io.EOF {
						return outputErrorTo(errW, fmt.Sprintf("operation %d not found", targetSeq))
					}
					return outputErrorTo(errW, fmt.Sprintf("reading record: %v", err))
				}
				switch v := rec.Parsed.(type) {
				case format.Op:
					if v.Sequence != uint32(targetSeq) {
						continue
					}
					m := map[string]any{
						"seq":     v.Sequence,
						"ts":      time.UnixMilli(v.Timestamp).UTC().Format(time.RFC3339),
						"action":  actionString(v.Action),
						"sheet":   v.Sheet,
						"range":   v.Range,
						"message": v.Message,
						"values":  flatToGrid(v.Cells, int(v.NumRows), int(v.NumCols)),
					}
					if len(v.Comments) > 0 {
						comments := make([]map[string]any, len(v.Comments))
						for i, c := range v.Comments {
							comments[i] = map[string]any{"cell": c.Cell, "author": c.Author, "text": c.Text}
						}
						m["comments"] = comments
					}
					return outputJSON(cmdOut(cmd), m)
				case format.CommentOp:
					if v.Sequence != uint32(targetSeq) {
						continue
					}
					m := map[string]any{
						"seq":     v.Sequence,
						"ts":      time.UnixMilli(v.Timestamp).UTC().Format(time.RFC3339),
						"type":    "comment",
						"action":  commentActionString(v.Action),
						"sheet":   v.Sheet,
						"range":   v.Range,
						"message": v.Message,
					}
					if len(v.Entries) > 0 {
						entries := make([]map[string]any, len(v.Entries))
						for i, e := range v.Entries {
							entries[i] = map[string]any{"cell": e.Cell, "author": e.Author, "text": e.Text}
						}
						m["entries"] = entries
					}
					return outputJSON(cmdOut(cmd), m)
				default:
					continue
				}
			}
		},
	}
}
