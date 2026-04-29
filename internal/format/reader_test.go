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
	data[6] = VersionLatest
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
	if hdr.CreatedAt != 1000 || hdr.WorkspaceName != "test.xlsx" {
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
	data := make([]byte, PreambleSize+3)
	copy(data[:6], Magic[:])
	data[6] = VersionLatest
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

func TestReaderCommentOp(t *testing.T) {
	var buf bytes.Buffer
	w, _ := NewWriter(&buf)
	w.WriteHeader(1000, "test.xlsx")
	cop := CommentOp{
		Timestamp:  4000,
		Sequence:   1,
		Action:     ActionCommentSet,
		Sheet:      "Sheet1",
		Range:      "A1",
		Message:    "set comment",
		NumEntries: 1,
		Entries:    []CommentEntry{{Cell: "A1", Author: "bob", Text: "note"}},
	}
	w.WriteCommentOp(cop)

	rd, _ := NewReader(bytes.NewReader(buf.Bytes()))
	rd.Next() // header
	rec, err := rd.Next()
	if err != nil {
		t.Fatal(err)
	}
	if rec.Opcode != OpcodeCommentOp {
		t.Fatalf("opcode = 0x%02x, want OpcodeCommentOp", rec.Opcode)
	}
	parsed := rec.Parsed.(CommentOp)
	if parsed.Timestamp != 4000 || parsed.Sequence != 1 || parsed.Action != ActionCommentSet {
		t.Fatalf("comment op = %+v", parsed)
	}
	if parsed.Sheet != "Sheet1" || parsed.Range != "A1" || parsed.Message != "set comment" {
		t.Fatalf("comment op strings = %+v", parsed)
	}
	if len(parsed.Entries) != 1 || parsed.Entries[0].Cell != "A1" || parsed.Entries[0].Author != "bob" || parsed.Entries[0].Text != "note" {
		t.Fatalf("comment entries = %+v", parsed.Entries)
	}
}

func TestReaderOpWithComments(t *testing.T) {
	var buf bytes.Buffer
	w, _ := NewWriter(&buf)
	w.WriteHeader(1000, "test.xlsx")
	w.WriteOp(Op{
		Timestamp: 2000, Sequence: 1, Action: ActionRead,
		Sheet: "Sheet1", Range: "A1", Message: "read",
		NumRows: 1, NumCols: 1,
		Cells:    []Cell{{Type: CellString, String: "val"}},
		Comments: []CommentEntry{{Cell: "A1", Author: "alice", Text: "important"}},
	})

	rd, _ := NewReader(bytes.NewReader(buf.Bytes()))
	rd.Next() // header
	rec, err := rd.Next()
	if err != nil {
		t.Fatal(err)
	}
	op := rec.Parsed.(Op)
	if len(op.Comments) != 1 {
		t.Fatalf("comments count = %d, want 1", len(op.Comments))
	}
	if op.Comments[0].Cell != "A1" || op.Comments[0].Author != "alice" || op.Comments[0].Text != "important" {
		t.Fatalf("comment = %+v", op.Comments[0])
	}
	if len(op.Cells) != 1 || op.Cells[0].String != "val" {
		t.Fatalf("cells should still decode: %+v", op.Cells)
	}
}

func TestReaderOpWithoutComments(t *testing.T) {
	var buf bytes.Buffer
	w, _ := NewWriter(&buf)
	w.WriteHeader(1000, "test.xlsx")
	w.WriteOp(Op{
		Timestamp: 2000, Sequence: 1, Action: ActionRead,
		Sheet: "Sheet1", Range: "A1", Message: "read",
		NumRows: 1, NumCols: 1,
		Cells: []Cell{{Type: CellString, String: "val"}},
	})

	rd, _ := NewReader(bytes.NewReader(buf.Bytes()))
	rd.Next() // header
	rec, err := rd.Next()
	if err != nil {
		t.Fatal(err)
	}
	op := rec.Parsed.(Op)
	if len(op.Comments) != 0 {
		t.Fatalf("expected no comments, got %d", len(op.Comments))
	}
	if len(op.Cells) != 1 || op.Cells[0].String != "val" {
		t.Fatalf("cells = %+v", op.Cells)
	}
}

func TestWriterReaderRoundTripCommentOp(t *testing.T) {
	var buf bytes.Buffer
	w, _ := NewWriter(&buf)
	w.WriteHeader(1000, "test.xlsx")

	entries := []CommentEntry{
		{Cell: "A1", Author: "alice", Text: "first comment"},
		{Cell: "B2", Author: "bob", Text: "second comment"},
		{Cell: "C3", Author: "charlie", Text: "third comment"},
	}
	w.WriteCommentOp(CommentOp{
		Timestamp:  5000,
		Sequence:   1,
		Action:     ActionCommentSet,
		Sheet:      "Sheet1",
		Range:      "A1:C3",
		Message:    "batch comments",
		NumEntries: 3,
		Entries:    entries,
	})

	rd, _ := NewReader(bytes.NewReader(buf.Bytes()))
	rd.Next() // header
	rec, err := rd.Next()
	if err != nil {
		t.Fatal(err)
	}
	cop := rec.Parsed.(CommentOp)
	if cop.NumEntries != 3 || len(cop.Entries) != 3 {
		t.Fatalf("entry count = %d, want 3", len(cop.Entries))
	}
	for i, want := range entries {
		got := cop.Entries[i]
		if got.Cell != want.Cell || got.Author != want.Author || got.Text != want.Text {
			t.Fatalf("entry[%d] = %+v, want %+v", i, got, want)
		}
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

func TestV2RoundTrip(t *testing.T) {
	var buf bytes.Buffer
	w, err := NewWriter(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.WriteHeader(1000, "my-workspace"); err != nil {
		t.Fatal(err)
	}
	if err := w.WriteOp(Op{
		TargetFile: "data.xlsx",
		Timestamp:  2000,
		Sequence:   1,
		Action:     ActionRead,
		Sheet:      "Sheet1",
		Range:      "A1",
		Message:    "read",
		NumRows:    1,
		NumCols:    1,
		Cells:      []Cell{{Type: CellString, String: "val"}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := w.WriteCommentOp(CommentOp{
		TargetFile: "data.xlsx",
		Timestamp:  3000,
		Sequence:   2,
		Action:     ActionCommentSet,
		Sheet:      "Sheet1",
		Range:      "A1",
		Message:    "comment",
		NumEntries: 1,
		Entries:    []CommentEntry{{Cell: "A1", Author: "bob", Text: "note"}},
	}); err != nil {
		t.Fatal(err)
	}

	rd, err := NewReader(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if rd.Version() != VersionV2 {
		t.Fatalf("version = %d, want VersionV2", rd.Version())
	}

	rec, _ := rd.Next()
	hdr := rec.Parsed.(Header)
	if hdr.WorkspaceName != "my-workspace" || hdr.CreatedAt != 1000 {
		t.Fatalf("header = %+v", hdr)
	}

	rec, _ = rd.Next()
	op := rec.Parsed.(Op)
	if op.TargetFile != "data.xlsx" {
		t.Fatalf("op.TargetFile = %q, want data.xlsx", op.TargetFile)
	}
	if op.Timestamp != 2000 || op.Sheet != "Sheet1" {
		t.Fatalf("op = %+v", op)
	}

	rec, _ = rd.Next()
	cop := rec.Parsed.(CommentOp)
	if cop.TargetFile != "data.xlsx" {
		t.Fatalf("cop.TargetFile = %q, want data.xlsx", cop.TargetFile)
	}
	if cop.Timestamp != 3000 || cop.Entries[0].Text != "note" {
		t.Fatalf("cop = %+v", cop)
	}
}

func buildV1Blob(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer

	// v1 preamble
	var preamble [PreambleSize]byte
	copy(preamble[:6], Magic[:])
	preamble[6] = VersionV1
	buf.Write(preamble[:])

	// v1 header: CreatedAt(8) + lpstring(TargetFile)
	hdrPayload := encodeHeaderPayload(Header{CreatedAt: 1000, TargetFile: "budget.xlsx"})
	buf.Write(encodeRecord(OpcodeHeader, hdrPayload))

	// v1 op: no TargetFile prefix
	var opBuf bytes.Buffer
	var tmp [8]byte
	binary.LittleEndian.PutUint64(tmp[:], uint64(2000))
	opBuf.Write(tmp[:])
	binary.LittleEndian.PutUint32(tmp[:4], 1)
	opBuf.Write(tmp[:4])
	opBuf.WriteByte(ActionRead)
	encodeLPString(&opBuf, "Sheet1")
	encodeLPString(&opBuf, "A1")
	encodeLPString(&opBuf, "read it")
	binary.LittleEndian.PutUint32(tmp[:4], 1)
	opBuf.Write(tmp[:4])
	binary.LittleEndian.PutUint32(tmp[:4], 1)
	opBuf.Write(tmp[:4])
	EncodeCell(&opBuf, Cell{Type: CellString, String: "hello"})
	opBuf.WriteByte(0)
	buf.Write(encodeRecord(OpcodeOp, opBuf.Bytes()))

	return buf.Bytes()
}

func TestV1BackwardCompat(t *testing.T) {
	data := buildV1Blob(t)

	rd, err := NewReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if rd.Version() != VersionV1 {
		t.Fatalf("version = %d, want VersionV1", rd.Version())
	}

	rec, err := rd.Next()
	if err != nil {
		t.Fatal(err)
	}
	hdr := rec.Parsed.(Header)
	if hdr.TargetFile != "budget.xlsx" || hdr.CreatedAt != 1000 {
		t.Fatalf("v1 header = %+v", hdr)
	}

	rec, err = rd.Next()
	if err != nil {
		t.Fatal(err)
	}
	op := rec.Parsed.(Op)
	if op.TargetFile != "budget.xlsx" {
		t.Fatalf("v1 op.TargetFile = %q, want budget.xlsx (backfilled from header)", op.TargetFile)
	}
	if op.Timestamp != 2000 || op.Sheet != "Sheet1" || op.Cells[0].String != "hello" {
		t.Fatalf("v1 op = %+v", op)
	}
}

func TestV1BackwardCompatCommentOp(t *testing.T) {
	var buf bytes.Buffer

	var preamble [PreambleSize]byte
	copy(preamble[:6], Magic[:])
	preamble[6] = VersionV1
	buf.Write(preamble[:])

	hdrPayload := encodeHeaderPayload(Header{CreatedAt: 1000, TargetFile: "data.xlsx"})
	buf.Write(encodeRecord(OpcodeHeader, hdrPayload))

	// v1 comment op: no TargetFile prefix
	var copBuf bytes.Buffer
	var tmp [8]byte
	binary.LittleEndian.PutUint64(tmp[:], uint64(3000))
	copBuf.Write(tmp[:])
	binary.LittleEndian.PutUint32(tmp[:4], 1)
	copBuf.Write(tmp[:4])
	copBuf.WriteByte(ActionCommentSet)
	encodeLPString(&copBuf, "Sheet1")
	encodeLPString(&copBuf, "A1")
	encodeLPString(&copBuf, "add comment")
	binary.LittleEndian.PutUint32(tmp[:4], 1)
	copBuf.Write(tmp[:4])
	encodeCommentEntry(&copBuf, CommentEntry{Cell: "A1", Author: "alice", Text: "note"})
	buf.Write(encodeRecord(OpcodeCommentOp, copBuf.Bytes()))

	rd, _ := NewReader(bytes.NewReader(buf.Bytes()))
	rd.Next() // header
	rec, err := rd.Next()
	if err != nil {
		t.Fatal(err)
	}
	cop := rec.Parsed.(CommentOp)
	if cop.TargetFile != "data.xlsx" {
		t.Fatalf("v1 commentop.TargetFile = %q, want data.xlsx (backfilled)", cop.TargetFile)
	}
}

func TestV2HeaderRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	w, _ := NewWriter(&buf)
	w.WriteHeader(9999, "workspace-名前")

	rd, _ := NewReader(bytes.NewReader(buf.Bytes()))
	rec, err := rd.Next()
	if err != nil {
		t.Fatal(err)
	}
	hdr := rec.Parsed.(Header)
	if hdr.CreatedAt != 9999 || hdr.WorkspaceName != "workspace-名前" {
		t.Fatalf("header = %+v", hdr)
	}
	if hdr.TargetFile != "" {
		t.Fatalf("v2 header should have empty TargetFile, got %q", hdr.TargetFile)
	}
}

func TestReaderVersion(t *testing.T) {
	// v2 file
	var buf bytes.Buffer
	NewWriter(&buf)
	rd, _ := NewReader(bytes.NewReader(buf.Bytes()))
	if rd.Version() != VersionV2 {
		t.Fatalf("v2 version = %d", rd.Version())
	}

	// v1 file
	data := buildV1Blob(t)
	rd, _ = NewReader(bytes.NewReader(data))
	if rd.Version() != VersionV1 {
		t.Fatalf("v1 version = %d", rd.Version())
	}
}
