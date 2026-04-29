package cli

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"hash/crc32"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/prosights/xhist/internal/format"
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

func setupWorkspace(t *testing.T) (dir string, xhist string) {
	t.Helper()
	dir = t.TempDir()
	name := filepath.Base(dir)
	xhist = filepath.Join(dir, name+".xhist")
	f, err := os.Create(xhist)
	if err != nil {
		t.Fatalf("creating workspace: %v", err)
	}
	w, err := format.NewWriter(f)
	if err != nil {
		f.Close()
		t.Fatalf("creating writer: %v", err)
	}
	if err := w.WriteHeader(time.Now().UnixMilli(), name); err != nil {
		f.Close()
		t.Fatalf("writing header: %v", err)
	}
	f.Close()
	return dir, xhist
}

func TestFullWorkflow(t *testing.T) {
	dir, ws := setupWorkspace(t)
	xlsx := filepath.Join(dir, "test.xlsx")

	out, _, err := runCLI(t, "--workspace", ws, "init", "--force")
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	m := parseJSON(t, out)
	if _, ok := m["created"]; !ok {
		t.Fatalf("init: expected 'created' key, got %v", m)
	}

	out, _, err = runCLI(t, "--workspace", ws, "write", "--message", "set A1", xlsx, "Sheet1!A1", "hello")
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

	out, _, err = runCLI(t, "--workspace", ws, "write", "--message", "fill range", "--json", `[["a","b"],[1,2]]`, xlsx, "Sheet1!A2:B3")
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

	out, _, err = runCLI(t, "--workspace", ws, "read", xlsx, "Sheet1!A1")
	if err != nil {
		t.Fatalf("read single: %v", err)
	}
	trimmed := strings.TrimSpace(out)
	if trimmed != `"hello"` {
		t.Fatalf("read single: expected \"hello\", got %q", trimmed)
	}

	out, _, err = runCLI(t, "--workspace", ws, "read", xlsx, "Sheet1!A2:B3")
	if err != nil {
		t.Fatalf("read range: %v", err)
	}
	grid := parseJSONArray(t, out)
	if len(grid) != 2 {
		t.Fatalf("read range: expected 2 rows, got %d", len(grid))
	}

	out, _, err = runCLI(t, "--workspace", ws, "log")
	if err != nil {
		t.Fatalf("log: %v", err)
	}
	ops := parseJSONArray(t, out)
	if len(ops) < 3 {
		t.Fatalf("log: expected at least 3 ops, got %d", len(ops))
	}
	firstOp := ops[0].(map[string]any)
	if _, ok := firstOp["file"]; !ok {
		t.Fatalf("log: expected 'file' field in output")
	}

	out, _, err = runCLI(t, "--workspace", ws, "show", "1")
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
	if _, ok := m["file"]; !ok {
		t.Fatalf("show: expected 'file' field")
	}

	out, _, err = runCLI(t, "--workspace", ws, "state", xlsx)
	if err != nil {
		t.Fatalf("state: %v", err)
	}
	m = parseJSON(t, out)
	if _, ok := m["Sheet1"]; !ok {
		t.Fatalf("state: expected Sheet1, got keys %v", m)
	}

	out, _, err = runCLI(t, "--workspace", ws, "verify")
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	m = parseJSON(t, out)
	if m["ok"] != true {
		t.Fatalf("verify: expected ok=true, got %v", m)
	}

	out, _, err = runCLI(t, "--workspace", ws, "reindex")
	if err != nil {
		t.Fatalf("reindex: %v", err)
	}
	m = parseJSON(t, out)
	if _, ok := m["entries"]; !ok {
		t.Fatalf("reindex: expected entries key, got %v", m)
	}

	out, _, err = runCLI(t, "--workspace", ws, "info")
	if err != nil {
		t.Fatalf("info: %v", err)
	}
	m = parseJSON(t, out)
	if _, ok := m["workspace"]; !ok {
		t.Fatalf("info: expected 'workspace' key, got %v", m)
	}
	writes := m["writes"].(float64)
	if writes < 2 {
		t.Fatalf("info: expected at least 2 writes, got %v", writes)
	}
	if _, ok := m["files"]; !ok {
		t.Fatalf("info: expected 'files' key")
	}
}

func TestAutoInit(t *testing.T) {
	dir, ws := setupWorkspace(t)
	xlsx := filepath.Join(dir, "auto.xlsx")

	out, _, err := runCLI(t, "--workspace", ws, "write", "--message", "auto-init write", xlsx, "Sheet1!A1", "42")
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	m := parseJSON(t, out)
	if m["seq"].(float64) != 1 {
		t.Fatalf("expected seq=1, got %v", m["seq"])
	}

	if _, err := os.Stat(xlsx); err != nil {
		t.Fatalf("xlsx not created: %v", err)
	}

	out, _, err = runCLI(t, "--workspace", ws, "read", xlsx, "Sheet1!A1")
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	trimmed := strings.TrimSpace(out)
	if trimmed != "42" {
		t.Fatalf("expected 42, got %q", trimmed)
	}

	out, _, err = runCLI(t, "--workspace", ws, "log")
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
	dir, ws := setupWorkspace(t)
	xlsx := filepath.Join(dir, "test.xlsx")

	_, _, err := runCLI(t, "--workspace", ws, "write", xlsx, "Sheet1!A1", "hello")
	if err == nil {
		t.Fatalf("expected error when --message not provided")
	}
}

func TestReadNoLog(t *testing.T) {
	dir, ws := setupWorkspace(t)
	xlsx := filepath.Join(dir, "test.xlsx")
	runCLI(t, "--workspace", ws, "write", "--message", "seed", xlsx, "Sheet1!A1", "val")

	_, _, err := runCLI(t, "--workspace", ws, "read", "--no-log", xlsx, "Sheet1!A1")
	if err != nil {
		t.Fatalf("read --no-log: %v", err)
	}

	out, _, err := runCLI(t, "--workspace", ws, "log")
	if err != nil {
		t.Fatalf("log: %v", err)
	}
	ops := parseJSONArray(t, out)
	for _, op := range ops {
		entry := op.(map[string]any)
		if entry["action"] == "read" {
			t.Fatalf("log should not contain read op when --no-log used")
		}
	}
}

func TestLogFilters(t *testing.T) {
	dir, ws := setupWorkspace(t)
	xlsx := filepath.Join(dir, "test.xlsx")

	runCLI(t, "--workspace", ws, "write", "--message", "w1", xlsx, "Sheet1!A1", "1")
	runCLI(t, "--workspace", ws, "write", "--message", "w2", xlsx, "Sheet1!A2", "2")
	runCLI(t, "--workspace", ws, "write", "--message", "w3", xlsx, "Sheet1!A3", "3")
	runCLI(t, "--workspace", ws, "read", xlsx, "Sheet1!A1")
	runCLI(t, "--workspace", ws, "read", xlsx, "Sheet1!A2")

	out, _, err := runCLI(t, "--workspace", ws, "log", "--action", "write")
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

	out, _, err = runCLI(t, "--workspace", ws, "log", "--action", "read")
	if err != nil {
		t.Fatalf("log --action read: %v", err)
	}
	ops = parseJSONArray(t, out)
	if len(ops) != 2 {
		t.Fatalf("expected 2 read ops, got %d", len(ops))
	}

	out, _, err = runCLI(t, "--workspace", ws, "log", "--last", "2")
	if err != nil {
		t.Fatalf("log --last 2: %v", err)
	}
	ops = parseJSONArray(t, out)
	if len(ops) != 2 {
		t.Fatalf("expected 2 ops with --last 2, got %d", len(ops))
	}

	out, _, err = runCLI(t, "--workspace", ws, "log", "--sheet", "Sheet1")
	if err != nil {
		t.Fatalf("log --sheet: %v", err)
	}
	ops = parseJSONArray(t, out)
	if len(ops) != 5 {
		t.Fatalf("expected 5 ops on Sheet1, got %d", len(ops))
	}

	out, _, err = runCLI(t, "--workspace", ws, "log", "--sheet", "NonExistent")
	if err != nil {
		t.Fatalf("log --sheet NonExistent: %v", err)
	}
	ops = parseJSONArray(t, out)
	if len(ops) != 0 {
		t.Fatalf("expected 0 ops on NonExistent sheet, got %d", len(ops))
	}
}

func TestLogActionCommentShorthand(t *testing.T) {
	dir, ws := setupWorkspace(t)
	xlsx := filepath.Join(dir, "test.xlsx")

	runCLI(t, "--workspace", ws, "write", "--message", "w1", xlsx, "Sheet1!A1", "1")
	runCLI(t, "--workspace", ws, "comment", "set", "--message", "c1", xlsx, "Sheet1!A1", "first")
	runCLI(t, "--workspace", ws, "comment", "set", "--message", "c2", xlsx, "Sheet1!A2", "second")
	runCLI(t, "--workspace", ws, "comment", "delete", "--message", "d1", xlsx, "Sheet1!A1")

	shorthandOut, _, err := runCLI(t, "--workspace", ws, "log", "--action", "comment")
	if err != nil {
		t.Fatalf("log --action comment: %v", err)
	}
	shorthandOps := parseJSONArray(t, shorthandOut)
	if len(shorthandOps) != 3 {
		t.Fatalf("expected 3 comment_* ops via --action comment shorthand, got %d", len(shorthandOps))
	}
	for _, op := range shorthandOps {
		entry := op.(map[string]any)
		action, _ := entry["action"].(string)
		if !strings.HasPrefix(action, "comment_") {
			t.Fatalf("expected comment_* action, got %q", action)
		}
	}

	exactOut, _, err := runCLI(t, "--workspace", ws, "log", "--action", "comment_set")
	if err != nil {
		t.Fatalf("log --action comment_set: %v", err)
	}
	exactOps := parseJSONArray(t, exactOut)
	if len(exactOps) != 2 {
		t.Fatalf("expected 2 comment_set ops via exact filter, got %d", len(exactOps))
	}

	writeOnlyOut, _, err := runCLI(t, "--workspace", ws, "log", "--action", "write")
	if err != nil {
		t.Fatalf("log --action write: %v", err)
	}
	writeOnlyOps := parseJSONArray(t, writeOnlyOut)
	if len(writeOnlyOps) != 1 {
		t.Fatalf("expected 1 write op (comment ops must not leak into --action write), got %d", len(writeOnlyOps))
	}
}

func TestVerifyAndRepair(t *testing.T) {
	dir, ws := setupWorkspace(t)
	xlsx := filepath.Join(dir, "test.xlsx")
	runCLI(t, "--workspace", ws, "write", "--message", "w1", xlsx, "Sheet1!A1", "1")
	runCLI(t, "--workspace", ws, "write", "--message", "w2", xlsx, "Sheet1!A2", "2")

	out, _, err := runCLI(t, "--workspace", ws, "verify")
	if err != nil {
		t.Fatalf("verify (pre-corruption): %v", err)
	}
	m := parseJSON(t, out)
	if m["ok"] != true {
		t.Fatalf("verify: expected ok=true")
	}

	data, err := os.ReadFile(ws)
	if err != nil {
		t.Fatalf("reading xhist: %v", err)
	}
	corruptIdx := len(data) - 5
	if corruptIdx > 0 {
		data[corruptIdx] ^= 0xFF
		data[corruptIdx-1] ^= 0xFF
	}
	if err := os.WriteFile(ws, data, 0644); err != nil {
		t.Fatalf("writing corrupted xhist: %v", err)
	}

	out, _, err = runCLI(t, "--workspace", ws, "verify")
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

	out, _, err = runCLI(t, "--workspace", ws, "repair", "--dry-run")
	if err != nil {
		t.Fatalf("repair --dry-run: %v", err)
	}
	m = parseJSON(t, out)
	if _, ok := m["bytes_removed"]; !ok {
		t.Fatalf("repair --dry-run: expected bytes_removed key")
	}

	dataDryRun, _ := os.ReadFile(ws)
	if len(dataDryRun) != len(data) {
		t.Fatalf("dry-run should not modify file")
	}

	out, _, err = runCLI(t, "--workspace", ws, "repair")
	if err != nil {
		t.Fatalf("repair: %v", err)
	}
	_ = parseJSON(t, out)

	out, _, err = runCLI(t, "--workspace", ws, "verify")
	if err != nil {
		t.Fatalf("verify (post-repair): %v", err)
	}
	m = parseJSON(t, out)
	if m["ok"] != true {
		t.Fatalf("verify (post-repair): expected ok=true, got %v", m)
	}
}

func TestStateDiff(t *testing.T) {
	dir, ws := setupWorkspace(t)
	xlsx := filepath.Join(dir, "test.xlsx")
	runCLI(t, "--workspace", ws, "write", "--message", "set A1", xlsx, "Sheet1!A1", "10")
	runCLI(t, "--workspace", ws, "write", "--message", "set B1", xlsx, "Sheet1!B1", "20")
	runCLI(t, "--workspace", ws, "write", "--message", "set A2", xlsx, "Sheet1!A2", "30")

	out, _, err := runCLI(t, "--workspace", ws, "state", xlsx)
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

	out, _, err = runCLI(t, "--workspace", ws, "state", "--diff", xlsx)
	if err != nil {
		t.Fatalf("state --diff: %v", err)
	}
	m = parseJSON(t, out)
	if m["match"] != true {
		t.Fatalf("state --diff: expected match=true, got %v", m)
	}
}

func TestSheetsCommand(t *testing.T) {
	dir, ws := setupWorkspace(t)
	xlsx := filepath.Join(dir, "test.xlsx")
	runCLI(t, "--workspace", ws, "write", "--message", "data", xlsx, "Sheet1!A1", "hello")

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
}

func TestInitForce(t *testing.T) {
	_, ws := setupWorkspace(t)

	_, stderr, err := runCLI(t, "--workspace", ws, "init")
	if err == nil {
		t.Fatalf("expected error on second init without --force")
	}
	if !strings.Contains(stderr, "already exists") {
		t.Fatalf("expected 'already exists' in stderr, got %q", stderr)
	}

	out, _, err := runCLI(t, "--workspace", ws, "init", "--force")
	if err != nil {
		t.Fatalf("init --force: %v", err)
	}
	m := parseJSON(t, out)
	if _, ok := m["created"]; !ok {
		t.Fatalf("init --force: expected 'created' key")
	}
}

func TestInfoMetadata(t *testing.T) {
	_, ws := setupWorkspace(t)

	_, _, err := runCLI(t, "--workspace", ws, "init", "--force", "--agent", "test-agent", "--model", "gpt-4", "--session", "sess-123")
	if err != nil {
		t.Fatalf("init with metadata: %v", err)
	}

	out, _, err := runCLI(t, "--workspace", ws, "info")
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

func TestWriteWithComment(t *testing.T) {
	dir, ws := setupWorkspace(t)
	xlsx := filepath.Join(dir, "test.xlsx")

	out, _, err := runCLI(t, "--workspace", ws, "write", "-m", "with comment", "--comment", "Source: SAP", xlsx, "Sheet1!A1", "1250000")
	if err != nil {
		t.Fatalf("write with comment: %v", err)
	}
	m := parseJSON(t, out)
	if m["seq"].(float64) != 1 {
		t.Fatalf("expected seq=1, got %v", m["seq"])
	}
	if m["comments_written"].(float64) != 1 {
		t.Fatalf("expected comments_written=1, got %v", m["comments_written"])
	}
}

func TestWriteWithCommentsRange(t *testing.T) {
	dir, ws := setupWorkspace(t)
	xlsx := filepath.Join(dir, "test.xlsx")

	out, _, err := runCLI(t, "--workspace", ws, "write", "-m", "range with comments",
		"--json", `[["Revenue","Cost"],["1250000","830000"]]`,
		"--comments", `[["Source: SAP","Source: SAP"],["","Source: Oracle"]]`,
		xlsx, "Sheet1!A1:B2")
	if err != nil {
		t.Fatalf("write with comments: %v", err)
	}
	m := parseJSON(t, out)
	if m["comments_written"].(float64) != 3 {
		t.Fatalf("expected comments_written=3, got %v", m["comments_written"])
	}
}

func TestWriteWithCommentAuthor(t *testing.T) {
	dir, ws := setupWorkspace(t)
	xlsx := filepath.Join(dir, "test.xlsx")

	_, _, err := runCLI(t, "--workspace", ws, "write", "-m", "custom author", "--comment", "test", "--comment-author", "agent-1", xlsx, "Sheet1!A1", "val")
	if err != nil {
		t.Fatalf("write with comment-author: %v", err)
	}
}

func TestWriteCommentOnRangeErrors(t *testing.T) {
	dir, ws := setupWorkspace(t)
	xlsx := filepath.Join(dir, "test.xlsx")

	_, _, err := runCLI(t, "--workspace", ws, "write", "-m", "bad", "--comment", "test",
		"--json", `[["a","b"]]`, xlsx, "Sheet1!A1:B1")
	if err == nil {
		t.Fatal("expected error using --comment with range write")
	}
}

func TestCommentSetAndGet(t *testing.T) {
	dir, ws := setupWorkspace(t)
	xlsx := filepath.Join(dir, "test.xlsx")
	runCLI(t, "--workspace", ws, "write", "-m", "seed", xlsx, "Sheet1!A1", "100")

	out, _, err := runCLI(t, "--workspace", ws, "comment", "set", "-m", "adding source", xlsx, "Sheet1!A1", "Source: SAP report")
	if err != nil {
		t.Fatalf("comment set: %v", err)
	}
	m := parseJSON(t, out)
	if m["comments_written"].(float64) != 1 {
		t.Fatalf("expected comments_written=1, got %v", m["comments_written"])
	}

	out, _, err = runCLI(t, "--workspace", ws, "comment", "get", xlsx, "Sheet1!A1")
	if err != nil {
		t.Fatalf("comment get: %v", err)
	}
	m = parseJSON(t, out)
	if m["text"] != "Source: SAP report" {
		t.Fatalf("expected comment text, got %v", m["text"])
	}
	if m["cell"] != "A1" {
		t.Fatalf("expected cell A1, got %v", m["cell"])
	}
}

func TestCommentSetRange(t *testing.T) {
	dir, ws := setupWorkspace(t)
	xlsx := filepath.Join(dir, "test.xlsx")

	out, _, err := runCLI(t, "--workspace", ws, "comment", "set", "-m", "citations",
		"--json", `[["Source: SAP",""],["Source: Oracle","Source: Bloomberg"]]`,
		xlsx, "Sheet1!A1:B2")
	if err != nil {
		t.Fatalf("comment set range: %v", err)
	}
	m := parseJSON(t, out)
	if m["comments_written"].(float64) != 3 {
		t.Fatalf("expected 3 comments, got %v", m["comments_written"])
	}
}

func TestCommentDelete(t *testing.T) {
	dir, ws := setupWorkspace(t)
	xlsx := filepath.Join(dir, "test.xlsx")
	runCLI(t, "--workspace", ws, "write", "-m", "seed", xlsx, "Sheet1!A1", "100")
	runCLI(t, "--workspace", ws, "comment", "set", "-m", "add", xlsx, "Sheet1!A1", "to delete")

	out, _, err := runCLI(t, "--workspace", ws, "comment", "delete", "-m", "removing", xlsx, "Sheet1!A1")
	if err != nil {
		t.Fatalf("comment delete: %v", err)
	}
	m := parseJSON(t, out)
	if m["comments_deleted"].(float64) != 1 {
		t.Fatalf("expected comments_deleted=1, got %v", m["comments_deleted"])
	}

	out, _, err = runCLI(t, "--workspace", ws, "comment", "get", xlsx, "Sheet1!A1")
	if err != nil {
		t.Fatalf("comment get after delete: %v", err)
	}
	if strings.TrimSpace(out) != "null" {
		t.Fatalf("expected null after delete, got %q", out)
	}
}

func TestCommentGetRange(t *testing.T) {
	dir, ws := setupWorkspace(t)
	xlsx := filepath.Join(dir, "test.xlsx")
	runCLI(t, "--workspace", ws, "comment", "set", "-m", "a", xlsx, "Sheet1!A1", "first")
	runCLI(t, "--workspace", ws, "comment", "set", "-m", "b", xlsx, "Sheet1!B2", "second")
	runCLI(t, "--workspace", ws, "comment", "set", "-m", "c", xlsx, "Sheet1!C3", "outside")

	out, _, err := runCLI(t, "--workspace", ws, "comment", "get", xlsx, "Sheet1!A1:B2")
	if err != nil {
		t.Fatalf("comment get range: %v", err)
	}
	arr := parseJSONArray(t, out)
	if len(arr) != 2 {
		t.Fatalf("expected 2 comments in range, got %d", len(arr))
	}
}

func TestCommentSequenceContinuity(t *testing.T) {
	dir, ws := setupWorkspace(t)
	xlsx := filepath.Join(dir, "test.xlsx")
	runCLI(t, "--workspace", ws, "write", "-m", "w1", xlsx, "Sheet1!A1", "100")

	out, _, err := runCLI(t, "--workspace", ws, "comment", "set", "-m", "c1", xlsx, "Sheet1!A1", "comment")
	if err != nil {
		t.Fatalf("comment set: %v", err)
	}
	m := parseJSON(t, out)
	if m["seq"].(float64) != 2 {
		t.Fatalf("expected seq=2 after write+comment, got %v", m["seq"])
	}

	out, _, err = runCLI(t, "--workspace", ws, "write", "-m", "w2", xlsx, "Sheet1!A2", "200")
	if err != nil {
		t.Fatalf("write after comment: %v", err)
	}
	m = parseJSON(t, out)
	if m["seq"].(float64) != 3 {
		t.Fatalf("expected seq=3, got %v", m["seq"])
	}
}

func TestLogFileFilter(t *testing.T) {
	dir, ws := setupWorkspace(t)
	xlsx1 := filepath.Join(dir, "budget.xlsx")
	xlsx2 := filepath.Join(dir, "forecast.xlsx")

	runCLI(t, "--workspace", ws, "write", "-m", "b1", xlsx1, "Sheet1!A1", "100")
	runCLI(t, "--workspace", ws, "write", "-m", "f1", xlsx2, "Sheet1!A1", "200")
	runCLI(t, "--workspace", ws, "write", "-m", "b2", xlsx1, "Sheet1!A2", "300")

	out, _, err := runCLI(t, "--workspace", ws, "log")
	if err != nil {
		t.Fatalf("log all: %v", err)
	}
	ops := parseJSONArray(t, out)
	if len(ops) != 3 {
		t.Fatalf("expected 3 ops, got %d", len(ops))
	}

	out, _, err = runCLI(t, "--workspace", ws, "log", xlsx1)
	if err != nil {
		t.Fatalf("log filtered: %v", err)
	}
	ops = parseJSONArray(t, out)
	if len(ops) != 2 {
		t.Fatalf("expected 2 ops for budget.xlsx, got %d", len(ops))
	}
	for _, op := range ops {
		entry := op.(map[string]any)
		if entry["file"] != "budget.xlsx" {
			t.Fatalf("expected file=budget.xlsx, got %v", entry["file"])
		}
	}
}

func TestShowWithoutFileArg(t *testing.T) {
	dir, ws := setupWorkspace(t)
	xlsx := filepath.Join(dir, "test.xlsx")
	runCLI(t, "--workspace", ws, "write", "-m", "w1", xlsx, "Sheet1!A1", "hello")

	out, _, err := runCLI(t, "--workspace", ws, "show", "1")
	if err != nil {
		t.Fatalf("show: %v", err)
	}
	m := parseJSON(t, out)
	if m["seq"].(float64) != 1 {
		t.Fatalf("show: expected seq=1")
	}
	if m["file"] != "test.xlsx" {
		t.Fatalf("show: expected file=test.xlsx, got %v", m["file"])
	}
}

func TestInfoWithoutFileArg(t *testing.T) {
	dir, ws := setupWorkspace(t)
	xlsx := filepath.Join(dir, "test.xlsx")
	runCLI(t, "--workspace", ws, "write", "-m", "w1", xlsx, "Sheet1!A1", "hello")

	out, _, err := runCLI(t, "--workspace", ws, "info")
	if err != nil {
		t.Fatalf("info: %v", err)
	}
	m := parseJSON(t, out)
	if _, ok := m["workspace"]; !ok {
		t.Fatalf("info: expected workspace key")
	}
	files := m["files"].([]any)
	if len(files) != 1 {
		t.Fatalf("info: expected 1 file, got %d", len(files))
	}
}

func TestV1RefusalOnWrite(t *testing.T) {
	dir := t.TempDir()
	ws := filepath.Join(dir, "old.xhist")
	xlsx := filepath.Join(dir, "test.xlsx")

	f, _ := os.Create(ws)
	var preamble [7]byte
	copy(preamble[:6], []byte("XHIST\x00"))
	preamble[6] = 0x01
	f.Write(preamble[:])
	f.Close()

	_, stderr, err := runCLI(t, "--workspace", ws, "write", "-m", "test", xlsx, "Sheet1!A1", "val")
	if err == nil {
		t.Fatal("expected error writing to v1 log")
	}
	if !strings.Contains(stderr, "v1 log detected") {
		t.Fatalf("expected v1 log error, got %q", stderr)
	}
}

func TestV1ReadOnlyCommands(t *testing.T) {
	dir := t.TempDir()
	ws := filepath.Join(dir, "old.xhist")

	f, _ := os.Create(ws)
	var preamble [7]byte
	copy(preamble[:6], []byte("XHIST\x00"))
	preamble[6] = 0x01
	f.Write(preamble[:])

	hdr := format.Header{CreatedAt: time.Now().UnixMilli(), TargetFile: "test.xlsx"}
	payload := makeV1HeaderPayload(hdr)
	rec := makeFramedRecord(0x01, payload)
	f.Write(rec)
	f.Close()

	out, _, err := runCLI(t, "--workspace", ws, "verify")
	if err != nil {
		t.Fatalf("verify on v1: %v", err)
	}
	m := parseJSON(t, out)
	if m["ok"] != true {
		t.Fatalf("verify on v1: expected ok=true")
	}

	out, _, err = runCLI(t, "--workspace", ws, "log")
	if err != nil {
		t.Fatalf("log on v1: %v", err)
	}
	ops := parseJSONArray(t, out)
	if len(ops) != 0 {
		t.Fatalf("log on v1: expected 0 ops, got %d", len(ops))
	}

	out, _, err = runCLI(t, "--workspace", ws, "info")
	if err != nil {
		t.Fatalf("info on v1: %v", err)
	}
	_ = parseJSON(t, out)
}

func TestWorkspaceFlag(t *testing.T) {
	dir, ws := setupWorkspace(t)
	xlsx := filepath.Join(dir, "test.xlsx")

	out, _, err := runCLI(t, "--workspace", ws, "write", "-m", "explicit workspace", xlsx, "Sheet1!A1", "hello")
	if err != nil {
		t.Fatalf("write with --workspace: %v", err)
	}
	m := parseJSON(t, out)
	if m["seq"].(float64) != 1 {
		t.Fatalf("expected seq=1, got %v", m["seq"])
	}
}

func makeV1HeaderPayload(h format.Header) []byte {
	var buf bytes.Buffer
	var tmp [8]byte
	binary.LittleEndian.PutUint64(tmp[:], uint64(h.CreatedAt))
	buf.Write(tmp[:])
	binary.LittleEndian.PutUint32(tmp[:4], uint32(len(h.TargetFile)))
	buf.Write(tmp[:4])
	buf.WriteString(h.TargetFile)
	return buf.Bytes()
}

func makeFramedRecord(opcode uint8, payload []byte) []byte {
	total := 1 + 4 + len(payload) + 4
	out := make([]byte, total)
	out[0] = opcode
	binary.LittleEndian.PutUint32(out[1:5], uint32(len(payload)))
	copy(out[5:5+len(payload)], payload)
	crc := crc32.ChecksumIEEE(out[:5+len(payload)])
	binary.LittleEndian.PutUint32(out[5+len(payload):], crc)
	return out
}
