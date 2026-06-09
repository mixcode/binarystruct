// Copyright 2026 github.com/mixcode

//go:generate go run github.com/mixcode/binarystruct/binarystruct-codegen -type Samples -output samples_binary.go
package example

// Samples is a benchmark fixture for the codegen bulk-buffer scalar-slice path: a
// length-prefixed slice of multibyte scalars (the shape that previously generated
// a per-element write/read loop).
type Samples struct {
	_ struct{} `binary:"endian=little"`
	N uint32   `binary:"uint32,valueof=count(V)"`
	V []uint32 `binary:"[N]uint32"`
}
