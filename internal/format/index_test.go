package format

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func writeTestXhistFile(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "test.xhist")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	w, err := NewWriter(f)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.WriteHeader(1000, "data.xlsx"); err != nil {
		t.Fatal(err)
	}
	if err := w.WriteMetadata("key", "val"); err != nil {
		t.Fatal(err)
	}
	if err := w.WriteOp(Op{
		Timestamp: 2000, Sequence: 1, Action: ActionRead,
		Sheet: "Sheet1", Range: "A1", Message: "first",
		NumRows: 1, NumCols: 1, Cells: []Cell{{Type: CellString, String: "x"}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := w.WriteOp(Op{
		Timestamp: 3000, Sequence: 2, Action: ActionWrite,
		Sheet: "データ", Range: "B1:C3", Message: "second",
		NumRows: 1, NumCols: 1, Cells: []Cell{{Type: CellNumber, Number: 99}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := w.WriteFooter(2, 2); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeTestXhistFileWithComments(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "test.xhist")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	w, err := NewWriter(f)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.WriteHeader(1000, "data.xlsx"); err != nil {
		t.Fatal(err)
	}
	if err := w.WriteOp(Op{
		Timestamp: 2000, Sequence: 1, Action: ActionRead,
		Sheet: "Sheet1", Range: "A1", Message: "first",
		NumRows: 1, NumCols: 1, Cells: []Cell{{Type: CellString, String: "x"}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := w.WriteCommentOp(CommentOp{
		Timestamp: 3000, Sequence: 2, Action: ActionCommentSet,
		Sheet: "Sheet1", Range: "A1", Message: "add comment",
		NumEntries: 1, Entries: []CommentEntry{{Cell: "A1", Author: "alice", Text: "note"}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := w.WriteOp(Op{
		Timestamp: 4000, Sequence: 3, Action: ActionWrite,
		Sheet: "Sheet2", Range: "B1", Message: "write",
		NumRows: 1, NumCols: 1, Cells: []Cell{{Type: CellNumber, Number: 42}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := w.WriteFooter(3, 3); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestBuildAndReadIndex(t *testing.T) {
	dir := t.TempDir()
	xhistPath := writeTestXhistFile(t, dir)

	if err := BuildIndex(xhistPath); err != nil {
		t.Fatal(err)
	}

	idxPath := xhistPath + ".idx"
	entries, err := ReadIndex(idxPath)
	if err != nil {
		t.Fatal(err)
	}

	if len(entries) != 2 {
		t.Fatalf("entry count = %d, want 2", len(entries))
	}

	e1 := entries[0]
	if e1.Timestamp != 2000 || e1.Sequence != 1 || e1.Action != ActionRead {
		t.Fatalf("entry 0 = %+v", e1)
	}
	if e1.Sheet != "Sheet1" {
		t.Fatalf("entry 0 sheet = %q", e1.Sheet)
	}

	e2 := entries[1]
	if e2.Timestamp != 3000 || e2.Sequence != 2 || e2.Action != ActionWrite {
		t.Fatalf("entry 1 = %+v", e2)
	}
	if e2.Sheet != "データ" {
		t.Fatalf("entry 1 sheet = %q (unicode)", e2.Sheet)
	}
}

func TestIndexPreambleByteLayout(t *testing.T) {
	dir := t.TempDir()
	xhistPath := writeTestXhistFile(t, dir)
	BuildIndex(xhistPath)

	idxData, err := os.ReadFile(xhistPath + ".idx")
	if err != nil {
		t.Fatal(err)
	}

	if len(idxData) < IndexPreambleSize {
		t.Fatalf("index too small: %d bytes", len(idxData))
	}

	for i := 0; i < 6; i++ {
		if idxData[i] != IndexMagic[i] {
			t.Fatalf("index magic[%d] = 0x%02x, want 0x%02x", i, idxData[i], IndexMagic[i])
		}
	}
	if idxData[6] != IndexVersion {
		t.Fatalf("index version = 0x%02x", idxData[6])
	}

	logSize := binary.LittleEndian.Uint64(idxData[7:15])
	fi, _ := os.Stat(xhistPath)
	if logSize != uint64(fi.Size()) {
		t.Fatalf("logSize = %d, actual = %d", logSize, fi.Size())
	}

	entryCount := binary.LittleEndian.Uint32(idxData[15:19])
	if entryCount != 2 {
		t.Fatalf("entryCount = %d, want 2", entryCount)
	}
}

func TestIndexEntryByteLayout(t *testing.T) {
	dir := t.TempDir()
	xhistPath := writeTestXhistFile(t, dir)
	BuildIndex(xhistPath)

	idxData, err := os.ReadFile(xhistPath + ".idx")
	if err != nil {
		t.Fatal(err)
	}

	entryData := idxData[IndexPreambleSize:]

	offset := binary.LittleEndian.Uint64(entryData[0:8])
	if offset <= uint64(PreambleSize) {
		t.Fatalf("first entry offset = %d, should be > preamble", offset)
	}

	opcode := entryData[8]
	if opcode != OpcodeOp {
		t.Fatalf("opcode = 0x%02x, want OpcodeOp", opcode)
	}

	timestamp := int64(binary.LittleEndian.Uint64(entryData[9:17]))
	if timestamp != 2000 {
		t.Fatalf("timestamp = %d, want 2000", timestamp)
	}

	seq := binary.LittleEndian.Uint32(entryData[17:21])
	if seq != 1 {
		t.Fatalf("sequence = %d, want 1", seq)
	}

	action := entryData[21]
	if action != ActionRead {
		t.Fatalf("action = %d, want ActionRead", action)
	}

	// v3: TargetFile comes before Sheet
	fileLen := binary.LittleEndian.Uint16(entryData[22:24])
	off := 24 + int(fileLen)
	sheetLen := binary.LittleEndian.Uint16(entryData[off : off+2])
	if sheetLen != 6 {
		t.Fatalf("sheetLen = %d, want 6", sheetLen)
	}
	sheet := string(entryData[off+2 : off+2+int(sheetLen)])
	if sheet != "Sheet1" {
		t.Fatalf("sheet = %q", sheet)
	}
}

func TestIsStaleNotStale(t *testing.T) {
	dir := t.TempDir()
	xhistPath := writeTestXhistFile(t, dir)
	BuildIndex(xhistPath)

	stale, err := IsStale(xhistPath+".idx", xhistPath)
	if err != nil {
		t.Fatal(err)
	}
	if stale {
		t.Fatal("index should not be stale right after build")
	}
}

func TestIsStaleAfterAppend(t *testing.T) {
	dir := t.TempDir()
	xhistPath := writeTestXhistFile(t, dir)
	BuildIndex(xhistPath)

	// append more data to make it stale
	f, _ := os.OpenFile(xhistPath, os.O_APPEND|os.O_WRONLY, 0644)
	f.Write([]byte("extra data"))
	f.Close()

	stale, err := IsStale(xhistPath+".idx", xhistPath)
	if err != nil {
		t.Fatal(err)
	}
	if !stale {
		t.Fatal("index should be stale after appending data")
	}
}

func TestBuildIndexNoOps(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.xhist")
	f, _ := os.Create(path)
	w, _ := NewWriter(f)
	w.WriteHeader(1000, "empty.xlsx")
	w.WriteFooter(0, 0)
	f.Close()

	if err := BuildIndex(path); err != nil {
		t.Fatal(err)
	}

	entries, err := ReadIndex(path + ".idx")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected 0 entries, got %d", len(entries))
	}
}

func TestReadIndexBadMagic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.xhist.idx")
	os.WriteFile(path, bytes.Repeat([]byte{0}, IndexPreambleSize), 0644)

	_, err := ReadIndex(path)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestIndexRebuild(t *testing.T) {
	dir := t.TempDir()
	xhistPath := writeTestXhistFile(t, dir)

	BuildIndex(xhistPath)
	entries1, _ := ReadIndex(xhistPath + ".idx")

	// rebuild should produce identical results
	BuildIndex(xhistPath)
	entries2, _ := ReadIndex(xhistPath + ".idx")

	if len(entries1) != len(entries2) {
		t.Fatalf("entry count mismatch after rebuild: %d vs %d", len(entries1), len(entries2))
	}
	for i := range entries1 {
		if entries1[i].Offset != entries2[i].Offset ||
			entries1[i].Sequence != entries2[i].Sequence ||
			entries1[i].Sheet != entries2[i].Sheet {
			t.Fatalf("entry %d mismatch after rebuild", i)
		}
	}
}

func TestBuildAndReadIndexWithComments(t *testing.T) {
	dir := t.TempDir()
	xhistPath := writeTestXhistFileWithComments(t, dir)

	if err := BuildIndex(xhistPath); err != nil {
		t.Fatal(err)
	}

	entries, err := ReadIndex(xhistPath + ".idx")
	if err != nil {
		t.Fatal(err)
	}

	if len(entries) != 3 {
		t.Fatalf("entry count = %d, want 3", len(entries))
	}

	if entries[0].Opcode != OpcodeOp || entries[0].Sequence != 1 {
		t.Fatalf("entry 0 = %+v", entries[0])
	}
	if entries[1].Opcode != OpcodeCommentOp || entries[1].Sequence != 2 || entries[1].Action != ActionCommentSet {
		t.Fatalf("entry 1 = %+v", entries[1])
	}
	if entries[2].Opcode != OpcodeOp || entries[2].Sequence != 3 || entries[2].Sheet != "Sheet2" {
		t.Fatalf("entry 2 = %+v", entries[2])
	}
}

func TestIndexEntryV2ByteLayout(t *testing.T) {
	dir := t.TempDir()
	xhistPath := writeTestXhistFileWithComments(t, dir)
	BuildIndex(xhistPath)

	idxData, err := os.ReadFile(xhistPath + ".idx")
	if err != nil {
		t.Fatal(err)
	}

	if idxData[6] != IndexVersionV3 {
		t.Fatalf("index version = 0x%02x, want 0x%02x", idxData[6], IndexVersionV3)
	}

	entryCount := binary.LittleEndian.Uint32(idxData[15:19])
	if entryCount != 3 {
		t.Fatalf("entryCount = %d, want 3", entryCount)
	}

	// Skip first entry to find second: fixed 22 bytes + fileLen(2) + file + sheetLen(2) + sheet
	pos := IndexPreambleSize
	// First entry: read fileLen at pos+22
	fileLen1 := binary.LittleEndian.Uint16(idxData[pos+22 : pos+24])
	sheetLen1Off := pos + 24 + int(fileLen1)
	sheetLen1 := binary.LittleEndian.Uint16(idxData[sheetLen1Off : sheetLen1Off+2])
	pos = sheetLen1Off + 2 + int(sheetLen1)

	entry2 := idxData[pos:]

	opcode := entry2[8]
	if opcode != OpcodeCommentOp {
		t.Fatalf("second entry opcode = 0x%02x, want OpcodeCommentOp", opcode)
	}

	ts := int64(binary.LittleEndian.Uint64(entry2[9:17]))
	if ts != 3000 {
		t.Fatalf("second entry timestamp = %d, want 3000", ts)
	}

	seq := binary.LittleEndian.Uint32(entry2[17:21])
	if seq != 2 {
		t.Fatalf("second entry sequence = %d, want 2", seq)
	}

	action := entry2[21]
	if action != ActionCommentSet {
		t.Fatalf("second entry action = %d, want ActionCommentSet", action)
	}
}

func TestIndexV3WithTargetFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.xhist")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}

	w, _ := NewWriter(f)
	w.WriteHeader(1000, "my-workspace")
	w.WriteOp(Op{
		TargetFile: "file1.xlsx",
		Timestamp:  2000, Sequence: 1, Action: ActionRead,
		Sheet: "Sheet1", Range: "A1", Message: "read",
		NumRows: 1, NumCols: 1, Cells: []Cell{{Type: CellString, String: "x"}},
	})
	w.WriteOp(Op{
		TargetFile: "file2.xlsx",
		Timestamp:  3000, Sequence: 2, Action: ActionWrite,
		Sheet: "Sheet2", Range: "B1", Message: "write",
		NumRows: 1, NumCols: 1, Cells: []Cell{{Type: CellNumber, Number: 42}},
	})
	w.WriteCommentOp(CommentOp{
		TargetFile: "file1.xlsx",
		Timestamp:  4000, Sequence: 3, Action: ActionCommentSet,
		Sheet: "Sheet1", Range: "A1", Message: "comment",
		NumEntries: 1, Entries: []CommentEntry{{Cell: "A1", Author: "bob", Text: "note"}},
	})
	f.Close()

	if err := BuildIndex(path); err != nil {
		t.Fatal(err)
	}

	entries, err := ReadIndex(path + ".idx")
	if err != nil {
		t.Fatal(err)
	}

	if len(entries) != 3 {
		t.Fatalf("entry count = %d, want 3", len(entries))
	}

	if entries[0].TargetFile != "file1.xlsx" || entries[0].Sheet != "Sheet1" {
		t.Fatalf("entry 0 = %+v", entries[0])
	}
	if entries[1].TargetFile != "file2.xlsx" || entries[1].Sheet != "Sheet2" {
		t.Fatalf("entry 1 = %+v", entries[1])
	}
	if entries[2].TargetFile != "file1.xlsx" || entries[2].Opcode != OpcodeCommentOp {
		t.Fatalf("entry 2 = %+v", entries[2])
	}
}

func TestIndexPreambleV3Version(t *testing.T) {
	dir := t.TempDir()
	xhistPath := writeTestXhistFile(t, dir)
	BuildIndex(xhistPath)

	idxData, err := os.ReadFile(xhistPath + ".idx")
	if err != nil {
		t.Fatal(err)
	}
	if idxData[6] != IndexVersionV3 {
		t.Fatalf("index version = 0x%02x, want 0x%02x", idxData[6], IndexVersionV3)
	}
}
