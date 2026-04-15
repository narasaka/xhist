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
				op, ok := rec.Parsed.(format.Op)
				if !ok || op.Sequence != uint32(targetSeq) {
					continue
				}

				m := map[string]any{
					"seq":     op.Sequence,
					"ts":      time.UnixMilli(op.Timestamp).UTC().Format(time.RFC3339),
					"action":  actionString(op.Action),
					"sheet":   op.Sheet,
					"range":   op.Range,
					"message": op.Message,
					"values":  flatToGrid(op.Cells, int(op.NumRows), int(op.NumCols)),
				}
				return outputJSON(cmdOut(cmd), m)
			}
		},
	}
}
