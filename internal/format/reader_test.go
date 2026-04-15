package format

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"testing"
)

func writeTestFile(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	w, err := NewWriter(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.WriteHeader(1000, "test.xlsx"); err != nil {
		t.Fatal(err)
	}
	if err := w.WriteMetadata("agent.name", "test"); err != nil {
		t.Fatal(err)
	}
	if err := w.WriteOp(Op{
		Timestamp: 2000,
		Sequence:  1,
		Action:    ActionRead,
		Sheet:     "Sheet1",
		Range:     "A1",
		Message:   "read it",
		NumRows:   1,
		NumCols:   1,
		Cells:     []Cell{{Type: CellString, String: "hello"}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := w.WriteOp(Op{
		Timestamp: 3000,
		Sequence:  2,
		Action:    ActionWrite,
		Sheet:     "Sheet1",
		Range:     "B1",
		Message:   "write it",
		NumRows:   1,
		NumCols:   1,
		Cells:     []Cell{{Type: CellNumber, Number: 42}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := w.WriteFooter(2, 2); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestReaderPreambleValidation(t *testing.T) {
	data := writeTestFile(t)
	rd, err := NewReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if rd == nil {
		t.Fatal("reader is nil")
	}
}

func TestReaderBadMagic(t *testing.T) {
	data := make([]byte, PreambleSize)
	copy(data, []byte("WRONG\x00"))
	data[6] = Version
	_, err := NewReader(bytes.NewReader(data))
	if err == nil {
		t.Fatal("expected error")
	}
	var badMagic *ErrBadMagic
	if !errors.As(err, &badMagic) {
		t.Fatalf("expected ErrBadMagic, got %T: %v", err, err)
	}
}

func TestReaderBadVersion(t *testing.T) {
	data := make([]byte, PreambleSize)
	copy(data[:6], Magic[:])
	data[6] = 0x99
	_, err := NewReader(bytes.NewReader(data))
	if err == nil {
		t.Fatal("expected error")
	}
	var badVer *ErrBadVersion
	if !errors.As(err, &badVer) {
		t.Fatalf("expected ErrBadVersion, got %T: %v", err, err)
	}
	if badVer.Got != 0x99 {
		t.Fatalf("got version = 0x%02x", badVer.Got)
	}
}

func TestReaderEmptyFile(t *testing.T) {
	_, err := NewReader(bytes.NewReader(nil))
	if err == nil {
		t.Fatal("expected error for empty file")
	}
}

func TestReaderAllRecords(t *testing.T) {
	data := writeTestFile(t)
	rd, _ := NewReader(bytes.NewReader(data))

	// Header
	rec, err := rd.Next()
	if err != nil {
		t.Fatal(err)
	}
	if rec.Opcode != OpcodeHeader {
		t.Fatalf("expected header, got 0x%02x", rec.Opcode)
	}
	hdr := rec.Parsed.(Header)
	if hdr.CreatedAt != 1000 || hdr.TargetFile != "test.xlsx" {
		t.Fatalf("header = %+v", hdr)
	}

	// Metadata
	rec, err = rd.Next()
	if err != nil {
		t.Fatal(err)
	}
	if rec.Opcode != OpcodeMetadata {
		t.Fatalf("expected metadata, got 0x%02x", rec.Opcode)
	}
	meta := rec.Parsed.(Metadata)
	if meta.Key != "agent.name" || meta.Value != "test" {
		t.Fatalf("metadata = %+v", meta)
	}

	// Op 1
	rec, err = rd.Next()
	if err != nil {
		t.Fatal(err)
	}
	op := rec.Parsed.(Op)
	if op.Timestamp != 2000 || op.Sequence != 1 || op.Action != ActionRead {
		t.Fatalf("op1 = %+v", op)
	}
	if op.Sheet != "Sheet1" || op.Range != "A1" || op.Message != "read it" {
		t.Fatalf("op1 strings = %+v", op)
	}
	if len(op.Cells) != 1 || op.Cells[0].Type != CellString || op.Cells[0].String != "hello" {
		t.Fatalf("op1 cells = %+v", op.Cells)
	}

	// Op 2
	rec, err = rd.Next()
	if err != nil {
		t.Fatal(err)
	}
	op = rec.Parsed.(Op)
	if op.Sequence != 2 || op.Action != ActionWrite {
		t.Fatalf("op2 = %+v", op)
	}

	// Footer
	rec, err = rd.Next()
	if err != nil {
		t.Fatal(err)
	}
	foot := rec.Parsed.(Footer)
	if foot.OpCount != 2 || foot.LastSequence != 2 {
		t.Fatalf("footer = %+v", foot)
	}

	// EOF
	_, err = rd.Next()
	if err != io.EOF {
		t.Fatalf("expected EOF, got %v", err)
	}
}

func TestReaderCRCCorruption(t *testing.T) {
	data := writeTestFile(t)

	// corrupt a payload byte in the first record (header)
	// header record starts at offset 7
	corruptOffset := PreambleSize + 5 + 2 // inside payload
	data[corruptOffset] ^= 0xFF

	rd, _ := NewReader(bytes.NewReader(data))
	_, err := rd.Next()
	if err == nil {
		t.Fatal("expected corruption error")
	}
	var corr *ErrCorruption
	if !errors.As(err, &corr) {
		t.Fatalf("expected ErrCorruption, got %T: %v", err, err)
	}
	if corr.Offset != PreambleSize {
		t.Fatalf("corruption offset = %d, want %d", corr.Offset, PreambleSize)
	}
}

func TestReaderPartialTrailingRecord(t *testing.T) {
	data := writeTestFile(t)

	// truncate mid-record: cut off last 5 bytes
	truncated := data[:len(data)-5]

	rd, _ := NewReader(bytes.NewReader(truncated))
	var count int
	for {
		_, err := rd.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		count++
	}
	// should get header + metadata + 2 ops = 4, footer is truncated
	if count != 4 {
		t.Fatalf("read %d records, want 4", count)
	}
}

func TestReaderPartialHeader(t *testing.T) {
	// only preamble + 3 bytes of a record (incomplete opcode+length)
	data := make([]byte, PreambleSize+3)
	copy(data[:6], Magic[:])
	data[6] = Version
	data[7] = OpcodeHeader
	data[8] = 0x10
	data[9] = 0x00

	rd, _ := NewReader(bytes.NewReader(data))
	_, err := rd.Next()
	if err != io.EOF {
		t.Fatalf("expected EOF for partial header, got %v", err)
	}
}

func TestReaderUnknownOpcode(t *testing.T) {
	var buf bytes.Buffer
	w, _ := NewWriter(&buf)
	w.WriteHeader(1000, "test.xlsx")

	// manually inject an unknown opcode record (opcode 0x7F)
	payload := []byte("unknown data")
	rec := encodeRecord(0x7F, payload)
	buf.Write(rec)

	// then write a metadata record after it
	w2 := &Writer{w: &buf}
	w2.WriteMetadata("key", "val")

	data := buf.Bytes()
	rd, _ := NewReader(bytes.NewReader(data))

	// header
	r, err := rd.Next()
	if err != nil {
		t.Fatal(err)
	}
	if r.Opcode != OpcodeHeader {
		t.Fatal("expected header")
	}

	// unknown opcode — should be read successfully but Parsed is nil
	r, err = rd.Next()
	if err != nil {
		t.Fatalf("unknown opcode should not error: %v", err)
	}
	if r.Opcode != 0x7F {
		t.Fatalf("expected opcode 0x7F, got 0x%02x", r.Opcode)
	}
	if r.Parsed != nil {
		t.Fatal("unknown opcode should have nil Parsed")
	}

	// metadata after unknown
	r, err = rd.Next()
	if err != nil {
		t.Fatal(err)
	}
	if r.Opcode != OpcodeMetadata {
		t.Fatal("expected metadata after unknown")
	}
}

func TestReaderFormulaCell(t *testing.T) {
	var buf bytes.Buffer
	w, _ := NewWriter(&buf)
	w.WriteHeader(1000, "test.xlsx")
	w.WriteOp(Op{
		Timestamp: 2000,
		Sequence:  1,
		Action:    ActionRead,
		Sheet:     "Sheet1",
		Range:     "A1",
		Message:   "",
		NumRows:   1,
		NumCols:   1,
		Cells: []Cell{{
			Type:        CellFormula,
			FormulaText: "=SUM(B1:B10)",
			CachedValue: &Cell{Type: CellNumber, Number: 100},
		}},
	})

	rd, _ := NewReader(bytes.NewReader(buf.Bytes()))
	rd.Next() // header
	rec, err := rd.Next()
	if err != nil {
		t.Fatal(err)
	}
	op := rec.Parsed.(Op)
	cell := op.Cells[0]
	if cell.Type != CellFormula || cell.FormulaText != "=SUM(B1:B10)" {
		t.Fatalf("formula cell wrong: %+v", cell)
	}
	if cell.CachedValue.Number != 100 {
		t.Fatalf("cached value wrong: %+v", cell.CachedValue)
	}
}

func TestWriterReaderRoundTripAllCellTypes(t *testing.T) {
	cells := []Cell{
		{Type: CellEmpty},
		{Type: CellString, String: "hello"},
		{Type: CellNumber, Number: 3.14},
		{Type: CellBool, Bool: true},
		{Type: CellFormula, FormulaText: "=A1+B1", CachedValue: &Cell{Type: CellNumber, Number: 5}},
		{Type: CellError, Error: "#REF!"},
	}

	var buf bytes.Buffer
	w, _ := NewWriter(&buf)
	w.WriteHeader(1000, "test.xlsx")
	w.WriteOp(Op{
		Timestamp: 2000,
		Sequence:  1,
		Action:    ActionWrite,
		Sheet:     "Sheet1",
		Range:     "A1:F1",
		Message:   "all types",
		NumRows:   1,
		NumCols:   6,
		Cells:     cells,
	})

	rd, _ := NewReader(bytes.NewReader(buf.Bytes()))
	rd.Next() // header
	rec, _ := rd.Next()
	op := rec.Parsed.(Op)

	if len(op.Cells) != 6 {
		t.Fatalf("cell count = %d, want 6", len(op.Cells))
	}
	if op.Cells[0].Type != CellEmpty {
		t.Fatal("cell 0 wrong")
	}
	if op.Cells[1].String != "hello" {
		t.Fatal("cell 1 wrong")
	}
	if op.Cells[2].Number != 3.14 {
		t.Fatal("cell 2 wrong")
	}
	if !op.Cells[3].Bool {
		t.Fatal("cell 3 wrong")
	}
	if op.Cells[4].FormulaText != "=A1+B1" || op.Cells[4].CachedValue.Number != 5 {
		t.Fatal("cell 4 wrong")
	}
	if op.Cells[5].Error != "#REF!" {
		t.Fatal("cell 5 wrong")
	}
}

func TestReaderCRCCoversOpcodeAndLength(t *testing.T) {
	data := writeTestFile(t)

	// corrupt the opcode byte of the first record
	data[PreambleSize] ^= 0x10

	rd, _ := NewReader(bytes.NewReader(data))
	_, err := rd.Next()
	var corr *ErrCorruption
	if !errors.As(err, &corr) {
		t.Fatalf("changing opcode should cause CRC error, got %T: %v", err, err)
	}
}

func TestReaderCRCCoversLengthField(t *testing.T) {
	data := writeTestFile(t)

	// corrupt a length byte of the first record
	data[PreambleSize+1] ^= 0x01

	rd, _ := NewReader(bytes.NewReader(data))
	_, err := rd.Next()
	// this will either cause CRC error or a read error (if length becomes huge)
	if err == nil {
		t.Fatal("expected error from corrupted length")
	}
}

func TestRecordFrameOverhead(t *testing.T) {
	payload := []byte("test")
	frame := encodeRecord(OpcodeMetadata, payload)
	// 1 opcode + 4 length + 4 payload + 4 CRC = 13
	if len(frame) != 1+4+4+4 {
		t.Fatalf("frame size = %d, want 13", len(frame))
	}
	if frame[0] != OpcodeMetadata {
		t.Fatal("opcode wrong")
	}
	storedLen := binary.LittleEndian.Uint32(frame[1:5])
	if storedLen != 4 {
		t.Fatalf("stored length = %d, want 4", storedLen)
	}
}
