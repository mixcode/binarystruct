// Copyright 2026 github.com/mixcode

package binarystruct_test

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestCodegen_Integration(t *testing.T) {
	// 1. Create a temp directory inside workspace
	tmpDir, err := ioutil.TempDir(".", "tmp-binarystruct-codegen-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Build the codegen binary
	codegenBin := filepath.Join(tmpDir, "binarystruct-codegen")
	buildCmd := exec.Command("go", "build", "-o", codegenBin, "./binarystruct-codegen")
	if buildOut, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to build codegen tool: %v\n%s", err, buildOut)
	}

	// 2. Write the test struct file
	structFile := filepath.Join(tmpDir, "types.go")
	structContent := `package tmp_codegen_test

type TestNested struct {
	Val uint16 ` + "`" + `binary:"uint16"` + "`" + `
}

type TestStruct struct {
	Age    uint8      ` + "`" + `binary:"uint8,range=18..120"` + "`" + `
	Code   string     ` + "`" + `binary:"string(6),match=^[A-Z]{2}\\d{4}$"` + "`" + `
	Buffer []byte     ` + "`" + `binary:"[2]byte"` + "`" + `
	Pad    []byte     ` + "`" + `binary:"pad(2)"` + "`" + `
	Nested TestNested ` + "`" + `binary:"any"` + "`" + `
}
`
	if err := ioutil.WriteFile(structFile, []byte(structContent), 0644); err != nil {
		t.Fatalf("failed to write test struct file: %v", err)
	}

	// 3. Run the codegen tool on the temp directory
	genCmd := exec.Command(codegenBin, "-type", "TestStruct,TestNested", tmpDir)
	var genStderr bytes.Buffer
	genCmd.Stderr = &genStderr
	if err := genCmd.Run(); err != nil {
		t.Fatalf("codegen run failed: %v\nStderr: %s", err, genStderr.String())
	}

	// 4. Write the integration test file
	testFile := filepath.Join(tmpDir, "types_test.go")
	testContent := `package tmp_codegen_test

import (
	"bytes"
	"testing"
)

func TestGeneratedMethods(t *testing.T) {
	s := TestStruct{
		Age:    25,
		Code:   "US1234",
		Buffer: []byte{0xAA, 0xBB},
		Nested: TestNested{Val: 0x1234},
	}
	blob, err := s.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary failed: %v", err)
	}

	expectedLen := 13 // 1 (Age) + 6 (Code) + 2 (Buffer) + 2 (Pad) + 2 (Nested)
	if len(blob) != expectedLen {
		t.Errorf("expected length %d, got %d", expectedLen, len(blob))
	}

	var s2 TestStruct
	err = s2.UnmarshalBinary(blob)
	if err != nil {
		t.Fatalf("UnmarshalBinary failed: %v", err)
	}

	if s2.Age != 25 || s2.Code != "US1234" || s2.Nested.Val != 0x1234 || !bytes.Equal(s2.Buffer, s.Buffer) {
		t.Errorf("unmarshalled values mismatch: %+v", s2)
	}

	// Test range validation failure
	blob[0] = 10 // invalid age (10 < 18)
	err = s2.UnmarshalBinary(blob)
	if err == nil {
		t.Error("expected error for invalid range age, got nil")
	}

	// Test regex validation failure
	blob[0] = 25 // restore age
	blob[1] = 'a' // invalid code (lowercase)
	err = s2.UnmarshalBinary(blob)
	if err == nil {
		t.Error("expected error for invalid code match, got nil")
	}
}
`
	if err := ioutil.WriteFile(testFile, []byte(testContent), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	// 5. Run the generated tests using go test
	testArgs := []string{"test", "./" + tmpDir}
	if testing.Verbose() {
		testArgs = append(testArgs, "-v")
	}
	testCmd := exec.Command("go", testArgs...)
	testOutput, err := testCmd.CombinedOutput()
	if testing.Verbose() {
		t.Log(string(testOutput))
	}
	if err != nil {
		if !testing.Verbose() {
			t.Log(string(testOutput))
		}
		t.Errorf("generated tests failed: %v", err)
	}
}

func TestCodegen_JSONOutput(t *testing.T) {
	relTmpDir, err := ioutil.TempDir(".", "tmp-binarystruct-codegen-json-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(relTmpDir)

	tmpDir, err := filepath.Abs(relTmpDir)
	if err != nil {
		t.Fatalf("failed to get absolute path: %v", err)
	}

	codegenBin := filepath.Join(tmpDir, "binarystruct-codegen")
	buildCmd := exec.Command("go", "build", "-o", codegenBin, "./binarystruct-codegen")
	if buildOut, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to build codegen tool: %v\n%s", err, buildOut)
	}

	structFile := filepath.Join(tmpDir, "types.go")
	structContent := `package tmp_codegen_test

type TestStruct struct {
	Age  uint8  ` + "`" + `binary:"uint8,range=18..120"` + "`" + `
	Code string ` + "`" + `binary:"string(6),match=^[A-Z]{2}\\d{4}$"` + "`" + `
}
`
	if err := ioutil.WriteFile(structFile, []byte(structContent), 0644); err != nil {
		t.Fatalf("failed to write struct file: %v", err)
	}

	jsonFile := filepath.Join(tmpDir, "layout.json")
	genCmd := exec.Command(codegenBin, "-type", "TestStruct", "-json", "-output", jsonFile, tmpDir)
	var genStderr bytes.Buffer
	genCmd.Stderr = &genStderr
	if err := genCmd.Run(); err != nil {
		t.Fatalf("codegen json run failed: %v\nStderr: %s", err, genStderr.String())
	}

	// Verify JSON content
	jsData, err := ioutil.ReadFile(jsonFile)
	if err != nil {
		t.Fatalf("failed to read generated JSON file: %v", err)
	}

	type CodegenField struct {
		Name         string            `json:"name"`
		GoType       string            `json:"go_type"`
		BinaryType   string            `json:"binary_type"`
		IsArray      bool              `json:"is_array"`
		ArrayLenExpr string            `json:"array_len_expr,omitempty"`
		BufLenExpr   string            `json:"buf_len_expr,omitempty"`
		Options      map[string]string `json:"options,omitempty"`
	}

	type CodegenStruct struct {
		Name   string         `json:"name"`
		Fields []CodegenField `json:"fields"`
	}

	var results []CodegenStruct
	if err := json.Unmarshal(jsData, &results); err != nil {
		t.Fatalf("failed to parse generated JSON: %v\nData: %s", err, jsData)
	}

	if len(results) != 1 || results[0].Name != "TestStruct" {
		t.Fatalf("unexpected JSON results: %+v", results)
	}

	fields := results[0].Fields
	if len(fields) != 2 {
		t.Fatalf("expected 2 fields, got %d", len(fields))
	}

	if fields[0].Name != "Age" || fields[0].GoType != "uint8" || fields[0].BinaryType != "uint8" || fields[0].Options["range"] != "18..120" {
		t.Errorf("unexpected Age field: %+v", fields[0])
	}

	if fields[1].Name != "Code" || fields[1].GoType != "string" || fields[1].BinaryType != "string" || fields[1].BufLenExpr != "6" || fields[1].Options["match"] != "^[A-Z]{2}\\d{4}$" {
		t.Errorf("unexpected Code field: %+v", fields[1])
	}
}

func TestCodegen_Valueof(t *testing.T) {
	tmpDir, err := ioutil.TempDir(".", "tmp-binarystruct-codegen-valueof-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	codegenBin := filepath.Join(tmpDir, "binarystruct-codegen")
	buildCmd := exec.Command("go", "build", "-o", codegenBin, "./binarystruct-codegen")
	if buildOut, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to build codegen tool: %v\n%s", err, buildOut)
	}

	// A ZIP-like header: NameLen/Count are computed from Name/Items via valueof.
	structFile := filepath.Join(tmpDir, "types.go")
	structContent := `package tmp_codegen_valueof_test

type Packet struct {
	NameLen uint16 ` + "`" + `binary:"uint16,valueof=bytelen(Name)"` + "`" + `
	Count   uint8  ` + "`" + `binary:"uint8,valueof=count(Items)+1"` + "`" + `
	Name    []byte ` + "`" + `binary:"[NameLen]byte"` + "`" + `
	Items   []byte ` + "`" + `binary:"[Count-1]byte"` + "`" + `
}
`
	if err := ioutil.WriteFile(structFile, []byte(structContent), 0644); err != nil {
		t.Fatalf("failed to write test struct file: %v", err)
	}

	genCmd := exec.Command(codegenBin, "-type", "Packet", tmpDir)
	var genStderr bytes.Buffer
	genCmd.Stderr = &genStderr
	if err := genCmd.Run(); err != nil {
		t.Fatalf("codegen run failed: %v\nStderr: %s", err, genStderr.String())
	}

	testFile := filepath.Join(tmpDir, "types_test.go")
	testContent := `package tmp_codegen_valueof_test

import (
	"bytes"
	"testing"
)

func TestGeneratedValueof(t *testing.T) {
	// NameLen and Count are seeded WRONG to prove valueof overrides them.
	s := Packet{NameLen: 999, Count: 0, Name: []byte("hello.txt"), Items: []byte{1, 2, 3}}
	blob, err := s.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary failed: %v", err)
	}
	// BigEndian: NameLen=9 (00 09), Count=4 (04), "hello.txt", 1 2 3
	want := append([]byte{0x00, 0x09, 0x04}, append([]byte("hello.txt"), 1, 2, 3)...)
	if !bytes.Equal(blob, want) {
		t.Fatalf("blob = % x\nwant  = % x", blob, want)
	}
	// emit-only: source struct unchanged
	if s.NameLen != 999 || s.Count != 0 {
		t.Errorf("source mutated: NameLen=%d Count=%d", s.NameLen, s.Count)
	}

	var s2 Packet
	if err := s2.UnmarshalBinary(blob); err != nil {
		t.Fatalf("UnmarshalBinary failed: %v", err)
	}
	if s2.NameLen != 9 || s2.Count != 4 || string(s2.Name) != "hello.txt" || !bytes.Equal(s2.Items, []byte{1, 2, 3}) {
		t.Errorf("round-trip mismatch: %+v", s2)
	}
}
`
	if err := ioutil.WriteFile(testFile, []byte(testContent), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	testArgs := []string{"test", "./" + tmpDir}
	if testing.Verbose() {
		testArgs = append(testArgs, "-v")
	}
	testCmd := exec.Command("go", testArgs...)
	testOutput, err := testCmd.CombinedOutput()
	if testing.Verbose() {
		t.Log(string(testOutput))
	}
	if err != nil {
		if !testing.Verbose() {
			t.Log(string(testOutput))
		}
		t.Errorf("generated valueof tests failed: %v", err)
	}
}

func TestCodegen_Const(t *testing.T) {
	tmpDir, err := ioutil.TempDir(".", "tmp-binarystruct-codegen-const-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	codegenBin := filepath.Join(tmpDir, "binarystruct-codegen")
	buildCmd := exec.Command("go", "build", "-o", codegenBin, "./binarystruct-codegen")
	if buildOut, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to build codegen tool: %v\n%s", err, buildOut)
	}

	// Both const shapes: an integer magic and a byte-sequence magic.
	structFile := filepath.Join(tmpDir, "types.go")
	structContent := `package tmp_codegen_const_test

type Packet struct {
	Sig   uint32  ` + "`" + `binary:"uint32,const=0x04034b50"` + "`" + `
	Magic [4]byte ` + "`" + `binary:"[4]byte,const=0x504b0304"` + "`" + `
	N     uint8   ` + "`" + `binary:"uint8"` + "`" + `
}
`
	if err := ioutil.WriteFile(structFile, []byte(structContent), 0644); err != nil {
		t.Fatalf("failed to write test struct file: %v", err)
	}

	genCmd := exec.Command(codegenBin, "-type", "Packet", tmpDir)
	var genStderr bytes.Buffer
	genCmd.Stderr = &genStderr
	if err := genCmd.Run(); err != nil {
		t.Fatalf("codegen run failed: %v\nStderr: %s", err, genStderr.String())
	}

	testFile := filepath.Join(tmpDir, "types_test.go")
	testContent := `package tmp_codegen_const_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/mixcode/binarystruct"
)

func TestGeneratedConst(t *testing.T) {
	// Seed Sig/Magic WRONG to prove const overrides them on encode.
	s := Packet{Sig: 0xdeadbeef, Magic: [4]byte{1, 2, 3, 4}, N: 7}
	blob, err := s.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary failed: %v", err)
	}
	// BigEndian: Sig=04 03 4b 50, Magic (natural order)=50 4b 03 04, N=07
	want := []byte{0x04, 0x03, 0x4b, 0x50, 0x50, 0x4b, 0x03, 0x04, 0x07}
	if !bytes.Equal(blob, want) {
		t.Fatalf("blob = % x\nwant  = % x", blob, want)
	}

	var ok Packet
	if err := ok.UnmarshalBinary(blob); err != nil {
		t.Fatalf("UnmarshalBinary good failed: %v", err)
	}
	if ok.Sig != 0x04034b50 || ok.Magic != [4]byte{0x50, 0x4b, 0x03, 0x04} || ok.N != 7 {
		t.Fatalf("round-trip mismatch: %+v", ok)
	}

	// Corrupt the integer magic -> validation error.
	badSig := append([]byte{}, want...)
	badSig[0] = 0xff
	var b1 Packet
	if err := b1.UnmarshalBinary(badSig); !errors.Is(err, binarystruct.ErrValidationError) {
		t.Fatalf("bad Sig: want ErrValidationError, got %v", err)
	}

	// Corrupt the byte-sequence magic -> validation error.
	badMagic := append([]byte{}, want...)
	badMagic[5] = 0xff
	var b2 Packet
	if err := b2.UnmarshalBinary(badMagic); !errors.Is(err, binarystruct.ErrValidationError) {
		t.Fatalf("bad Magic: want ErrValidationError, got %v", err)
	}
}
`
	if err := ioutil.WriteFile(testFile, []byte(testContent), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	testArgs := []string{"test", "./" + tmpDir}
	if testing.Verbose() {
		testArgs = append(testArgs, "-v")
	}
	testCmd := exec.Command("go", testArgs...)
	testOutput, err := testCmd.CombinedOutput()
	if testing.Verbose() {
		t.Log(string(testOutput))
	}
	if err != nil {
		if !testing.Verbose() {
			t.Log(string(testOutput))
		}
		t.Errorf("generated const tests failed: %v", err)
	}
}
