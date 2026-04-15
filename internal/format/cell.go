package format

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"math"
)

// Cell represents a single cell value with a type tag.
type Cell struct {
	Type        uint8
	String      string
	Number      float64
	Bool        bool
	FormulaText string
	CachedValue *Cell
	Error       string
}

// EncodeCell writes a cell to buf.
func EncodeCell(buf *bytes.Buffer, c Cell) error {
	buf.WriteByte(c.Type)
	switch c.Type {
	case CellEmpty:
		// no value bytes
	case CellString:
		encodeLPString(buf, c.String)
	case CellNumber:
		var tmp [8]byte
		binary.LittleEndian.PutUint64(tmp[:], math.Float64bits(c.Number))
		buf.Write(tmp[:])
	case CellBool:
		if c.Bool {
			buf.WriteByte(1)
		} else {
			buf.WriteByte(0)
		}
	case CellFormula:
		encodeLPString(buf, c.FormulaText)
		if c.CachedValue == nil {
			// encode an empty cell as the cached value
			buf.WriteByte(CellEmpty)
		} else {
			if err := EncodeCell(buf, *c.CachedValue); err != nil {
				return err
			}
		}
	case CellError:
		encodeLPString(buf, c.Error)
	default:
		return fmt.Errorf("unknown cell type: 0x%02x", c.Type)
	}
	return nil
}

// DecodeCell reads a cell from r.
func DecodeCell(r io.Reader) (Cell, error) {
	var tag [1]byte
	if _, err := io.ReadFull(r, tag[:]); err != nil {
		return Cell{}, err
	}
	c := Cell{Type: tag[0]}
	switch c.Type {
	case CellEmpty:
		// nothing
	case CellString:
		s, err := decodeLPString(r)
		if err != nil {
			return c, err
		}
		c.String = s
	case CellNumber:
		var tmp [8]byte
		if _, err := io.ReadFull(r, tmp[:]); err != nil {
			return c, err
		}
		c.Number = math.Float64frombits(binary.LittleEndian.Uint64(tmp[:]))
	case CellBool:
		var tmp [1]byte
		if _, err := io.ReadFull(r, tmp[:]); err != nil {
			return c, err
		}
		c.Bool = tmp[0] != 0
	case CellFormula:
		s, err := decodeLPString(r)
		if err != nil {
			return c, err
		}
		c.FormulaText = s
		cached, err := DecodeCell(r)
		if err != nil {
			return c, err
		}
		c.CachedValue = &cached
	case CellError:
		s, err := decodeLPString(r)
		if err != nil {
			return c, err
		}
		c.Error = s
	default:
		return c, fmt.Errorf("unknown cell type: 0x%02x", c.Type)
	}
	return c, nil
}
