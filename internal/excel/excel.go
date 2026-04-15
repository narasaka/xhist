package excel

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/prosights/xhist/internal/format"
	"github.com/xuri/excelize/v2"
)

// Comment represents an Excel cell comment.
type Comment struct {
	Cell   string // A1 notation, no sheet prefix
	Author string
	Text   string
}

// ReadCells reads cells from an xlsx file and returns them as a format.Cell grid.
func ReadCells(path, sheet, topLeft, bottomRight string) ([][]format.Cell, error) {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return nil, fmt.Errorf("opening %s: %w", path, err)
	}
	defer f.Close()

	if sheet == "" {
		sheet = f.GetSheetName(0)
	}

	rows, cols, err := RangeSize(topLeft, bottomRight)
	if err != nil {
		return nil, err
	}

	startRow, startCol, err := ParseCellRef(topLeft)
	if err != nil {
		return nil, err
	}

	grid := make([][]format.Cell, rows)
	for r := range rows {
		grid[r] = make([]format.Cell, cols)
		for c := range cols {
			ref := CellRef(startRow+r, startCol+c)
			cell, err := readOneCell(f, sheet, ref)
			if err != nil {
				return nil, fmt.Errorf("reading cell %s: %w", ref, err)
			}
			grid[r][c] = cell
		}
	}
	return grid, nil
}

func readOneCell(f *excelize.File, sheet, ref string) (format.Cell, error) {
	ct, err := f.GetCellType(sheet, ref)
	if err != nil {
		return format.Cell{}, err
	}

	switch ct {
	case excelize.CellTypeUnset:
		val, err := f.GetCellValue(sheet, ref)
		if err != nil {
			return format.Cell{}, err
		}
		if val == "" {
			return format.Cell{Type: format.CellEmpty}, nil
		}
		n, err := strconv.ParseFloat(val, 64)
		if err != nil {
			return format.Cell{Type: format.CellString, String: val}, nil
		}
		return format.Cell{Type: format.CellNumber, Number: n}, nil

	case excelize.CellTypeBool:
		val, err := f.GetCellValue(sheet, ref)
		if err != nil {
			return format.Cell{}, err
		}
		return format.Cell{Type: format.CellBool, Bool: strings.EqualFold(val, "TRUE")}, nil

	case excelize.CellTypeNumber:
		val, err := f.GetCellValue(sheet, ref)
		if err != nil {
			return format.Cell{}, err
		}
		n, err := strconv.ParseFloat(val, 64)
		if err != nil {
			return format.Cell{}, fmt.Errorf("parsing number %q: %w", val, err)
		}
		return format.Cell{Type: format.CellNumber, Number: n}, nil

	case excelize.CellTypeSharedString, excelize.CellTypeInlineString:
		val, err := f.GetCellValue(sheet, ref)
		if err != nil {
			return format.Cell{}, err
		}
		return format.Cell{Type: format.CellString, String: val}, nil

	case excelize.CellTypeFormula:
		formula, err := f.GetCellFormula(sheet, ref)
		if err != nil {
			return format.Cell{}, err
		}
		cachedVal, _ := f.GetCellValue(sheet, ref)
		cached := &format.Cell{Type: format.CellString, String: cachedVal}
		if n, parseErr := strconv.ParseFloat(cachedVal, 64); parseErr == nil {
			cached = &format.Cell{Type: format.CellNumber, Number: n}
		} else if strings.EqualFold(cachedVal, "TRUE") || strings.EqualFold(cachedVal, "FALSE") {
			cached = &format.Cell{Type: format.CellBool, Bool: strings.EqualFold(cachedVal, "TRUE")}
		}
		return format.Cell{Type: format.CellFormula, FormulaText: formula, CachedValue: cached}, nil

	case excelize.CellTypeError:
		val, err := f.GetCellValue(sheet, ref)
		if err != nil {
			return format.Cell{}, err
		}
		return format.Cell{Type: format.CellError, Error: val}, nil

	default:
		return format.Cell{Type: format.CellEmpty}, nil
	}
}

// WriteCells writes cells to an xlsx file.
func WriteCells(path, sheet, topLeft string, cells [][]format.Cell) error {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return fmt.Errorf("opening %s: %w", path, err)
	}
	defer f.Close()

	startRow, startCol, err := ParseCellRef(topLeft)
	if err != nil {
		return err
	}

	for r, row := range cells {
		for c, cell := range row {
			ref := CellRef(startRow+r, startCol+c)
			if err := writeOneCell(f, sheet, ref, cell); err != nil {
				return fmt.Errorf("writing cell %s: %w", ref, err)
			}
		}
	}

	return f.SaveAs(path)
}

func writeOneCell(f *excelize.File, sheet, ref string, cell format.Cell) error {
	switch cell.Type {
	case format.CellEmpty:
		return f.SetCellValue(sheet, ref, nil)
	case format.CellString:
		return f.SetCellStr(sheet, ref, cell.String)
	case format.CellNumber:
		return f.SetCellFloat(sheet, ref, cell.Number, -1, 64)
	case format.CellBool:
		return f.SetCellBool(sheet, ref, cell.Bool)
	case format.CellFormula:
		return f.SetCellFormula(sheet, ref, cell.FormulaText)
	case format.CellError:
		return f.SetCellStr(sheet, ref, cell.Error)
	default:
		return fmt.Errorf("unknown cell type: 0x%02x", cell.Type)
	}
}

// ListSheets returns sheet info (name + dimensions) for all sheets.
func ListSheets(path string) ([]SheetInfo, error) {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return nil, fmt.Errorf("opening %s: %w", path, err)
	}
	defer f.Close()

	var sheets []SheetInfo
	for _, name := range f.GetSheetList() {
		info := SheetInfo{Name: name}
		rows, err := f.GetRows(name)
		if err == nil {
			info.Rows = len(rows)
			maxCols := 0
			for _, row := range rows {
				if len(row) > maxCols {
					maxCols = len(row)
				}
			}
			info.Cols = maxCols
		}
		sheets = append(sheets, info)
	}
	return sheets, nil
}

// CreateWorkbook creates a new empty xlsx file with a default "Sheet1".
func CreateWorkbook(path string) error {
	f := excelize.NewFile()
	defer f.Close()
	return f.SaveAs(path)
}

func GetComments(path, sheet string) ([]Comment, error) {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return nil, fmt.Errorf("opening %s: %w", path, err)
	}
	defer f.Close()

	if sheet == "" {
		sheet = f.GetSheetName(0)
	}

	exComments, err := f.GetComments(sheet)
	if err != nil {
		return nil, fmt.Errorf("getting comments from %s: %w", sheet, err)
	}

	var comments []Comment
	for _, ec := range exComments {
		comments = append(comments, Comment{
			Cell:   ec.Cell,
			Author: ec.Author,
			Text:   ec.Text,
		})
	}
	return comments, nil
}

func GetCellComment(path, sheet, cell string) (*Comment, error) {
	comments, err := GetComments(path, sheet)
	if err != nil {
		return nil, err
	}
	for _, c := range comments {
		if c.Cell == cell {
			return &c, nil
		}
	}
	return nil, nil
}

func SetComment(path, sheet, cell, author, text string) error {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return fmt.Errorf("opening %s: %w", path, err)
	}
	defer f.Close()

	if sheet == "" {
		sheet = f.GetSheetName(0)
	}

	_ = f.DeleteComment(sheet, cell)

	if err := f.AddComment(sheet, excelize.Comment{
		Cell:   cell,
		Author: author,
		Text:   text,
	}); err != nil {
		return fmt.Errorf("adding comment to %s: %w", cell, err)
	}

	return f.SaveAs(path)
}

func SetComments(path, sheet string, comments []Comment) error {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return fmt.Errorf("opening %s: %w", path, err)
	}
	defer f.Close()

	if sheet == "" {
		sheet = f.GetSheetName(0)
	}

	for _, c := range comments {
		_ = f.DeleteComment(sheet, c.Cell)
		if err := f.AddComment(sheet, excelize.Comment{
			Cell:   c.Cell,
			Author: c.Author,
			Text:   c.Text,
		}); err != nil {
			return fmt.Errorf("adding comment to %s: %w", c.Cell, err)
		}
	}

	return f.SaveAs(path)
}

func DeleteComment(path, sheet, cell string) error {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return fmt.Errorf("opening %s: %w", path, err)
	}
	defer f.Close()

	if sheet == "" {
		sheet = f.GetSheetName(0)
	}

	if err := f.DeleteComment(sheet, cell); err != nil {
		return fmt.Errorf("deleting comment from %s: %w", cell, err)
	}

	return f.SaveAs(path)
}

func DeleteComments(path, sheet string, cells []string) error {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return fmt.Errorf("opening %s: %w", path, err)
	}
	defer f.Close()

	if sheet == "" {
		sheet = f.GetSheetName(0)
	}

	for _, cell := range cells {
		if err := f.DeleteComment(sheet, cell); err != nil {
			return fmt.Errorf("deleting comment from %s: %w", cell, err)
		}
	}

	return f.SaveAs(path)
}
