package binarystruct

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"golang.org/x/text/encoding/japanese"
)

// vfBasic exercises bytelen() on a byte slice and count()+constant on another.
// The length fields are seeded with WRONG values to prove valueof overrides
// them at encode time (emit-only) without mutating the source struct.
type vfBasic struct {
	NameLen uint16 `binary:"uint16,valueof=bytelen(Name)"`
	Count   uint8  `binary:"uint8,valueof=count(Items)+1"`
	Name    []byte `binary:"[NameLen]byte"`
	Items   []byte `binary:"[Count-1]byte"`
}

func TestValueof_RoundTrip(t *testing.T) {
	in := vfBasic{
		NameLen: 999, // wrong on purpose
		Count:   0,   // wrong on purpose
		Name:    []byte("hello.txt"),
		Items:   []byte{1, 2, 3},
	}
	blob, err := Marshal(&in, BigEndian)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	// NameLen=9 (BE uint16), Count=4, "hello.txt", 1 2 3
	want := append([]byte{0x00, 0x09, 0x04}, append([]byte("hello.txt"), 1, 2, 3)...)
	if !bytes.Equal(blob, want) {
		t.Fatalf("blob = % x\nwant  = % x", blob, want)
	}
	// emit-only: source struct must be unchanged
	if in.NameLen != 999 || in.Count != 0 {
		t.Errorf("source mutated: NameLen=%d Count=%d", in.NameLen, in.Count)
	}

	var out vfBasic
	if _, err := Unmarshal(blob, BigEndian, &out); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if out.NameLen != 9 || out.Count != 4 || string(out.Name) != "hello.txt" || !bytes.Equal(out.Items, []byte{1, 2, 3}) {
		t.Errorf("decoded = %+v", out)
	}
}

// vfStr pairs a length field with a raw (UTF-8) string sized by it.
type vfStr struct {
	Len  uint16 `binary:"uint16,valueof=bytelen(Text)"`
	Text string `binary:"string(Len)"`
}

func TestValueof_StringBytelen(t *testing.T) {
	in := vfStr{Text: "binary"}
	blob, err := Marshal(&in, LittleEndian)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if blob[0] != 6 || blob[1] != 0 {
		t.Fatalf("len prefix = % x, want 06 00", blob[:2])
	}
	var out vfStr
	if _, err := Unmarshal(blob, LittleEndian, &out); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if out.Len != 6 || out.Text != "binary" {
		t.Errorf("decoded Len=%d Text=%q", out.Len, out.Text)
	}
}

// vfSJIS proves bytelen() measures the *encoded* byte length: the Shift-JIS
// encoding of "あい" is 4 bytes, while its UTF-8 length is 6. A naive len()
// would store 6 and corrupt the stream.
type vfSJIS struct {
	Len  uint16 `binary:"uint16,valueof=bytelen(Text)"`
	Text string `binary:"string(Len),encoding=sjis"`
}

func TestValueof_BytelenRespectsEncoding(t *testing.T) {
	var ms Marshaller
	ms.AddTextEncoding("sjis", japanese.ShiftJIS)

	in := vfSJIS{Text: "あい"}
	if len(in.Text) != 6 {
		t.Fatalf("precondition: UTF-8 len of test string = %d, want 6", len(in.Text))
	}
	blob, err := ms.Marshal(&in, LittleEndian)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	// Shift-JIS encodes each kana in 2 bytes => Len must be 4, not 6.
	if blob[0] != 4 || blob[1] != 0 {
		t.Fatalf("len prefix = % x, want 04 00 (Shift-JIS byte length)", blob[:2])
	}
	if len(blob) != 2+4 {
		t.Fatalf("blob length = %d, want 6", len(blob))
	}

	var out vfSJIS
	if _, err := ms.Unmarshal(blob, LittleEndian, &out); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if out.Len != 4 || out.Text != "あい" {
		t.Errorf("decoded Len=%d Text=%q", out.Len, out.Text)
	}
}

// vfNested measures bytelen() of a nested struct (3+2 = 5 bytes fixed).
type vfInner struct {
	A uint16 `binary:"uint16"`
	B uint8  `binary:"uint8"`
}
type vfNested struct {
	Size uint16  `binary:"uint16,valueof=bytelen(Body)"`
	Body vfInner `binary:""`
	Tail uint8   `binary:"uint8"`
}

func TestValueof_NestedStructBytelen(t *testing.T) {
	in := vfNested{Body: vfInner{A: 0x1122, B: 0x33}, Tail: 0x99}
	blob, err := Marshal(&in, BigEndian)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	// Size = bytelen(Body) = 2 + 1 = 3
	if blob[0] != 0 || blob[1] != 3 {
		t.Fatalf("size = % x, want 00 03", blob[:2])
	}
	var out vfNested
	if _, err := Unmarshal(blob, BigEndian, &out); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if out.Size != 3 || out.Body != in.Body || out.Tail != 0x99 {
		t.Errorf("decoded = %+v", out)
	}
}

func TestValueof_MetadataErrors(t *testing.T) {
	cases := []struct {
		name string
		typ  reflect.Type
		want string
	}{
		{"non-int target", reflect.TypeOf(struct {
			F float32 `binary:"float32,valueof=count(S)"`
			S string  `binary:"string(4)"`
		}{}), "integer"},
		{"array target", reflect.TypeOf(struct {
			B [2]byte `binary:"[2]byte,valueof=count(B)"`
		}{}), "array"},
		{"count on string", reflect.TypeOf(struct {
			L uint16 `binary:"uint16,valueof=count(S)"`
			S string `binary:"string(L)"`
		}{}), "slice or array"},
		{"unknown field", reflect.TypeOf(struct {
			L uint16 `binary:"uint16,valueof=bytelen(Nope)"`
		}{}), "unknown field"},
		{"bad function", reflect.TypeOf(struct {
			L uint16 `binary:"uint16,valueof=widthof(X)"`
			X []byte `binary:"[L]byte"`
		}{}), "unknown function"},
		{"function in decode expr", reflect.TypeOf(struct {
			L uint16 `binary:"uint16"`
			X []byte `binary:"[bytelen(X)]byte"`
		}{}), "not allowed"},
		{"reference cycle", reflect.TypeOf(struct {
			A uint16 `binary:"uint16,valueof=B"`
			B uint16 `binary:"uint16,valueof=A"`
		}{}), "cycle"},
	}
	for _, c := range cases {
		_, err := getStructMetadata(c.typ)
		if err == nil {
			t.Errorf("%s: expected error, got nil", c.name)
			continue
		}
		if !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: error %q does not contain %q", c.name, err.Error(), c.want)
		}
	}
}
