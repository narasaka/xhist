package cli

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/prosights/xhist/internal/excel"
	"github.com/prosights/xhist/internal/format"
	"github.com/urfave/cli/v3"
)

func newStateCmd() *cli.Command {
	return &cli.Command{
		Name:  "state",
		Usage: "Reconstruct spreadsheet state from WRITE ops",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "sheet", Usage: "Show specific sheet only"},
			&cli.IntFlag{Name: "at", Usage: "Reconstruct state as of operation N"},
			&cli.StringFlag{Name: "range", Usage: "Show specific range only"},
			&cli.BoolFlag{Name: "diff", Usage: "Compare reconstructed state vs actual xlsx"},
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			errW := cmdErr(cmd)
			xlsxPath := cmd.Args().Get(0)
			if xlsxPath == "" {
				return outputErrorTo(errW, "missing required argument: <file.xlsx>")
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

			atSeq := uint32(cmd.Int("at"))
			sheetFilter := cmd.String("sheet")

			state := map[string]map[int]map[int]format.Cell{}

			for {
				rec, err := rd.Next()
				if err != nil {
					if err == io.EOF {
						break
					}
					return outputErrorTo(errW, fmt.Sprintf("reading record: %v", err))
				}
				op, ok := rec.Parsed.(format.Op)
				if !ok || op.Action != format.ActionWrite {
					continue
				}
				if atSeq > 0 && op.Sequence > atSeq {
					break
				}
				if sheetFilter != "" && op.Sheet != sheetFilter {
					continue
				}

				sheetName := op.Sheet
				if sheetName == "" {
					sheetName = "Sheet1"
				}
				if state[sheetName] == nil {
					state[sheetName] = map[int]map[int]format.Cell{}
				}

				topLeft := op.Range
				if idx := len(topLeft); idx > 0 {
					for i, ch := range topLeft {
						if ch == ':' {
							topLeft = topLeft[:i]
							break
						}
					}
				}
				startRow, startCol, err := excel.ParseCellRef(topLeft)
				if err != nil {
					continue
				}

				for r := range int(op.NumRows) {
					for c := range int(op.NumCols) {
						idx := r*int(op.NumCols) + c
						if idx >= len(op.Cells) {
							continue
						}
						row := startRow + r
						col := startCol + c
						if state[sheetName][row] == nil {
							state[sheetName][row] = map[int]format.Cell{}
						}
						state[sheetName][row][col] = op.Cells[idx]
					}
				}
			}

			result := map[string]any{}
			for sheetName, rows := range state {
				if sheetFilter != "" && sheetName != sheetFilter {
					continue
				}
				minR, maxR, minC, maxC := gridBounds(rows)
				if minR < 0 {
					continue
				}

				numRows := maxR - minR + 1
				numCols := maxC - minC + 1
				grid := make([][]any, numRows)
				for r := range numRows {
					grid[r] = make([]any, numCols)
					for c := range numCols {
						if cell, ok := rows[minR+r][minC+c]; ok {
							grid[r][c] = cellToJSON(cell)
						}
					}
				}

				tl := excel.CellRef(minR, minC)
				br := excel.CellRef(maxR, maxC)
				rangeStr := tl
				if tl != br {
					rangeStr = tl + ":" + br
				}

				result[sheetName] = map[string]any{
					"range":  rangeStr,
					"values": grid,
				}
			}

			if cmd.Bool("diff") {
				return outputDiff(cmdOut(cmd), xlsxPath, result)
			}

			return outputJSON(cmdOut(cmd), result)
		},
	}
}

func gridBounds(rows map[int]map[int]format.Cell) (minR, maxR, minC, maxC int) {
	minR, maxR, minC, maxC = -1, -1, -1, -1
	for r, cols := range rows {
		for c := range cols {
			if minR < 0 || r < minR {
				minR = r
			}
			if r > maxR {
				maxR = r
			}
			if minC < 0 || c < minC {
				minC = c
			}
			if c > maxC {
				maxC = c
			}
		}
	}
	return
}

func outputDiff(w io.Writer, xlsxPath string, reconstructed map[string]any) error {
	diffs := map[string]any{}
	for sheetName, v := range reconstructed {
		sheetData := v.(map[string]any)
		rangeStr := sheetData["range"].(string)
		values := sheetData["values"].([][]any)

		_, topLeft, bottomRight, err := excel.ParseRange(rangeStr)
		if err != nil {
			continue
		}
		actual, err := excel.ReadCells(xlsxPath, sheetName, topLeft, bottomRight)
		if err != nil {
			continue
		}

		actualGrid := gridToJSON(actual)
		hasDiff := false
		diffGrid := make([][]any, len(values))
		for r := range values {
			diffGrid[r] = make([]any, len(values[r]))
			for c := range values[r] {
				expected := values[r][c]
				var got any
				if r < len(actualGrid) && c < len(actualGrid[r]) {
					got = actualGrid[r][c]
				}
				if fmt.Sprintf("%v", expected) != fmt.Sprintf("%v", got) {
					hasDiff = true
					diffGrid[r][c] = map[string]any{"expected": expected, "actual": got}
				} else {
					diffGrid[r][c] = "ok"
				}
			}
		}
		if hasDiff {
			diffs[sheetName] = map[string]any{
				"range": rangeStr,
				"cells": diffGrid,
			}
		}
	}

	if len(diffs) == 0 {
		return outputJSON(w, map[string]any{"match": true})
	}
	return outputJSON(w, map[string]any{"match": false, "diffs": diffs})
}
