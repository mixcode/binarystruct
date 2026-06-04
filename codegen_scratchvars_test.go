// Copyright 2026 github.com/mixcode

package binarystruct_test

import "testing"

// Regression: a struct whose generated method body references neither the `tmp`
// nor the `m` scratch local must still compile. An unbounded string is the
// canonical trigger — its write touches `m` but not `tmp`, and its read (via
// io.ReadAll) touches neither — so the unconditional `var tmp`/`var m`
// declarations previously produced "declared and not used". See TODO.md.
func TestCodegenScratchVars_UnboundedString(t *testing.T) {
	types := `type Blob struct {
	Data string ` + "`" + `binary:"string"` + "`" + `
}
`
	test := `import (
	"bytes"
	"testing"
)

func TestBlob(t *testing.T) {
	s := Blob{Data: "hello world"}
	blob, err := s.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary: %v", err)
	}
	if !bytes.Equal(blob, []byte("hello world")) {
		t.Fatalf("blob = %q, want %q", blob, "hello world")
	}
	var s2 Blob
	if err := s2.UnmarshalBinary(blob); err != nil {
		t.Fatalf("UnmarshalBinary: %v", err)
	}
	if s2.Data != "hello world" {
		t.Fatalf("round-trip mismatch: %q", s2.Data)
	}
}
`
	genBytelenCase(t, "tmp_scratch_str", types, "Blob", test)
}
