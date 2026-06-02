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

func TestCodegen_Integration(t *testing.T) {
	// 1. Create a temp directory inside workspace
	tmpDir, err := ioutil.TempDir(".", "tmp-codegen-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Build the codegen binary
	codegenBin := filepath.Join(tmpDir, "codegen")
	buildCmd := exec.Command("go", "build", "-o", codegenBin, "./codegen")
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
