// Copyright 2026 github.com/mixcode

package binarystruct_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// sharedCodegenBin is the codegen tool, built once for the whole test binary by
// TestMain. The codegen integration tests each spawn a `go test` subprocess on a
// freshly generated package, so rebuilding the tool per test was pure overhead.
var sharedCodegenBin string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "binarystruct-codegen-bin")
	if err != nil {
		fmt.Fprintln(os.Stderr, "TestMain: temp dir:", err)
		os.Exit(1)
	}
	bin := filepath.Join(dir, "binarystruct-codegen")
	if out, err := exec.Command("go", "build", "-o", bin, "./binarystruct-codegen").CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "TestMain: build codegen tool: %v\n%s", err, out)
		os.RemoveAll(dir)
		os.Exit(1)
	}
	sharedCodegenBin = bin

	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}
