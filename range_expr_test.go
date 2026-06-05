package binarystruct

import (
	"errors"
	"testing"
)

// range bounds should accept the same numeric syntax as size expressions:
// hex/octal/binary literals and arithmetic, in addition to plain decimal and
// floating-point bounds (which must keep working).

func TestRange_HexBounds(t *testing.T) {
	type S struct {
		// 0x10 = 16, 0x20 = 32
		V uint8 `binary:"uint8,range=0x10..0x20"`
	}
	mustPass := func(val uint8) {
		blob, err := NewMarshalerOrder(BigEndian).Marshal(&S{V: val})
		if err != nil {
			t.Fatalf("marshal %#x: %v", val, err)
		}
		var out S
		if _, err := NewMarshalerOrder(BigEndian).Unmarshal(blob, &out); err != nil {
			t.Fatalf("unmarshal %#x rejected: %v", val, err)
		}
	}
	mustFail := func(val uint8) {
		blob, err := NewMarshalerOrder(BigEndian).Marshal(&S{V: val})
		if err != nil {
			t.Fatalf("marshal %#x: %v", val, err)
		}
		var out S
		_, err = NewMarshalerOrder(BigEndian).Unmarshal(blob, &out)
		if !errors.Is(err, ErrValidationError) {
			t.Fatalf("value %#x: want ErrValidationError, got %v", val, err)
		}
	}
	mustPass(0x10)
	mustPass(0x18)
	mustPass(0x20)
	mustFail(0x0f)
	mustFail(0x21)
}

func TestRange_ArithmeticBounds(t *testing.T) {
	type S struct {
		V uint16 `binary:"uint16,range=10*10..(20+5)*4"` // 100..100? no: 100..100 -> 100..100
	}
	// 10*10 = 100 ; (20+5)*4 = 100  -> exact pin at 100
	var out S
	good, _ := NewMarshalerOrder(BigEndian).Marshal(&S{V: 100})
	if _, err := NewMarshalerOrder(BigEndian).Unmarshal(good, &out); err != nil {
		t.Fatalf("100 rejected: %v", err)
	}
	bad, _ := NewMarshalerOrder(BigEndian).Marshal(&S{V: 101})
	if _, err := NewMarshalerOrder(BigEndian).Unmarshal(bad, &out); !errors.Is(err, ErrValidationError) {
		t.Fatalf("101: want ErrValidationError, got %v", err)
	}
}

func TestRange_FloatBounds(t *testing.T) {
	type S struct {
		F float64 `binary:"float64,range=1.5..3.5"`
	}
	var out S
	good, _ := NewMarshalerOrder(BigEndian).Marshal(&S{F: 2.0})
	if _, err := NewMarshalerOrder(BigEndian).Unmarshal(good, &out); err != nil {
		t.Fatalf("2.0 rejected: %v", err)
	}
	bad, _ := NewMarshalerOrder(BigEndian).Marshal(&S{F: 9.0})
	if _, err := NewMarshalerOrder(BigEndian).Unmarshal(bad, &out); !errors.Is(err, ErrValidationError) {
		t.Fatalf("9.0: want ErrValidationError, got %v", err)
	}
}

func TestRange_OpenHexBound(t *testing.T) {
	type S struct {
		V uint16 `binary:"uint16,range=0x100.."` // >= 256, open upper
	}
	var out S
	good, _ := NewMarshalerOrder(BigEndian).Marshal(&S{V: 0x200})
	if _, err := NewMarshalerOrder(BigEndian).Unmarshal(good, &out); err != nil {
		t.Fatalf("0x200 rejected: %v", err)
	}
	bad, _ := NewMarshalerOrder(BigEndian).Marshal(&S{V: 0xff})
	if _, err := NewMarshalerOrder(BigEndian).Unmarshal(bad, &out); !errors.Is(err, ErrValidationError) {
		t.Fatalf("0xff: want ErrValidationError, got %v", err)
	}
}
