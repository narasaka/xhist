package format

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"io"
)

// Record is a raw framed record with opcode and payload.
type Record struct {
	Opcode  uint8
	Payload []byte
}

// Header is the first record in an xhist file.
type Header struct {
	CreatedAt     int64
	TargetFile    string // v1 only
	WorkspaceName string // v2 only
}

// Op is the core operation record.
type Op struct {
	TargetFile string
	Timestamp  int64
	Sequence   uint32
	Action     uint8
	Sheet      string
	Range      string
	Message    string
	NumRows    uint32
	NumCols    uint32
	Cells      []Cell
	Comments   []CommentEntry
}

// CommentEntry is one comment in a CommentOp or Op record.
type CommentEntry struct {
	Cell   string
	Author string
	Text   string
}

// CommentOp is a standalone comment operation record.
type CommentOp struct {
	TargetFile string
	Timestamp  int64
	Sequence   uint32
	Action     uint8
	Sheet      string
	Range      string
	Message    string
	NumEntries uint32
	Entries    []CommentEntry
}

// ConfuseOp is a standalone confusion/reconciliation operation record.
type ConfuseOp struct {
	TargetFile     string
	Timestamp      int64
	Sequence       uint32
	Action         uint8
	ID             string
	Sheet          string
	Cell           string
	Archetype      string
	Headline       string
	Description    string
	PayloadJSON    string
	ResolutionJSON string
	Message        string
}

// Metadata is a key-value pair record.
type Metadata struct {
	Key   string
	Value string
}

// Footer is the optional closing record.
type Footer struct {
	OpCount      uint32
	LastSequence uint32
}

// encodeLPString writes a length-prefixed string to buf.
func encodeLPString(buf *bytes.Buffer, s string) {
	var tmp [4]byte
	binary.LittleEndian.PutUint32(tmp[:], uint32(len(s)))
	buf.Write(tmp[:])
	buf.WriteString(s)
}

// decodeLPString reads a length-prefixed string from r.
func decodeLPString(r io.Reader) (string, error) {
	var tmp [4]byte
	if _, err := io.ReadFull(r, tmp[:]); err != nil {
		return "", err
	}
	n := binary.LittleEndian.Uint32(tmp[:])
	b := make([]byte, n)
	if _, err := io.ReadFull(r, b); err != nil {
		return "", err
	}
	return string(b), nil
}

// encodeRecord produces the full framed record bytes: opcode + length + payload + CRC.
func encodeRecord(opcode uint8, payload []byte) []byte {
	total := 1 + 4 + len(payload) + 4
	out := make([]byte, total)
	out[0] = opcode
	binary.LittleEndian.PutUint32(out[1:5], uint32(len(payload)))
	copy(out[5:5+len(payload)], payload)

	// CRC covers opcode + length + payload
	crc := crc32.ChecksumIEEE(out[:5+len(payload)])
	binary.LittleEndian.PutUint32(out[5+len(payload):], crc)
	return out
}

// encodeHeaderPayload encodes a v1 Header into its payload bytes.
func encodeHeaderPayload(h Header) []byte {
	var buf bytes.Buffer
	var tmp [8]byte
	binary.LittleEndian.PutUint64(tmp[:], uint64(h.CreatedAt))
	buf.Write(tmp[:])
	encodeLPString(&buf, h.TargetFile)
	return buf.Bytes()
}

// decodeHeaderPayload decodes a v1 Header from payload bytes.
func decodeHeaderPayload(data []byte) (Header, error) {
	r := bytes.NewReader(data)
	var h Header
	if err := binary.Read(r, binary.LittleEndian, &h.CreatedAt); err != nil {
		return h, err
	}
	var err error
	h.TargetFile, err = decodeLPString(r)
	if err != nil {
		return h, err
	}
	return h, nil
}

func encodeHeaderPayloadV2(h Header) []byte {
	var buf bytes.Buffer
	var tmp [8]byte
	binary.LittleEndian.PutUint64(tmp[:], uint64(h.CreatedAt))
	buf.Write(tmp[:])
	encodeLPString(&buf, h.WorkspaceName)
	return buf.Bytes()
}

func decodeHeaderPayloadV2(data []byte) (Header, error) {
	r := bytes.NewReader(data)
	var h Header
	if err := binary.Read(r, binary.LittleEndian, &h.CreatedAt); err != nil {
		return h, err
	}
	var err error
	h.WorkspaceName, err = decodeLPString(r)
	if err != nil {
		return h, err
	}
	return h, nil
}

func encodeConfuseOpPayload(c ConfuseOp) []byte {
	var buf bytes.Buffer
	var tmp [8]byte

	encodeLPString(&buf, c.TargetFile)
	binary.LittleEndian.PutUint64(tmp[:], uint64(c.Timestamp))
	buf.Write(tmp[:])
	binary.LittleEndian.PutUint32(tmp[:4], c.Sequence)
	buf.Write(tmp[:4])
	buf.WriteByte(c.Action)
	encodeLPString(&buf, c.ID)
	encodeLPString(&buf, c.Sheet)
	encodeLPString(&buf, c.Cell)
	encodeLPString(&buf, c.Archetype)
	encodeLPString(&buf, c.Headline)
	encodeLPString(&buf, c.Description)
	encodeLPString(&buf, c.PayloadJSON)
	encodeLPString(&buf, c.ResolutionJSON)
	encodeLPString(&buf, c.Message)
	return buf.Bytes()
}

func decodeConfuseOpPayload(data []byte) (ConfuseOp, error) {
	r := bytes.NewReader(data)
	var c ConfuseOp
	var err error

	c.TargetFile, err = decodeLPString(r)
	if err != nil {
		return c, err
	}
	if err := binary.Read(r, binary.LittleEndian, &c.Timestamp); err != nil {
		return c, err
	}
	if err := binary.Read(r, binary.LittleEndian, &c.Sequence); err != nil {
		return c, err
	}
	if err := binary.Read(r, binary.LittleEndian, &c.Action); err != nil {
		return c, err
	}
	c.ID, err = decodeLPString(r)
	if err != nil {
		return c, err
	}
	c.Sheet, err = decodeLPString(r)
	if err != nil {
		return c, err
	}
	c.Cell, err = decodeLPString(r)
	if err != nil {
		return c, err
	}
	c.Archetype, err = decodeLPString(r)
	if err != nil {
		return c, err
	}
	c.Headline, err = decodeLPString(r)
	if err != nil {
		return c, err
	}
	c.Description, err = decodeLPString(r)
	if err != nil {
		return c, err
	}
	c.PayloadJSON, err = decodeLPString(r)
	if err != nil {
		return c, err
	}
	c.ResolutionJSON, err = decodeLPString(r)
	if err != nil {
		return c, err
	}
	c.Message, err = decodeLPString(r)
	if err != nil {
		return c, err
	}
	return c, nil
}

// encodeOpPayload encodes an Op into its v2 payload bytes (with TargetFile).
func encodeOpPayload(op Op) ([]byte, error) {
	var buf bytes.Buffer
	var tmp [8]byte

	encodeLPString(&buf, op.TargetFile)

	binary.LittleEndian.PutUint64(tmp[:], uint64(op.Timestamp))
	buf.Write(tmp[:])

	binary.LittleEndian.PutUint32(tmp[:4], op.Sequence)
	buf.Write(tmp[:4])

	buf.WriteByte(op.Action)

	encodeLPString(&buf, op.Sheet)
	encodeLPString(&buf, op.Range)
	encodeLPString(&buf, op.Message)

	binary.LittleEndian.PutUint32(tmp[:4], op.NumRows)
	buf.Write(tmp[:4])
	binary.LittleEndian.PutUint32(tmp[:4], op.NumCols)
	buf.Write(tmp[:4])

	for i := range op.Cells {
		if err := EncodeCell(&buf, op.Cells[i]); err != nil {
			return nil, err
		}
	}

	if len(op.Comments) > 0 {
		buf.WriteByte(1)
		binary.LittleEndian.PutUint32(tmp[:4], uint32(len(op.Comments)))
		buf.Write(tmp[:4])
		for _, c := range op.Comments {
			encodeCommentEntry(&buf, c)
		}
	} else {
		buf.WriteByte(0)
	}

	return buf.Bytes(), nil
}

func decodeOpPayloadV1(data []byte) (Op, error) {
	r := bytes.NewReader(data)
	var op Op

	if err := binary.Read(r, binary.LittleEndian, &op.Timestamp); err != nil {
		return op, err
	}
	if err := binary.Read(r, binary.LittleEndian, &op.Sequence); err != nil {
		return op, err
	}
	if err := binary.Read(r, binary.LittleEndian, &op.Action); err != nil {
		return op, err
	}

	var err error
	op.Sheet, err = decodeLPString(r)
	if err != nil {
		return op, err
	}
	op.Range, err = decodeLPString(r)
	if err != nil {
		return op, err
	}
	op.Message, err = decodeLPString(r)
	if err != nil {
		return op, err
	}

	if err := binary.Read(r, binary.LittleEndian, &op.NumRows); err != nil {
		return op, err
	}
	if err := binary.Read(r, binary.LittleEndian, &op.NumCols); err != nil {
		return op, err
	}

	cellCount := int(op.NumRows) * int(op.NumCols)
	op.Cells = make([]Cell, cellCount)
	for i := 0; i < cellCount; i++ {
		op.Cells[i], err = DecodeCell(r)
		if err != nil {
			return op, err
		}
	}

	if r.Len() > 0 {
		var hasComments uint8
		if err := binary.Read(r, binary.LittleEndian, &hasComments); err == nil && hasComments == 1 {
			var numComments uint32
			if err := binary.Read(r, binary.LittleEndian, &numComments); err != nil {
				return op, err
			}
			op.Comments = make([]CommentEntry, numComments)
			for i := range numComments {
				op.Comments[i], err = decodeCommentEntry(r)
				if err != nil {
					return op, err
				}
			}
		}
	}

	return op, nil
}

func decodeOpPayloadV2(data []byte) (Op, error) {
	r := bytes.NewReader(data)
	var op Op
	var err error

	op.TargetFile, err = decodeLPString(r)
	if err != nil {
		return op, err
	}

	if err := binary.Read(r, binary.LittleEndian, &op.Timestamp); err != nil {
		return op, err
	}
	if err := binary.Read(r, binary.LittleEndian, &op.Sequence); err != nil {
		return op, err
	}
	if err := binary.Read(r, binary.LittleEndian, &op.Action); err != nil {
		return op, err
	}

	op.Sheet, err = decodeLPString(r)
	if err != nil {
		return op, err
	}
	op.Range, err = decodeLPString(r)
	if err != nil {
		return op, err
	}
	op.Message, err = decodeLPString(r)
	if err != nil {
		return op, err
	}

	if err := binary.Read(r, binary.LittleEndian, &op.NumRows); err != nil {
		return op, err
	}
	if err := binary.Read(r, binary.LittleEndian, &op.NumCols); err != nil {
		return op, err
	}

	cellCount := int(op.NumRows) * int(op.NumCols)
	op.Cells = make([]Cell, cellCount)
	for i := 0; i < cellCount; i++ {
		op.Cells[i], err = DecodeCell(r)
		if err != nil {
			return op, err
		}
	}

	if r.Len() > 0 {
		var hasComments uint8
		if err := binary.Read(r, binary.LittleEndian, &hasComments); err == nil && hasComments == 1 {
			var numComments uint32
			if err := binary.Read(r, binary.LittleEndian, &numComments); err != nil {
				return op, err
			}
			op.Comments = make([]CommentEntry, numComments)
			for i := range numComments {
				op.Comments[i], err = decodeCommentEntry(r)
				if err != nil {
					return op, err
				}
			}
		}
	}

	return op, nil
}

// encodeMetadataPayload encodes a Metadata record payload.
func encodeMetadataPayload(m Metadata) []byte {
	var buf bytes.Buffer
	encodeLPString(&buf, m.Key)
	encodeLPString(&buf, m.Value)
	return buf.Bytes()
}

// decodeMetadataPayload decodes a Metadata record from payload bytes.
func decodeMetadataPayload(data []byte) (Metadata, error) {
	r := bytes.NewReader(data)
	var m Metadata
	var err error
	m.Key, err = decodeLPString(r)
	if err != nil {
		return m, err
	}
	m.Value, err = decodeLPString(r)
	if err != nil {
		return m, err
	}
	return m, nil
}

// encodeFooterPayload encodes a Footer record payload.
func encodeFooterPayload(f Footer) []byte {
	var buf [8]byte
	binary.LittleEndian.PutUint32(buf[:4], f.OpCount)
	binary.LittleEndian.PutUint32(buf[4:8], f.LastSequence)
	return buf[:]
}

// decodeFooterPayload decodes a Footer from payload bytes.
func decodeFooterPayload(data []byte) (Footer, error) {
	if len(data) < 8 {
		return Footer{}, io.ErrUnexpectedEOF
	}
	var f Footer
	f.OpCount = binary.LittleEndian.Uint32(data[:4])
	f.LastSequence = binary.LittleEndian.Uint32(data[4:8])
	return f, nil
}

func encodeCommentEntry(buf *bytes.Buffer, e CommentEntry) {
	encodeLPString(buf, e.Cell)
	encodeLPString(buf, e.Author)
	encodeLPString(buf, e.Text)
}

func decodeCommentEntry(r io.Reader) (CommentEntry, error) {
	var e CommentEntry
	var err error
	e.Cell, err = decodeLPString(r)
	if err != nil {
		return e, err
	}
	e.Author, err = decodeLPString(r)
	if err != nil {
		return e, err
	}
	e.Text, err = decodeLPString(r)
	if err != nil {
		return e, err
	}
	return e, nil
}

func encodeCommentOpPayload(c CommentOp) []byte {
	var buf bytes.Buffer
	var tmp [8]byte

	encodeLPString(&buf, c.TargetFile)

	binary.LittleEndian.PutUint64(tmp[:], uint64(c.Timestamp))
	buf.Write(tmp[:])

	binary.LittleEndian.PutUint32(tmp[:4], c.Sequence)
	buf.Write(tmp[:4])

	buf.WriteByte(c.Action)

	encodeLPString(&buf, c.Sheet)
	encodeLPString(&buf, c.Range)
	encodeLPString(&buf, c.Message)

	binary.LittleEndian.PutUint32(tmp[:4], c.NumEntries)
	buf.Write(tmp[:4])

	for _, e := range c.Entries {
		encodeCommentEntry(&buf, e)
	}

	return buf.Bytes()
}

func decodeCommentOpPayloadV1(data []byte) (CommentOp, error) {
	r := bytes.NewReader(data)
	var c CommentOp

	if err := binary.Read(r, binary.LittleEndian, &c.Timestamp); err != nil {
		return c, err
	}
	if err := binary.Read(r, binary.LittleEndian, &c.Sequence); err != nil {
		return c, err
	}
	if err := binary.Read(r, binary.LittleEndian, &c.Action); err != nil {
		return c, err
	}

	var err error
	c.Sheet, err = decodeLPString(r)
	if err != nil {
		return c, err
	}
	c.Range, err = decodeLPString(r)
	if err != nil {
		return c, err
	}
	c.Message, err = decodeLPString(r)
	if err != nil {
		return c, err
	}

	if err := binary.Read(r, binary.LittleEndian, &c.NumEntries); err != nil {
		return c, err
	}

	c.Entries = make([]CommentEntry, c.NumEntries)
	for i := range c.NumEntries {
		c.Entries[i], err = decodeCommentEntry(r)
		if err != nil {
			return c, err
		}
	}

	return c, nil
}

func decodeCommentOpPayloadV2(data []byte) (CommentOp, error) {
	r := bytes.NewReader(data)
	var c CommentOp
	var err error

	c.TargetFile, err = decodeLPString(r)
	if err != nil {
		return c, err
	}

	if err := binary.Read(r, binary.LittleEndian, &c.Timestamp); err != nil {
		return c, err
	}
	if err := binary.Read(r, binary.LittleEndian, &c.Sequence); err != nil {
		return c, err
	}
	if err := binary.Read(r, binary.LittleEndian, &c.Action); err != nil {
		return c, err
	}

	c.Sheet, err = decodeLPString(r)
	if err != nil {
		return c, err
	}
	c.Range, err = decodeLPString(r)
	if err != nil {
		return c, err
	}
	c.Message, err = decodeLPString(r)
	if err != nil {
		return c, err
	}

	if err := binary.Read(r, binary.LittleEndian, &c.NumEntries); err != nil {
		return c, err
	}

	c.Entries = make([]CommentEntry, c.NumEntries)
	for i := range c.NumEntries {
		c.Entries[i], err = decodeCommentEntry(r)
		if err != nil {
			return c, err
		}
	}

	return c, nil
}
