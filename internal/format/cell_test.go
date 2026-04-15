package format

import (
	"bytes"
	"math"
	"testing"
)

func TestCellEmpty(t *testing.T) {
	c := Cell{Type: CellEmpty}
	var buf bytes.Buffer
	if err := EncodeCell(&buf, c); err != nil {
		t.Fatal(err)
	}
	if buf.Len() != 1 {
		t.Fatalf("empty cell should be 1 byte, got %d", buf.Len())
	}
	if buf.Bytes()[0] != CellEmpty {
		t.Fatalf("tag byte = 0x%02x, want 0x00", buf.Bytes()[0])
	}
	got, err := DecodeCell(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if got.Type != CellEmpty {
		t.Fatalf("decoded type = %d, want CellEmpty", got.Type)
	}
}

func TestCellString(t *testing.T) {
	c := Cell{Type: CellString, String: "hello"}
	var buf bytes.Buffer
	if err := EncodeCell(&buf, c); err != nil {
		t.Fatal(err)
	}
	// 1 tag + 4 length + 5 bytes = 10
	if buf.Len() != 10 {
		t.Fatalf("string cell size = %d, want 10", buf.Len())
	}
	got, err := DecodeCell(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if got.String != "hello" {
		t.Fatalf("decoded string = %q, want %q", got.String, "hello")
	}
}

func TestCellStringUnicode(t *testing.T) {
	c := Cell{Type: CellString, String: "日本語テスト🎉"}
	var buf bytes.Buffer
	if err := EncodeCell(&buf, c); err != nil {
		t.Fatal(err)
	}
	got, err := DecodeCell(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if got.String != c.String {
		t.Fatalf("decoded string = %q, want %q", got.String, c.String)
	}
}

func TestCellStringEmpty(t *testing.T) {
	c := Cell{Type: CellString, String: ""}
	var buf bytes.Buffer
	if err := EncodeCell(&buf, c); err != nil {
		t.Fatal(err)
	}
	// 1 tag + 4 length + 0 bytes = 5
	if buf.Len() != 5 {
		t.Fatalf("empty string cell size = %d, want 5", buf.Len())
	}
	got, err := DecodeCell(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if got.Type != CellString || got.String != "" {
		t.Fatalf("expected empty string cell, got type=%d str=%q", got.Type, got.String)
	}
}

func TestCellNumber(t *testing.T) {
	for _, val := range []float64{0, 1.5, -42.0, math.Pi, math.Inf(1), math.Inf(-1)} {
		c := Cell{Type: CellNumber, Number: val}
		var buf bytes.Buffer
		if err := EncodeCell(&buf, c); err != nil {
			t.Fatal(err)
		}
		// 1 tag + 8 float64 = 9
		if buf.Len() != 9 {
			t.Fatalf("number cell size = %d, want 9", buf.Len())
		}
		got, err := DecodeCell(bytes.NewReader(buf.Bytes()))
		if err != nil {
			t.Fatal(err)
		}
		if got.Number != val {
			t.Fatalf("decoded number = %v, want %v", got.Number, val)
		}
	}
}

func TestCellNumberNaN(t *testing.T) {
	c := Cell{Type: CellNumber, Number: math.NaN()}
	var buf bytes.Buffer
	if err := EncodeCell(&buf, c); err != nil {
		t.Fatal(err)
	}
	got, err := DecodeCell(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if !math.IsNaN(got.Number) {
		t.Fatalf("expected NaN, got %v", got.Number)
	}
}

func TestCellBool(t *testing.T) {
	for _, val := range []bool{true, false} {
		c := Cell{Type: CellBool, Bool: val}
		var buf bytes.Buffer
		if err := EncodeCell(&buf, c); err != nil {
			t.Fatal(err)
		}
		// 1 tag + 1 byte = 2
		if buf.Len() != 2 {
			t.Fatalf("bool cell size = %d, want 2", buf.Len())
		}
		got, err := DecodeCell(bytes.NewReader(buf.Bytes()))
		if err != nil {
			t.Fatal(err)
		}
		if got.Bool != val {
			t.Fatalf("decoded bool = %v, want %v", got.Bool, val)
		}
	}
}

func TestCellFormula(t *testing.T) {
	cached := Cell{Type: CellNumber, Number: 42.0}
	c := Cell{Type: CellFormula, FormulaText: "=SUM(A1:A10)", CachedValue: &cached}
	var buf bytes.Buffer
	if err := EncodeCell(&buf, c); err != nil {
		t.Fatal(err)
	}
	got, err := DecodeCell(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if got.Type != CellFormula {
		t.Fatal("wrong type")
	}
	if got.FormulaText != "=SUM(A1:A10)" {
		t.Fatalf("formula text = %q", got.FormulaText)
	}
	if got.CachedValue == nil {
		t.Fatal("cached value is nil")
	}
	if got.CachedValue.Type != CellNumber || got.CachedValue.Number != 42.0 {
		t.Fatalf("cached value wrong: %+v", got.CachedValue)
	}
}

func TestCellFormulaCachedString(t *testing.T) {
	cached := Cell{Type: CellString, String: "result"}
	c := Cell{Type: CellFormula, FormulaText: "=IF(A1,\"yes\",\"no\")", CachedValue: &cached}
	var buf bytes.Buffer
	if err := EncodeCell(&buf, c); err != nil {
		t.Fatal(err)
	}
	got, err := DecodeCell(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if got.CachedValue.Type != CellString || got.CachedValue.String != "result" {
		t.Fatalf("cached string wrong: %+v", got.CachedValue)
	}
}

func TestCellFormulaCachedBool(t *testing.T) {
	cached := Cell{Type: CellBool, Bool: true}
	c := Cell{Type: CellFormula, FormulaText: "=TRUE()", CachedValue: &cached}
	var buf bytes.Buffer
	if err := EncodeCell(&buf, c); err != nil {
		t.Fatal(err)
	}
	got, err := DecodeCell(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if got.CachedValue.Type != CellBool || !got.CachedValue.Bool {
		t.Fatalf("cached bool wrong: %+v", got.CachedValue)
	}
}

func TestCellFormulaCachedError(t *testing.T) {
	cached := Cell{Type: CellError, Error: "#DIV/0!"}
	c := Cell{Type: CellFormula, FormulaText: "=1/0", CachedValue: &cached}
	var buf bytes.Buffer
	if err := EncodeCell(&buf, c); err != nil {
		t.Fatal(err)
	}
	got, err := DecodeCell(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if got.CachedValue.Type != CellError || got.CachedValue.Error != "#DIV/0!" {
		t.Fatalf("cached error wrong: %+v", got.CachedValue)
	}
}

func TestCellFormulaNilCached(t *testing.T) {
	c := Cell{Type: CellFormula, FormulaText: "=NOW()", CachedValue: nil}
	var buf bytes.Buffer
	if err := EncodeCell(&buf, c); err != nil {
		t.Fatal(err)
	}
	got, err := DecodeCell(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if got.CachedValue == nil || got.CachedValue.Type != CellEmpty {
		t.Fatalf("nil cached should decode to empty cell, got %+v", got.CachedValue)
	}
}

func TestCellError(t *testing.T) {
	for _, errStr := range []string{"#N/A", "#REF!", "#VALUE!", "#DIV/0!", "#NAME?"} {
		c := Cell{Type: CellError, Error: errStr}
		var buf bytes.Buffer
		if err := EncodeCell(&buf, c); err != nil {
			t.Fatal(err)
		}
		got, err := DecodeCell(bytes.NewReader(buf.Bytes()))
		if err != nil {
			t.Fatal(err)
		}
		if got.Error != errStr {
			t.Fatalf("decoded error = %q, want %q", got.Error, errStr)
		}
	}
}

func TestCellEmptyVsEmptyString(t *testing.T) {
	empty := Cell{Type: CellEmpty}
	emptyStr := Cell{Type: CellString, String: ""}

	var buf1, buf2 bytes.Buffer
	EncodeCell(&buf1, empty)
	EncodeCell(&buf2, emptyStr)

	if bytes.Equal(buf1.Bytes(), buf2.Bytes()) {
		t.Fatal("empty cell and empty string cell should have different encodings")
	}
	if buf1.Len() != 1 {
		t.Fatalf("empty cell = %d bytes, want 1", buf1.Len())
	}
	if buf2.Len() != 5 {
		t.Fatalf("empty string cell = %d bytes, want 5", buf2.Len())
	}
}

func TestCellAllTypesRoundTrip(t *testing.T) {
	cells := []Cell{
		{Type: CellEmpty},
		{Type: CellString, String: "test"},
		{Type: CellNumber, Number: 3.14},
		{Type: CellBool, Bool: true},
		{Type: CellFormula, FormulaText: "=A1", CachedValue: &Cell{Type: CellNumber, Number: 1}},
		{Type: CellError, Error: "#N/A"},
	}
	for _, c := range cells {
		var buf bytes.Buffer
		if err := EncodeCell(&buf, c); err != nil {
			t.Fatalf("encode %d: %v", c.Type, err)
		}
		got, err := DecodeCell(bytes.NewReader(buf.Bytes()))
		if err != nil {
			t.Fatalf("decode %d: %v", c.Type, err)
		}
		if got.Type != c.Type {
			t.Fatalf("type mismatch: got %d want %d", got.Type, c.Type)
		}
	}
}
