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

// TestCodegen_StructEncoding_Errors verifies codegen fails loud (rather than
// emitting silently-unencoded output) when a string field would rely on a
// struct-level encoding= it cannot honor.
func TestCodegen_StructEncoding_Errors(t *testing.T) {
	t.Parallel()
	tmpDir, err := os.MkdirTemp(".", "tmp-bs-structenc-")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	src := "package p\n\ntype T struct {\n" +
		"\t_ struct{} `binary:\"endian=big,encoding=sjis\"`\n" +
		"\tS string   `binary:\"wstring\"`\n}\n"
	if err := os.WriteFile(filepath.Join(tmpDir, "t.go"), []byte(src), 0o644); err != nil {
		t.Fatalf("write t.go: %v", err)
	}

	out, err := exec.Command(sharedCodegenBin, "-type", "T", "-endian", "big", tmpDir).CombinedOutput()
	if err == nil {
		t.Fatalf("expected generation to fail for a struct-level encoding on an un-tagged string field; output:\n%s", out)
	}
	if !strings.Contains(string(out), "struct-level encoding") {
		t.Errorf("error should mention struct-level encoding; got:\n%s", out)
	}
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
