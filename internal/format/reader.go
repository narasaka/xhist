package format

import (
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io"
)

// Reader reads xhist records sequentially from an io.ReadSeeker.
type Reader struct {
	r          io.ReadSeeker
	offset     int64
	version    uint8
	targetFile string // v1: backfill from header into Op/CommentOp
}

// NewReader validates the preamble and returns a Reader.
func NewReader(r io.ReadSeeker) (*Reader, error) {
	var preamble [PreambleSize]byte
	if _, err := io.ReadFull(r, preamble[:]); err != nil {
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			return nil, &ErrBadMagic{}
		}
		return nil, err
	}
	if preamble[0] != Magic[0] || preamble[1] != Magic[1] || preamble[2] != Magic[2] ||
		preamble[3] != Magic[3] || preamble[4] != Magic[4] || preamble[5] != Magic[5] {
		return nil, &ErrBadMagic{}
	}
	v := preamble[6]
	if v != VersionV1 && v != VersionV2 {
		return nil, &ErrBadVersion{Got: v}
	}
	return &Reader{r: r, offset: PreambleSize, version: v}, nil
}

func (rd *Reader) Version() uint8 {
	return rd.version
}

// ParsedRecord holds a raw Record plus its parsed typed struct.
type ParsedRecord struct {
	Record
	Parsed interface{} // Header, Op, Metadata, or Footer
}

// Next reads the next record. Returns io.EOF when no more complete records.
func (rd *Reader) Next() (*ParsedRecord, error) {
	recordStart := rd.offset

	var hdr [5]byte
	n, err := io.ReadFull(rd.r, hdr[:])
	if err != nil {
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			return nil, io.EOF
		}
		return nil, err
	}
	rd.offset += int64(n)

	opcode := hdr[0]
	length := binary.LittleEndian.Uint32(hdr[1:5])

	payloadAndCRC := make([]byte, int(length)+4)
	n, err = io.ReadFull(rd.r, payloadAndCRC)
	if err != nil {
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			return nil, io.EOF
		}
		return nil, err
	}
	rd.offset += int64(n)

	payload := payloadAndCRC[:length]
	storedCRC := binary.LittleEndian.Uint32(payloadAndCRC[length:])

	h := crc32.NewIEEE()
	h.Write(hdr[:])
	h.Write(payload)
	if h.Sum32() != storedCRC {
		return nil, &ErrCorruption{
			Offset: recordStart,
			Msg:    fmt.Sprintf("CRC mismatch: stored=0x%08x computed=0x%08x", storedCRC, h.Sum32()),
		}
	}

	rec := &ParsedRecord{
		Record: Record{Opcode: opcode, Payload: append([]byte(nil), payload...)},
	}

	switch opcode {
	case OpcodeHeader:
		if rd.version == VersionV2 {
			parsed, err := decodeHeaderPayloadV2(payload)
			if err != nil {
				return nil, &ErrCorruption{Offset: recordStart, Msg: "invalid header payload: " + err.Error()}
			}
			rec.Parsed = parsed
		} else {
			parsed, err := decodeHeaderPayload(payload)
			if err != nil {
				return nil, &ErrCorruption{Offset: recordStart, Msg: "invalid header payload: " + err.Error()}
			}
			rd.targetFile = parsed.TargetFile
			rec.Parsed = parsed
		}
	case OpcodeOp:
		if rd.version == VersionV2 {
			parsed, err := decodeOpPayloadV2(payload)
			if err != nil {
				return nil, &ErrCorruption{Offset: recordStart, Msg: "invalid op payload: " + err.Error()}
			}
			rec.Parsed = parsed
		} else {
			parsed, err := decodeOpPayloadV1(payload)
			if err != nil {
				return nil, &ErrCorruption{Offset: recordStart, Msg: "invalid op payload: " + err.Error()}
			}
			parsed.TargetFile = rd.targetFile
			rec.Parsed = parsed
		}
	case OpcodeCommentOp:
		if rd.version == VersionV2 {
			parsed, err := decodeCommentOpPayloadV2(payload)
			if err != nil {
				return nil, &ErrCorruption{Offset: recordStart, Msg: "invalid comment op payload: " + err.Error()}
			}
			rec.Parsed = parsed
		} else {
			parsed, err := decodeCommentOpPayloadV1(payload)
			if err != nil {
				return nil, &ErrCorruption{Offset: recordStart, Msg: "invalid comment op payload: " + err.Error()}
			}
			parsed.TargetFile = rd.targetFile
			rec.Parsed = parsed
		}
	case OpcodeConfuseOp:
		if rd.version != VersionV2 {
			return nil, &ErrCorruption{Offset: recordStart, Msg: "confuse records require v2 logs"}
		}
		parsed, err := decodeConfuseOpPayload(payload)
		if err != nil {
			return nil, &ErrCorruption{Offset: recordStart, Msg: "invalid confuse op payload: " + err.Error()}
		}
		rec.Parsed = parsed
	case OpcodeMetadata:
		parsed, err := decodeMetadataPayload(payload)
		if err != nil {
			return nil, &ErrCorruption{Offset: recordStart, Msg: "invalid metadata payload: " + err.Error()}
		}
		rec.Parsed = parsed
	case OpcodeFooter:
		parsed, err := decodeFooterPayload(payload)
		if err != nil {
			return nil, &ErrCorruption{Offset: recordStart, Msg: "invalid footer payload: " + err.Error()}
		}
		rec.Parsed = parsed
	default:
		// unknown opcode — Parsed stays nil
	}

	return rec, nil
}
