// Copyright 2026 github.com/mixcode

package binarystruct_test

import (
	"bytes"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// genBytelenCase builds the codegen tool, generates serialization code for
// typesSrc, drops in the supplied test file, and runs `go test` over the temp
// package. It asserts the whole pipeline succeeds; cases that codegen does not
// yet support therefore fail here (red) until they are implemented.
func genBytelenCase(t *testing.T, pkg, typesSrc, typeList, testSrc string) {
	t.Helper()
	t.Parallel()

	tmpDir, err := ioutil.TempDir(".", "tmp-bs-bytelen-")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	codegenBin := sharedCodegenBin

	if err := ioutil.WriteFile(filepath.Join(tmpDir, "types.go"), []byte("package "+pkg+"\n\n"+typesSrc), 0644); err != nil {
		t.Fatalf("failed to write types.go: %v", err)
	}

	genCmd := exec.Command(codegenBin, "-type", typeList, tmpDir)
	var genStderr bytes.Buffer
	genCmd.Stderr = &genStderr
	if err := genCmd.Run(); err != nil {
		t.Fatalf("codegen run failed: %v\nStderr: %s", err, genStderr.String())
	}

	if err := ioutil.WriteFile(filepath.Join(tmpDir, "types_test.go"), []byte("package "+pkg+"\n\n"+testSrc), 0644); err != nil {
		t.Fatalf("failed to write types_test.go: %v", err)
	}

	testArgs := []string{"test", "./" + tmpDir}
	if testing.Verbose() {
		testArgs = append(testArgs, "-v")
	}
	out, err := exec.Command("go", testArgs...).CombinedOutput()
	if testing.Verbose() {
		t.Log(string(out))
	}
	if err != nil {
		if !testing.Verbose() {
			t.Log(string(out))
		}
		t.Errorf("generated tests failed: %v", err)
	}
}

// --- Implemented cases (must pass) ---------------------------------------

// Case 5: bytelen() of a plain nested struct, measured at runtime in the
// generated code. Also covers the codegen-vs-runtime byte-equivalence invariant.
func TestCodegenBytelen_NestedStruct(t *testing.T) {
	types := `type Inner struct {
	A uint16 ` + "`" + `binary:"uint16"` + "`" + `
	B uint8  ` + "`" + `binary:"uint8"` + "`" + `
}

type Msg struct {
	BodyLen uint16 ` + "`" + `binary:"uint16,valueof=bytelen(Body)"` + "`" + `
	Body    Inner  ` + "`" + `binary:"any"` + "`" + `
}
`
	test := `import (
	"bytes"
	"testing"

	"github.com/mixcode/binarystruct"
)

func TestNested(t *testing.T) {
	s := Msg{BodyLen: 999, Body: Inner{A: 0x0102, B: 0x03}}
	blob, err := s.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary: %v", err)
	}
	// bytelen(Body) = 2 + 1 = 3; emit-only overrides the seeded 999.
	want := []byte{0x00, 0x03, 0x01, 0x02, 0x03}
	if !bytes.Equal(blob, want) {
		t.Fatalf("blob = % x\nwant  = % x", blob, want)
	}
	rt, err := binarystruct.Marshal(s, binarystruct.BigEndian)
	if err != nil {
		t.Fatalf("runtime Marshal: %v", err)
	}
	if !bytes.Equal(blob, rt) {
		t.Fatalf("codegen != runtime:\n codegen = % x\n runtime = % x", blob, rt)
	}
	var s2 Msg
	if err := s2.UnmarshalBinary(blob); err != nil {
		t.Fatalf("UnmarshalBinary: %v", err)
	}
	if s2.BodyLen != 3 || s2.Body.A != 0x0102 || s2.Body.B != 0x03 {
		t.Fatalf("round-trip mismatch: %+v", s2)
	}
}
`
	genBytelenCase(t, "tmp_bytelen_nested", types, "Msg,Inner", test)
}

// Case 5 + variable content: bytelen() of a nested struct whose size depends on
// its data (a length-prefixed bstring) must reflect the true serialized size,
// proving the measurement is a real re-encode rather than a static guess.
func TestCodegenBytelen_NestedVariableLen(t *testing.T) {
	types := `type Inner struct {
	Kind uint8  ` + "`" + `binary:"uint8"` + "`" + `
	Name string ` + "`" + `binary:"bstring"` + "`" + `
}

type Msg struct {
	BodyLen uint16 ` + "`" + `binary:"uint16,valueof=bytelen(Body)"` + "`" + `
	Body    Inner  ` + "`" + `binary:"any"` + "`" + `
}
`
	test := `import (
	"bytes"
	"testing"

	"github.com/mixcode/binarystruct"
)

func TestNestedVarLen(t *testing.T) {
	s := Msg{Body: Inner{Kind: 1, Name: "hello"}}
	blob, err := s.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary: %v", err)
	}
	// bytelen(Body) = Kind(1) + bstring len-prefix(1) + "hello"(5) = 7.
	want := append([]byte{0x00, 0x07, 0x01, 0x05}, []byte("hello")...)
	if !bytes.Equal(blob, want) {
		t.Fatalf("blob = % x\nwant  = % x", blob, want)
	}
	rt, err := binarystruct.Marshal(s, binarystruct.BigEndian)
	if err != nil {
		t.Fatalf("runtime Marshal: %v", err)
	}
	if !bytes.Equal(blob, rt) {
		t.Fatalf("codegen != runtime:\n codegen = % x\n runtime = % x", blob, rt)
	}
	var s2 Msg
	if err := s2.UnmarshalBinary(blob); err != nil {
		t.Fatalf("UnmarshalBinary: %v", err)
	}
	if s2.BodyLen != 7 || s2.Body.Kind != 1 || s2.Body.Name != "hello" {
		t.Fatalf("round-trip mismatch: %+v", s2)
	}
}
`
	genBytelenCase(t, "tmp_bytelen_nestedvar", types, "Msg,Inner", test)
}

// Case 5 + arithmetic: bytelen(Body)+2 composes with the hoisted measurement.
func TestCodegenBytelen_NestedArithmetic(t *testing.T) {
	types := `type Inner struct {
	A uint16 ` + "`" + `binary:"uint16"` + "`" + `
	B uint8  ` + "`" + `binary:"uint8"` + "`" + `
}

type Msg struct {
	BodyLen uint16 ` + "`" + `binary:"uint16,valueof=bytelen(Body)+2"` + "`" + `
	Body    Inner  ` + "`" + `binary:"any"` + "`" + `
}
`
	test := `import (
	"bytes"
	"testing"

	"github.com/mixcode/binarystruct"
)

func TestNestedArith(t *testing.T) {
	s := Msg{Body: Inner{A: 0x0102, B: 0x03}}
	blob, err := s.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary: %v", err)
	}
	// bytelen(Body)+2 = 3 + 2 = 5.
	want := []byte{0x00, 0x05, 0x01, 0x02, 0x03}
	if !bytes.Equal(blob, want) {
		t.Fatalf("blob = % x\nwant  = % x", blob, want)
	}
	rt, err := binarystruct.Marshal(s, binarystruct.BigEndian)
	if err != nil {
		t.Fatalf("runtime Marshal: %v", err)
	}
	if !bytes.Equal(blob, rt) {
		t.Fatalf("codegen != runtime:\n codegen = % x\n runtime = % x", blob, rt)
	}
}
`
	genBytelenCase(t, "tmp_bytelen_arith", types, "Msg,Inner", test)
}

// Case 2: bytelen() of a fixed-width scalar array is width*count (here 4*2).
func TestCodegenBytelen_ScalarArray(t *testing.T) {
	types := `type Msg struct {
	DataLen uint16  ` + "`" + `binary:"uint16,valueof=bytelen(Data)"` + "`" + `
	Data    []int16 ` + "`" + `binary:"[4]int16"` + "`" + `
}
`
	test := `import (
	"bytes"
	"testing"

	"github.com/mixcode/binarystruct"
)

func TestScalarArray(t *testing.T) {
	s := Msg{Data: []int16{1, 2, 3, 4}}
	blob, err := s.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary: %v", err)
	}
	// bytelen(Data) = 4 elements * 2 bytes = 8.
	want := []byte{0x00, 0x08, 0x00, 0x01, 0x00, 0x02, 0x00, 0x03, 0x00, 0x04}
	if !bytes.Equal(blob, want) {
		t.Fatalf("blob = % x\nwant  = % x", blob, want)
	}
	rt, err := binarystruct.Marshal(s, binarystruct.BigEndian)
	if err != nil {
		t.Fatalf("runtime Marshal: %v", err)
	}
	if !bytes.Equal(blob, rt) {
		t.Fatalf("codegen != runtime:\n codegen = % x\n runtime = % x", blob, rt)
	}
}
`
	genBytelenCase(t, "tmp_bytelen_scalararr", types, "Msg", test)
}

// Case 3: bytelen() of a fixed-width string(N) is the buffer width N, not the
// length of the actual content.
func TestCodegenBytelen_FixedString(t *testing.T) {
	types := `type Msg struct {
	NameLen uint8  ` + "`" + `binary:"uint8,valueof=bytelen(Name)"` + "`" + `
	Name    string ` + "`" + `binary:"string(6)"` + "`" + `
}
`
	test := `import (
	"bytes"
	"testing"

	"github.com/mixcode/binarystruct"
)

func TestFixedString(t *testing.T) {
	s := Msg{Name: "hi"} // content shorter than the 6-byte buffer
	blob, err := s.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary: %v", err)
	}
	// bytelen(Name) = 6 (the buffer width), not 2.
	want := []byte{0x06, 'h', 'i', 0, 0, 0, 0}
	if !bytes.Equal(blob, want) {
		t.Fatalf("blob = % x\nwant  = % x", blob, want)
	}
	rt, err := binarystruct.Marshal(s, binarystruct.BigEndian)
	if err != nil {
		t.Fatalf("runtime Marshal: %v", err)
	}
	if !bytes.Equal(blob, rt) {
		t.Fatalf("codegen != runtime:\n codegen = % x\n runtime = % x", blob, rt)
	}
}
`
	genBytelenCase(t, "tmp_bytelen_fixedstr", types, "Msg", test)
}

// Prefixed/terminated strings: bytelen() = prefix/terminator width + content.
func TestCodegenBytelen_PrefixedTerminatedStrings(t *testing.T) {
	types := `type Msg struct {
	LB uint16 ` + "`" + `binary:"uint16,valueof=bytelen(B)"` + "`" + `
	LW uint16 ` + "`" + `binary:"uint16,valueof=bytelen(W)"` + "`" + `
	LZ uint16 ` + "`" + `binary:"uint16,valueof=bytelen(Z)"` + "`" + `
	B  string ` + "`" + `binary:"bstring"` + "`" + `
	W  string ` + "`" + `binary:"wstring"` + "`" + `
	Z  string ` + "`" + `binary:"zstring"` + "`" + `
}
`
	test := `import (
	"bytes"
	"testing"

	"github.com/mixcode/binarystruct"
)

func TestPrefixTerm(t *testing.T) {
	s := Msg{B: "hello", W: "hi", Z: "yo"}
	blob, err := s.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary: %v", err)
	}
	rt, err := binarystruct.Marshal(s, binarystruct.BigEndian)
	if err != nil {
		t.Fatalf("runtime Marshal: %v", err)
	}
	if !bytes.Equal(blob, rt) {
		t.Fatalf("codegen != runtime:\n codegen = % x\n runtime = % x", blob, rt)
	}
	var s2 Msg
	if err := s2.UnmarshalBinary(blob); err != nil {
		t.Fatalf("UnmarshalBinary: %v", err)
	}
	// bstring: 1+5=6; wstring: 2+2=4; zstring: 2+1=3.
	if s2.LB != 6 || s2.LW != 4 || s2.LZ != 3 {
		t.Fatalf("lengths: LB=%d LW=%d LZ=%d, want 6/4/3", s2.LB, s2.LW, s2.LZ)
	}
	if s2.B != "hello" || s2.W != "hi" || s2.Z != "yo" {
		t.Fatalf("round-trip mismatch: %+v", s2)
	}
}
`
	genBytelenCase(t, "tmp_bytelen_prefixterm", types, "Msg", test)
}

// Prefixed + text-encoded string: bytelen() = prefix width + encoded content,
// measured through the Marshaller's custom encoding.
func TestCodegenBytelen_PrefixedEncodedString(t *testing.T) {
	types := `type Msg struct {
	LB uint16 ` + "`" + `binary:"uint16,valueof=bytelen(B)"` + "`" + `
	B  string ` + "`" + `binary:"bstring,encoding=sjis"` + "`" + `
}
`
	test := `import (
	"bytes"
	"testing"

	"github.com/mixcode/binarystruct"
	"golang.org/x/text/encoding/japanese"
)

func TestPrefixEncoded(t *testing.T) {
	var ms binarystruct.Marshaller
	ms.AddTextEncoding("sjis", japanese.ShiftJIS)

	s := Msg{B: "ああ"} // 6 bytes UTF-8, 4 bytes Shift-JIS
	var buf bytes.Buffer
	if _, err := s.WriteBinaryWithMarshaller(&ms, &buf, binarystruct.BigEndian); err != nil {
		t.Fatalf("WriteBinaryWithMarshaller: %v", err)
	}
	blob := buf.Bytes()
	rt, err := ms.Marshal(&s, binarystruct.BigEndian)
	if err != nil {
		t.Fatalf("runtime Marshal: %v", err)
	}
	if !bytes.Equal(blob, rt) {
		t.Fatalf("codegen != runtime:\n codegen = % x\n runtime = % x", blob, rt)
	}
	// LB = bstring prefix(1) + Shift-JIS content(4) = 5.
	if blob[0] != 0x00 || blob[1] != 0x05 {
		t.Fatalf("LB = % x, want 00 05", blob[:2])
	}
}
`
	genBytelenCase(t, "tmp_bytelen_prefixenc", types, "Msg", test)
}

// Pointer-to-struct: bytelen() measures the pointee, and a nil pointer is 0.
func TestCodegenBytelen_PointerStruct(t *testing.T) {
	types := `type Header struct {
	A uint16 ` + "`" + `binary:"uint16"` + "`" + `
	B uint8  ` + "`" + `binary:"uint8"` + "`" + `
}

type Msg struct {
	BodyLen uint16  ` + "`" + `binary:"uint16,valueof=bytelen(Body)"` + "`" + `
	Body    *Header ` + "`" + `binary:"any"` + "`" + `
}
`
	test := `import (
	"bytes"
	"testing"

	"github.com/mixcode/binarystruct"
)

func TestPointerStruct(t *testing.T) {
	s := Msg{Body: &Header{A: 0x0102, B: 0x03}}
	blob, err := s.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary: %v", err)
	}
	// bytelen(Body) = 2 + 1 = 3.
	want := []byte{0x00, 0x03, 0x01, 0x02, 0x03}
	if !bytes.Equal(blob, want) {
		t.Fatalf("blob = % x\nwant  = % x", blob, want)
	}
	rt, err := binarystruct.Marshal(s, binarystruct.BigEndian)
	if err != nil {
		t.Fatalf("runtime Marshal: %v", err)
	}
	if !bytes.Equal(blob, rt) {
		t.Fatalf("codegen != runtime:\n codegen = % x\n runtime = % x", blob, rt)
	}

	// A nil pointer contributes zero bytes.
	ns := Msg{Body: nil}
	nblob, err := ns.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary(nil): %v", err)
	}
	if !bytes.Equal(nblob, []byte{0x00, 0x00}) {
		t.Fatalf("nil blob = % x, want 00 00", nblob)
	}
}
`
	genBytelenCase(t, "tmp_bytelen_ptrstruct", types, "Msg,Header", test)
}

// Case 4: bytelen() of a variable-length text-encoded string. The generated
// measurement mirrors the encode path's ms-guarded EncodeText, so the length is
// the encoded (Shift-JIS) byte count, threaded through a custom Marshaller.
func TestCodegenBytelen_VariableTextString(t *testing.T) {
	types := `type Msg struct {
	TextLen uint16 ` + "`" + `binary:"uint16,valueof=bytelen(Text)"` + "`" + `
	Text    string ` + "`" + `binary:"string,encoding=sjis"` + "`" + `
}
`
	test := `import (
	"bytes"
	"testing"

	"github.com/mixcode/binarystruct"
	"golang.org/x/text/encoding/japanese"
)

func TestVarText(t *testing.T) {
	var ms binarystruct.Marshaller
	ms.AddTextEncoding("sjis", japanese.ShiftJIS)

	s := Msg{Text: "ああ"} // 6 bytes UTF-8, 4 bytes Shift-JIS
	var buf bytes.Buffer
	if _, err := s.WriteBinaryWithMarshaller(&ms, &buf, binarystruct.BigEndian); err != nil {
		t.Fatalf("WriteBinaryWithMarshaller: %v", err)
	}
	blob := buf.Bytes()

	rt, err := ms.Marshal(&s, binarystruct.BigEndian)
	if err != nil {
		t.Fatalf("runtime Marshal: %v", err)
	}
	if !bytes.Equal(blob, rt) {
		t.Fatalf("codegen != runtime:\n codegen = % x\n runtime = % x", blob, rt)
	}
	// TextLen must be the Shift-JIS byte length (4), not the UTF-8 len (6).
	if blob[0] != 0x00 || blob[1] != 0x04 {
		t.Fatalf("TextLen prefix = % x, want 00 04", blob[:2])
	}
}
`
	genBytelenCase(t, "tmp_bytelen_vartext", types, "Msg", test)
}

// Case 5 (array): bytelen() of a tag-counted array of structs is measured by a
// generated loop that mirrors the encode's per-element write and element count.
func TestCodegenBytelen_StructArray(t *testing.T) {
	types := `type Elem struct {
	X uint16 ` + "`" + `binary:"uint16"` + "`" + `
}

type Msg struct {
	TotalLen uint16 ` + "`" + `binary:"uint16,valueof=bytelen(Items)"` + "`" + `
	Items    []Elem ` + "`" + `binary:"[2]struct"` + "`" + `
}
`
	test := `import (
	"bytes"
	"testing"

	"github.com/mixcode/binarystruct"
)

func TestStructArray(t *testing.T) {
	s := Msg{Items: []Elem{{X: 0x0102}, {X: 0x0304}}}
	blob, err := s.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary: %v", err)
	}
	// bytelen(Items) = 2 elements * 2 bytes = 4.
	want := []byte{0x00, 0x04, 0x01, 0x02, 0x03, 0x04}
	if !bytes.Equal(blob, want) {
		t.Fatalf("blob = % x\nwant  = % x", blob, want)
	}
	rt, err := binarystruct.Marshal(s, binarystruct.BigEndian)
	if err != nil {
		t.Fatalf("runtime Marshal: %v", err)
	}
	if !bytes.Equal(blob, rt) {
		t.Fatalf("codegen != runtime:\n codegen = % x\n runtime = % x", blob, rt)
	}
}
`
	genBytelenCase(t, "tmp_bytelen_structarr", types, "Msg,Elem", test)
}
