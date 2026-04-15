package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	urfcli "github.com/urfave/cli/v3"
)

func runCLI(t *testing.T, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	var outBuf, errBuf bytes.Buffer
	cmd := buildRootCommand()
	cmd.Writer = &outBuf
	cmd.ErrWriter = &errBuf
	cmd.ExitErrHandler = func(_ context.Context, _ *urfcli.Command, _ error) {}
	fullArgs := append([]string{"xhist"}, args...)
	err = cmd.Run(context.Background(), fullArgs)
	return outBuf.String(), errBuf.String(), err
}

func parseJSON(t *testing.T, s string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		t.Fatalf("failed to parse JSON %q: %v", s, err)
	}
	return m
}

func parseJSONArray(t *testing.T, s string) []any {
	t.Helper()
	var a []any
	if err := json.Unmarshal([]byte(s), &a); err != nil {
		t.Fatalf("failed to parse JSON array %q: %v", s, err)
	}
	return a
}

func TestFullWorkflow(t *testing.T) {
	dir := t.TempDir()
	xlsx := filepath.Join(dir, "test.xlsx")

	// init
	out, _, err := runCLI(t, "init", xlsx)
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	m := parseJSON(t, out)
	if _, ok := m["created"]; !ok {
		t.Fatalf("init: expected 'created' key, got %v", m)
	}

	// write single cell
	out, _, err = runCLI(t, "write", "--message", "set A1", xlsx, "Sheet1!A1", "hello")
	if err != nil {
		t.Fatalf("write single: %v", err)
	}
	m = parseJSON(t, out)
	if seq, ok := m["seq"].(float64); !ok || seq != 1 {
		t.Fatalf("write single: expected seq=1, got %v", m["seq"])
	}
	if cw, ok := m["cells_written"].(float64); !ok || cw != 1 {
		t.Fatalf("write single: expected cells_written=1, got %v", m["cells_written"])
	}

	// write range
	out, _, err = runCLI(t, "write", "--message", "fill range", "--json", `[["a","b"],[1,2]]`, xlsx, "Sheet1!A2:B3")
	if err != nil {
		t.Fatalf("write range: %v", err)
	}
	m = parseJSON(t, out)
	if seq, ok := m["seq"].(float64); !ok || seq != 2 {
		t.Fatalf("write range: expected seq=2, got %v", m["seq"])
	}
	if cw, ok := m["cells_written"].(float64); !ok || cw != 4 {
		t.Fatalf("write range: expected cells_written=4, got %v", m["cells_written"])
	}

	// read single cell
	out, _, err = runCLI(t, "read", xlsx, "Sheet1!A1")
	if err != nil {
		t.Fatalf("read single: %v", err)
	}
	trimmed := strings.TrimSpace(out)
	if trimmed != `"hello"` {
		t.Fatalf("read single: expected \"hello\", got %q", trimmed)
	}

	// read range
	out, _, err = runCLI(t, "read", xlsx, "Sheet1!A2:B3")
	if err != nil {
		t.Fatalf("read range: %v", err)
	}
	grid := parseJSONArray(t, out)
	if len(grid) != 2 {
		t.Fatalf("read range: expected 2 rows, got %d", len(grid))
	}

	// log
	out, _, err = runCLI(t, "log", xlsx)
	if err != nil {
		t.Fatalf("log: %v", err)
	}
	ops := parseJSONArray(t, out)
	if len(ops) < 3 {
		t.Fatalf("log: expected at least 3 ops (2 writes + 1 read with log), got %d", len(ops))
	}

	// show op 1
	out, _, err = runCLI(t, "show", xlsx, "1")
	if err != nil {
		t.Fatalf("show: %v", err)
	}
	m = parseJSON(t, out)
	if m["seq"].(float64) != 1 {
		t.Fatalf("show: expected seq=1, got %v", m["seq"])
	}
	if m["action"] != "write" {
		t.Fatalf("show: expected action=write, got %v", m["action"])
	}

	// state
	out, _, err = runCLI(t, "state", xlsx)
	if err != nil {
		t.Fatalf("state: %v", err)
	}
	m = parseJSON(t, out)
	if _, ok := m["Sheet1"]; !ok {
		t.Fatalf("state: expected Sheet1, got keys %v", m)
	}

	// verify
	out, _, err = runCLI(t, "verify", xlsx)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	m = parseJSON(t, out)
	if m["ok"] != true {
		t.Fatalf("verify: expected ok=true, got %v", m)
	}

	// reindex
	out, _, err = runCLI(t, "reindex", xlsx)
	if err != nil {
		t.Fatalf("reindex: %v", err)
	}
	m = parseJSON(t, out)
	if _, ok := m["entries"]; !ok {
		t.Fatalf("reindex: expected entries key, got %v", m)
	}

	// info
	out, _, err = runCLI(t, "info", xlsx)
	if err != nil {
		t.Fatalf("info: %v", err)
	}
	m = parseJSON(t, out)
	if m["target"] != "test.xlsx" {
		t.Fatalf("info: expected target=test.xlsx, got %v", m["target"])
	}
	writes := m["writes"].(float64)
	if writes < 2 {
		t.Fatalf("info: expected at least 2 writes, got %v", writes)
	}
}

func TestAutoInit(t *testing.T) {
	dir := t.TempDir()
	xlsx := filepath.Join(dir, "auto.xlsx")

	out, _, err := runCLI(t, "write", "--message", "auto-init write", xlsx, "Sheet1!A1", "42")
	if err != nil {
		t.Fatalf("write with auto-init: %v", err)
	}
	m := parseJSON(t, out)
	if m["seq"].(float64) != 1 {
		t.Fatalf("expected seq=1, got %v", m["seq"])
	}

	if _, err := os.Stat(xlsx); err != nil {
		t.Fatalf("xlsx not created: %v", err)
	}
	xhp := strings.TrimSuffix(xlsx, ".xlsx") + ".xhist"
	if _, err := os.Stat(xhp); err != nil {
		t.Fatalf("xhist not created: %v", err)
	}

	out, _, err = runCLI(t, "read", xlsx, "Sheet1!A1")
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	trimmed := strings.TrimSpace(out)
	if trimmed != "42" {
		t.Fatalf("expected 42, got %q", trimmed)
	}

	out, _, err = runCLI(t, "log", xlsx)
	if err != nil {
		t.Fatalf("log: %v", err)
	}
	ops := parseJSONArray(t, out)
	found := false
	for _, op := range ops {
		entry := op.(map[string]any)
		if entry["action"] == "write" && entry["message"] == "auto-init write" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("log does not contain auto-init write op")
	}
}

func TestWriteRequiresMessage(t *testing.T) {
	dir := t.TempDir()
	xlsx := filepath.Join(dir, "test.xlsx")
	runCLI(t, "init", xlsx)

	_, _, err := runCLI(t, "write", xlsx, "Sheet1!A1", "hello")
	if err == nil {
		t.Fatalf("expected error when --message not provided")
	}
}

func TestReadNoLog(t *testing.T) {
	dir := t.TempDir()
	xlsx := filepath.Join(dir, "test.xlsx")
	runCLI(t, "init", xlsx)
	runCLI(t, "write", "--message", "seed", xlsx, "Sheet1!A1", "val")

	_, _, err := runCLI(t, "read", "--no-log", xlsx, "Sheet1!A1")
	if err != nil {
		t.Fatalf("read --no-log: %v", err)
	}

	out, _, err := runCLI(t, "log", xlsx)
	if err != nil {
		t.Fatalf("log: %v", err)
	}
	ops := parseJSONArray(t, out)
	for _, op := range ops {
		entry := op.(map[string]any)
		if entry["action"] == "read" {
			t.Fatalf("log should not contain read op when --no-log used, found: %v", entry)
		}
	}
}

func TestLogFilters(t *testing.T) {
	dir := t.TempDir()
	xlsx := filepath.Join(dir, "test.xlsx")
	runCLI(t, "init", xlsx)

	runCLI(t, "write", "--message", "w1", xlsx, "Sheet1!A1", "1")
	runCLI(t, "write", "--message", "w2", xlsx, "Sheet1!A2", "2")
	runCLI(t, "write", "--message", "w3", xlsx, "Sheet1!A3", "3")
	runCLI(t, "read", xlsx, "Sheet1!A1")
	runCLI(t, "read", xlsx, "Sheet1!A2")

	// filter --action write
	out, _, err := runCLI(t, "log", "--action", "write", xlsx)
	if err != nil {
		t.Fatalf("log --action write: %v", err)
	}
	ops := parseJSONArray(t, out)
	for _, op := range ops {
		entry := op.(map[string]any)
		if entry["action"] != "write" {
			t.Fatalf("expected only write ops, got action=%v", entry["action"])
		}
	}
	if len(ops) != 3 {
		t.Fatalf("expected 3 write ops, got %d", len(ops))
	}

	// filter --action read
	out, _, err = runCLI(t, "log", "--action", "read", xlsx)
	if err != nil {
		t.Fatalf("log --action read: %v", err)
	}
	ops = parseJSONArray(t, out)
	for _, op := range ops {
		entry := op.(map[string]any)
		if entry["action"] != "read" {
			t.Fatalf("expected only read ops, got action=%v", entry["action"])
		}
	}
	if len(ops) != 2 {
		t.Fatalf("expected 2 read ops, got %d", len(ops))
	}

	// filter --last 2
	out, _, err = runCLI(t, "log", "--last", "2", xlsx)
	if err != nil {
		t.Fatalf("log --last 2: %v", err)
	}
	ops = parseJSONArray(t, out)
	if len(ops) != 2 {
		t.Fatalf("expected 2 ops with --last 2, got %d", len(ops))
	}

	// filter --sheet Sheet1
	out, _, err = runCLI(t, "log", "--sheet", "Sheet1", xlsx)
	if err != nil {
		t.Fatalf("log --sheet: %v", err)
	}
	ops = parseJSONArray(t, out)
	if len(ops) != 5 {
		t.Fatalf("expected 5 ops on Sheet1, got %d", len(ops))
	}

	// filter --sheet NonExistent
	out, _, err = runCLI(t, "log", "--sheet", "NonExistent", xlsx)
	if err != nil {
		t.Fatalf("log --sheet NonExistent: %v", err)
	}
	ops = parseJSONArray(t, out)
	if len(ops) != 0 {
		t.Fatalf("expected 0 ops on NonExistent sheet, got %d", len(ops))
	}
}

func TestVerifyAndRepair(t *testing.T) {
	dir := t.TempDir()
	xlsx := filepath.Join(dir, "test.xlsx")
	runCLI(t, "init", xlsx)
	runCLI(t, "write", "--message", "w1", xlsx, "Sheet1!A1", "1")
	runCLI(t, "write", "--message", "w2", xlsx, "Sheet1!A2", "2")

	// verify passes initially
	out, _, err := runCLI(t, "verify", xlsx)
	if err != nil {
		t.Fatalf("verify (pre-corruption): %v", err)
	}
	m := parseJSON(t, out)
	if m["ok"] != true {
		t.Fatalf("verify: expected ok=true")
	}

	// corrupt the xhist file by flipping bytes near the end
	xhp := strings.TrimSuffix(xlsx, ".xlsx") + ".xhist"
	data, err := os.ReadFile(xhp)
	if err != nil {
		t.Fatalf("reading xhist: %v", err)
	}
	corruptIdx := len(data) - 5
	if corruptIdx > 0 {
		data[corruptIdx] ^= 0xFF
		data[corruptIdx-1] ^= 0xFF
	}
	if err := os.WriteFile(xhp, data, 0644); err != nil {
		t.Fatalf("writing corrupted xhist: %v", err)
	}

	// verify detects corruption
	out, _, err = runCLI(t, "verify", xlsx)
	if err != nil {
		if out != "" {
			m = parseJSON(t, out)
			if m["ok"] != false {
				t.Fatalf("verify should report ok=false on corruption")
			}
		}
	} else {
		m = parseJSON(t, out)
		if m["ok"] == true {
			t.Fatalf("verify should detect corruption")
		}
	}

	// repair --dry-run
	out, _, err = runCLI(t, "repair", "--dry-run", xlsx)
	if err != nil {
		t.Fatalf("repair --dry-run: %v", err)
	}
	m = parseJSON(t, out)
	if _, ok := m["bytes_removed"]; !ok {
		t.Fatalf("repair --dry-run: expected bytes_removed key")
	}

	// verify file unchanged after dry-run
	dataDryRun, _ := os.ReadFile(xhp)
	if len(dataDryRun) != len(data) {
		t.Fatalf("dry-run should not modify file: was %d, now %d", len(data), len(dataDryRun))
	}

	// actual repair
	out, _, err = runCLI(t, "repair", xlsx)
	if err != nil {
		t.Fatalf("repair: %v", err)
	}
	m = parseJSON(t, out)

	// verify succeeds after repair
	out, _, err = runCLI(t, "verify", xlsx)
	if err != nil {
		t.Fatalf("verify (post-repair): %v", err)
	}
	m = parseJSON(t, out)
	if m["ok"] != true {
		t.Fatalf("verify (post-repair): expected ok=true, got %v", m)
	}
}

func TestStateDiff(t *testing.T) {
	dir := t.TempDir()
	xlsx := filepath.Join(dir, "test.xlsx")
	runCLI(t, "init", xlsx)
	runCLI(t, "write", "--message", "set A1", xlsx, "Sheet1!A1", "10")
	runCLI(t, "write", "--message", "set B1", xlsx, "Sheet1!B1", "20")
	runCLI(t, "write", "--message", "set A2", xlsx, "Sheet1!A2", "30")

	// state shows reconstructed values
	out, _, err := runCLI(t, "state", xlsx)
	if err != nil {
		t.Fatalf("state: %v", err)
	}
	m := parseJSON(t, out)
	sheet1, ok := m["Sheet1"].(map[string]any)
	if !ok {
		t.Fatalf("state: expected Sheet1 in output, got %v", m)
	}
	vals, ok := sheet1["values"].([]any)
	if !ok {
		t.Fatalf("state: expected values array in Sheet1")
	}
	if len(vals) != 2 {
		t.Fatalf("state: expected 2 rows, got %d", len(vals))
	}

	// state --diff should match since we only wrote through CLI
	out, _, err = runCLI(t, "state", "--diff", xlsx)
	if err != nil {
		t.Fatalf("state --diff: %v", err)
	}
	m = parseJSON(t, out)
	if m["match"] != true {
		t.Fatalf("state --diff: expected match=true, got %v", m)
	}
}

func TestSheetsCommand(t *testing.T) {
	dir := t.TempDir()
	xlsx := filepath.Join(dir, "test.xlsx")
	runCLI(t, "init", xlsx)
	runCLI(t, "write", "--message", "data", xlsx, "Sheet1!A1", "hello")

	out, _, err := runCLI(t, "sheets", xlsx)
	if err != nil {
		t.Fatalf("sheets: %v", err)
	}
	sheets := parseJSONArray(t, out)
	if len(sheets) == 0 {
		t.Fatalf("sheets: expected at least 1 sheet")
	}
	first := sheets[0].(map[string]any)
	if first["name"] != "Sheet1" {
		t.Fatalf("sheets: expected Sheet1, got %v", first["name"])
	}

	// verify sheets does not create a log entry
	logOut, _, err := runCLI(t, "log", xlsx)
	if err != nil {
		t.Fatalf("log: %v", err)
	}
	ops := parseJSONArray(t, logOut)
	for _, op := range ops {
		entry := op.(map[string]any)
		if entry["message"] == "sheets" {
			t.Fatalf("sheets command should not create log entry")
		}
	}
}

func TestInitForce(t *testing.T) {
	dir := t.TempDir()
	xlsx := filepath.Join(dir, "test.xlsx")

	_, _, err := runCLI(t, "init", xlsx)
	if err != nil {
		t.Fatalf("first init: %v", err)
	}

	// second init without --force should fail
	_, stderr, err := runCLI(t, "init", xlsx)
	if err == nil {
		t.Fatalf("expected error on second init without --force")
	}
	if !strings.Contains(stderr, "already exists") {
		t.Fatalf("expected 'already exists' in stderr, got %q", stderr)
	}

	// init with --force succeeds
	out, _, err := runCLI(t, "init", "--force", xlsx)
	if err != nil {
		t.Fatalf("init --force: %v", err)
	}
	m := parseJSON(t, out)
	if _, ok := m["created"]; !ok {
		t.Fatalf("init --force: expected 'created' key")
	}
}

func TestInfoMetadata(t *testing.T) {
	dir := t.TempDir()
	xlsx := filepath.Join(dir, "test.xlsx")

	_, _, err := runCLI(t, "init", "--agent", "test-agent", "--model", "gpt-4", "--session", "sess-123", xlsx)
	if err != nil {
		t.Fatalf("init with metadata: %v", err)
	}

	out, _, err := runCLI(t, "info", xlsx)
	if err != nil {
		t.Fatalf("info: %v", err)
	}
	m := parseJSON(t, out)
	meta, ok := m["metadata"].(map[string]any)
	if !ok {
		t.Fatalf("info: expected metadata map, got %v", m["metadata"])
	}
	if meta["agent.name"] != "test-agent" {
		t.Fatalf("info: expected agent.name=test-agent, got %v", meta["agent.name"])
	}
	if meta["agent.model"] != "gpt-4" {
		t.Fatalf("info: expected agent.model=gpt-4, got %v", meta["agent.model"])
	}
	if meta["session.id"] != "sess-123" {
		t.Fatalf("info: expected session.id=sess-123, got %v", meta["session.id"])
	}
}
