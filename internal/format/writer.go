package format

import (
	"bytes"
	"encoding/binary"
	"io"
)

// Writer writes xhist records to an underlying io.Writer.
type Writer struct {
	w   io.Writer
	buf bytes.Buffer
}

// NewWriter creates a Writer and writes the preamble immediately.
func NewWriter(w io.Writer) (*Writer, error) {
	wr := &Writer{w: w}
	var preamble [PreambleSize]byte
	copy(preamble[:6], Magic[:])
	preamble[6] = VersionLatest
	if _, err := w.Write(preamble[:]); err != nil {
		return nil, err
	}
	return wr, nil
}

func (wr *Writer) writeRecord(opcode uint8, payload []byte) error {
	rec := encodeRecord(opcode, payload)
	_, err := wr.w.Write(rec)
	return err
}

// WriteHeader writes a v2 Header record with workspace name.
func (wr *Writer) WriteHeader(createdAt int64, workspaceName string) error {
	wr.buf.Reset()
	var tmp [8]byte
	binary.LittleEndian.PutUint64(tmp[:], uint64(createdAt))
	wr.buf.Write(tmp[:])
	encodeLPString(&wr.buf, workspaceName)
	return wr.writeRecord(OpcodeHeader, wr.buf.Bytes())
}

// WriteOp writes an Op record.
func (wr *Writer) WriteOp(op Op) error {
	wr.buf.Reset()
	var tmp [8]byte

	encodeLPString(&wr.buf, op.TargetFile)

	binary.LittleEndian.PutUint64(tmp[:], uint64(op.Timestamp))
	wr.buf.Write(tmp[:])

	binary.LittleEndian.PutUint32(tmp[:4], op.Sequence)
	wr.buf.Write(tmp[:4])

	wr.buf.WriteByte(op.Action)

	encodeLPString(&wr.buf, op.Sheet)
	encodeLPString(&wr.buf, op.Range)
	encodeLPString(&wr.buf, op.Message)

	binary.LittleEndian.PutUint32(tmp[:4], op.NumRows)
	wr.buf.Write(tmp[:4])
	binary.LittleEndian.PutUint32(tmp[:4], op.NumCols)
	wr.buf.Write(tmp[:4])

	for i := range op.Cells {
		if err := EncodeCell(&wr.buf, op.Cells[i]); err != nil {
			return err
		}
	}

	if len(op.Comments) > 0 {
		wr.buf.WriteByte(1)
		binary.LittleEndian.PutUint32(tmp[:4], uint32(len(op.Comments)))
		wr.buf.Write(tmp[:4])
		for _, c := range op.Comments {
			encodeCommentEntry(&wr.buf, c)
		}
	} else {
		wr.buf.WriteByte(0)
	}

	return wr.writeRecord(OpcodeOp, wr.buf.Bytes())
}

func (wr *Writer) WriteCommentOp(c CommentOp) error {
	wr.buf.Reset()
	var tmp [8]byte

	encodeLPString(&wr.buf, c.TargetFile)

	binary.LittleEndian.PutUint64(tmp[:], uint64(c.Timestamp))
	wr.buf.Write(tmp[:])

	binary.LittleEndian.PutUint32(tmp[:4], c.Sequence)
	wr.buf.Write(tmp[:4])

	wr.buf.WriteByte(c.Action)

	encodeLPString(&wr.buf, c.Sheet)
	encodeLPString(&wr.buf, c.Range)
	encodeLPString(&wr.buf, c.Message)

	binary.LittleEndian.PutUint32(tmp[:4], c.NumEntries)
	wr.buf.Write(tmp[:4])

	for _, e := range c.Entries {
		encodeCommentEntry(&wr.buf, e)
	}

	return wr.writeRecord(OpcodeCommentOp, wr.buf.Bytes())
}

// WriteMetadata writes a Metadata record.
func (wr *Writer) WriteMetadata(key, value string) error {
	wr.buf.Reset()
	encodeLPString(&wr.buf, key)
	encodeLPString(&wr.buf, value)
	return wr.writeRecord(OpcodeMetadata, wr.buf.Bytes())
}

// NewAppendWriter creates a Writer that appends to an existing log file
// without writing the preamble. Use this when the file already has a valid
// preamble and you want to append new records.
func NewAppendWriter(w io.Writer) *Writer {
	return &Writer{w: w}
}

// WriteFooter writes a Footer record.
func (wr *Writer) WriteFooter(opCount, lastSeq uint32) error {
	var payload [8]byte
	binary.LittleEndian.PutUint32(payload[:4], opCount)
	binary.LittleEndian.PutUint32(payload[4:], lastSeq)
	return wr.writeRecord(OpcodeFooter, payload[:])
}
