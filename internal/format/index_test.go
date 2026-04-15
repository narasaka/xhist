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

	timestamp := int64(binary.LittleEndian.Uint64(entryData[8:16]))
	if timestamp != 2000 {
		t.Fatalf("timestamp = %d, want 2000", timestamp)
	}

	seq := binary.LittleEndian.Uint32(entryData[16:20])
	if seq != 1 {
		t.Fatalf("sequence = %d, want 1", seq)
	}

	action := entryData[20]
	if action != ActionRead {
		t.Fatalf("action = %d, want ActionRead", action)
	}

	sheetLen := binary.LittleEndian.Uint16(entryData[21:23])
	if sheetLen != 6 {
		t.Fatalf("sheetLen = %d, want 6", sheetLen)
	}
	sheet := string(entryData[23 : 23+sheetLen])
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
