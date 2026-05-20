package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"time"

	"github.com/narasaka/xhist/internal/format"
	"github.com/urfave/cli/v3"
)

func newShowCmd() *cli.Command {
	return &cli.Command{
		Name:  "show",
		Usage: "Show full details of a specific operation",
		Action: func(_ context.Context, cmd *cli.Command) error {
			errW := cmdErr(cmd)
			seqStr := cmd.Args().Get(0)
			if seqStr == "" {
				return outputErrorTo(errW, "missing required argument: <seq>")
			}
			targetSeq, err := strconv.ParseUint(seqStr, 10, 32)
			if err != nil {
				return outputErrorTo(errW, fmt.Sprintf("invalid sequence number: %s", seqStr))
			}

			xhp, _, err := resolveWorkspaceReadOnly(cmd)
			if err != nil {
				return outputErrorTo(errW, fmt.Sprintf("workspace: %v", err))
			}

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
						"file":    v.TargetFile,
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
						"file":    v.TargetFile,
					}
					if len(v.Entries) > 0 {
						entries := make([]map[string]any, len(v.Entries))
						for i, e := range v.Entries {
							entries[i] = map[string]any{"cell": e.Cell, "author": e.Author, "text": e.Text}
						}
						m["entries"] = entries
					}
					return outputJSON(cmdOut(cmd), m)
				case format.ConfuseOp:
					if v.Sequence != uint32(targetSeq) {
						continue
					}
					m := map[string]any{
						"seq":         v.Sequence,
						"ts":          time.UnixMilli(v.Timestamp).UTC().Format(time.RFC3339),
						"type":        "confusion",
						"action":      confusionActionString(v.Action),
						"id":          v.ID,
						"sheet":       v.Sheet,
						"cell":        v.Cell,
						"archetype":   v.Archetype,
						"headline":    v.Headline,
						"description": v.Description,
						"message":     v.Message,
						"file":        v.TargetFile,
					}
					if v.PayloadJSON != "" {
						m["payload"] = jsonRaw(v.PayloadJSON)
					}
					if v.ResolutionJSON != "" {
						m["resolution"] = jsonRaw(v.ResolutionJSON)
					}
					return outputJSON(cmdOut(cmd), m)
				default:
					continue
				}
			}
		},
	}
}
