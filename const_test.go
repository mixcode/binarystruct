package binarystruct

import (
	"bytes"
	"errors"
	"testing"
)

// const= emits a fixed value on encode (ignoring the struct field) and
// validates it on decode. Two target shapes: integer/bitmap and raw
// byte-sequence (hex blob).

func TestConst_IntegerEmitAndValidate(t *testing.T) {
	type Hdr struct {
		Sig  uint32 `binary:"uint32,const=0x04034b50,endian=little"`
		Body uint16 `binary:"uint16,endian=little"`
	}
	// Emit-only: leave Sig at zero; const must still write the magic.
	blob, err := Marshal(&Hdr{Body: 0x1122}, BigEndian)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	// endian=little on Sig pins its bytes regardless of the BigEndian arg.
	want := []byte{0x50, 0x4b, 0x03, 0x04, 0x22, 0x11} // Sig LE magic, Body LE
	if !bytes.Equal(blob, want) {
		t.Fatalf("got %x want %x", blob, want)
	}

	// Decode validates: correct magic passes, the Go field is populated.
	var out Hdr
	if _, err := Unmarshal(blob, BigEndian, &out); err != nil {
		t.Fatalf("decode good: %v", err)
	}
	if out.Sig != 0x04034b50 {
		t.Fatalf("Sig = %#x, want 0x04034b50", out.Sig)
	}
	// Wrong magic is rejected.
	bad := append([]byte{}, blob...)
	bad[0] = 0xff
	var out2 Hdr
	if _, err := Unmarshal(bad, BigEndian, &out2); !errors.Is(err, ErrValidationError) {
		t.Fatalf("decode bad: want ErrValidationError, got %v", err)
	}
}

func TestConst_IntegerEndianSensitivity(t *testing.T) {
	// Without an explicit endian, the integer const rides the marshaller order.
	type H struct {
		Sig uint32 `binary:"uint32,const=0x04034b50"`
	}
	le, _ := Marshal(&H{}, LittleEndian)
	be, _ := Marshal(&H{}, BigEndian)
	if bytes.Equal(le, be) {
		t.Fatalf("expected endian to affect integer const bytes")
	}
	if !bytes.Equal(le, []byte{0x50, 0x4b, 0x03, 0x04}) {
		t.Fatalf("LE got %x", le)
	}
	if !bytes.Equal(be, []byte{0x04, 0x03, 0x4b, 0x50}) {
		t.Fatalf("BE got %x", be)
	}
}

func TestConst_ByteSequenceArray(t *testing.T) {
	// PK\x03\x04 in natural byte order — endianness-independent.
	type H struct {
		Magic [4]byte `binary:"[4]byte,const=0x504b0304"`
		N     uint8   `binary:"uint8"`
	}
	for _, order := range []ByteOrder{LittleEndian, BigEndian} {
		blob, err := Marshal(&H{N: 7}, order)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		want := []byte{0x50, 0x4b, 0x03, 0x04, 0x07}
		if !bytes.Equal(blob, want) {
			t.Fatalf("order %v: got %x want %x", order, blob, want)
		}
		var out H
		if _, err := Unmarshal(blob, order, &out); err != nil {
			t.Fatalf("decode good: %v", err)
		}
		if out.Magic != [4]byte{0x50, 0x4b, 0x03, 0x04} {
			t.Fatalf("Magic = %x", out.Magic)
		}
	}
	// Wrong magic rejected.
	bad := []byte{0x50, 0x4b, 0x03, 0xff, 0x07}
	var out H
	if _, err := Unmarshal(bad, LittleEndian, &out); !errors.Is(err, ErrValidationError) {
		t.Fatalf("decode bad: want ErrValidationError, got %v", err)
	}
}

func TestConst_StringSized(t *testing.T) {
	type H struct {
		Magic string `binary:"string(4),const=0x25504446"` // %PDF
	}
	blob, err := Marshal(&H{}, BigEndian)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(blob) != "%PDF" {
		t.Fatalf("got %q want %q", blob, "%PDF")
	}
	var out H
	if _, err := Unmarshal(blob, BigEndian, &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Magic != "%PDF" {
		t.Fatalf("Magic = %q", out.Magic)
	}
}

func TestConst_MetadataErrors(t *testing.T) {
	cases := []struct {
		name string
		val  any
		want string
	}{
		{"float target", new(struct {
			F float32 `binary:"float32,const=1"`
		}), "integer/bitmap or raw byte-sequence"},
		{"const+valueof", new(struct {
			A uint8  `binary:"uint8,const=1,valueof=count(B)"`
			B []byte `binary:"[2]byte"`
		}), "cannot be combined with valueof"},
		{"byte len mismatch", new(struct {
			M [4]byte `binary:"[4]byte,const=0x504b03"` // 3 bytes vs 4
		}), "field size"},
		{"odd hex", new(struct {
			M [2]byte `binary:"[2]byte,const=0x504"`
		}), "even number of hex digits"},
		{"non-hex bytes", new(struct {
			M [2]byte `binary:"[2]byte,const=4242"` // missing 0x
		}), "hex blob"},
		{"encoding combo", new(struct {
			M string `binary:"string(2),const=0x4142,encoding=shift-jis"`
		}), "encoding"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := Marshal(c.val, BigEndian)
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", c.want)
			}
			if !contains(err.Error(), c.want) {
				t.Fatalf("error %q does not contain %q", err.Error(), c.want)
			}
		})
	}
}

func contains(s, sub string) bool { return bytes.Contains([]byte(s), []byte(sub)) }
