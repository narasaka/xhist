package cli

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/prosights/xhist/internal/excel"
	"github.com/prosights/xhist/internal/format"
	"github.com/prosights/xhist/internal/lock"
	"github.com/urfave/cli/v3"
)

func newReadCmd() *cli.Command {
	return &cli.Command{
		Name:  "read",
		Usage: "Read cells from Excel and log a READ op",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "message", Aliases: []string{"m"}, Usage: "Why this read was performed"},
			&cli.BoolFlag{Name: "no-log", Usage: "Read without logging"},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			errW := cmdErr(cmd)
			xlsxPath := cmd.Args().Get(0)
			if xlsxPath == "" {
				return outputErrorTo(errW, "missing required argument: <file.xlsx>")
			}
			rangeRef := cmd.Args().Get(1)
			if rangeRef == "" {
				return outputErrorTo(errW, "missing required argument: <range>")
			}

			xhp, wsRoot, err := resolveWorkspace(cmd)
			if err != nil {
				return outputErrorTo(errW, fmt.Sprintf("workspace: %v", err))
			}
			if err := requireV2(xhp); err != nil {
				return outputErrorTo(errW, fmt.Sprintf("version: %v", err))
			}

			targetFile, err := resolveTargetFile(wsRoot, xlsxPath)
			if err != nil {
				return outputErrorTo(errW, fmt.Sprintf("resolving target: %v", err))
			}

			sheet, topLeft, bottomRight, err := excel.ParseRange(rangeRef)
			if err != nil {
				return outputErrorTo(errW, fmt.Sprintf("parsing range: %v", err))
			}

			if _, err := os.Stat(xlsxPath); os.IsNotExist(err) {
				if err := excel.CreateWorkbook(xlsxPath); err != nil {
					return outputErrorTo(errW, fmt.Sprintf("creating workbook: %v", err))
				}
			}

			ul, err := lock.Acquire(ctx, xhp, xlsxPath)
			if err != nil {
				return outputErrorTo(errW, fmt.Sprintf("acquiring lock: %v", err))
			}
			defer ul.Release()

			cells, err := excel.ReadCells(xlsxPath, sheet, topLeft, bottomRight)
			if err != nil {
				return outputErrorTo(errW, fmt.Sprintf("reading cells: %v", err))
			}

			if !cmd.Bool("no-log") {
				seq, err := lastSequence(xhp)
				if err != nil {
					return outputErrorTo(errW, fmt.Sprintf("reading sequence: %v", err))
				}
				seq++

				rows, cols, _ := excel.RangeSize(topLeft, bottomRight)
				var flat []format.Cell
				for _, row := range cells {
					flat = append(flat, row...)
				}

				displayRange := rangeRef
				if sheet != "" {
					displayRange = topLeft
					if topLeft != bottomRight {
						displayRange = topLeft + ":" + bottomRight
					}
				}

				f, w, err := openLogForAppend(xhp)
				if err != nil {
					return outputErrorTo(errW, fmt.Sprintf("opening log: %v", err))
				}
				defer f.Close()

				op := format.Op{
					TargetFile: targetFile,
					Timestamp:  time.Now().UnixMilli(),
					Sequence:   seq,
					Action:     format.ActionRead,
					Sheet:      sheet,
					Range:      displayRange,
					Message:    cmd.String("message"),
					NumRows:    uint32(rows),
					NumCols:    uint32(cols),
					Cells:      flat,
				}
				if err := w.WriteOp(op); err != nil {
					return outputErrorTo(errW, fmt.Sprintf("writing op: %v", err))
				}
			}

			outW := cmdOut(cmd)
			isSingle := topLeft == bottomRight
			if isSingle {
				return outputJSON(outW, cellToJSON(cells[0][0]))
			}
			return outputJSON(outW, gridToJSON(cells))
		},
	}
}
