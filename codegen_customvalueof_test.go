// Copyright 2026 github.com/mixcode

package binarystruct_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// genCustomValueofCase generates code for a struct using a custom valueof
// evaluator, drops in a test file that registers the evaluator on a Marshaler and
// drives the generated WithMarshaler methods, and runs `go test` over the temp
// package. noValidate toggles the -no-validate flag (strips decode-time checking;
// by default the generated decode validates, matching the runtime interpreter).
// The struct declares its own endian, so no -endian flag is passed.
func genCustomValueofCase(t *testing.T, typesSrc, typeList, testSrc string, noValidate bool) {
	t.Helper()
	t.Parallel()

	tmpDir, err := os.MkdirTemp(".", "tmp-bs-cv-")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	if err := os.WriteFile(filepath.Join(tmpDir, "types.go"), []byte("package p\n\n"+typesSrc), 0o644); err != nil {
		t.Fatalf("write types.go: %v", err)
	}

	args := []string{"-type", typeList}
	if noValidate {
		args = append(args, "-no-validate")
	}
	args = append(args, tmpDir)
	var genStderr bytes.Buffer
	cmd := exec.Command(sharedCodegenBin, args...)
	cmd.Stderr = &genStderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("codegen failed: %v\nstderr: %s", err, genStderr.String())
	}

	if err := os.WriteFile(filepath.Join(tmpDir, "types_test.go"), []byte("package p\n\n"+testSrc), 0o644); err != nil {
		t.Fatalf("write types_test.go: %v", err)
	}

	testArgs := []string{"test", "./" + tmpDir}
	if testing.Verbose() {
		testArgs = append(testArgs, "-v")
	}
	out, err := exec.Command("go", testArgs...).CombinedOutput()
	if err != nil {
		t.Errorf("generated tests failed: %v\n%s", err, out)
	} else if testing.Verbose() {
		t.Log(string(out))
	}
}

// A PNG-chunk-shaped struct: built-in bytelen for the length, a custom CRC32
// evaluator over the encoded bytes of Type+Data for the trailing checksum.
const cvChunkSrc = "type Chunk struct {\n" +
	"\t_      struct{} `binary:\"endian=big\"`\n" +
	"\tLength uint32   `binary:\"uint32,valueof=bytelen(Data)\"`\n" +
	"\tType   string   `binary:\"string(4)\"`\n" +
	"\tData   []byte   `binary:\"[Length]byte\"`\n" +
	"\tCRC    uint32   `binary:\"uint32,valueof=CRC32(Type, Data)\"`\n}\n"

// cvHelperSrc is the CRC32-registering Marshaler helper (no import block — each
// test source supplies its own imports first).
const cvHelperSrc = `
func crcMarshaler() *binarystruct.Marshaler {
	ms := binarystruct.NewMarshaler()
	ms.AddValueOf("CRC32", func(c binarystruct.ValueOfContext) (uint64, error) {
		h := crc32.NewIEEE()
		for _, a := range c.Args {
			h.Write(a.Bytes)
		}
		return uint64(h.Sum32()), nil
	})
	return ms
}
`

// TestCodegen_CustomValueof_RoundTrip: generated encode computes the CRC (matching
// an independent hash/crc32) and the round trip recovers the struct. By default
// (no -no-validate) the generated decode validates the CRC, matching the runtime
// interpreter, so a corrupted CRC is rejected.
func TestCodegen_CustomValueof_RoundTrip(t *testing.T) {
	testSrc := `
import (
	"bytes"
	"hash/crc32"
	"testing"

	"github.com/mixcode/binarystruct"
)
` + cvHelperSrc + `
func TestRT(t *testing.T) {
	ms := crcMarshaler()
	in := Chunk{Type: "IHDR", Data: []byte{1, 2, 3, 4, 5}}
	var b bytes.Buffer
	if _, err := in.WriteBinaryWithMarshaler(ms, &b, binarystruct.BigEndian); err != nil {
		t.Fatalf("encode: %v", err)
	}
	blob := b.Bytes()
	if len(blob) != 4+4+5+4 {
		t.Fatalf("len = %d, want 17: % x", len(blob), blob)
	}
	want := crc32.ChecksumIEEE(append([]byte("IHDR"), in.Data...))
	got := uint32(blob[13])<<24 | uint32(blob[14])<<16 | uint32(blob[15])<<8 | uint32(blob[16])
	if got != want {
		t.Fatalf("encoded CRC = %#08x, want %#08x", got, want)
	}
	var out Chunk
	if _, err := out.ReadBinaryWithMarshaler(ms, bytes.NewReader(blob), binarystruct.BigEndian); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Type != "IHDR" || !bytes.Equal(out.Data, in.Data) {
		t.Fatalf("round trip mismatch: %+v", out)
	}
	// Default build verifies on decode (parity with the runtime): a corrupted CRC errors.
	bad := append([]byte(nil), blob...)
	bad[len(bad)-1] ^= 0xff
	var out2 Chunk
	if _, err := out2.ReadBinaryWithMarshaler(ms, bytes.NewReader(bad), binarystruct.BigEndian); err == nil {
		t.Fatal("default build should verify the CRC on decode; a corrupted CRC must error")
	}
}
`
	genCustomValueofCase(t, cvChunkSrc, "Chunk", testSrc, false)
}

// TestCodegen_CustomValueof_Validate: by default the generated decode recomputes
// the CRC and rejects a corrupted one with a *DecodeError wrapping
// ErrValidationError (parity with the runtime interpreter), while a clean stream
// decodes fine.
func TestCodegen_CustomValueof_Validate(t *testing.T) {
	testSrc := `
import (
	"bytes"
	"errors"
	"hash/crc32"
	"testing"

	"github.com/mixcode/binarystruct"
)
` + cvHelperSrc + `
func TestValidate(t *testing.T) {
	ms := crcMarshaler()
	in := Chunk{Type: "IHDR", Data: []byte{9, 8, 7}}
	var b bytes.Buffer
	if _, err := in.WriteBinaryWithMarshaler(ms, &b, binarystruct.BigEndian); err != nil {
		t.Fatalf("encode: %v", err)
	}
	blob := b.Bytes()
	var ok Chunk
	if _, err := ok.ReadBinaryWithMarshaler(ms, bytes.NewReader(blob), binarystruct.BigEndian); err != nil {
		t.Fatalf("clean decode should pass, got: %v", err)
	}
	bad := append([]byte(nil), blob...)
	bad[len(bad)-1] ^= 0xff
	var out Chunk
	_, err := out.ReadBinaryWithMarshaler(ms, bytes.NewReader(bad), binarystruct.BigEndian)
	if err == nil {
		t.Fatal("expected a validation error on a corrupted CRC")
	}
	if !errors.Is(err, binarystruct.ErrValidationError) {
		t.Fatalf("error %q does not wrap ErrValidationError", err)
	}
	// Parity with the runtime interpreter: a *DecodeError naming the field.
	var de *binarystruct.DecodeError
	if !errors.As(err, &de) {
		t.Fatalf("error is not a *binarystruct.DecodeError: %v", err)
	}
	if de.Field != "CRC" {
		t.Fatalf("DecodeError.Field = %q, want CRC", de.Field)
	}
}
`
	genCustomValueofCase(t, cvChunkSrc, "Chunk", testSrc, false)
}

// TestCodegen_CustomValueof_NoValidate: with -no-validate, the generated decode
// skips the CRC recompute, so a corrupted CRC decodes WITHOUT error (the opt-out
// for trusted-input / hot-path decoding).
func TestCodegen_CustomValueof_NoValidate(t *testing.T) {
	testSrc := `
import (
	"bytes"
	"hash/crc32"
	"testing"

	"github.com/mixcode/binarystruct"
)
` + cvHelperSrc + `
func TestNoValidate(t *testing.T) {
	ms := crcMarshaler()
	in := Chunk{Type: "IHDR", Data: []byte{9, 8, 7}}
	var b bytes.Buffer
	if _, err := in.WriteBinaryWithMarshaler(ms, &b, binarystruct.BigEndian); err != nil {
		t.Fatalf("encode: %v", err)
	}
	blob := b.Bytes()
	bad := append([]byte(nil), blob...)
	bad[len(bad)-1] ^= 0xff
	var out Chunk
	if _, err := out.ReadBinaryWithMarshaler(ms, bytes.NewReader(bad), binarystruct.BigEndian); err != nil {
		t.Fatalf("with -no-validate, a corrupted CRC must decode without error, got: %v", err)
	}
	if out.Type != "IHDR" || !bytes.Equal(out.Data, in.Data) {
		t.Fatalf("decode mismatch: %+v", out)
	}
}
`
	genCustomValueofCase(t, cvChunkSrc, "Chunk", testSrc, true)
}

// TestCodegen_CustomValueof_ScalarArg: a custom evaluator over a fixed-width
// integer scalar (uint16) plus a byte slice. Asserts the generated output is
// byte-identical to the runtime interpreter for the same value (three-path
// parity) and that the CRC covers the scalar's big-endian wire bytes.
func TestCodegen_CustomValueof_ScalarArg(t *testing.T) {
	src := "type Frame struct {\n" +
		"\t_    struct{} `binary:\"endian=big\"`\n" +
		"\tVer  uint16   `binary:\"uint16\"`\n" +
		"\tData []byte   `binary:\"[]byte\"`\n" +
		"\tCRC  uint32   `binary:\"uint32,valueof=CRC32(Ver, Data)\"`\n}\n"
	testSrc := `
import (
	"bytes"
	"hash/crc32"
	"testing"

	"github.com/mixcode/binarystruct"
)
` + cvHelperSrc + `
func TestScalarArg(t *testing.T) {
	ms := crcMarshaler()
	in := Frame{Ver: 0x0102, Data: []byte{0xaa, 0xbb, 0xcc}}
	var b bytes.Buffer
	if _, err := in.WriteBinaryWithMarshaler(ms, &b, binarystruct.BigEndian); err != nil {
		t.Fatalf("codegen encode: %v", err)
	}
	gen := b.Bytes()
	rt, err := ms.Marshal(&in) // runtime interpreter, same struct + evaluator
	if err != nil {
		t.Fatalf("runtime encode: %v", err)
	}
	if !bytes.Equal(gen, rt) {
		t.Fatalf("codegen vs runtime differ:\n codegen=% x\n runtime=% x", gen, rt)
	}
	// CRC must cover Ver's big-endian bytes (01 02) followed by Data.
	want := crc32.ChecksumIEEE([]byte{0x01, 0x02, 0xaa, 0xbb, 0xcc})
	got := uint32(gen[len(gen)-4])<<24 | uint32(gen[len(gen)-3])<<16 | uint32(gen[len(gen)-2])<<8 | uint32(gen[len(gen)-1])
	if got != want {
		t.Fatalf("CRC = %#08x, want %#08x", got, want)
	}
}
`
	genCustomValueofCase(t, src, "Frame", testSrc, false)
}

// TestCodegen_CustomValueof_NonByteRegion_Errors: a custom evaluator over a
// text-encoded (non-byte-region) field fails generation with a clear message.
func TestCodegen_CustomValueof_NonByteRegion_Errors(t *testing.T) {
	t.Parallel()
	tmpDir, err := os.MkdirTemp(".", "tmp-bs-cverr-")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	src := "package p\n\ntype Bad struct {\n" +
		"\t_    struct{} `binary:\"endian=big\"`\n" +
		"\tName string   `binary:\"wstring,encoding=sjis\"`\n" +
		"\tCRC  uint32   `binary:\"uint32,valueof=CRC32(Name)\"`\n}\n"
	if err := os.WriteFile(filepath.Join(tmpDir, "t.go"), []byte(src), 0o644); err != nil {
		t.Fatalf("write t.go: %v", err)
	}

	out, err := exec.Command(sharedCodegenBin, "-type", "Bad", tmpDir).CombinedOutput()
	if err == nil {
		t.Fatalf("expected a generation error for a non-byte-region arg; output:\n%s", out)
	}
	if !strings.Contains(string(out), "byte-region") {
		t.Errorf("error should explain the byte-region limit; got:\n%s", out)
	}
}
