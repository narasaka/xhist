package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/prosights/xhist/internal/excel"
	"github.com/prosights/xhist/internal/format"
	"github.com/prosights/xhist/internal/lock"
	"github.com/urfave/cli/v3"
)

func newCommentCmd() *cli.Command {
	return &cli.Command{
		Name:  "comment",
		Usage: "Manage cell comments",
		Commands: []*cli.Command{
			newCommentSetCmd(),
			newCommentGetCmd(),
			newCommentDeleteCmd(),
		},
	}
}

func newCommentSetCmd() *cli.Command {
	return &cli.Command{
		Name:  "set",
		Usage: "Set comments on cells",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "message", Aliases: []string{"m"}, Usage: "Why this operation was performed", Required: true},
			&cli.StringFlag{Name: "json", Usage: "Comments as JSON array of arrays for range"},
			&cli.StringFlag{Name: "author", Usage: "Comment author (default: xhist)"},
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

			xhp, err := ensureInit(xlsxPath)
			if err != nil {
				return outputErrorTo(errW, fmt.Sprintf("init: %v", err))
			}

			sheet, topLeft, bottomRight, err := excel.ParseRange(rangeRef)
			if err != nil {
				return outputErrorTo(errW, fmt.Sprintf("parsing range: %v", err))
			}

			author := cmd.String("author")
			if author == "" {
				author = "xhist"
			}

			isSingle := topLeft == bottomRight
			var entries []format.CommentEntry

			if isSingle {
				text := cmd.Args().Get(2)
				if text == "" && cmd.String("json") == "" {
					return outputErrorTo(errW, "missing comment text (positional arg or --json)")
				}
				if text != "" {
					if err := setCommentWithLock(ctx, xhp, xlsxPath, sheet, topLeft, author, text); err != nil {
						return outputErrorTo(errW, err.Error())
					}
					entries = append(entries, format.CommentEntry{Cell: topLeft, Author: author, Text: text})
				}
			}

			if cmd.String("json") != "" {
				var commentsGrid [][]string
				if err := json.Unmarshal([]byte(cmd.String("json")), &commentsGrid); err != nil {
					return outputErrorTo(errW, fmt.Sprintf("parsing comments JSON: %v", err))
				}
				startRow, startCol, err := excel.ParseCellRef(topLeft)
				if err != nil {
					return outputErrorTo(errW, fmt.Sprintf("parsing range: %v", err))
				}
				var excelComments []excel.Comment
				for r, row := range commentsGrid {
					for c, text := range row {
						if text != "" {
							ref := excel.CellRef(startRow+r, startCol+c)
							excelComments = append(excelComments, excel.Comment{
								Cell: ref, Author: author, Text: text,
							})
							entries = append(entries, format.CommentEntry{
								Cell: ref, Author: author, Text: text,
							})
						}
					}
				}
				if len(excelComments) > 0 {
					ul, err := lock.Acquire(ctx, xhp, xlsxPath)
					if err != nil {
						return outputErrorTo(errW, fmt.Sprintf("acquiring lock: %v", err))
					}
					defer ul.Release()
					if err := excel.SetComments(xlsxPath, sheet, excelComments); err != nil {
						return outputErrorTo(errW, fmt.Sprintf("setting comments: %v", err))
					}
				}
			}

			if len(entries) == 0 {
				return outputErrorTo(errW, "no comments to set")
			}

			seq, err := lastSequence(xhp)
			if err != nil {
				return outputErrorTo(errW, fmt.Sprintf("reading sequence: %v", err))
			}
			seq++

			displayRange := topLeft
			if topLeft != bottomRight {
				displayRange = topLeft + ":" + bottomRight
			}

			f, w, err := openLogForAppend(xhp)
			if err != nil {
				return outputErrorTo(errW, fmt.Sprintf("opening log: %v", err))
			}
			defer f.Close()

			cop := format.CommentOp{
				Timestamp:  time.Now().UnixMilli(),
				Sequence:   seq,
				Action:     format.ActionCommentSet,
				Sheet:      sheet,
				Range:      displayRange,
				Message:    cmd.String("message"),
				NumEntries: uint32(len(entries)),
				Entries:    entries,
			}
			if err := w.WriteCommentOp(cop); err != nil {
				return outputErrorTo(errW, fmt.Sprintf("writing comment op: %v", err))
			}

			return outputJSON(cmdOut(cmd), map[string]any{
				"seq":              seq,
				"comments_written": len(entries),
			})
		},
	}
}

func setCommentWithLock(ctx context.Context, xhp, xlsxPath, sheet, cell, author, text string) error {
	ul, err := lock.Acquire(ctx, xhp, xlsxPath)
	if err != nil {
		return fmt.Errorf("acquiring lock: %v", err)
	}
	defer ul.Release()
	return excel.SetComment(xlsxPath, sheet, cell, author, text)
}

func newCommentGetCmd() *cli.Command {
	return &cli.Command{
		Name:  "get",
		Usage: "Get comments from cells",
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

			xhp, err := ensureInit(xlsxPath)
			if err != nil {
				return outputErrorTo(errW, fmt.Sprintf("init: %v", err))
			}

			sheet, topLeft, bottomRight, err := excel.ParseRange(rangeRef)
			if err != nil {
				return outputErrorTo(errW, fmt.Sprintf("parsing range: %v", err))
			}

			ul, err := lock.Acquire(ctx, xhp, xlsxPath)
			if err != nil {
				return outputErrorTo(errW, fmt.Sprintf("acquiring lock: %v", err))
			}
			defer ul.Release()

			allComments, err := excel.GetComments(xlsxPath, sheet)
			if err != nil {
				return outputErrorTo(errW, fmt.Sprintf("getting comments: %v", err))
			}

			isSingle := topLeft == bottomRight
			var filtered []excel.Comment

			if isSingle {
				for _, c := range allComments {
					if c.Cell == topLeft {
						filtered = append(filtered, c)
					}
				}
			} else {
				startRow, startCol, _ := excel.ParseCellRef(topLeft)
				endRow, endCol, _ := excel.ParseCellRef(bottomRight)
				for _, c := range allComments {
					cr, cc, err := excel.ParseCellRef(c.Cell)
					if err != nil {
						continue
					}
					if cr >= startRow && cr <= endRow && cc >= startCol && cc <= endCol {
						filtered = append(filtered, c)
					}
				}
			}

			outW := cmdOut(cmd)
			if isSingle {
				if len(filtered) == 0 {
					return outputJSON(outW, nil)
				}
				c := filtered[0]
				return outputJSON(outW, map[string]any{
					"cell": c.Cell, "author": c.Author, "text": c.Text,
				})
			}

			result := make([]map[string]any, len(filtered))
			for i, c := range filtered {
				result[i] = map[string]any{
					"cell": c.Cell, "author": c.Author, "text": c.Text,
				}
			}
			return outputJSON(outW, result)
		},
	}
}

func newCommentDeleteCmd() *cli.Command {
	return &cli.Command{
		Name:  "delete",
		Usage: "Delete comments from cells",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "message", Aliases: []string{"m"}, Usage: "Why this operation was performed", Required: true},
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

			xhp, err := ensureInit(xlsxPath)
			if err != nil {
				return outputErrorTo(errW, fmt.Sprintf("init: %v", err))
			}

			sheet, topLeft, bottomRight, err := excel.ParseRange(rangeRef)
			if err != nil {
				return outputErrorTo(errW, fmt.Sprintf("parsing range: %v", err))
			}

			ul, err := lock.Acquire(ctx, xhp, xlsxPath)
			if err != nil {
				return outputErrorTo(errW, fmt.Sprintf("acquiring lock: %v", err))
			}
			defer ul.Release()

			allComments, err := excel.GetComments(xlsxPath, sheet)
			if err != nil {
				return outputErrorTo(errW, fmt.Sprintf("getting comments: %v", err))
			}

			isSingle := topLeft == bottomRight
			var toDelete []string
			var entries []format.CommentEntry

			if isSingle {
				for _, c := range allComments {
					if c.Cell == topLeft {
						toDelete = append(toDelete, c.Cell)
						entries = append(entries, format.CommentEntry{Cell: c.Cell})
					}
				}
			} else {
				startRow, startCol, _ := excel.ParseCellRef(topLeft)
				endRow, endCol, _ := excel.ParseCellRef(bottomRight)
				for _, c := range allComments {
					cr, cc, err := excel.ParseCellRef(c.Cell)
					if err != nil {
						continue
					}
					if cr >= startRow && cr <= endRow && cc >= startCol && cc <= endCol {
						toDelete = append(toDelete, c.Cell)
						entries = append(entries, format.CommentEntry{Cell: c.Cell})
					}
				}
			}

			if len(toDelete) > 0 {
				if err := excel.DeleteComments(xlsxPath, sheet, toDelete); err != nil {
					return outputErrorTo(errW, fmt.Sprintf("deleting comments: %v", err))
				}
			}

			seq, err := lastSequence(xhp)
			if err != nil {
				return outputErrorTo(errW, fmt.Sprintf("reading sequence: %v", err))
			}
			seq++

			displayRange := topLeft
			if topLeft != bottomRight {
				displayRange = topLeft + ":" + bottomRight
			}

			f, w, err := openLogForAppend(xhp)
			if err != nil {
				return outputErrorTo(errW, fmt.Sprintf("opening log: %v", err))
			}
			defer f.Close()

			cop := format.CommentOp{
				Timestamp:  time.Now().UnixMilli(),
				Sequence:   seq,
				Action:     format.ActionCommentDelete,
				Sheet:      sheet,
				Range:      displayRange,
				Message:    cmd.String("message"),
				NumEntries: uint32(len(entries)),
				Entries:    entries,
			}
			if err := w.WriteCommentOp(cop); err != nil {
				return outputErrorTo(errW, fmt.Sprintf("writing comment op: %v", err))
			}

			return outputJSON(cmdOut(cmd), map[string]any{
				"seq":              seq,
				"comments_deleted": len(entries),
			})
		},
	}
}
