package excel

import (
	"fmt"
	"strings"

	"github.com/xuri/excelize/v2"
)

// SheetInfo holds sheet name and dimensions.
type SheetInfo struct {
	Name string
	Rows int
	Cols int
}

// ParseRange parses a cell reference like "Sheet1!A1:C10" or "A1:C10" or "A1".
// Returns: sheet (empty string if not specified), topLeft, bottomRight (same as topLeft for single cell).
func ParseRange(ref string) (sheet, topLeft, bottomRight string, err error) {
	if ref == "" {
		return "", "", "", fmt.Errorf("empty range reference")
	}

	rest := ref
	if idx := strings.LastIndex(ref, "!"); idx >= 0 {
		sheet = ref[:idx]
		rest = ref[idx+1:]
	}

	if idx := strings.Index(rest, ":"); idx >= 0 {
		topLeft = rest[:idx]
		bottomRight = rest[idx+1:]
	} else {
		topLeft = rest
		bottomRight = rest
	}

	if topLeft == "" || bottomRight == "" {
		return "", "", "", fmt.Errorf("invalid range reference: %q", ref)
	}

	return sheet, topLeft, bottomRight, nil
}

// CellRef converts 0-based row/col to Excel notation (e.g. 0,0 -> "A1").
func CellRef(row, col int) string {
	name, _ := excelize.CoordinatesToCellName(col+1, row+1)
	return name
}

// ParseCellRef converts Excel notation to 0-based row/col (e.g. "A1" -> 0,0).
func ParseCellRef(ref string) (row, col int, err error) {
	c, r, err := excelize.CellNameToCoordinates(ref)
	if err != nil {
		return 0, 0, err
	}
	return r - 1, c - 1, nil
}

// RangeSize returns the row and column count for a range like "A1:C10".
func RangeSize(topLeft, bottomRight string) (rows, cols int, err error) {
	r1, c1, err := ParseCellRef(topLeft)
	if err != nil {
		return 0, 0, fmt.Errorf("parsing top-left %q: %w", topLeft, err)
	}
	r2, c2, err := ParseCellRef(bottomRight)
	if err != nil {
		return 0, 0, fmt.Errorf("parsing bottom-right %q: %w", bottomRight, err)
	}
	return r2 - r1 + 1, c2 - c1 + 1, nil
}
