package cli

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/narasaka/xhist/internal/excel"
	"github.com/narasaka/xhist/internal/format"
	"github.com/urfave/cli/v3"
)

const (
	maxConfusionEvidenceEntries = 8
	maxConfusionEvidenceBytes   = 16 * 1024
	maxConfusionEvidenceString  = 4096
)

func newConfuseCmd() *cli.Command {
	return &cli.Command{
		Name:  "confuse",
		Usage: "Record spreadsheet confusion and resolution events in the xhist log",
		Commands: []*cli.Command{
			newConfuseRaiseCmd(),
			newConfuseResolveCmd(),
			newConfuseSkipCmd(),
			newConfuseExportCmd(),
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
			&cli.StringFlag{Name: "evidence", Usage: "Required reconciliation evidence as a JSON array"},
			&cli.StringFlag{Name: "payload", Usage: "Additional archetype payload as JSON object"},
			&cli.StringFlag{Name: "dest-table", Usage: "Destination table this confusion relates to"},
			&cli.StringFlag{Name: "source-id", Usage: "Source file identifier"},
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

			defaults := map[string]any{
				"archetype":     strings.ToUpper(cmd.String("archetype")),
				"cell":          qualifiedCell(sheet, cell),
				"headline":      cmd.String("headline"),
				"description":   cmd.String("description"),
				"targetFile":    targetFile,
				"sourceCommand": "xhist confuse raise",
			}
			if destTable := cmd.String("dest-table"); destTable != "" {
				defaults["destTable"] = destTable
			}
			if sourceID := cmd.String("source-id"); sourceID != "" {
				defaults["sourceId"] = sourceID
			}
			if evidence := cmd.String("evidence"); evidence != "" {
				var parsedEvidence []any
				if err := json.Unmarshal([]byte(evidence), &parsedEvidence); err != nil {
					return outputErrorTo(errW, fmt.Sprintf("evidence: %v", err))
				}
				defaults["evidence"] = parsedEvidence
			}

			payload, err := confusionPayload(cmd.String("payload"), defaults)
			if err != nil {
				return outputErrorTo(errW, fmt.Sprintf("payload: %v", err))
			}
			if err := requireConfusionEvidence(payload); err != nil {
				return outputErrorTo(errW, fmt.Sprintf("evidence: %v", err))
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

func newConfuseExportCmd() *cli.Command {
	return &cli.Command{
		Name:  "export",
		Usage: "Export raised confusions for reconciliation",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "format", Usage: "Export format", Value: "reconciliation"},
			&cli.StringFlag{Name: "dest-table", Usage: "Destination table for reconciliation items"},
			&cli.StringFlag{Name: "run-id", Usage: "Architect run ID"},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			_ = ctx
			errW := cmdErr(cmd)
			if cmd.String("format") != "reconciliation" {
				return outputErrorTo(errW, fmt.Sprintf("unsupported export format: %s", cmd.String("format")))
			}

			xhp, _, err := resolveWorkspaceReadOnly(cmd)
			if err != nil {
				return outputErrorTo(errW, fmt.Sprintf("workspace: %v", err))
			}
			if err := requireV2(xhp); err != nil {
				return outputErrorTo(errW, fmt.Sprintf("version: %v", err))
			}

			records, err := scanRecords(xhp)
			if err != nil {
				return outputErrorTo(errW, fmt.Sprintf("reading log: %v", err))
			}

			items := make([]map[string]any, 0)
			for _, record := range records {
				if record.ConfuseOp == nil || record.ConfuseOp.Action != format.ActionConfusionRaise {
					continue
				}
				item, err := reconciliationItem(*record.ConfuseOp, cmd.String("dest-table"), cmd.String("run-id"))
				if err != nil {
					return outputErrorTo(errW, fmt.Sprintf("confusion %s: %v", record.ConfuseOp.ID, err))
				}
				items = append(items, item)
			}

			return outputJSON(cmdOut(cmd), map[string]any{"items": items})
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

func requireConfusionEvidence(payload string) error {
	var obj map[string]any
	if err := json.Unmarshal([]byte(payload), &obj); err != nil {
		return err
	}
	evidence, ok := obj["evidence"].([]any)
	if !ok || len(evidence) == 0 {
		return fmt.Errorf("at least one evidence entry is required")
	}
	if len(evidence) > maxConfusionEvidenceEntries {
		return fmt.Errorf("at most %d evidence entries are allowed", maxConfusionEvidenceEntries)
	}
	evidenceBytes, err := json.Marshal(evidence)
	if err != nil {
		return err
	}
	if len(evidenceBytes) > maxConfusionEvidenceBytes {
		return fmt.Errorf("evidence JSON must be at most %d bytes", maxConfusionEvidenceBytes)
	}
	for i, item := range evidence {
		ev, ok := item.(map[string]any)
		if !ok {
			return fmt.Errorf("entry %d must be an object", i)
		}
		if err := requireBoundedEvidenceValue(ev, "entry "+strconv.Itoa(i), 0); err != nil {
			return err
		}
		ref, ok := evidenceSourceRef(ev)
		if !ok {
			return fmt.Errorf("entry %d must include sourceRef", i)
		}
		if stringValue(ref, "sourceId") == "" && stringValue(ref, "source_id") == "" {
			return fmt.Errorf("entry %d sourceRef must include sourceId", i)
		}
		if _, ok := ref["locator"].(map[string]any); !ok {
			return fmt.Errorf("entry %d sourceRef must include locator", i)
		}
	}
	return nil
}

func requireBoundedEvidenceValue(value any, path string, depth int) error {
	if depth > 5 {
		return fmt.Errorf("%s is nested too deeply", path)
	}
	switch v := value.(type) {
	case string:
		if len(v) > maxConfusionEvidenceString {
			return fmt.Errorf("%s string must be at most %d bytes", path, maxConfusionEvidenceString)
		}
	case []any:
		if len(v) > maxConfusionEvidenceEntries {
			return fmt.Errorf("%s array has too many entries", path)
		}
		for i, item := range v {
			if err := requireBoundedEvidenceValue(item, path+"["+strconv.Itoa(i)+"]", depth+1); err != nil {
				return err
			}
		}
	case map[string]any:
		for key, item := range v {
			if err := requireBoundedEvidenceValue(item, path+"."+key, depth+1); err != nil {
				return err
			}
		}
	}
	return nil
}

func evidenceSourceRef(ev map[string]any) (map[string]any, bool) {
	if ref, ok := ev["sourceRef"].(map[string]any); ok {
		return ref, true
	}
	if ref, ok := ev["source_ref"].(map[string]any); ok {
		return ref, true
	}
	return nil, false
}

func reconciliationItem(op format.ConfuseOp, destTable, runID string) (map[string]any, error) {
	payload, err := confusionPayloadObject(op, runID)
	if err != nil {
		return nil, err
	}
	if destTable == "" {
		destTable = stringValue(payload, "destTable")
	}
	sourceID := stringValue(payload, "sourceId")
	if sourceID == "" {
		sourceID = stringValue(payload, "targetFile")
	}
	if sourceID == "" {
		sourceID = op.TargetFile
	}

	column := columnFromCell(op.Cell)
	contextTag := qualifiedCell(op.Sheet, op.Cell)
	evidence := arrayValue(payload, "evidence")
	primarySource := objectValue(payload, "primary_source")
	if primarySource == nil {
		primarySource = objectValue(payload, "primarySource")
	}
	if primarySource == nil {
		primarySource = map[string]any{
			"source_id": sourceID,
			"locator": map[string]any{
				"kind":  "xlsx",
				"sheet": op.Sheet,
				"range": op.Cell,
			},
		}
	}

	return map[string]any{
		"id":          op.ID,
		"archetype":   "CONFLICT",
		"headline":    firstNonEmpty(op.Headline, stringValue(payload, "headline")),
		"description": firstNonEmpty(op.Description, stringValue(payload, "description")),
		"state":       "escalated",
		"context_tag": contextTag,
		"dest_slot": map[string]any{
			"descriptor": joinDescriptor(destTable, column),
			"label":      column,
		},
		"primary_source": primarySource,
		"evidence":       evidence,
		"payload":        payload,
		"source_field":   contextTag,
	}, nil
}

func confusionPayloadObject(op format.ConfuseOp, runID string) (map[string]any, error) {
	payload := map[string]any{
		"archetype":     op.Archetype,
		"cell":          qualifiedCell(op.Sheet, op.Cell),
		"headline":      op.Headline,
		"description":   op.Description,
		"targetFile":    op.TargetFile,
		"sourceCommand": "xhist confuse raise",
	}
	if op.PayloadJSON != "" {
		if err := json.Unmarshal([]byte(op.PayloadJSON), &payload); err != nil {
			return nil, fmt.Errorf("payload: %v", err)
		}
	}
	if runID != "" {
		payload["runId"] = runID
	}
	return payload, nil
}

func stringValue(obj map[string]any, key string) string {
	if v, ok := obj[key].(string); ok {
		return v
	}
	return ""
}

func objectValue(obj map[string]any, key string) map[string]any {
	if v, ok := obj[key].(map[string]any); ok {
		return v
	}
	return nil
}

func arrayValue(obj map[string]any, key string) []any {
	if v, ok := obj[key].([]any); ok {
		return v
	}
	return []any{}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func columnFromCell(cell string) string {
	for i, r := range cell {
		if r >= '0' && r <= '9' {
			return strings.TrimLeft(cell[:i], "$")
		}
	}
	return strings.TrimLeft(cell, "$")
}

func joinDescriptor(destTable, column string) string {
	if destTable == "" {
		return column
	}
	if column == "" {
		return destTable
	}
	return destTable + "." + column
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
