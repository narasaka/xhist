package format

import (
	"encoding/binary"
	"io"
	"os"
)

// IndexEntry is one entry in the sidecar index, corresponding to an Op record.
type IndexEntry struct {
	Offset    uint64
	Opcode    uint8
	Timestamp int64
	Sequence  uint32
	Action    uint8
	Sheet     string
}

// BuildIndex scans the xhist log file and writes a sidecar .xhist.idx file.
func BuildIndex(xhistPath string) error {
	f, err := os.Open(xhistPath)
	if err != nil {
		return err
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return err
	}
	logSize := uint64(fi.Size())

	rd, err := NewReader(f)
	if err != nil {
		return err
	}

	var entries []IndexEntry
	for {
		recordOffset := rd.offset
		rec, err := rd.Next()
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
		if rec.Opcode == OpcodeOp {
			op := rec.Parsed.(Op)
			entries = append(entries, IndexEntry{
				Offset:    uint64(recordOffset),
				Opcode:    OpcodeOp,
				Timestamp: op.Timestamp,
				Sequence:  op.Sequence,
				Action:    op.Action,
				Sheet:     op.Sheet,
			})
		} else if rec.Opcode == OpcodeCommentOp {
			cop := rec.Parsed.(CommentOp)
			entries = append(entries, IndexEntry{
				Offset:    uint64(recordOffset),
				Opcode:    OpcodeCommentOp,
				Timestamp: cop.Timestamp,
				Sequence:  cop.Sequence,
				Action:    cop.Action,
				Sheet:     cop.Sheet,
			})
		}
	}

	idxPath := xhistPath + ".idx"
	out, err := os.Create(idxPath)
	if err != nil {
		return err
	}
	defer out.Close()

	if err := writeIndexPreamble(out, logSize, uint32(len(entries))); err != nil {
		return err
	}

	for _, e := range entries {
		if err := writeIndexEntry(out, e); err != nil {
			return err
		}
	}

	return nil
}

func writeIndexPreamble(w io.Writer, logSize uint64, entryCount uint32) error {
	var buf [IndexPreambleSize]byte
	copy(buf[:6], IndexMagic[:])
	buf[6] = IndexVersion
	binary.LittleEndian.PutUint64(buf[7:15], logSize)
	binary.LittleEndian.PutUint32(buf[15:19], entryCount)
	_, err := w.Write(buf[:])
	return err
}

func writeIndexEntry(w io.Writer, e IndexEntry) error {
	sheetBytes := []byte(e.Sheet)
	buf := make([]byte, 8+1+8+4+1+2+len(sheetBytes))
	binary.LittleEndian.PutUint64(buf[0:8], e.Offset)
	buf[8] = e.Opcode
	binary.LittleEndian.PutUint64(buf[9:17], uint64(e.Timestamp))
	binary.LittleEndian.PutUint32(buf[17:21], e.Sequence)
	buf[21] = e.Action
	binary.LittleEndian.PutUint16(buf[22:24], uint16(len(sheetBytes)))
	copy(buf[24:], sheetBytes)
	_, err := w.Write(buf)
	return err
}

// ReadIndex reads all index entries from an .xhist.idx file.
func ReadIndex(idxPath string) ([]IndexEntry, error) {
	f, err := os.Open(idxPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var preamble [IndexPreambleSize]byte
	if _, err := io.ReadFull(f, preamble[:]); err != nil {
		return nil, err
	}
	if preamble[0] != IndexMagic[0] || preamble[1] != IndexMagic[1] || preamble[2] != IndexMagic[2] ||
		preamble[3] != IndexMagic[3] || preamble[4] != IndexMagic[4] || preamble[5] != IndexMagic[5] {
		return nil, &ErrBadMagic{}
	}
	if preamble[6] != IndexVersion {
		return nil, &ErrBadVersion{Got: preamble[6]}
	}
	entryCount := binary.LittleEndian.Uint32(preamble[15:19])

	entries := make([]IndexEntry, 0, entryCount)
	for i := uint32(0); i < entryCount; i++ {
		var fixed [24]byte
		if _, err := io.ReadFull(f, fixed[:]); err != nil {
			return nil, err
		}
		e := IndexEntry{
			Offset:    binary.LittleEndian.Uint64(fixed[0:8]),
			Opcode:    fixed[8],
			Timestamp: int64(binary.LittleEndian.Uint64(fixed[9:17])),
			Sequence:  binary.LittleEndian.Uint32(fixed[17:21]),
			Action:    fixed[21],
		}
		sheetLen := binary.LittleEndian.Uint16(fixed[22:24])
		if sheetLen > 0 {
			sheetBuf := make([]byte, sheetLen)
			if _, err := io.ReadFull(f, sheetBuf); err != nil {
				return nil, err
			}
			e.Sheet = string(sheetBuf)
		}
		entries = append(entries, e)
	}

	return entries, nil
}

// IsStale checks if the index is stale by comparing stored LogSize to actual xhist file size.
func IsStale(idxPath, xhistPath string) (bool, error) {
	idxF, err := os.Open(idxPath)
	if err != nil {
		return false, err
	}
	defer idxF.Close()

	var preamble [IndexPreambleSize]byte
	if _, err := io.ReadFull(idxF, preamble[:]); err != nil {
		return false, err
	}
	storedLogSize := binary.LittleEndian.Uint64(preamble[7:15])

	fi, err := os.Stat(xhistPath)
	if err != nil {
		return false, err
	}

	return uint64(fi.Size()) != storedLogSize, nil
}
