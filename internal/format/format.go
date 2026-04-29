package format

import "fmt"

// Magic bytes: ASCII "XHIST" + NUL.
var Magic = [6]byte{0x58, 0x48, 0x49, 0x53, 0x54, 0x00}

// Format versions.
const (
	VersionV1     = 0x01
	VersionV2     = 0x02
	VersionLatest = VersionV2
)

// Version is kept for backward compatibility with external callers.
const Version = VersionV1

// PreambleSize is the size of the file preamble in bytes (6 magic + 1 version).
const PreambleSize = 7

// Record opcodes.
const (
	OpcodeHeader    uint8 = 0x01
	OpcodeOp        uint8 = 0x02
	OpcodeMetadata  uint8 = 0x03
	OpcodeCommentOp uint8 = 0x04
	OpcodeFooter    uint8 = 0xFF
)

// Action types for Op records.
const (
	ActionRead  uint8 = 1
	ActionWrite uint8 = 2
)

// Comment action types for CommentOp records.
const (
	ActionCommentSet    uint8 = 1
	ActionCommentGet    uint8 = 2
	ActionCommentDelete uint8 = 3
)

// Cell type tags.
const (
	CellEmpty   uint8 = 0x00
	CellString  uint8 = 0x01
	CellNumber  uint8 = 0x02
	CellBool    uint8 = 0x03
	CellFormula uint8 = 0x04
	CellError   uint8 = 0x05
)

// Index file constants.
var IndexMagic = [6]byte{0x58, 0x48, 0x49, 0x44, 0x58, 0x00}

// IndexVersion constants.
const (
	IndexVersionV2 = 0x02
	IndexVersionV3 = 0x03
	IndexVersion   = IndexVersionV3
)

// IndexPreambleSize is the size of the index preamble (6 magic + 1 version + 8 logsize + 4 entrycount).
const IndexPreambleSize = 19

// RecordFramingOverhead is the per-record overhead: 1 opcode + 4 length + 4 CRC.
const RecordFramingOverhead = 9

// ErrCorruption indicates a CRC mismatch or other data integrity failure.
type ErrCorruption struct {
	Offset int64
	Msg    string
}

func (e *ErrCorruption) Error() string {
	return fmt.Sprintf("corruption at offset %d: %s", e.Offset, e.Msg)
}

// ErrBadMagic indicates the file does not start with the expected magic bytes.
type ErrBadMagic struct{}

func (e *ErrBadMagic) Error() string {
	return "bad magic bytes"
}

// ErrBadVersion indicates an unsupported format version.
type ErrBadVersion struct {
	Got byte
}

func (e *ErrBadVersion) Error() string {
	return fmt.Sprintf("unsupported version: %d", e.Got)
}
