package cli

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/narasaka/xhist/internal/excel"
	"github.com/narasaka/xhist/internal/format"
	"github.com/urfave/cli/v3"
)

func newConfuseCmd() *cli.Command {
	return &cli.Command{
		Name:  "confuse",
		Usage: "Record spreadsheet confusion and resolution events in the xhist log",
		Commands: []*cli.Command{
			newConfuseRaiseCmd(),
			newConfuseResolveCmd(),
			newConfuseSkipCmd(),
		},
	}
}

func newConfuseRaiseCmd() *cli.Command {
	return &cli.Command{
		Name:  "raise",
		Usage: "Raise a cell-level confusion",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "id", Usage: "Confusion id (default: generated)"},
			&cli.StringFlag{Name: "archetype", Aliases: []string{"a"}, Usage: "Confusion archetype", Required: true},
			&cli.StringFlag{Name: "headline", Usage: "Short summary", Required: true},
			&cli.StringFlag{Name: "description", Aliases: []string{"d"}, Usage: "Detailed explanation", Required: true},
			&cli.StringFlag{Name: "payload", Usage: "Additional archetype payload as JSON object"},
			&cli.StringFlag{Name: "message", Aliases: []string{"m"}, Usage: "Why this confusion was raised"},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			_ = ctx
			errW := cmdErr(cmd)
			xlsxPath := cmd.Args().Get(0)
			if xlsxPath == "" {
				return outputErrorTo(errW, "missing required argument: <file.xlsx>")
			}
			cellRef := cmd.Args().Get(1)
			if cellRef == "" {
				return outputErrorTo(errW, "missing required argument: <cell>")
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

			sheet, cell, err := parseSingleCell(cellRef)
			if err != nil {
				return outputErrorTo(errW, fmt.Sprintf("parsing cell: %v", err))
			}

			payload, err := confusionPayload(cmd.String("payload"), map[string]any{
				"archetype":     strings.ToUpper(cmd.String("archetype")),
				"cell":          qualifiedCell(sheet, cell),
				"headline":      cmd.String("headline"),
				"description":   cmd.String("description"),
				"targetFile":    targetFile,
				"sourceCommand": "xhist confuse raise",
			})
			if err != nil {
				return outputErrorTo(errW, fmt.Sprintf("payload: %v", err))
			}

			id := cmd.String("id")
			if id == "" {
				id, err = newConfusionID()
				if err != nil {
					return outputErrorTo(errW, fmt.Sprintf("generating id: %v", err))
				}
			}

			seq, err := lastSequence(xhp)
			if err != nil {
				return outputErrorTo(errW, fmt.Sprintf("reading sequence: %v", err))
			}
			seq++

			f, w, err := openLogForAppend(xhp)
			if err != nil {
				return outputErrorTo(errW, fmt.Sprintf("opening log: %v", err))
			}
			defer f.Close()

			op := format.ConfuseOp{
				TargetFile:  targetFile,
				Timestamp:   time.Now().UnixMilli(),
				Sequence:    seq,
				Action:      format.ActionConfusionRaise,
				ID:          id,
				Sheet:       sheet,
				Cell:        cell,
				Archetype:   strings.ToUpper(cmd.String("archetype")),
				Headline:    cmd.String("headline"),
				Description: cmd.String("description"),
				PayloadJSON: payload,
				Message:     cmd.String("message"),
			}
			if err := w.WriteConfuseOp(op); err != nil {
				return outputErrorTo(errW, fmt.Sprintf("writing confuse op: %v", err))
			}

			return outputJSON(cmdOut(cmd), map[string]any{"seq": seq, "id": id, "action": "confusion_raise"})
		},
	}
}

func newConfuseResolveCmd() *cli.Command {
	return &cli.Command{
		Name:  "resolve",
		Usage: "Resolve a confusion",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "value", Usage: "Resolved value"},
			&cli.StringFlag{Name: "confidence", Usage: "Resolution confidence (high|medium|low)", Required: true},
			&cli.StringFlag{Name: "reasoning", Aliases: []string{"r"}, Usage: "Resolution reasoning", Required: true},
			&cli.StringFlag{Name: "source", Usage: "Resolution source (auto|human|retry)", Value: "human"},
			&cli.StringFlag{Name: "message", Aliases: []string{"m"}, Usage: "Why this resolution was recorded"},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			_ = ctx
			return appendConfusionDecision(cmd, format.ActionConfusionResolve)
		},
	}
}

func newConfuseSkipCmd() *cli.Command {
	return &cli.Command{
		Name:  "skip",
		Usage: "Mark a confusion as skipped",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "reasoning", Aliases: []string{"r"}, Usage: "Skip reasoning", Required: true},
			&cli.StringFlag{Name: "source", Usage: "Decision source (auto|human|retry)", Value: "human"},
			&cli.StringFlag{Name: "message", Aliases: []string{"m"}, Usage: "Why this skip was recorded"},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			_ = ctx
			return appendConfusionDecision(cmd, format.ActionConfusionSkip)
		},
	}
}

func appendConfusionDecision(cmd *cli.Command, action uint8) error {
	errW := cmdErr(cmd)
	id := cmd.Args().Get(0)
	if id == "" {
		return outputErrorTo(errW, "missing required argument: <id>")
	}

	xhp, _, err := resolveWorkspace(cmd)
	if err != nil {
		return outputErrorTo(errW, fmt.Sprintf("workspace: %v", err))
	}
	if err := requireV2(xhp); err != nil {
		return outputErrorTo(errW, fmt.Sprintf("version: %v", err))
	}

	prior, err := findConfusion(xhp, id)
	if err != nil {
		return outputErrorTo(errW, fmt.Sprintf("reading confusion: %v", err))
	}
	if prior == nil {
		return outputErrorTo(errW, fmt.Sprintf("confusion not found: %s", id))
	}

	resolution, err := json.Marshal(map[string]any{
		"value":      cmd.String("value"),
		"confidence": cmd.String("confidence"),
		"reasoning":  cmd.String("reasoning"),
		"source":     cmd.String("source"),
	})
	if err != nil {
		return outputErrorTo(errW, fmt.Sprintf("encoding resolution: %v", err))
	}

	seq, err := lastSequence(xhp)
	if err != nil {
		return outputErrorTo(errW, fmt.Sprintf("reading sequence: %v", err))
	}
	seq++

	f, w, err := openLogForAppend(xhp)
	if err != nil {
		return outputErrorTo(errW, fmt.Sprintf("opening log: %v", err))
	}
	defer f.Close()

	op := *prior
	op.Timestamp = time.Now().UnixMilli()
	op.Sequence = seq
	op.Action = action
	op.ResolutionJSON = string(resolution)
	op.Message = cmd.String("message")
	if err := w.WriteConfuseOp(op); err != nil {
		return outputErrorTo(errW, fmt.Sprintf("writing confuse op: %v", err))
	}

	return outputJSON(cmdOut(cmd), map[string]any{"seq": seq, "id": id, "action": confusionActionString(action)})
}

func parseSingleCell(ref string) (string, string, error) {
	sheet, topLeft, bottomRight, err := excel.ParseRange(ref)
	if err != nil {
		return "", "", err
	}
	if topLeft != bottomRight {
		return "", "", fmt.Errorf("expected a single cell, got range %s:%s", topLeft, bottomRight)
	}
	if _, _, err := excel.ParseCellRef(topLeft); err != nil {
		return "", "", err
	}
	return sheet, topLeft, nil
}

func qualifiedCell(sheet, cell string) string {
	if sheet == "" {
		return cell
	}
	return sheet + "!" + cell
}

func confusionPayload(raw string, defaults map[string]any) (string, error) {
	if raw == "" {
		b, err := json.Marshal(defaults)
		return string(b), err
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(raw), &obj); err != nil {
		return "", err
	}
	for k, v := range defaults {
		if _, ok := obj[k]; !ok {
			obj[k] = v
		}
	}
	b, err := json.Marshal(obj)
	return string(b), err
}

func newConfusionID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

func findConfusion(xhp, id string) (*format.ConfuseOp, error) {
	records, err := scanRecords(xhp)
	if err != nil {
		return nil, err
	}
	for i := len(records) - 1; i >= 0; i-- {
		if records[i].ConfuseOp != nil && records[i].ConfuseOp.ID == id {
			op := *records[i].ConfuseOp
			return &op, nil
		}
	}
	return nil, nil
}
