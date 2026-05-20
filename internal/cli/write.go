package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"time"

	"github.com/narasaka/xhist/internal/excel"
	"github.com/narasaka/xhist/internal/format"
	"github.com/narasaka/xhist/internal/lock"
	"github.com/urfave/cli/v3"
)

func newWriteCmd() *cli.Command {
	return &cli.Command{
		Name:  "write",
		Usage: "Write cells to Excel and log a WRITE op",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "message", Aliases: []string{"m"}, Usage: "Why this write was performed (required)", Required: true},
			&cli.StringFlag{Name: "json", Usage: "Values as JSON array of arrays"},
			&cli.StringFlag{Name: "file", Aliases: []string{"f"}, Usage: "Read values from a JSON file"},
			&cli.BoolFlag{Name: "stdin", Usage: "Read values from stdin"},
			&cli.StringFlag{Name: "comment", Usage: "Comment text for the cell (single-cell writes only)"},
			&cli.StringFlag{Name: "comments", Usage: "Comments as JSON array of arrays matching value grid"},
			&cli.StringFlag{Name: "comment-author", Usage: "Author name for comments (default: xhist)"},
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

			isSingle := topLeft == bottomRight
			var cells [][]format.Cell

			if isSingle {
				rawVal := cmd.Args().Get(2)
				cell := parseScalarValue(rawVal)
				cells = [][]format.Cell{{cell}}
			} else {
				jsonData, err := resolveJSONInput(cmd)
				if err != nil {
					return outputErrorTo(errW, fmt.Sprintf("reading input: %v", err))
				}
				cells, err = parseJSONGrid(jsonData)
				if err != nil {
					return outputErrorTo(errW, fmt.Sprintf("parsing values: %v", err))
				}
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

			if err := excel.WriteCells(xlsxPath, sheet, topLeft, cells); err != nil {
				return outputErrorTo(errW, fmt.Sprintf("writing cells: %v", err))
			}

			if isSingle && cmd.String("comments") != "" {
				return outputErrorTo(errW, "--comments is only valid for range writes")
			}
			if !isSingle && cmd.String("comment") != "" {
				return outputErrorTo(errW, "--comment is only valid for single-cell writes")
			}

			var commentEntries []format.CommentEntry
			commentAuthor := cmd.String("comment-author")
			if commentAuthor == "" {
				commentAuthor = "xhist"
			}

			if isSingle && cmd.String("comment") != "" {
				commentText := cmd.String("comment")
				if err := excel.SetComment(xlsxPath, sheet, topLeft, commentAuthor, commentText); err != nil {
					return outputErrorTo(errW, fmt.Sprintf("setting comment: %v", err))
				}
				commentEntries = append(commentEntries, format.CommentEntry{
					Cell: topLeft, Author: commentAuthor, Text: commentText,
				})
			} else if cmd.String("comments") != "" {
				var commentsGrid [][]string
				if err := json.Unmarshal([]byte(cmd.String("comments")), &commentsGrid); err != nil {
					return outputErrorTo(errW, fmt.Sprintf("parsing comments: %v", err))
				}
				if len(commentsGrid) != len(cells) {
					return outputErrorTo(errW, fmt.Sprintf("comments rows (%d) don't match value rows (%d)", len(commentsGrid), len(cells)))
				}
				for i, row := range commentsGrid {
					if len(row) != len(cells[i]) {
						return outputErrorTo(errW, fmt.Sprintf("comments cols in row %d (%d) don't match value cols (%d)", i, len(row), len(cells[i])))
					}
				}
				var excelComments []excel.Comment
				startRow, startCol, _ := excel.ParseCellRef(topLeft)
				for r, row := range commentsGrid {
					for c, text := range row {
						if text != "" {
							ref := excel.CellRef(startRow+r, startCol+c)
							excelComments = append(excelComments, excel.Comment{
								Cell: ref, Author: commentAuthor, Text: text,
							})
							commentEntries = append(commentEntries, format.CommentEntry{
								Cell: ref, Author: commentAuthor, Text: text,
							})
						}
					}
				}
				if len(excelComments) > 0 {
					if err := excel.SetComments(xlsxPath, sheet, excelComments); err != nil {
						return outputErrorTo(errW, fmt.Sprintf("setting comments: %v", err))
					}
				}
			}

			seq, err := lastSequence(xhp)
			if err != nil {
				return outputErrorTo(errW, fmt.Sprintf("reading sequence: %v", err))
			}
			seq++

			rows := len(cells)
			cols := 0
			if rows > 0 {
				cols = len(cells[0])
			}
			var flat []format.Cell
			for _, row := range cells {
				flat = append(flat, row...)
			}

			displayRange := topLeft
			if topLeft != bottomRight {
				displayRange = topLeft + ":" + bottomRight
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
				Action:     format.ActionWrite,
				Sheet:      sheet,
				Range:      displayRange,
				Message:    cmd.String("message"),
				NumRows:    uint32(rows),
				NumCols:    uint32(cols),
				Cells:      flat,
				Comments:   commentEntries,
			}
			if err := w.WriteOp(op); err != nil {
				return outputErrorTo(errW, fmt.Sprintf("writing op: %v", err))
			}

			cellCount := 0
			for _, row := range cells {
				cellCount += len(row)
			}
			result := map[string]any{"seq": seq, "cells_written": cellCount}
			if len(commentEntries) > 0 {
				result["comments_written"] = len(commentEntries)
			}
			return outputJSON(cmdOut(cmd), result)
		},
	}
}

func resolveJSONInput(cmd *cli.Command) ([]byte, error) {
	if v := cmd.String("json"); v != "" {
		return []byte(v), nil
	}
	if v := cmd.String("file"); v != "" {
		return os.ReadFile(v)
	}
	if cmd.Bool("stdin") {
		return io.ReadAll(os.Stdin)
	}
	return nil, fmt.Errorf("range write requires --json, --file, or --stdin")
}

func parseJSONGrid(data []byte) ([][]format.Cell, error) {
	var raw [][]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	grid := make([][]format.Cell, len(raw))
	for r, row := range raw {
		grid[r] = make([]format.Cell, len(row))
		for c, v := range row {
			grid[r][c] = jsonToCell(v)
		}
	}
	return grid, nil
}

func parseScalarValue(s string) format.Cell {
	if s == "" {
		return format.Cell{Type: format.CellEmpty}
	}
	if len(s) > 0 && s[0] == '=' {
		return format.Cell{Type: format.CellFormula, FormulaText: s[1:]}
	}
	if n, err := strconv.ParseFloat(s, 64); err == nil {
		return format.Cell{Type: format.CellNumber, Number: n}
	}
	if s == "true" {
		return format.Cell{Type: format.CellBool, Bool: true}
	}
	if s == "false" {
		return format.Cell{Type: format.CellBool, Bool: false}
	}
	return format.Cell{Type: format.CellString, String: s}
}
