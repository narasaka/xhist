package cli

import (
	"context"
	"fmt"

	"github.com/prosights/xhist/internal/format"
	"github.com/urfave/cli/v3"
)

func newReindexCmd() *cli.Command {
	return &cli.Command{
		Name:  "reindex",
		Usage: "Rebuild the sidecar index from the log",
		Action: func(_ context.Context, cmd *cli.Command) error {
			errW := cmdErr(cmd)
			xlsxPath := cmd.Args().Get(0)
			if xlsxPath == "" {
				return outputErrorTo(errW, "missing required argument: <file.xlsx>")
			}

			xhp := xhistPath(xlsxPath)
			if err := format.BuildIndex(xhp); err != nil {
				return outputErrorTo(errW, fmt.Sprintf("building index: %v", err))
			}

			idxPath := xhp + ".idx"
			entries, err := format.ReadIndex(idxPath)
			if err != nil {
				return outputErrorTo(errW, fmt.Sprintf("reading index: %v", err))
			}

			return outputJSON(cmdOut(cmd), map[string]any{
				"entries":    len(entries),
				"index_file": idxPath,
			})
		},
	}
}
