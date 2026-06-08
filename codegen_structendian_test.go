// Copyright 2021-2026 github.com/mixcode

package binarystruct_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestCodegen_BytelenCycle_Errors verifies the generator emits a clean error
// (rather than crashing with a stack overflow) on a self-referential bytelen
// cycle: valueof=bytelen(Name) where Name is string(NameLen) and NameLen is that
// very valueof field. (Surfaced by the 0.3.0 clean-agent evaluation.)
func TestCodegen_BytelenCycle_Errors(t *testing.T) {
	t.Parallel()
	tmpDir, err := os.MkdirTemp(".", "tmp-bs-cycle-")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	src := "package p\n\ntype Rec struct {\n" +
		"\t_       struct{} `binary:\"endian=little\"`\n" +
		"\tNameLen uint16   `binary:\"uint16,valueof=bytelen(Name)\"`\n" +
		"\tName    string   `binary:\"string(NameLen)\"`\n}\n"
	if err := os.WriteFile(filepath.Join(tmpDir, "t.go"), []byte(src), 0o644); err != nil {
		t.Fatalf("write t.go: %v", err)
	}

	out, err := exec.Command(sharedCodegenBin, "-type", "Rec", "-endian", "little", tmpDir).CombinedOutput()
	if err == nil {
		t.Fatalf("expected a clean error for the self-referential bytelen cycle; output:\n%s", out)
	}
	if strings.Contains(string(out), "stack overflow") {
		t.Fatalf("generator crashed (stack overflow) instead of erroring cleanly:\n%s", out)
	}
	if !strings.Contains(string(out), "self-referential") {
		t.Errorf("error should explain the cycle; got:\n%s", out)
	}
}

// TestCodegen_StructEncoding_Applied verifies generated code honors a struct-level
// `encoding=` default for a string field that declares none of its own — the
// generated methods, driven through a configured Marshaler, encode/decode the
// field with that encoding (matching the runtime). Round-tripped end to end.
func TestCodegen_StructEncoding_Applied(t *testing.T) {
	typesSrc := "type Msg struct {\n" +
		"\t_ struct{} `binary:\"endian=big,encoding=sjis\"`\n" +
		"\tS string   `binary:\"wstring\"` // no field encoding → struct default sjis\n}\n"
	// "あ" is e3 81 82 in UTF-8 but 82 a0 in Shift-JIS; a wstring writes a 2-byte
	// big-endian length prefix, so a correctly sjis-encoded result is 00 02 82 a0.
	testSrc := "import (\n" +
		"\t\"bytes\"\n\t\"testing\"\n\n" +
		"\t\"github.com/mixcode/binarystruct\"\n" +
		"\t\"golang.org/x/text/encoding/japanese\"\n)\n\n" +
		"func TestStructEnc(t *testing.T) {\n" +
		"\tms := binarystruct.NewMarshaler()\n" +
		"\tms.AddTextEncoding(\"sjis\", japanese.ShiftJIS)\n" +
		"\tgot, err := ms.Marshal(&Msg{S: \"あ\"}) // fast-paths to the generated method with ms set\n" +
		"\tif err != nil {\n\t\tt.Fatal(err)\n\t}\n" +
		"\tif want := []byte{0x00, 0x02, 0x82, 0xa0}; !bytes.Equal(got, want) {\n" +
		"\t\tt.Fatalf(\"encode: got %x, want %x (struct-level sjis via codegen)\", got, want)\n\t}\n" +
		"\tvar out Msg\n" +
		"\tif _, err := ms.Unmarshal(got, &out); err != nil {\n\t\tt.Fatal(err)\n\t}\n" +
		"\tif out.S != \"あ\" {\n\t\tt.Fatalf(\"round-trip: got %q\", out.S)\n\t}\n}\n"
	genBytelenCase(t, "p", typesSrc, "Msg", testSrc)
}

// TestCodegen_Validation_DecodeError verifies generated const/range validation
// failures surface as a *binarystruct.DecodeError with the field name and the
// field's START byte offset — matching the runtime interpreter (errors.As and
// the Offset/Field accessors behave identically across paths).
func TestCodegen_Validation_DecodeError(t *testing.T) {
	typesSrc := "type Rec struct {\n" +
		"\t_   struct{} `binary:\"endian=big\"`\n" +
		"\tSig [4]byte  `binary:\"[4]byte,const=0x89504e47\"`\n" +
		"\tN   uint16   `binary:\"uint16,range=1..100\"`\n}\n"
	testSrc := "import (\n\t\"errors\"\n\t\"testing\"\n\n\t\"github.com/mixcode/binarystruct\"\n)\n\n" +
		"func TestDecErr(t *testing.T) {\n" +
		"\tvar de *binarystruct.DecodeError\n" +
		"\t// wrong magic → const mismatch at the Sig field (offset 0)\n" +
		"\tvar r Rec\n" +
		"\terr := r.UnmarshalBinary([]byte{0, 0, 0, 0, 0, 5})\n" +
		"\tif !errors.As(err, &de) || !errors.Is(err, binarystruct.ErrValidationError) {\n\t\tt.Fatalf(\"const: want *DecodeError wrapping ErrValidationError, got %v\", err)\n\t}\n" +
		"\tif de.Field != \"Sig\" || de.Offset != 0 {\n\t\tt.Errorf(\"const: Field=%q Offset=%d, want Sig/0\", de.Field, de.Offset)\n\t}\n" +
		"\t// good magic, out-of-range N → range error at N (offset 4)\n" +
		"\tvar r2 Rec\n" +
		"\terr = r2.UnmarshalBinary([]byte{0x89, 0x50, 0x4e, 0x47, 0, 0})\n" +
		"\tif !errors.As(err, &de) || !errors.Is(err, binarystruct.ErrValidationError) {\n\t\tt.Fatalf(\"range: want *DecodeError wrapping ErrValidationError, got %v\", err)\n\t}\n" +
		"\tif de.Field != \"N\" || de.Offset != 4 {\n\t\tt.Errorf(\"range: Field=%q Offset=%d, want N/4\", de.Field, de.Offset)\n\t}\n" +
		"}\n"
	genBytelenCase(t, "p", typesSrc, "Rec", testSrc)
}

// TestCodegen_StructEndian_NoFlag generates code for a struct that declares its
// byte order via the `_` sentinel WITHOUT passing -endian, and verifies the
// generated methods bake that order — including that binarystruct.Marshal(v),
// called with no caller order, still produces it (the struct's declaration is
// seeded inside the generated method, so it wins even over a nil runtime order).
func TestCodegen_StructEndian_NoFlag(t *testing.T) {
	t.Parallel()

	tmpDir, err := os.MkdirTemp(".", "tmp-bs-structendian-")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	typesSrc := "package p\n\n" +
		"type T struct {\n" +
		"\t_ struct{} `binary:\"endian=big\"`\n" +
		"\tV uint16   `binary:\"uint16\"`\n" +
		"}\n"
	if err := os.WriteFile(filepath.Join(tmpDir, "types.go"), []byte(typesSrc), 0o644); err != nil {
		t.Fatalf("write types.go: %v", err)
	}

	// No -endian flag: the sentinel supplies the order. Generation must succeed.
	gen := exec.Command(sharedCodegenBin, "-type", "T", tmpDir)
	var stderr bytes.Buffer
	gen.Stderr = &stderr
	if err := gen.Run(); err != nil {
		t.Fatalf("codegen without -endian failed (the sentinel should supply the order): %v\n%s", err, stderr.String())
	}

	testSrc := "package p\n\n" +
		"import (\n\t\"bytes\"\n\t\"testing\"\n\n\t\"github.com/mixcode/binarystruct\"\n)\n\n" +
		"func TestGen(t *testing.T) {\n" +
		"\tv := T{V: 0x0102}\n" +
		"\tb, err := v.MarshalBinary()\n" +
		"\tif err != nil {\n\t\tt.Fatal(err)\n\t}\n" +
		"\tif !bytes.Equal(b, []byte{0x01, 0x02}) {\n\t\tt.Fatalf(\"MarshalBinary = %x, want 01 02 (big-endian)\", b)\n\t}\n" +
		"\t// No caller order: the struct's declared big-endian must still win.\n" +
		"\tb2, err := binarystruct.Marshal(&v)\n" +
		"\tif err != nil {\n\t\tt.Fatal(err)\n\t}\n" +
		"\tif !bytes.Equal(b2, []byte{0x01, 0x02}) {\n\t\tt.Fatalf(\"Marshal = %x, want 01 02 (struct order must win)\", b2)\n\t}\n" +
		"}\n"
	if err := os.WriteFile(filepath.Join(tmpDir, "types_test.go"), []byte(testSrc), 0o644); err != nil {
		t.Fatalf("write types_test.go: %v", err)
	}

	out, err := exec.Command("go", "test", "./"+tmpDir).CombinedOutput()
	if err != nil {
		t.Errorf("generated tests failed: %v\n%s", err, out)
	}
}
