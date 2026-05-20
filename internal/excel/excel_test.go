package excel

import (
	"path/filepath"
	"testing"

	"github.com/narasaka/xhist/internal/format"
	"github.com/xuri/excelize/v2"
)

func TestCreateWorkbook(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.xlsx")
	if err := CreateWorkbook(path); err != nil {
		t.Fatalf("CreateWorkbook: %v", err)
	}

	sheets, err := ListSheets(path)
	if err != nil {
		t.Fatalf("ListSheets: %v", err)
	}
	if len(sheets) != 1 {
		t.Fatalf("expected 1 sheet, got %d", len(sheets))
	}
	if sheets[0].Name != "Sheet1" {
		t.Fatalf("expected Sheet1, got %q", sheets[0].Name)
	}
}

func TestWriteReadSingleString(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.xlsx")
	if err := CreateWorkbook(path); err != nil {
		t.Fatal(err)
	}

	cells := [][]format.Cell{
		{{Type: format.CellString, String: "hello"}},
	}
	if err := WriteCells(path, "Sheet1", "A1", cells); err != nil {
		t.Fatalf("WriteCells: %v", err)
	}

	got, err := ReadCells(path, "Sheet1", "A1", "A1")
	if err != nil {
		t.Fatalf("ReadCells: %v", err)
	}
	if len(got) != 1 || len(got[0]) != 1 {
		t.Fatalf("expected 1x1 grid, got %dx%d", len(got), len(got[0]))
	}
	if got[0][0].Type != format.CellString || got[0][0].String != "hello" {
		t.Fatalf("expected string 'hello', got type=%d val=%q", got[0][0].Type, got[0][0].String)
	}
}

func TestWriteReadMixedTypes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.xlsx")
	if err := CreateWorkbook(path); err != nil {
		t.Fatal(err)
	}

	cells := [][]format.Cell{
		{
			{Type: format.CellString, String: "name"},
			{Type: format.CellNumber, Number: 42.5},
			{Type: format.CellBool, Bool: true},
			{Type: format.CellEmpty},
		},
		{
			{Type: format.CellString, String: "row2"},
			{Type: format.CellNumber, Number: -100},
			{Type: format.CellBool, Bool: false},
			{Type: format.CellError, Error: "#N/A"},
		},
	}
	if err := WriteCells(path, "Sheet1", "B2", cells); err != nil {
		t.Fatalf("WriteCells: %v", err)
	}

	got, err := ReadCells(path, "Sheet1", "B2", "E3")
	if err != nil {
		t.Fatalf("ReadCells: %v", err)
	}
	if len(got) != 2 || len(got[0]) != 4 {
		t.Fatalf("expected 2x4 grid, got %dx%d", len(got), len(got[0]))
	}

	assertCell := func(r, c int, wantType uint8, desc string) {
		t.Helper()
		if got[r][c].Type != wantType {
			t.Errorf("cell [%d][%d] (%s): expected type %d, got %d", r, c, desc, wantType, got[r][c].Type)
		}
	}

	assertCell(0, 0, format.CellString, "string")
	if got[0][0].String != "name" {
		t.Errorf("expected 'name', got %q", got[0][0].String)
	}

	assertCell(0, 1, format.CellNumber, "number")
	if got[0][1].Number != 42.5 {
		t.Errorf("expected 42.5, got %f", got[0][1].Number)
	}

	assertCell(0, 2, format.CellBool, "bool-true")
	if !got[0][2].Bool {
		t.Error("expected true")
	}

	assertCell(0, 3, format.CellEmpty, "empty")

	assertCell(1, 0, format.CellString, "string-row2")
	assertCell(1, 1, format.CellNumber, "negative")
	if got[1][1].Number != -100 {
		t.Errorf("expected -100, got %f", got[1][1].Number)
	}

	assertCell(1, 2, format.CellBool, "bool-false")
	if got[1][2].Bool {
		t.Error("expected false")
	}
}

func TestWriteReadFormula(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.xlsx")
	if err := CreateWorkbook(path); err != nil {
		t.Fatal(err)
	}

	cells := [][]format.Cell{
		{
			{Type: format.CellNumber, Number: 10},
			{Type: format.CellNumber, Number: 20},
			{Type: format.CellFormula, FormulaText: "A1+B1"},
		},
	}
	if err := WriteCells(path, "Sheet1", "A1", cells); err != nil {
		t.Fatalf("WriteCells: %v", err)
	}

	got, err := ReadCells(path, "Sheet1", "C1", "C1")
	if err != nil {
		t.Fatalf("ReadCells: %v", err)
	}
	if got[0][0].Type != format.CellFormula {
		t.Fatalf("expected formula type, got %d", got[0][0].Type)
	}
	if got[0][0].FormulaText != "A1+B1" {
		t.Fatalf("expected formula 'A1+B1', got %q", got[0][0].FormulaText)
	}
}

func TestListSheetsDimensions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.xlsx")
	if err := CreateWorkbook(path); err != nil {
		t.Fatal(err)
	}

	cells := [][]format.Cell{
		{{Type: format.CellString, String: "a"}, {Type: format.CellString, String: "b"}},
		{{Type: format.CellString, String: "c"}, {Type: format.CellString, String: "d"}},
		{{Type: format.CellString, String: "e"}, {Type: format.CellString, String: "f"}},
	}
	if err := WriteCells(path, "Sheet1", "A1", cells); err != nil {
		t.Fatal(err)
	}

	sheets, err := ListSheets(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(sheets) != 1 {
		t.Fatalf("expected 1 sheet, got %d", len(sheets))
	}
	if sheets[0].Rows != 3 || sheets[0].Cols != 2 {
		t.Errorf("expected 3x2, got %dx%d", sheets[0].Rows, sheets[0].Cols)
	}
}

func TestReadCellsDefaultSheet(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.xlsx")
	if err := CreateWorkbook(path); err != nil {
		t.Fatal(err)
	}

	cells := [][]format.Cell{{{Type: format.CellString, String: "auto"}}}
	if err := WriteCells(path, "Sheet1", "A1", cells); err != nil {
		t.Fatal(err)
	}

	got, err := ReadCells(path, "", "A1", "A1")
	if err != nil {
		t.Fatal(err)
	}
	if got[0][0].String != "auto" {
		t.Errorf("expected 'auto', got %q", got[0][0].String)
	}
}

func TestUnicodeString(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.xlsx")
	if err := CreateWorkbook(path); err != nil {
		t.Fatal(err)
	}

	want := "日本語テスト 🎉"
	cells := [][]format.Cell{{{Type: format.CellString, String: want}}}
	if err := WriteCells(path, "Sheet1", "A1", cells); err != nil {
		t.Fatal(err)
	}

	got, err := ReadCells(path, "Sheet1", "A1", "A1")
	if err != nil {
		t.Fatal(err)
	}
	if got[0][0].String != want {
		t.Errorf("expected %q, got %q", want, got[0][0].String)
	}
}

func TestLargeNumber(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.xlsx")
	if err := CreateWorkbook(path); err != nil {
		t.Fatal(err)
	}

	want := 1e15
	cells := [][]format.Cell{{{Type: format.CellNumber, Number: want}}}
	if err := WriteCells(path, "Sheet1", "A1", cells); err != nil {
		t.Fatal(err)
	}

	got, err := ReadCells(path, "Sheet1", "A1", "A1")
	if err != nil {
		t.Fatal(err)
	}
	if got[0][0].Number != want {
		t.Errorf("expected %g, got %g", want, got[0][0].Number)
	}
}

func TestParseRange(t *testing.T) {
	tests := []struct {
		input                       string
		sheet, topLeft, bottomRight string
		wantErr                     bool
	}{
		{"Sheet1!A1:C10", "Sheet1", "A1", "C10", false},
		{"A1:C10", "", "A1", "C10", false},
		{"A1", "", "A1", "A1", false},
		{"My Sheet!B2:D5", "My Sheet", "B2", "D5", false},
		{"", "", "", "", true},
	}

	for _, tt := range tests {
		sheet, tl, br, err := ParseRange(tt.input)
		if tt.wantErr {
			if err == nil {
				t.Errorf("ParseRange(%q): expected error", tt.input)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseRange(%q): %v", tt.input, err)
			continue
		}
		if sheet != tt.sheet || tl != tt.topLeft || br != tt.bottomRight {
			t.Errorf("ParseRange(%q) = (%q, %q, %q), want (%q, %q, %q)",
				tt.input, sheet, tl, br, tt.sheet, tt.topLeft, tt.bottomRight)
		}
	}
}

func TestCellRefRoundTrip(t *testing.T) {
	tests := []struct {
		row, col int
		ref      string
	}{
		{0, 0, "A1"},
		{0, 25, "Z1"},
		{0, 26, "AA1"},
		{9, 2, "C10"},
	}

	for _, tt := range tests {
		ref := CellRef(tt.row, tt.col)
		if ref != tt.ref {
			t.Errorf("CellRef(%d,%d) = %q, want %q", tt.row, tt.col, ref, tt.ref)
		}
		r, c, err := ParseCellRef(ref)
		if err != nil {
			t.Errorf("ParseCellRef(%q): %v", ref, err)
			continue
		}
		if r != tt.row || c != tt.col {
			t.Errorf("ParseCellRef(%q) = (%d,%d), want (%d,%d)", ref, r, c, tt.row, tt.col)
		}
	}
}

func TestRangeSize(t *testing.T) {
	rows, cols, err := RangeSize("A1", "C10")
	if err != nil {
		t.Fatal(err)
	}
	if rows != 10 || cols != 3 {
		t.Errorf("expected 10x3, got %dx%d", rows, cols)
	}

	rows, cols, err = RangeSize("B2", "B2")
	if err != nil {
		t.Fatal(err)
	}
	if rows != 1 || cols != 1 {
		t.Errorf("expected 1x1, got %dx%d", rows, cols)
	}
}

func TestSetAndGetComment(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.xlsx")
	if err := CreateWorkbook(path); err != nil {
		t.Fatal(err)
	}

	if err := SetComment(path, "Sheet1", "A1", "agent-1", "Source: SAP report"); err != nil {
		t.Fatalf("SetComment: %v", err)
	}

	c, err := GetCellComment(path, "Sheet1", "A1")
	if err != nil {
		t.Fatalf("GetCellComment: %v", err)
	}
	if c == nil {
		t.Fatal("expected comment, got nil")
	}
	if c.Author != "agent-1" {
		t.Errorf("expected author 'agent-1', got %q", c.Author)
	}
	if c.Text != "Source: SAP report" {
		t.Errorf("expected text 'Source: SAP report', got %q", c.Text)
	}
}

func TestSetCommentReplace(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.xlsx")
	if err := CreateWorkbook(path); err != nil {
		t.Fatal(err)
	}

	if err := SetComment(path, "Sheet1", "A1", "agent-1", "first"); err != nil {
		t.Fatal(err)
	}

	if err := SetComment(path, "Sheet1", "A1", "agent-2", "second"); err != nil {
		t.Fatal(err)
	}

	c, err := GetCellComment(path, "Sheet1", "A1")
	if err != nil {
		t.Fatal(err)
	}
	if c == nil {
		t.Fatal("expected comment, got nil")
	}
	if c.Author != "agent-2" || c.Text != "second" {
		t.Errorf("expected replaced comment, got author=%q text=%q", c.Author, c.Text)
	}
}

func TestGetCommentsMultiple(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.xlsx")
	if err := CreateWorkbook(path); err != nil {
		t.Fatal(err)
	}

	comments := []Comment{
		{Cell: "A1", Author: "agent-1", Text: "first"},
		{Cell: "B2", Author: "agent-1", Text: "second"},
		{Cell: "C3", Author: "agent-2", Text: "third"},
	}
	if err := SetComments(path, "Sheet1", comments); err != nil {
		t.Fatal(err)
	}

	got, err := GetComments(path, "Sheet1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 comments, got %d", len(got))
	}
}

func TestDeleteComment(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.xlsx")
	if err := CreateWorkbook(path); err != nil {
		t.Fatal(err)
	}

	if err := SetComment(path, "Sheet1", "A1", "agent-1", "to delete"); err != nil {
		t.Fatal(err)
	}

	if err := DeleteComment(path, "Sheet1", "A1"); err != nil {
		t.Fatal(err)
	}

	c, err := GetCellComment(path, "Sheet1", "A1")
	if err != nil {
		t.Fatal(err)
	}
	if c != nil {
		t.Fatalf("expected nil after delete, got %+v", c)
	}
}

func TestDeleteComments(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.xlsx")
	if err := CreateWorkbook(path); err != nil {
		t.Fatal(err)
	}

	comments := []Comment{
		{Cell: "A1", Author: "a", Text: "1"},
		{Cell: "B1", Author: "a", Text: "2"},
		{Cell: "C1", Author: "a", Text: "3"},
	}
	if err := SetComments(path, "Sheet1", comments); err != nil {
		t.Fatal(err)
	}

	if err := DeleteComments(path, "Sheet1", []string{"A1", "C1"}); err != nil {
		t.Fatal(err)
	}

	got, err := GetComments(path, "Sheet1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 remaining comment, got %d", len(got))
	}
	if got[0].Cell != "B1" {
		t.Errorf("expected B1 remaining, got %s", got[0].Cell)
	}
}

func TestGetCellCommentNotFound(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.xlsx")
	if err := CreateWorkbook(path); err != nil {
		t.Fatal(err)
	}

	c, err := GetCellComment(path, "Sheet1", "A1")
	if err != nil {
		t.Fatal(err)
	}
	if c != nil {
		t.Fatalf("expected nil for cell without comment, got %+v", c)
	}
}

func TestGetCommentsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.xlsx")
	if err := CreateWorkbook(path); err != nil {
		t.Fatal(err)
	}

	got, err := GetComments(path, "Sheet1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("expected 0 comments on empty sheet, got %d", len(got))
	}
}

func TestCommentDefaultSheet(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.xlsx")
	if err := CreateWorkbook(path); err != nil {
		t.Fatal(err)
	}

	if err := SetComment(path, "", "A1", "agent", "default sheet"); err != nil {
		t.Fatal(err)
	}

	c, err := GetCellComment(path, "", "A1")
	if err != nil {
		t.Fatal(err)
	}
	if c == nil || c.Text != "default sheet" {
		t.Fatalf("expected comment on default sheet, got %+v", c)
	}
}

func TestReadReturnsRawNumberIgnoringFormat(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.xlsx")
	if err := CreateWorkbook(path); err != nil {
		t.Fatal(err)
	}

	cells := [][]format.Cell{
		{
			{Type: format.CellNumber, Number: 231521},
			{Type: format.CellNumber, Number: -456},
			{Type: format.CellNumber, Number: 0.1234},
		},
	}
	if err := WriteCells(path, "Sheet1", "A1", cells); err != nil {
		t.Fatalf("WriteCells: %v", err)
	}

	f, err := excelize.OpenFile(path)
	if err != nil {
		t.Fatal(err)
	}
	accountingFmt := "#,##0;(#,##0)"
	percentFmt := "0.00%"
	thousandsFmt := `#,##0`
	thousandsStyle, err := f.NewStyle(&excelize.Style{CustomNumFmt: &thousandsFmt})
	if err != nil {
		t.Fatal(err)
	}
	accountingStyle, err := f.NewStyle(&excelize.Style{CustomNumFmt: &accountingFmt})
	if err != nil {
		t.Fatal(err)
	}
	percentStyle, err := f.NewStyle(&excelize.Style{CustomNumFmt: &percentFmt})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.SetCellStyle("Sheet1", "A1", "A1", thousandsStyle); err != nil {
		t.Fatal(err)
	}
	if err := f.SetCellStyle("Sheet1", "B1", "B1", accountingStyle); err != nil {
		t.Fatal(err)
	}
	if err := f.SetCellStyle("Sheet1", "C1", "C1", percentStyle); err != nil {
		t.Fatal(err)
	}
	if err := f.SaveAs(path); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()

	got, err := ReadCells(path, "Sheet1", "A1", "C1")
	if err != nil {
		t.Fatalf("ReadCells: %v", err)
	}
	if len(got) != 1 || len(got[0]) != 3 {
		t.Fatalf("expected 1x3 grid, got %dx%d", len(got), len(got[0]))
	}

	for i, want := range []struct {
		number  float64
		context string
	}{
		{231521, "thousands-formatted number must round-trip as raw 231521 (not \"231,521\")"},
		{-456, "accounting-formatted negative must round-trip as raw -456 (not \"(456)\")"},
		{0.1234, "percent-formatted number must round-trip as raw 0.1234 (not \"12.34%\")"},
	} {
		cell := got[0][i]
		if cell.Type != format.CellNumber {
			t.Fatalf("cell [0][%d] (%s): expected CellNumber, got type %d with String=%q", i, want.context, cell.Type, cell.String)
		}
		if cell.Number != want.number {
			t.Fatalf("cell [0][%d] (%s): expected Number=%v, got %v", i, want.context, want.number, cell.Number)
		}
	}
}

func TestReadReturnsRawBool(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.xlsx")
	if err := CreateWorkbook(path); err != nil {
		t.Fatal(err)
	}

	cells := [][]format.Cell{
		{{Type: format.CellBool, Bool: true}, {Type: format.CellBool, Bool: false}},
	}
	if err := WriteCells(path, "Sheet1", "A1", cells); err != nil {
		t.Fatal(err)
	}

	got, err := ReadCells(path, "Sheet1", "A1", "B1")
	if err != nil {
		t.Fatal(err)
	}
	if got[0][0].Type != format.CellBool || !got[0][0].Bool {
		t.Fatalf("A1: expected CellBool true, got type=%d bool=%v", got[0][0].Type, got[0][0].Bool)
	}
	if got[0][1].Type != format.CellBool || got[0][1].Bool {
		t.Fatalf("B1: expected CellBool false, got type=%d bool=%v", got[0][1].Type, got[0][1].Bool)
	}
}
