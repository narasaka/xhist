package format

import (
	"testing"
)

func TestMagicBytes(t *testing.T) {
	want := [6]byte{0x58, 0x48, 0x49, 0x53, 0x54, 0x00}
	if Magic != want {
		t.Fatalf("Magic = %x, want %x", Magic, want)
	}
	if string(Magic[:5]) != "XHIST" {
		t.Fatalf("Magic ASCII = %q, want XHIST", string(Magic[:5]))
	}
}

func TestIndexMagicBytes(t *testing.T) {
	want := [6]byte{0x58, 0x48, 0x49, 0x44, 0x58, 0x00}
	if IndexMagic != want {
		t.Fatalf("IndexMagic = %x, want %x", IndexMagic, want)
	}
	if string(IndexMagic[:5]) != "XHIDX" {
		t.Fatalf("IndexMagic ASCII = %q, want XHIDX", string(IndexMagic[:5]))
	}
}

func TestConstants(t *testing.T) {
	if Version != 0x01 {
		t.Fatalf("Version = %d, want 1", Version)
	}
	if VersionV1 != 0x01 {
		t.Fatalf("VersionV1 = %d, want 1", VersionV1)
	}
	if VersionV2 != 0x02 {
		t.Fatalf("VersionV2 = %d, want 2", VersionV2)
	}
	if VersionLatest != VersionV2 {
		t.Fatalf("VersionLatest = %d, want VersionV2", VersionLatest)
	}
	if PreambleSize != 7 {
		t.Fatalf("PreambleSize = %d, want 7", PreambleSize)
	}
	if IndexPreambleSize != 19 {
		t.Fatalf("IndexPreambleSize = %d, want 19", IndexPreambleSize)
	}
	if IndexVersion != IndexVersionV3 {
		t.Fatalf("IndexVersion = %d, want IndexVersionV3", IndexVersion)
	}
	if IndexVersionV3 != 0x03 {
		t.Fatalf("IndexVersionV3 = %d, want 3", IndexVersionV3)
	}
	if RecordFramingOverhead != 9 {
		t.Fatalf("RecordFramingOverhead = %d, want 9", RecordFramingOverhead)
	}
}

func TestOpcodes(t *testing.T) {
	if OpcodeHeader != 0x01 {
		t.Fatal("OpcodeHeader wrong")
	}
	if OpcodeOp != 0x02 {
		t.Fatal("OpcodeOp wrong")
	}
	if OpcodeMetadata != 0x03 {
		t.Fatal("OpcodeMetadata wrong")
	}
	if OpcodeCommentOp != 0x04 {
		t.Fatal("OpcodeCommentOp wrong")
	}
	if OpcodeFooter != 0xFF {
		t.Fatal("OpcodeFooter wrong")
	}
}

func TestActionTypes(t *testing.T) {
	if ActionRead != 1 {
		t.Fatal("ActionRead wrong")
	}
	if ActionWrite != 2 {
		t.Fatal("ActionWrite wrong")
	}
}

func TestCommentActionTypes(t *testing.T) {
	if ActionCommentSet != 1 {
		t.Fatal("ActionCommentSet wrong")
	}
	if ActionCommentGet != 2 {
		t.Fatal("ActionCommentGet wrong")
	}
	if ActionCommentDelete != 3 {
		t.Fatal("ActionCommentDelete wrong")
	}
}

func TestCellTypes(t *testing.T) {
	if CellEmpty != 0x00 {
		t.Fatal("CellEmpty wrong")
	}
	if CellString != 0x01 {
		t.Fatal("CellString wrong")
	}
	if CellNumber != 0x02 {
		t.Fatal("CellNumber wrong")
	}
	if CellBool != 0x03 {
		t.Fatal("CellBool wrong")
	}
	if CellFormula != 0x04 {
		t.Fatal("CellFormula wrong")
	}
	if CellError != 0x05 {
		t.Fatal("CellError wrong")
	}
}

func TestErrCorruption(t *testing.T) {
	e := &ErrCorruption{Offset: 42, Msg: "bad crc"}
	if e.Error() != "corruption at offset 42: bad crc" {
		t.Fatalf("unexpected: %s", e.Error())
	}
}

func TestErrBadMagic(t *testing.T) {
	e := &ErrBadMagic{}
	if e.Error() != "bad magic bytes" {
		t.Fatalf("unexpected: %s", e.Error())
	}
}

func TestErrBadVersion(t *testing.T) {
	e := &ErrBadVersion{Got: 99}
	if e.Error() != "unsupported version: 99" {
		t.Fatalf("unexpected: %s", e.Error())
	}
}
