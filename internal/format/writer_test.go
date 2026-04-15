package format

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"testing"
)

func TestWriterPreamble(t *testing.T) {
	var buf bytes.Buffer
	_, err := NewWriter(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if buf.Len() != PreambleSize {
		t.Fatalf("preamble size = %d, want %d", buf.Len(), PreambleSize)
	}
	b := buf.Bytes()
	for i := 0; i < 6; i++ {
		if b[i] != Magic[i] {
			t.Fatalf("magic[%d] = 0x%02x, want 0x%02x", i, b[i], Magic[i])
		}
	}
	if b[6] != Version {
		t.Fatalf("version = 0x%02x, want 0x%02x", b[6], Version)
	}
}

func TestWriteHeaderByteLayout(t *testing.T) {
	var buf bytes.Buffer
	w, _ := NewWriter(&buf)
	if err := w.WriteHeader(1000, "test.xlsx"); err != nil {
		t.Fatal(err)
	}

	data := buf.Bytes()[PreambleSize:]
	if data[0] != OpcodeHeader {
		t.Fatalf("opcode = 0x%02x, want 0x%02x", data[0], OpcodeHeader)
	}

	payloadLen := binary.LittleEndian.Uint32(data[1:5])
	// 8 (createdAt) + 4 (string len) + 9 (string bytes) = 21
	if payloadLen != 21 {
		t.Fatalf("payload length = %d, want 21", payloadLen)
	}

	payload := data[5 : 5+payloadLen]

	createdAt := int64(binary.LittleEndian.Uint64(payload[0:8]))
	if createdAt != 1000 {
		t.Fatalf("createdAt = %d, want 1000", createdAt)
	}

	strLen := binary.LittleEndian.Uint32(payload[8:12])
	if strLen != 9 {
		t.Fatalf("targetFile len = %d, want 9", strLen)
	}
	if string(payload[12:21]) != "test.xlsx" {
		t.Fatalf("targetFile = %q", string(payload[12:21]))
	}

	storedCRC := binary.LittleEndian.Uint32(data[5+payloadLen : 5+payloadLen+4])
	computedCRC := crc32.ChecksumIEEE(data[:5+payloadLen])
	if storedCRC != computedCRC {
		t.Fatalf("CRC mismatch: stored=0x%08x computed=0x%08x", storedCRC, computedCRC)
	}
}

func TestWriteOpByteLayout(t *testing.T) {
	var buf bytes.Buffer
	w, _ := NewWriter(&buf)
	op := Op{
		Timestamp: 2000,
		Sequence:  1,
		Action:    ActionWrite,
		Sheet:     "Sheet1",
		Range:     "A1",
		Message:   "",
		NumRows:   1,
		NumCols:   1,
		Cells:     []Cell{{Type: CellNumber, Number: 42.0}},
	}
	if err := w.WriteOp(op); err != nil {
		t.Fatal(err)
	}

	data := buf.Bytes()[PreambleSize:]
	if data[0] != OpcodeOp {
		t.Fatalf("opcode = 0x%02x, want 0x%02x", data[0], OpcodeOp)
	}

	payloadLen := binary.LittleEndian.Uint32(data[1:5])
	storedCRC := binary.LittleEndian.Uint32(data[5+payloadLen : 5+payloadLen+4])
	computedCRC := crc32.ChecksumIEEE(data[:5+payloadLen])
	if storedCRC != computedCRC {
		t.Fatalf("CRC mismatch")
	}
}

func TestWriteMetadataByteLayout(t *testing.T) {
	var buf bytes.Buffer
	w, _ := NewWriter(&buf)
	if err := w.WriteMetadata("agent.name", "test-agent"); err != nil {
		t.Fatal(err)
	}

	data := buf.Bytes()[PreambleSize:]
	if data[0] != OpcodeMetadata {
		t.Fatalf("opcode = 0x%02x", data[0])
	}

	payloadLen := binary.LittleEndian.Uint32(data[1:5])
	payload := data[5 : 5+payloadLen]

	keyLen := binary.LittleEndian.Uint32(payload[0:4])
	if keyLen != 10 {
		t.Fatalf("key len = %d", keyLen)
	}
	if string(payload[4:14]) != "agent.name" {
		t.Fatalf("key = %q", string(payload[4:14]))
	}
	valLen := binary.LittleEndian.Uint32(payload[14:18])
	if valLen != 10 {
		t.Fatalf("val len = %d", valLen)
	}
	if string(payload[18:28]) != "test-agent" {
		t.Fatalf("val = %q", string(payload[18:28]))
	}
}

func TestWriteFooterByteLayout(t *testing.T) {
	var buf bytes.Buffer
	w, _ := NewWriter(&buf)
	if err := w.WriteFooter(5, 5); err != nil {
		t.Fatal(err)
	}

	data := buf.Bytes()[PreambleSize:]
	if data[0] != OpcodeFooter {
		t.Fatalf("opcode = 0x%02x", data[0])
	}

	payloadLen := binary.LittleEndian.Uint32(data[1:5])
	if payloadLen != 8 {
		t.Fatalf("footer payload len = %d, want 8", payloadLen)
	}

	payload := data[5 : 5+payloadLen]
	opCount := binary.LittleEndian.Uint32(payload[0:4])
	lastSeq := binary.LittleEndian.Uint32(payload[4:8])
	if opCount != 5 || lastSeq != 5 {
		t.Fatalf("footer values: opCount=%d lastSeq=%d", opCount, lastSeq)
	}
}

func TestWriterSingleWritePerRecord(t *testing.T) {
	writeCalls := 0
	cw := &countWriter{fn: func(p []byte) (int, error) {
		writeCalls++
		return len(p), nil
	}}
	w, _ := NewWriter(cw)
	_ = w
	// preamble = 1 write call
	if writeCalls != 1 {
		t.Fatalf("preamble write calls = %d, want 1", writeCalls)
	}
	writeCalls = 0
	w.WriteHeader(1000, "test.xlsx")
	if writeCalls != 1 {
		t.Fatalf("header write calls = %d, want 1", writeCalls)
	}
}

type countWriter struct {
	fn func([]byte) (int, error)
}

func (cw *countWriter) Write(p []byte) (int, error) {
	return cw.fn(p)
}

func TestWriteFullFile(t *testing.T) {
	var buf bytes.Buffer
	w, _ := NewWriter(&buf)
	w.WriteHeader(1000, "data.xlsx")
	w.WriteMetadata("agent.name", "claude")
	w.WriteOp(Op{
		Timestamp: 2000,
		Sequence:  1,
		Action:    ActionRead,
		Sheet:     "Sheet1",
		Range:     "A1:B2",
		Message:   "reading data",
		NumRows:   2,
		NumCols:   2,
		Cells: []Cell{
			{Type: CellString, String: "a"},
			{Type: CellNumber, Number: 1},
			{Type: CellBool, Bool: true},
			{Type: CellEmpty},
		},
	})
	w.WriteFooter(1, 1)

	if buf.Len() == PreambleSize {
		t.Fatal("no records written")
	}
}
